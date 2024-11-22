package monitor

import (
	"fmt"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/message"
	"github.com/songquanpeng/one-api/model"
)

func notifyRootUser(subject string, content string) {
	if config.MessagePusherAddress != "" {
		err := message.SendMessage(subject, content, content)
		if err != nil {
			logger.SysError(fmt.Sprintf("failed to send message: %s", err.Error()))
		} else {
			return
		}
	}
	if config.RootUserEmail == "" {
		config.RootUserEmail = model.GetRootUserEmail()
	}
	err := message.SendEmail(subject, config.RootUserEmail, content)
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to send email: %s", err.Error()))
	}
}

func syncUpdateChannel() {
	//异步执行更新
	go func() {
		//写入一把锁用于并发锁
		if count, serr := common.RedisExists("CHANNEL_GENERATE_LOCK"); serr != nil || count == 0 {
			if ok, err := common.RedisSetNx("CHANNEL_GENERATE_LOCK", "1", time.Duration(10*time.Second)); ok || err == nil {
				//默认方式则重新初始化
				model.InitChannelCache()
			}
		}
	}()
}

// DisableChannel disable & notify
func SleepChannel(channelId int, channelName string, awakeTime int64) {
	model.SleepChannel(channelId, awakeTime)
	logger.SysLog(fmt.Sprintf("channel #%d has been disabled: %s", channelId, "自动睡眠"))
	//异步执行更新
	syncUpdateChannel()
}

// DisableChannel disable & notify
func DisableChannel(channelId int, channelName string, reason string) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusAutoDisabled)
	logger.SysLog(fmt.Sprintf("channel #%d has been disabled: %s", channelId, reason))
	subject := fmt.Sprintf("渠道「%s」（#%d）已被禁用", channelName, channelId)
	content := fmt.Sprintf("渠道「%s」（#%d）已被禁用，原因：%s", channelName, channelId, reason)
	notifyRootUser(subject, content)
	//异步执行更新
	syncUpdateChannel()
}

func MetricDisableChannel(channelId int, successRate float64) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusAutoDisabled)
	logger.SysLog(fmt.Sprintf("channel #%d has been disabled due to low success rate: %.2f", channelId, successRate*100))
	subject := fmt.Sprintf("渠道 #%d 已被禁用", channelId)
	content := fmt.Sprintf("该渠道（#%d）在最近 %d 次调用中成功率为 %.2f%%，低于阈值 %.2f%%，因此被系统自动禁用。",
		channelId, config.MetricQueueSize, successRate*100, config.MetricSuccessRateThreshold*100)
	notifyRootUser(subject, content)
}

// EnableChannel enable & notify
func EnableChannel(channelId int, channelName string) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusEnabled)
	logger.SysLog(fmt.Sprintf("channel #%d has been enabled", channelId))
	subject := fmt.Sprintf("渠道「%s」（#%d）已被启用", channelName, channelId)
	content := fmt.Sprintf("渠道「%s」（#%d）已被启用", channelName, channelId)
	notifyRootUser(subject, content)
}

// 渠道唤醒
func WakeupChannel(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		logger.SysLog("begining wakeup channel")
		ids, err := model.WakeupChannel()
		if err != nil {
			logger.SysError(fmt.Sprintf("SyncWakeupChannel error: %s", err.Error()))
			continue
		}

		if len(ids) > 0 {
			logger.SysLog(fmt.Sprintf("已唤醒休眠渠道: %d 个", len(ids)))
		}
		logger.SysLog("wakeup channel end")
	}
}

func DelFile(channelId int, fileId string) {
	err := model.DelFileByFileId(channelId, fileId)
	if err != nil {
		logger.SysLogf("DelFileByFileId failed: %s", err.Error())
	}
	logger.SysLogf("DelFileByFileId : %d - %s", channelId, fileId)
}
