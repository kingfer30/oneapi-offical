package gemini

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	channelhelper "github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"

	commonClient "github.com/songquanpeng/one-api/common/client"
)

type Adaptor struct {
}

func (a *Adaptor) Init(meta *meta.Meta) {

}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	version := helper.AssignOrDefault(meta.Config.APIVersion, config.GeminiVersion)
	action := ""
	switch meta.Mode {
	case relaymode.Embeddings:
		action = "batchEmbedContents"
	default:
		action = "generateContent"
	}

	if meta.IsStream {
		action = "streamGenerateContent?alt=sse"
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", meta.BaseURL, version, meta.ActualModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	channelhelper.SetupCommonRequestHeader(c, req, meta)
	key := c.GetString("x-new-api-key")
	if key != "" {
		logger.SysLogf("x-new-api-key: %s | old: %s ", key, meta.APIKey)
		req.Header.Set("x-goog-api-key", key)
	} else {
		req.Header.Set("x-goog-api-key", meta.APIKey)
	}
	req.Header.Set("User-agent", "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.4472.114 Safari/537.36")
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch relayMode {
	case relaymode.Embeddings:
		geminiEmbeddingRequest := ConvertEmbeddingRequest(*request)
		return geminiEmbeddingRequest, nil
	default:
		geminiRequest, err := ConvertRequest(c, *request)
		if err != nil {
			return nil, err
		}
		return geminiRequest, nil
	}
}

func (a *Adaptor) ConvertImageRequest(request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	body := &bytes.Buffer{}
	_, err := io.Copy(body, requestBody)
	if err != nil {
		return nil, fmt.Errorf("io.Copy failed: %w", err)
	}
	fullRequestURL, err := a.GetRequestURL(meta)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, body)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	err = a.SetupRequestHeader(c, req, meta)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	resp, err := doRequest(c, req)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		//429 直接发起再次重试
		var retryNum = 5
		for {
			if retryNum == 0 {
				break
			}
			retryNum--
			_, err := io.Copy(body, requestBody)
			if err != nil {
				return nil, fmt.Errorf("io.Copy failed: %w", err)
			}
			logger.SysLogf("触发429, 正在重试: %s , 剩余次数: %d ", meta.APIKey, retryNum)
			req, err = http.NewRequest(c.Request.Method, fullRequestURL, body)
			if err != nil {
				return nil, fmt.Errorf("new request failed: %w", err)
			}
			err = a.SetupRequestHeader(c, req, meta)
			if err != nil {
				return nil, fmt.Errorf("setup request header failed: %w", err)
			}
			resp, err = doRequest(c, req)
			if err != nil {
				return nil, fmt.Errorf("do request failed: %w", err)
			}
			if resp.StatusCode != http.StatusTooManyRequests {
				break
			}
		}
	}
	return resp, nil
}
func doRequest(c *gin.Context, req *http.Request) (*http.Response, error) {
	var client *http.Client
	if config.HttpProxy == "" {
		client = commonClient.HTTPClient
	} else {
		logger.SysLogf("使用代理: %s ", config.HttpProxy)
		url, err := url.Parse(config.HttpProxy)
		if err != nil {
			return nil, fmt.Errorf("url.Parse failed: %w", err)
		}
		client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(url),
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("resp is nil")
	}
	_ = req.Body.Close()
	_ = c.Request.Body.Close()
	return resp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		var responseText string
		err, responseText = StreamHandler(c, resp)
		usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
	} else {
		switch meta.Mode {
		case relaymode.Embeddings:
			err, usage = EmbeddingHandler(c, resp)
		default:
			err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "google gemini"
}
