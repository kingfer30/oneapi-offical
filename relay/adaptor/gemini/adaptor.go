package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

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

	version := helper.AssignOrDefault(meta.Config.APIVersion, defaultVersion)
	action := ""
	switch meta.Mode {
	case relaymode.Embeddings:
		action = "batchEmbedContents"
	case relaymode.VideoGenerations:
		action = "predictLongRunning"
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
	if strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		c.Request.Header.Set("Content-Type", "application/json")
	}
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
	if request.Thinking == nil || (request.Thinking != nil && request.Thinking.Type == "enabled" && request.Thinking.IncludeThinking != false) {
		c.Set("include_think", true)
		if request.Thinking != nil && request.Thinking.Type == "enabled" && request.Thinking.ThinkingTag != nil {
			c.Set("thinking_tag_start", request.Thinking.ThinkingTag.Start)
			c.Set("thinking_tag_end", request.Thinking.ThinkingTag.End)
			if request.Thinking.ThinkingTag.BlockTag {
				c.Set("thinking_tag_block", true)
			}
		} else {
			c.Set("thinking_tag_start", "<think>")
			c.Set("thinking_tag_end", "</think>")
		}
	}
	switch relayMode {
	case relaymode.Embeddings:
		geminiEmbeddingRequest := ConvertEmbeddingRequest(*request)
		return geminiEmbeddingRequest, nil
	default:
		needHighTPMRequest := false
		if IsLowTpmModel(request.Model) {
			apiKey := strings.TrimPrefix(c.Request.Header.Get("Authorization"), "Bearer ")
			total, err := GetTokens(c, request, apiKey)
			if err == nil {
				if total > LowTPMModelMapping[request.Model] {
					logger.SysLogf("[高Token重试] 当前TPM过高 %d", total)
					needHighTPMRequest = true
				}
			}
		}
		var geminiRequest *ChatRequest
		var err error
		if needHighTPMRequest {
			c.Set("no_retry_high_tpm", true)
			geminiRequest, err = ChangeChat2TxtRequest(c, *request)
		} else {
			geminiRequest, err = ConvertRequest(c, *request)
		}
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
	resp, err := DoRequest(c, req)
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

	return resp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if c.GetBool("include_think") {
		meta.IncludeThinking = true
	}
	if c.GetString("thinking_tag_start") != "" {
		meta.ThinkingTagStart = c.GetString("thinking_tag_start")
	}
	if c.GetString("thinking_tag_end") != "" {
		meta.ThinkingTagEnd = c.GetString("thinking_tag_end")
	}
	if c.GetBool("thinking_tag_block") {
		meta.EnableBlockTag = true
	}
	if meta.Mode == relaymode.VideoGenerations {
		VideoHandler(c, resp, meta)
	} else if !meta.SelfImplement || meta.Mode == relaymode.Embeddings {
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

func (a *Adaptor) ConvertVideoRequest(c *gin.Context, request *model.VideoRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	var geminiRequest *VideoRequest
	var err error
	geminiRequest, err = ConvertVideoRequest(c, request)
	if err != nil {
		return nil, err
	}
	return geminiRequest, nil
}
