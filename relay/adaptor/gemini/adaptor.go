package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	dbmodel "github.com/songquanpeng/one-api/model"
	channelhelper "github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
}

var (
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)

func (a *Adaptor) Init(meta *meta.Meta) {
	meta.SelfImplement = config.GeminiNewEnabled
	if IsImageModel(meta.OriginModelName) {
		meta.IsImageModel = true
	}
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	defaultVersion := config.GeminiVersion
	if meta.ActualModelName == "gemini-2.0-flash-exp" {
		defaultVersion = "v1beta"
	}

	version := helper.AssignOrDefault(meta.Config.APIVersion, defaultVersion)
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
	req.Header.Set("Host", "generativelanguage.googleapis.com")
	req.Header.Set("User-Agent", userAgent)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if request.Thinking {
		c.Set("hua_thinking", true)
	}
	switch relayMode {
	case relaymode.Embeddings:
		geminiEmbeddingRequest := ConvertEmbeddingRequest(*request)
		return geminiEmbeddingRequest, nil
	default:
		geminiRequest, err := ConvertRequest(c, *request)
		if err != nil {
			b, jerr := json.Marshal(geminiRequest)
			if jerr == nil {
				logger.SysErrorf("ConvertRequest error : %s\n, %s", err.Error(), string(b))
			}
			return nil, err
		}
		return geminiRequest, nil
	}
}

func (a *Adaptor) ConvertImageRequest(request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	geminiRequest, err := ConvertImageRequest(*request)
	if err != nil {
		b, jerr := json.Marshal(geminiRequest)
		if jerr == nil {
			logger.SysErrorf("ConvertImageRequest error : %s\n, %s", err.Error(), string(b))
		}
		return nil, err
	}
	return geminiRequest, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	bodyData, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error reading body: %s", err)
	}

	requestBody = bytes.NewBuffer(bodyData)
	fullRequestURL, err := a.GetRequestURL(meta)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
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

	if resp.StatusCode == http.StatusBadRequest && c.GetBool("file_upload") {
		//400 file文件不存在的问题处理
		defer resp.Body.Close()
		requestBody, err := io.ReadAll(resp.Body)
		if err == nil {
			var geminiErr *GeminiErrorResponse
			err = json.Unmarshal(requestBody, &geminiErr)
			if err == nil && (strings.Contains(geminiErr.Error.Message, "File ") && strings.Contains(geminiErr.Error.Message, "not exist in the Gemini API.")) {
				re := regexp.MustCompile(`https?://[^\s]+`)
				url := re.FindString(geminiErr.Error.Message)
				dbmodel.DelFileByFileId(url)
				openaiErr := openai.ErrorWrapper(
					fmt.Errorf("File not exist"),
					"bad_requests",
					http.StatusBadRequest,
				)
				errData, err := json.Marshal(openaiErr)
				if err == nil {
					resp.Body = io.NopCloser(bytes.NewBuffer(errData))
				} else {
					resp.Body = io.NopCloser(bytes.NewBuffer(requestBody))
				}
			} else {
				resp.Body = io.NopCloser(bytes.NewBuffer(requestBody))
			}
		}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		//429的问题处理
		defer resp.Body.Close()
		requestBody, err := io.ReadAll(resp.Body)
		if err == nil {
			var geminiErr *GeminiErrorResponse
			err = json.Unmarshal(requestBody, &geminiErr)
			if err == nil && len(geminiErr.Error.Details) > 0 {
				delay := 60
				re := regexp.MustCompile(`\d+`)
				for _, detail := range geminiErr.Error.Details {
					if detail.RetryDelay != "" {
						num := re.FindString(detail.RetryDelay)
						delay, _ = strconv.Atoi(num)
						break
					}
				}
				openaiErr := openai.ErrorWrapper(
					fmt.Errorf("Guo - Resource has been exhausted"),
					"too_many_requests",
					http.StatusTooManyRequests,
				)
				errData, err := json.Marshal(openaiErr)
				if err == nil {
					resp.Body = io.NopCloser(bytes.NewBuffer(errData))
				} else {
					resp.Body = io.NopCloser(bytes.NewBuffer(requestBody))
				}
				c.Set("gemini_delay", delay)
			} else {
				// if c.GetString("gemini-img-url") != "" {
				// 	//图片生成, 且报错429, 尝试改为file模式
				// 	imgUrl := c.GetString("gemini-img-url")
				// 	fieldUrl := ""
				// 	if strings.HasPrefix(imgUrl, "http") || strings.HasPrefix(imgUrl, "https") {
				// 		fieldUrl = imgUrl
				// 	} else {
				// 		fieldUrl = random.StrToMd5(imgUrl)
				// 	}
				// 	common.RedisHashDel("image_url", random.StrToMd5(imgUrl))
				// 	mimeType, fileName, err := image.GetImageFromUrl(imgUrl, true)
				// 	if err == nil {
				// 		_, fileData, err := FileHandler(c, fieldUrl, imgUrl, mimeType, fileName)
				// 		if err == nil {
				// 			image.UpdateImageCacheWithGeminiFile(imgUrl, fileData)
				// 			logger.SysLogf("图片-429尝试生成Gemini-File - 成功: %s", fileData)
				// 		}
				// 	} else {
				// 		logger.SysLogf("图片-429尝试生成Gemini-File - 错误: %s", err)
				// 	}
				// } else if !meta.IsImageModel {
				// 	//非图片模型, 报429的, 尝试使用官方的
				// 	logger.SysLog("chat 429 尝试转官方lib中...")
				// 	meta.SelfImplement = true
				// }
				// resp.Body = io.NopCloser(bytes.NewBuffer(requestBody))
			}
		}
	}
	return resp, nil
}
func doRequest(c *gin.Context, req *http.Request) (*http.Response, error) {
	var client *http.Client
	if config.HttpProxy == "" {
		var transport http.RoundTripper
		client = &http.Client{
			Timeout:   time.Duration(config.RelayGeminiTimeout) * time.Second,
			Transport: transport,
		}
	} else {
		url, err := url.Parse(config.HttpProxy)
		if err != nil {
			return nil, fmt.Errorf("url.Parse failed: %w", err)
		}
		client = &http.Client{
			Timeout: time.Duration(config.RelayGeminiTimeout) * time.Second,
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
	if c.GetBool("hua_thinking") {
		meta.Thinking = true
	}
	if !meta.SelfImplement || meta.Mode == relaymode.Embeddings {
		//标记了流式 走流式输出
		if meta.IsStream {
			var responseText string
			if meta.IsImageModel {
				//如果是chat, 但请求的画图模型, 则走画图模型的渲染
				meta.Image2Chat = true
				err, responseText, usage = ImageStreamHandler(c, resp, meta)
			} else {
				err, responseText, usage = StreamHandler(c, resp, meta)
			}
			if err == nil {
				if usage.PromptTokens == 0 || usage.TotalTokens == 0 {
					usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
				}
			}
		} else {
			switch meta.Mode {
			case relaymode.Embeddings:
				err, usage = EmbeddingHandler(c, resp)
			case relaymode.ImagesEdit:
				fallthrough
			case relaymode.ImagesGenerations:
				err, usage = ImageHandler(c, resp, meta)
			default:
				if meta.IsImageModel {
					//如果是chat, 但请求的画图模型, 则走画图模型的渲染
					meta.Image2Chat = true
					err, usage = ImageHandler(c, resp, meta)
				} else {
					err, usage = Handler(c, resp, meta)
				}
			}
		}
	} else {
		logger.SysLog("开始使用官方lib请求>>>>>>")
		var responseText string
		usage, responseText, err = DoChatByGenai(c, meta)
		if err == nil {
			if usage.PromptTokens == 0 || usage.TotalTokens == 0 {
				usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
			}
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

func ChatOnline(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) {

}
