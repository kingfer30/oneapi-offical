package aws

import (
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

var _ utils.AwsAdapter = new(Adaptor)

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
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, claudeReq)
	return claudeReq, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, awsCli *bedrockruntime.Client, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
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
		err, usage = StreamHandler(c, awsCli, meta)
	} else {
		err, usage = Handler(c, awsCli, meta)
	}
	return
}
