package vertexai

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

var ModelList = []string{
	"claude-3-haiku@20240307",
	"claude-3-sonnet@20240229",
	"claude-3-opus@20240229",
	"claude-3-5-sonnet@20240620",
	"claude-3-5-sonnet-v2@20241022",
	"claude-3-5-haiku@20241022",
}

const anthropicVersion = "vertex-2023-10-16"

type Adaptor struct {
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
	claudeReq := anthropic.ConvertRequest(*request)
	req := Request{
		AnthropicVersion: anthropicVersion,
		// Model:            claudeReq.Model,
		Messages:    claudeReq.Messages,
		System:      claudeReq.System,
		MaxTokens:   claudeReq.MaxTokens,
		Temperature: claudeReq.Temperature,
		TopP:        claudeReq.TopP,
		TopK:        claudeReq.TopK,
		Stream:      claudeReq.Stream,
		Tools:       claudeReq.Tools,
	}

	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, req)
	return req, nil
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
		err, usage = anthropic.StreamHandler(c, resp, meta)
	} else {
		err, usage = anthropic.Handler(c, resp, meta.PromptTokens, meta)
	}
	return
}
