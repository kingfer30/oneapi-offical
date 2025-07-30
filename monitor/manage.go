package monitor

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/message"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/model"
)

func ShouldDisableChannel(err *model.Error, statusCode int, channelId int, channelType int) bool {
	if !config.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if statusCode == http.StatusUnauthorized {
		return true
	}
	switch err.Type {
	case "insufficient_quota", "authentication_error", "permission_error", "forbidden":
		return true
	}
	if err.Code == "invalid_api_key" || err.Code == "account_deactivated" {
		return true
	}

	lowerMessage := strings.ToLower(err.Message)
	if strings.Contains(lowerMessage, "your access was terminated") ||
		strings.Contains(lowerMessage, "violation of our policies") ||
		strings.Contains(lowerMessage, "your credit balance is too low") ||
		strings.Contains(lowerMessage, "you have reached your specified api usage limits") ||
		strings.Contains(lowerMessage, "organization has been disabled") ||
		strings.Contains(lowerMessage, "credit") ||
		strings.Contains(lowerMessage, "balance") ||
		strings.Contains(lowerMessage, "permission denied") ||
		strings.Contains(lowerMessage, "organization has been restricted") || // groq
		strings.Contains(lowerMessage, "已欠费") ||
		strings.Contains(lowerMessage, "quota exceeded for quota metric 'generate content api requests per minute'") || // gemini
		strings.Contains(lowerMessage, "api key not found. please pass a valid api key") || // gemini
		strings.Contains(lowerMessage, "api key expired. please renew the api key") || // gemini
		strings.Contains(lowerMessage, "permission denied: consumer 'api_key:ai") {

		if channelType == channeltype.Custom {
			//需要针对自定义渠道做优化, 10次收到以下错误信息, 再做禁用
			channelCount, serr := common.RedisGet(fmt.Sprintf("channel_fail:%d", channelId))
			num, _ := strconv.Atoi(channelCount)
			if serr != nil || num <= 10 {
				num += 1
				ok, _ := common.RedisSetNx(fmt.Sprintf("channel_fail:%d", channelId), string(num), time.Duration(60*time.Second))
				if ok {
					subject := fmt.Sprintf("渠道ID「%d」返回错误(%s)，次数(%d)，请关注", channelId, lowerMessage, num)
					content := fmt.Sprintf("渠道ID「%d」出现错误: %s，累计10次将被禁用，当前累计错误次数: %d, 剩余次数: %d", channelId, lowerMessage, num, (10 - num))
					message.SendMailToAdmin(subject, content)
					return false
				}
			}
		}

		return true
	}
	return false
}

func ShouldDelFile(c *gin.Context, err *model.Error) bool {
	lowerMessage := strings.ToLower(err.Message)
	if strings.Contains(lowerMessage, "you do not have permission to access the file") ||
		strings.Contains(lowerMessage, "quota exceeded for quota metric 'generate content api requests per minute'") ||
		strings.Contains(lowerMessage, "permission denied: consumer 'api_key:ai") {
		return true
	}
	return false
}

func ShouldSleepChannel(channelType int, err *model.Error, statusCode int) bool {
	lowerMessage := strings.ToLower(err.Message)
	if strings.Contains(lowerMessage, "resource has been exhauste") ||
		(strings.Contains(lowerMessage, "you exceeded your current quota") && channelType == channeltype.Gemini) ||
		strings.Contains(lowerMessage, "e.g. check quota") {
		return true
	}
	return false
}

func ShouldEnableChannel(err error, openAIErr *model.Error) bool {
	if !config.AutomaticEnableChannelEnabled {
		return false
	}
	if err != nil {
		return false
	}
	if openAIErr != nil {
		return false
	}
	return true
}
