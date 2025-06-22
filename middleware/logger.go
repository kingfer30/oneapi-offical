package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/helper"
)

func SetUpLogger(server *gin.Engine) {
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		var tokenId int
		var tokenName string
		var log string
		if param.Keys != nil {
			requestID = param.Keys[helper.RequestIdKey].(string)
			if param.Keys["token_id"] != nil {
				tokenId = param.Keys["token_id"].(int)
			}
			if param.Keys["token_name"] != nil {
				tokenName = param.Keys["token_name"].(string)
			}
			if param.StatusCode != http.StatusOK {
				log = fmt.Sprintf("[GIN] %s | %s | %3d | %13v | %15s | %s(%d) | %7s %s\n",
					param.TimeStamp.Format("2006/01/02 15:04:05"),
					requestID,
					param.StatusCode,
					param.Latency,
					param.ClientIP,
					tokenName,
					tokenId,
					param.Method,
					param.Path,
				)
			} else {
				log = fmt.Sprintf("[GIN] %s | %3d | %13v | %15s | %s(%d) | %7s %s\n",
					param.TimeStamp.Format("2006/01/02 15:04:05"),
					param.StatusCode,
					param.Latency,
					param.ClientIP,
					tokenName,
					tokenId,
					param.Method,
					param.Path,
				)
			}
		} else {
			log = fmt.Sprintf("[GIN] %s | %s | %3d | %13v | %15s | %7s %s\n",
				param.TimeStamp.Format("2006/01/02 - 15:04:05"),
				requestID,
				param.StatusCode,
				param.Latency,
				param.ClientIP,
				param.Method,
				param.Path,
			)
		}
		return log
	}))
}
