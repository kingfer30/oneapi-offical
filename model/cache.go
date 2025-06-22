package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
)

var (
	TokenCacheSeconds         = config.SyncFrequency
	UserId2GroupCacheSeconds  = config.SyncFrequency
	UserId2QuotaCacheSeconds  = config.SyncFrequency
	UserId2StatusCacheSeconds = config.SyncFrequency
	GroupModelsCacheSeconds   = config.SyncFrequency
)

func CacheGetTokenByKey(key string, reGet bool) (*Token, error) {
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	var token Token
	if !common.RedisEnabled || reGet {
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		return &token, err
	}
	tokenObjectString, err := common.RedisGet(fmt.Sprintf("token:%s", key))
	if err != nil {
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		if err != nil {
			return nil, err
		}
		jsonBytes, err := json.Marshal(token)
		if err != nil {
			return nil, err
		}
		err = common.RedisSet(fmt.Sprintf("token:%s", key), string(jsonBytes), time.Duration(TokenCacheSeconds)*time.Second)
		if err != nil {
			logger.SysError("Redis set token error: " + err.Error())
		}
		return &token, nil
	}
	err = json.Unmarshal([]byte(tokenObjectString), &token)
	return &token, err
}

func CacheGetUserGroup(id int) (group string, err error) {
	if !common.RedisEnabled {
		return GetUserGroup(id)
	}
	group, err = common.RedisGet(fmt.Sprintf("user_group:%d", id))
	if err != nil {
		group, err = GetUserGroup(id)
		if err != nil {
			return "", err
		}
		err = common.RedisSet(fmt.Sprintf("user_group:%d", id), group, time.Duration(UserId2GroupCacheSeconds)*time.Second)
		if err != nil {
			logger.SysError("Redis set user group error: " + err.Error())
		}
	}
	return group, err
}

func fetchAndUpdateUserQuota(ctx context.Context, id int) (quota int64, err error) {
	quota, err = GetUserQuota(id)
	if err != nil {
		return 0, err
	}
	err = common.RedisSet(fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	if err != nil {
		logger.Error(ctx, "Redis set user quota error: "+err.Error())
	}
	return
}

func CacheGetUserQuota(ctx context.Context, id int) (quota int64, err error) {
	if !common.RedisEnabled {
		return GetUserQuota(id)
	}
	quotaString, err := common.RedisGet(fmt.Sprintf("user_quota:%d", id))
	if err != nil {
		return fetchAndUpdateUserQuota(ctx, id)
	}
	quota, err = strconv.ParseInt(quotaString, 10, 64)
	if err != nil {
		return 0, nil
	}
	if quota <= config.PreConsumedQuota { // when user's quota is less than pre-consumed quota, we need to fetch from db
		logger.Infof(ctx, "user %d's cached quota is too low: %d, refreshing from db", quota, id)
		return fetchAndUpdateUserQuota(ctx, id)
	}
	return quota, nil
}

func CacheUpdateUserQuota(ctx context.Context, id int) error {
	if !common.RedisEnabled {
		return nil
	}
	quota, err := CacheGetUserQuota(ctx, id)
	if err != nil {
		return err
	}
	err = common.RedisSet(fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	return err
}

func CacheDecreaseUserQuota(id int, quota int64) error {
	if !common.RedisEnabled {
		return nil
	}
	err := common.RedisDecrease(fmt.Sprintf("user_quota:%d", id), int64(quota))
	return err
}

func CacheIsUserEnabled(userId int) (bool, error) {
	if !common.RedisEnabled {
		return IsUserEnabled(userId)
	}
	enabled, err := common.RedisGet(fmt.Sprintf("user_enabled:%d", userId))
	if err == nil {
		return enabled == "1", nil
	}

	userEnabled, err := IsUserEnabled(userId)
	if err != nil {
		return false, err
	}
	enabled = "0"
	if userEnabled {
		enabled = "1"
	}
	err = common.RedisSet(fmt.Sprintf("user_enabled:%d", userId), enabled, time.Duration(UserId2StatusCacheSeconds)*time.Second)
	if err != nil {
		logger.SysError("Redis set user enabled error: " + err.Error())
	}
	return userEnabled, err
}

func CacheGetGroupModels(ctx context.Context, group string) ([]string, error) {
	if !common.RedisEnabled {
		return GetGroupModels(group)
	}
	modelsStr, err := common.RedisGet(fmt.Sprintf("group_models:%s", group))
	if err == nil {
		return strings.Split(modelsStr, ","), nil
	}
	models, err := GetGroupModels(group)
	if err != nil {
		return nil, err
	}
	err = common.RedisSet(fmt.Sprintf("group_models:%s", group), strings.Join(models, ","), time.Duration(GroupModelsCacheSeconds)*time.Second)
	if err != nil {
		logger.SysError("Redis set group models error: " + err.Error())
	}
	return models, nil
}

var group2model2channels map[string]map[string][]*Channel
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	//写入一把锁用于并发锁
	if count, serr := common.RedisExists("CHANNEL_GENERATE_LOCK"); serr != nil || count == 0 {
		if ok, err := common.RedisSetNx("CHANNEL_GENERATE_LOCK", "1", time.Duration(10*time.Second)); ok || err == nil {
			InitChannelCacheByMem()
		}
	}
}
func InitChannelCacheByMem() {
	var channels []*Channel
	DB.Where("status = ?", ChannelStatusEnabled).Find(&channels)
	if config.ChannelBaseUrlList == nil {
		config.ChannelBaseUrlList = make(map[int]string)
	}
	for _, channel := range channels {
		channel.SleepModels = make(map[string]int64)
		if *channel.BaseURL != "" {
			config.ChannelBaseUrlList[channel.Id] = *channel.BaseURL
		}
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]*Channel)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]*Channel)
	}
	for _, channel := range channels {
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]*Channel, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return channels[i].GetPriority() > channels[j].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	channelSyncLock.Unlock()
	common.RedisDel("CHANNEL_GENERATE_LOCK")
	logger.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		logger.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func CacheGetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	if !config.MemoryCacheEnabled {
		return GetRandomSatisfiedChannel(group, model, ignoreFirstPriority)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	channels := group2model2channels[group][model]
	if len(channels) == 0 {
		return nil, errors.New("channel not found")
	}

	// 过滤掉被禁用当前模型的渠道
	var validChannels []*Channel
	for _, ch := range channels {
		ch.SleepLock.RLock()
		wakeupAt := ch.SleepModels[model]
		ch.SleepLock.RUnlock()
		if wakeupAt == 0 {
			validChannels = append(validChannels, ch)
		}
	}

	if len(validChannels) == 0 {
		return nil, errors.New("channel not found")
	}

	endIdx := len(validChannels)

	// choose by priority
	firstChannel := validChannels[0]
	if firstChannel.GetPriority() > 0 {
		for i := range validChannels {
			if validChannels[i].GetPriority() != firstChannel.GetPriority() {
				endIdx = i
				break
			}
		}
	}
	idx := rand.Intn(endIdx)
	if ignoreFirstPriority {
		if endIdx < len(validChannels) { // which means there are more than one priority
			idx = random.RandRange(endIdx, len(validChannels))
		}
	}
	return validChannels[idx], nil
}

// 新版锁定模型
func SleepChannel(group string, model string, channelId int, awakeTime int64) {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	channels := group2model2channels[group][model]
	for _, channel := range channels {
		if channel.Id == channelId {
			channel.SleepLock.Lock()
			channel.SleepModels[model] = awakeTime
			channel.SleepLock.Unlock()
			logger.SysLogf("渠道 - [%s(%d)] , model - [%s] 已休眠至 %d", channel.Name, channel.Id, model, awakeTime)
			break
		}
	}
}

// 渠道唤醒
func WakeupChannel(frequency int) {
	for {
		logger.SysLog("begining wakeup channel")
		var tasks []struct {
			channel *Channel
			model   string
		}
		func() {
			channelSyncLock.RLock()
			defer channelSyncLock.RUnlock()
			for _, models := range group2model2channels {
			mLoop:
				for model, channels := range models {
					for _, channel := range channels {
						if len(channel.SleepModels) > 0 {
							for sm, awakeTime := range channel.SleepModels {
								if sm == model && awakeTime <= helper.GetTimestamp() {
									tasks = append(tasks, struct {
										channel *Channel
										model   string
									}{
										channel: channel,
										model:   model,
									})
								}
							}
							continue mLoop
						}
					}
				}
			}
		}()
		channelSyncLock.Lock()
		for _, task := range tasks {
			task.channel.SleepLock.Lock()
			delete(task.channel.SleepModels, task.model)
			task.channel.SleepLock.Unlock()
			logger.SysLogf("渠道 - [%s(%d)] , model - [%s] 已唤醒", task.channel.Name, task.channel.Id, task.model)
		}
		channelSyncLock.Unlock()
		logger.SysLog("wakeup channel end")
		time.Sleep(time.Duration(frequency) * time.Second)
	}
}

// 获取错误的缓存key
func GetErrorCacheByKey(key string) (int, map[string]any, int, string, error) {
	if errString, serr := common.RedisGet(fmt.Sprintf("Auth_Error:%s", key)); serr == nil || errString != "" {
		var info map[string]any
		err := json.Unmarshal([]byte(errString), &info)
		if err != nil {
			return 0, nil, 0, "", err
		}
		statusCode, ok := info["statusCode"].(float64)
		if !ok {
			return 0, nil, 0, "", fmt.Errorf("no statusCode")
		}
		response, ok := info["response"].(map[string]interface{})
		if !ok {
			return 0, nil, 0, "", fmt.Errorf("no response")
		}
		tokenId, ok := info[ctxkey.TokenId].(float64)
		tokenName, ok := info[ctxkey.TokenName].(string)
		if statusCode <= 0 || response == nil {
			return 0, nil, 0, "", fmt.Errorf("empty response")
		}
		return int(statusCode), response, int(tokenId), tokenName, nil
	}
	return 0, nil, 0, "", fmt.Errorf("no key exists")
}

// 异步停止渠道软限制
func SyncCloseSoftLimitChannel(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		logger.SysLog("begining close soft_limit channel")
		channels, err := GetSoftLimitChannel()
		if err != nil {
			logger.SysErrorf("SyncCloseSoftLimitChannel error: %s", err.Error())
			continue
		}
		for _, channel := range channels {
			reason := fmt.Sprintf("当前渠道触发软限制自动停止，软限制使用量: %f，已使用: %f", float64(channel.SoftLimitUsd/500000), float64(channel.UsedQuota/500000))
			DisableChannel(channel.Id, ChannelStatusManuallyDisabled, channel.Name, reason)

			logger.SysLogf("【%s(%d)】%s", channel.Name, channel.Id, reason)
		}
		logger.SysLog("close soft_limit channel end")
	}
}
