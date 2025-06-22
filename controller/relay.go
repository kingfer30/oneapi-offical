package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/middleware"
	dbmodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/monitor"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/controller"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// https://platform.openai.com/docs/api-reference/chat

func relayHelper(c *gin.Context, relayMode int) *model.ErrorWithStatusCode {
	var err *model.ErrorWithStatusCode
	switch relayMode {
	case relaymode.VideoGenerations:
		err = controller.RelayVideoHelper(c, relayMode)
	case relaymode.ImagesGenerations:
		err = controller.RelayImageHelper(c, relayMode)
	case relaymode.ImagesEdit:
		if c.GetInt(ctxkey.Channel) == channeltype.Gemini {
			err = controller.RelayImageHelper(c, relayMode)
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": model.Error{
					Message: "API not implemented",
					Type:    "guoguo_api_error",
					Param:   "",
					Code:    "api_not_implemented",
				},
			})
			return nil
		}
	case relaymode.AudioSpeech:
		fallthrough
	case relaymode.AudioTranslation:
		fallthrough
	case relaymode.AudioTranscription:
		err = controller.RelayAudioHelper(c, relayMode)
	case relaymode.Proxy:
		err = controller.RelayProxyHelper(c, relayMode)
	default:
		err = controller.RelayTextHelper(c)
	}
	return err
}

func Relay(c *gin.Context) {
	ctx := c.Request.Context()
	relayMode := relaymode.GetByPath(c.Request.URL.Path)
	if config.DebugEnabled {
		requestBody, _ := common.GetRequestBody(c)
		logger.Debugf(ctx, "request body: %s", string(requestBody))
	}
	channelId := c.GetInt(ctxkey.ChannelId)
	userId := c.GetInt(ctxkey.Id)
	bizErr := relayHelper(c, relayMode)
	if bizErr == nil {
		monitor.Emit(channelId, true)
		return
	}
	lastFailedChannelId := channelId
	channelName := c.GetString(ctxkey.ChannelName)
	channelType := c.GetInt(ctxkey.Channel)
	group := c.GetString(ctxkey.Group)
	tokenName := c.GetString(ctxkey.TokenName)
	originalModel := c.GetString(ctxkey.OriginalModel)
	go func(c *gin.Context) {
		processChannelRelayError(c, userId, channelId, channelName, tokenName, group, originalModel, channelType, bizErr)
	}(c.Copy())
	requestId := c.GetString(helper.RequestIdKey)
	retryTimes := config.RetryTimes
	if !shouldRetry(c, bizErr.StatusCode) {
		logger.Errorf(ctx, "relay error happen, status code is %d, won't retry in this case", bizErr.StatusCode)
		retryTimes = 0
	}
	for i := retryTimes; i > 0; i-- {
		channel, err := dbmodel.CacheGetRandomSatisfiedChannel(group, originalModel, i != retryTimes)
		if err != nil {
			logger.Errorf(ctx, "CacheGetRandomSatisfiedChannel failed: %+v", err)
			break
		}
		if channel.Id == lastFailedChannelId {
			continue
		}
		middleware.SetupContextForSelectedChannel(c, channel, originalModel)
		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		bizErr = relayHelper(c, relayMode)
		if bizErr == nil {
			return
		}
		channelId := c.GetInt(ctxkey.ChannelId)
		lastFailedChannelId = channelId
		channelName := c.GetString(ctxkey.ChannelName)
		// BUG: bizErr is in race condition
		go func(c *gin.Context) {
			processChannelRelayError(c, userId, channelId, channelName, tokenName, group, originalModel, channelType, bizErr)
		}(c.Copy())
	}
	if bizErr != nil {
		// if bizErr.StatusCode == http.StatusTooManyRequests {
		// 	bizErr.Error.Message = "The current group was overload, please try again later"
		// }

		// BUG: bizErr is in race condition
		bizErr.Error.Message = helper.MessageWithRequestId(bizErr.Error.Message, requestId)
		c.JSON(bizErr.StatusCode, gin.H{
			"error": bizErr.Error,
		})
	}
}

func shouldRetry(c *gin.Context, statusCode int) bool {
	if _, ok := c.Get(ctxkey.SpecificChannelId); ok {
		return false
	}
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	if statusCode/100 == 5 {
		return true
	}
	if statusCode == http.StatusBadRequest {
		return false
	}
	if statusCode/100 == 2 {
		return false
	}
	return true
}

func processChannelRelayError(c *gin.Context, userId int, channelId int, channelName string, tokenName string, group string, modelName string, channelType int, err *model.ErrorWithStatusCode) {
	logger.Errorf(c.Request.Context(), "relay error (channel id %d, user id: %d, token name: %s): %s", channelId, userId, tokenName, err.Message)

	if monitor.ShouldSleepChannel(channelType, &err.Error, err.StatusCode) {
		var awakeTime int64
		//gemini的重试
		delay := c.GetInt("gemini_delay")
		if delay > 0 {
			awakeTime = helper.GetTimestamp() + int64(delay)
		}
		if awakeTime == 0 {
			awakeTime = helper.GetTimestamp() + 60
		}
		monitor.SleepChannel(group, modelName, channelId, awakeTime)
	}
	if monitor.ShouldDelFile(c, &err.Error) {
		fileUri := c.GetString("FileUri")
		if fileUri != "" {
			monitor.DelFile(channelId, fileUri)
		}
	}
	// https://platform.openai.com/docs/guides/error-codes/api-errors
	if monitor.ShouldDisableChannel(&err.Error, err.StatusCode, channelId, channelType) {
		monitor.DisableChannel(channelId, channelName, err.Message)
	} else {
		monitor.Emit(channelId, false)
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := model.Error{
		Message: "API not implemented",
		Type:    "guoguo_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := model.Error{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}
