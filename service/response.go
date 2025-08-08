package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/message"
)

// 返回消息渲染
func RenderMessage(msg string, id string) string {
	//如果存在request id, 说明上游返回了, 这里不再继续添加
	if !strings.Contains(msg, "(request id:") {
		msg = fmt.Sprintf("%s (request id: %s)", msg, id)
	}
	//避免返回上游信息
	for channelId, baseURL := range config.ChannelBaseUrlList {
		if strings.Contains(msg, baseURL) {
			//如果存在上游信息,
			//1. 记录日志
			logger.SysLogf("getting fail message from provider: %s", msg)
			//2. 发送告警
			prefix := msg
			if len(msg) >= 10 {
				prefix = msg[:10]
			}
			subject := fmt.Sprintf("渠道id[%d] 返回错误: %s", channelId, prefix)
			content := fmt.Sprintf("渠道id[%d] 返回错误: %s", channelId, msg)
			message.SendMailToAdmin(subject, content)
			//3. 消息脱敏
			pattern := `".+?"\s*:\s*dial tcp\s+[\d\.]+:\d+`
			re := regexp.MustCompile(pattern)
			msg = re.ReplaceAllString(msg, "our site")

			pattern = `".+?"\s*:\s*`
			re = regexp.MustCompile(pattern)
			msg = re.ReplaceAllString(msg, "our site")

			pattern = `(https?:\/\/)?[\w.-]+(:\d+)?\/[\S]*`
			re = regexp.MustCompile(pattern)
			msg = re.ReplaceAllString(msg, "our site")
		}
	}
	return msg
}
