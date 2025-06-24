package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	if !strings.HasPrefix(c.Request.URL.RawQuery, "retry=") {
		rpm := c.GetInt("token_rpm")
		if rpm == 0 {
			c.Next()
			return
		}
		maxRequestNum = rpm
		ctx := context.Background()
		rdb := common.RDB
		apiKey := c.GetString("api_key")
		modelName := c.GetString("request_model")
		ip := c.ClientIP()
		var key string
		if apiKey != "" {
			apiKey = strings.TrimPrefix(apiKey, "sk-")
			key = "rateLimit:" + mark + "_" + apiKey + "_" + modelName
		} else {
			key = "rateLimit:" + mark + "_" + ip
		}
		listLength, err := rdb.LLen(ctx, key).Result()
		if err != nil {
			fmt.Println(err.Error())
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		if listLength < int64(maxRequestNum) {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.Expire(ctx, key, time.Duration(duration)*time.Second)
		} else {
			oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
			oldTime, err := time.Parse(timeFormat, oldTimeStr)
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				c.Abort()
				return
			}
			nowTimeStr := time.Now().Format(timeFormat)
			nowTime, err := time.Parse(timeFormat, nowTimeStr)
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				c.Abort()
				return
			}
			current := int64(nowTime.Sub(oldTime).Seconds())
			// time.Since will return negative number!
			// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
			if (current > duration && (listLength-1) > int64(maxRequestNum)) || current < duration {
				//存在超记录的, 需要清掉这一部分
				if current > duration && (listLength-1) > int64(maxRequestNum) {
					rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
				}
				rdb.Expire(ctx, key, time.Duration(duration)*time.Second)
				var openAIError gin.H
				c.Writer.Header().Set("X-Ratelimit-Limit-Requests", strconv.Itoa(maxRequestNum))
				c.Writer.Header().Set("X-Ratelimit-Remaining-Requests", strconv.Itoa(int(int64(maxRequestNum)-listLength)))
				if apiKey != "" && modelName != "" {
					openAIError = gin.H{
						"message": helper.GetCustomReturnError(c, fmt.Sprintf("Rate limit reached for %s in api-key %s on requests per minute (RPM): Limit %d, Used %d, Requested 1", modelName, helper.EncryptKey(apiKey), maxRequestNum, listLength)).Error(),
						"type":    "guoguo_api_error",
					}
				} else {
					openAIError = gin.H{
						"message": helper.GetCustomReturnError(c, fmt.Sprintf("Rate limit reached for %s on requests per minute (RPM): Limit %d, Used %d, Requested 1", ip, maxRequestNum, listLength)).Error(),
						"type":    "guoguo_api_error",
					}
				}

				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": openAIError,
				})
				c.Abort()
				return
			} else {
				rdb.LPush(ctx, key, time.Now().Format(timeFormat))
				if current > duration {
					rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
				}
				rdb.Expire(ctx, key, time.Duration(duration)*time.Second)
			}
		}
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if maxRequestNum == 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(config.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.GlobalWebRateLimitNum, config.GlobalWebRateLimitDuration, "GW")
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.GlobalApiRateLimitNum, config.GlobalApiRateLimitDuration, "GA")
}

func CriticalRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.CriticalRateLimitNum, config.CriticalRateLimitDuration, "CT")
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.DownloadRateLimitNum, config.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.UploadRateLimitNum, config.UploadRateLimitDuration, "UP")
}

func RalayRPMRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.RalayRateLimitNum, config.RalayRateLimitDuration, "RALAY")
}
