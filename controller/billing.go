package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/billing"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/model"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func GetSubscription(c *gin.Context) {
	var remainQuota float64
	var usedQuota float64
	var hardLimitUsd float64
	var token *model.Token
	var expiredTime int64
	value, exists := c.Get("token")
	if !exists {
		openAIError := relaymodel.Error{
			Message: helper.GetCustomReturnError(c, fmt.Sprintf("Incorrect API key provided: %s", helper.EncryptKey(token.Key))).Error(),
			Type:    "upstream_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	token = value.(*model.Token)
	if config.DisplayTokenStatEnabled {
		expiredTime = token.ExpiredTime
		remainQuota = float64(token.RemainQuota)
		usedQuota = float64(token.UsedQuota)
		hardLimitUsd = float64(token.HardLimitUsd)
	} else {
		r, _ := model.GetUserQuota(token.UserId)
		remainQuota = float64(r)
		u, _ := model.GetUserUsedQuota(token.UserId)
		usedQuota = float64(u)
	}
	if expiredTime <= 0 {
		expiredTime = 0
	}
	quota := remainQuota + usedQuota
	amount := float64(quota)
	if config.DisplayInCurrencyEnabled {
		amount /= config.QuotaPerUnit
		hardLimitUsd /= config.QuotaPerUnit
		usedQuota /= config.QuotaPerUnit
		remainQuota /= config.QuotaPerUnit
	}
	if token != nil && token.UnlimitedQuota {
		amount = 100000000
	}
	var status = "normal"
	switch token.Status {
	case model.TokenStatusDisabled:
		status = "banned"
	case model.TokenStatusExpired:
		status = "expired"
	case model.TokenStatusExhausted:
		status = "exhausted"
	}
	var isGPT4 = false
	group, err := model.GetUserGroup(token.UserId)
	if err == nil {
		if strings.Contains(group, "gpt4") {
			isGPT4 = true
		}
	}

	subscription := OpenAISubscriptionResponse{
		Object:             "billing_subscription",
		HasPaymentMethod:   true,
		SoftLimitUSD:       hardLimitUsd,
		HardLimitUSD:       hardLimitUsd,
		SystemHardLimitUSD: hardLimitUsd,
		UsedUSD:            usedQuota,
		BalanceUSD:         remainQuota,
		AccessUntil:        expiredTime,
		Status:             status,
		GPT4:               isGPT4,
		ExpireAt:           time.Unix(expiredTime, 0).Format("2006-01-02"),
	}
	c.JSON(200, subscription)
	return
}

func GetUsage(c *gin.Context) {
	var quota int64
	var err error
	var token *model.Token
	value, exists := c.Get("token")
	if !exists {
		openAIError := relaymodel.Error{
			Message: helper.GetCustomReturnError(c, fmt.Sprintf("Incorrect API key provided: %s", helper.EncryptKey(token.Key))).Error(),
			Type:    "upstream_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	token = value.(*model.Token)
	if config.DisplayTokenStatEnabled {
		quota = token.UsedQuota
	} else {
		quota, err = model.GetUserUsedQuota(token.UserId)
	}
	if err != nil {
		openAIError := relaymodel.Error{
			Message: err.Error(),
			Type:    "guoguo_api_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	amount := float64(quota)
	if config.DisplayInCurrencyEnabled {
		amount /= config.QuotaPerUnit
	}
	usage := billing.OpenAIUsageResponse{
		Object:     "list",
		DailyCosts: nil,
		TotalUsage: amount * 100,
	}
	c.JSON(200, usage)
	return
}

func GetUsageDetail(c *gin.Context) {
	var quota int64
	var err error
	var token *model.Token
	value, exists := c.Get("token")
	if !exists {
		openAIError := relaymodel.Error{
			Message: helper.GetCustomReturnError(c, fmt.Sprintf("Incorrect API key provided: %s", helper.EncryptKey(token.Key))).Error(),
			Type:    "upstream_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	token = value.(*model.Token)
	if config.DisplayTokenStatEnabled {
		quota = token.UsedQuota
	} else {
		quota, err = model.GetUserUsedQuota(token.UserId)
	}
	if err != nil {
		openAIError := relaymodel.Error{
			Message: err.Error(),
			Type:    "guoguo_api_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	amount := float64(quota)
	if config.DisplayInCurrencyEnabled {
		amount /= config.QuotaPerUnit
	}
	detail, err := model.GetTokenLog(token.Id)
	if err != nil {
		openAIError := relaymodel.Error{
			Message: err.Error(),
			Type:    "guoguo_api_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	usage := billing.OpenAIUsageDetailResponse{
		Object:      "detail",
		DetailCosts: detail,
	}
	c.JSON(200, usage)
	return
}
