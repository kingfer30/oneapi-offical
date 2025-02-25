package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"gorm.io/gorm"
)

const (
	ChannelStatusUnknown          = 0
	ChannelStatusEnabled          = 1 // don't use 0, 0 is the default value!
	ChannelStatusManuallyDisabled = 2 // also don't use 0
	ChannelStatusAutoDisabled     = 3
	ChannelStatusSleeping         = 4
	ChannelStatusUnActivate       = 5
)

type Channel struct {
	Id                 int     `json:"id"`
	Type               int     `json:"type" gorm:"default:0"`
	Key                string  `json:"key" gorm:"type:text"`
	Status             int     `json:"status" gorm:"default:1"`
	Name               string  `json:"name" gorm:"index"`
	UsedQuota          int64   `json:"used_quota" gorm:"bigint;default:0"`
	AwakeTime          int64   `json:"awake_time" gorm:"bigint;index:idx_status_awake_time"`
	Priority           *int64  `json:"priority" gorm:"bigint;default:0"`
	Config             string  `json:"config"`
	Weight             *uint   `json:"weight" gorm:"default:0"`
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`
	TestTime           int64   `json:"test_time" gorm:"bigint"`
	ResponseTime       int     `json:"response_time"` // in milliseconds
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`
	Other              *string `json:"other"`   // DEPRECATED: please save config to field Config
	Balance            float64 `json:"balance"` // in USD
	BalanceUpdatedTime int64   `json:"balance_updated_time" gorm:"bigint"`
	Models             string  `json:"models"`
	Group              string  `json:"group" gorm:"type:varchar(32);default:'default'"`
	ModelMapping       *string `json:"model_mapping" gorm:"type:varchar(1024);default:''"`
	SystemPrompt       *string `json:"system_prompt" gorm:"type:text"`
	RpmLimit           int     `json:"rpm_limit" gorm:"default:0"`
	DpmLimit           int     `json:"dpm_limit" gorm:"default:0"`
	TpmLimit           int     `json:"tpm_limit" gorm:"default:0"`
}

type ChannelConfig struct {
	Region            string `json:"region,omitempty"`
	SK                string `json:"sk,omitempty"`
	AK                string `json:"ak,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	APIVersion        string `json:"api_version,omitempty"`
	LibraryID         string `json:"library_id,omitempty"`
	Plugin            string `json:"plugin,omitempty"`
	VertexAIProjectID string `json:"vertex_ai_project_id,omitempty"`
	VertexAIADC       string `json:"vertex_ai_adc,omitempty"`
}

func GetAllChannels(startIdx int, num int, scope string) ([]*Channel, error) {
	var channels []*Channel
	var err error
	switch scope {
	case "all":
		err = DB.Order("id desc").Find(&channels).Error
	case "disabled":
		err = DB.Order("id desc").Where("status = ? or status = ?", ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled).Find(&channels).Error
	default:
		err = DB.Order("id desc").Limit(num).Offset(startIdx).Omit("key").Find(&channels).Error
	}
	return channels, err
}

func SearchChannels(keyword string) (channels []*Channel, err error) {
	err = DB.Omit("key").Where("id = ? or name LIKE ?", helper.String2Int(keyword), keyword+"%").Find(&channels).Error
	return channels, err
}

func GetChannelById(id int, selectAll bool) (*Channel, error) {
	channel := Channel{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&channel, "id = ?", id).Error
	} else {
		err = DB.Omit("key").First(&channel, "id = ?", id).Error
	}
	return &channel, err
}

func BatchInsertChannels(channels []Channel) error {
	var err error
	err = DB.Create(&channels).Error
	if err != nil {
		return err
	}
	for _, channel_ := range channels {
		err = channel_.AddAbilities()
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) GetPriority() int64 {
	if channel.Priority == nil {
		return 0
	}
	return *channel.Priority
}

func (channel *Channel) GetBaseURL() string {
	if channel.BaseURL == nil {
		return ""
	}
	return *channel.BaseURL
}

func (channel *Channel) GetModelMapping() map[string]string {
	if channel.ModelMapping == nil || *channel.ModelMapping == "" || *channel.ModelMapping == "{}" {
		return nil
	}
	modelMapping := make(map[string]string)
	err := json.Unmarshal([]byte(*channel.ModelMapping), &modelMapping)
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to unmarshal model mapping for channel %d, error: %s", channel.Id, err.Error()))
		return nil
	}
	return modelMapping
}

func (channel *Channel) Insert() error {
	var err error
	err = DB.Create(channel).Error
	if err != nil {
		return err
	}
	err = channel.AddAbilities()
	return err
}

func (channel *Channel) Update() error {
	var err error
	err = DB.Model(channel).Updates(channel).Error
	if err != nil {
		return err
	}
	DB.Model(channel).First(channel, "id = ?", channel.Id)
	err = channel.UpdateAbilities()
	return err
}

func (channel *Channel) UpdateResponseTime(responseTime int64) {
	err := DB.Model(channel).Select("response_time", "test_time").Updates(Channel{
		TestTime:     helper.GetTimestamp(),
		ResponseTime: int(responseTime),
	}).Error
	if err != nil {
		logger.SysError("failed to update response time: " + err.Error())
	}
}

func (channel *Channel) UpdateBalance(balance float64) {
	err := DB.Model(channel).Select("balance_updated_time", "balance").Updates(Channel{
		BalanceUpdatedTime: helper.GetTimestamp(),
		Balance:            balance,
	}).Error
	if err != nil {
		logger.SysError("failed to update balance: " + err.Error())
	}
}

func (channel *Channel) Delete() error {
	var err error
	err = DB.Delete(channel).Error
	if err != nil {
		return err
	}
	err = channel.DeleteAbilities()
	return err
}

func (channel *Channel) LoadConfig() (ChannelConfig, error) {
	var cfg ChannelConfig
	if channel.Config == "" {
		return cfg, nil
	}
	err := json.Unmarshal([]byte(channel.Config), &cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

func UpdateChannelStatusById(id int, status int) {
	err := UpdateAbilityStatus(id, status == ChannelStatusEnabled)
	if err != nil {
		logger.SysError("failed to update ability status: " + err.Error())
	}
	err = DB.Model(&Channel{}).Where("id = ?", id).Update("status", status).Error
	if err != nil {
		logger.SysError("failed to update channel status: " + err.Error())
	}
}

func SleepChannel(id int, awakeTime int64) (bool, error) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Updates(Channel{
		Status:    ChannelStatusSleeping,
		AwakeTime: awakeTime,
	}).Error
	if err != nil {
		return false, err
	}
	return true, nil
}

func WakeupChannel() ([]int, error) {
	var channelIDs []int
	err := DB.Model(&Channel{}).Where("status = ? and awake_time <= ?", ChannelStatusSleeping, helper.GetTimestamp()).Pluck("id", &channelIDs).Error
	if err != nil {
		logger.SysError(fmt.Sprintf("WakeupChannel faild: %s ", err.Error()))
	}
	result := DB.Model(&Channel{}).Where("id IN ?", channelIDs).Updates(Channel{
		Status: ChannelStatusEnabled,
	})
	return channelIDs, result.Error
}

func UpdateChannelUsedQuota(id int, quota int64) {
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeChannelUsedQuota, id, quota)
		return
	}
	updateChannelUsedQuota(id, quota)
}

func updateChannelUsedQuota(id int, quota int64) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error
	if err != nil {
		logger.SysError("failed to update channel used quota: " + err.Error())
	}
}

func DeleteChannelByStatus(status int64) (int64, error) {
	result := DB.Where("status = ?", status).Delete(&Channel{})
	return result.RowsAffected, result.Error
}

func DeleteDisabledChannel() (int64, error) {
	result := DB.Where("status = ? or status = ?", ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled).Delete(&Channel{})
	return result.RowsAffected, result.Error
}

// 激活渠道
func ActivateChannel(limit int64) bool {
	var data []map[string]interface{}
	err := DB.Model(&Channel{}).Select(fmt.Sprintf("`type`, `group`, CONVERT(sum(if(`status`= %d or `status`=%d,1,0)), SIGNED) as num", ChannelStatusEnabled, ChannelStatusSleeping)).Group("`type`, `group`").Scan(&data).Error
	if err != nil {
		logger.SysErrorf("ActivateChannel - failed to scan channel status: %s", err)
		return false
	}
	for _, record := range data {
		num := record["num"].(int64)
		t := record["type"].(int64)
		g := record["group"].(string)
		gp := GroupInfo[g]
		var l int64
		if gp == nil {
			l = limit
		} else {
			l = GroupInfo[g].ActiveNum
			if l == 0 {
				l = limit
			}
		}
		if num < l {
			actNum := l - num
			res := DB.Model(&Channel{}).Where("`type` = ? and `status` = ? and `group` = ?", t, ChannelStatusUnActivate, g).Limit(int(actNum)).Updates(Channel{
				Status: ChannelStatusEnabled,
			})
			if res.Error != nil {
				logger.SysErrorf("ActivateChannel - failed to update channel status: %s", res.Error)
			}
			logger.SysLogf("Task - ActivateChannel - update channel status, group: %s, type: %d, count: %d", g, t, res.RowsAffected)
		}
	}
	return true
}

func UpdateChannelsAbilities() (any, error) {
	result := []map[string]interface{}{}
	var err error = nil
	d := DB.Exec("SET SESSION group_concat_max_len=102400")
	if d.Error != nil {
		return false, d.Error
	}
	err = DB.Raw("select `group`, models, group_concat(id,'_',priority) as ids from `channels`  where `status` = ? group by `group`, models", ChannelStatusEnabled).Find(&result).Error
	if err != nil {
		return false, err
	}

	var abilities = []Ability{}
	for _, data := range result {
		groups := strings.Split(data["group"].(string), ",")
		models := strings.Split(data["models"].(string), ",")
		ids := strings.Split(data["ids"].(string), ",")
		if len(groups) > 0 {
			for _, group := range groups {
				if len(models) > 0 {
					for _, model := range models {
						if len(ids) > 0 {
							for _, idStr := range ids {
								var channel_id int
								var priority int64
								parts := strings.Split(idStr, "_")
								if len(parts) == 0 {
									continue
								}
								if len(parts) == 1 {
									channel_id, _ = strconv.Atoi(parts[0])
									priority = 0
								} else {
									channel_id, _ = strconv.Atoi(parts[0])
									priority, _ = strconv.ParseInt(parts[1], 10, 64)
								}
								abilities = append(abilities, Ability{
									Group:     group,
									Model:     model,
									Enabled:   true,
									ChannelId: channel_id,
									Priority:  &priority,
								})

							}
						}
					}
				}
			}
		}
	}
	if len(abilities) > 0 {
		//清空表数据
		// 开始事务
		tx := DB.Begin()
		if tx.Error != nil {
			return false, tx.Error
		}
		d := DB.Exec("TRUNCATE TABLE abilities")
		if d.Error != nil {
			return false, d.Error
		}
		//重新插入
		batchSize := 5000
		var ind = 1
		for i := 0; i < len(abilities); i += batchSize {
			end := i + batchSize
			if end > len(abilities) {
				end = len(abilities)
			}
			batch := abilities[i:end]
			// 插入数据
			if err := tx.Create(batch).Error; err != nil {
				tx.Rollback()
				return false, err
			}
			ind++
		}
		// 提交事务
		tx.Commit()
		//更新缓存
		InitChannelCache()
	}
	return true, nil
}
