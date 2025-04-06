package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/songquanpeng/one-api/common/config"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func RelayTextHelper(c *gin.Context) *model.ErrorWithStatusCode {
	ctx := c.Request.Context()
	meta := meta.GetByContext(c)
	// get & validate textRequest
	textRequest, err := getAndValidateTextRequest(c, meta.Mode)
	if err != nil {
		logger.Errorf(ctx, "getAndValidateTextRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "invalid_text_request", http.StatusBadRequest)
	}
	meta.IsStream = textRequest.Stream

	// map model name
	meta.OriginModelName = textRequest.Model
	textRequest.Model, _ = getMappedModelName(textRequest.Model, meta.ModelMapping)
	meta.ActualModelName = textRequest.Model
	// set system prompt if not empty
	systemPromptReset := setSystemPrompt(ctx, textRequest, meta.SystemPrompt)
	// get model ratio & group ratio
	modelRatio := billingratio.GetModelRatio(textRequest.Model, meta.ChannelType, meta.Group)
	groupRatio := billingratio.GetGroupRatio(meta.Group)
	ratio := modelRatio * groupRatio
	// pre-consume quota
	var promptTokens int
	//gemini不作计算了, 为了不触发token_counter
	if meta.APIType == apitype.Gemini {
		promptTokens = 0
	} else {
		promptTokens = getPromptTokens(textRequest, meta.Mode)
	}
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeQuota(ctx, textRequest, promptTokens, ratio, meta)
	if bizErr != nil {
		logger.Warnf(ctx, "preConsumeQuota failed: %+v", *bizErr)
		return bizErr
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(fmt.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	// no need to convert request for self-implement
	var resp *http.Response
	if !meta.SelfImplement || meta.Mode == relaymode.Embeddings {
		if meta.APIType == apitype.Gemini {
			meta.TextRequest = textRequest
		}
		// get request body
		requestBody, err := getRequestBody(c, meta, textRequest, adaptor)
		if err != nil {
			return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
		}

		// do request
		resp, err = adaptor.DoRequest(c, meta, requestBody)
		if err != nil {
			logger.Errorf(ctx, "DoRequest failed: %s", err.Error())
			return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
		}
		if !meta.SelfImplement && isErrorHappened(meta, resp) {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
			return RelayErrorHandler(resp)
		}
	} else {
		meta.TextRequest = textRequest
	}

	// do response
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		logger.Errorf(ctx, "respErr is not nil: %+v", respErr)
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return respErr
	}
	// post-consume quota
	go postConsumeQuota(ctx, usage, meta, textRequest, ratio, preConsumedQuota, modelRatio, groupRatio, systemPromptReset)
	return nil
}

func getRequestBody(c *gin.Context, meta *meta.Meta, textRequest *model.GeneralOpenAIRequest, adaptor adaptor.Adaptor) (io.Reader, error) {
	if !config.EnforceIncludeUsage && meta.APIType == apitype.OpenAI && meta.OriginModelName == meta.ActualModelName && meta.ChannelType != channeltype.Baichuan {
		// no need to convert request for openai
		return c.Request.Body, nil
	}

	// get request body
	var requestBody io.Reader
	convertedRequest, err := adaptor.ConvertRequest(c, meta.Mode, textRequest)
	if err != nil {
		logger.Debugf(c.Request.Context(), "converted request failed: %s\n", err.Error())
		return nil, err
	}
	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		logger.Debugf(c.Request.Context(), "converted request json_marshal_failed: %s\n", err.Error())
		return nil, err
	}
	logger.Debugf(c.Request.Context(), "converted request: \n%s", string(jsonData))
	requestBody = bytes.NewBuffer(jsonData)
	return requestBody, nil
}
