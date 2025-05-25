package anthropic

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

type Adaptor struct {
}

func (a *Adaptor) Init(meta *meta.Meta) {

}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return fmt.Sprintf("%s/v1/messages", meta.BaseURL), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("x-api-key", meta.APIKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", "messages-2023-12-15")

	// https://x.com/alexalbert__/status/1812921642143900036
	// claude-3-5-sonnet can support 8k context
	if strings.HasPrefix(meta.ActualModelName, "claude-3-5-sonnet") {
		req.Header.Set("anthropic-beta", "max-tokens-3-5-sonnet-2024-07-15")
	}

	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if request.Thinking != nil && request.Thinking.Type == "enabled" && request.Thinking.IncludeThinking {
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
	return ConvertRequest(*request), nil
}

func (a *Adaptor) ConvertImageRequest(request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
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
	if meta.IsStream {
		err, usage = StreamHandler(c, resp, meta)
	} else {
		err, usage = Handler(c, resp, meta.PromptTokens, meta)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "anthropic"
}

func (a *Adaptor) ConvertVideoRequest(request *model.VideoRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}
