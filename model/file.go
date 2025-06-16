package model

import (
	"fmt"

	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
)

type Files struct {
	Id          int    `json:"id"`
	Model       string `json:"model" gorm:"type:varchar(1024);index;"`
	TokenId     int    `json:"token_id" gorm:"index;"`
	Key         string `json:"key" gorm:"index;"`
	ChannelId   int    `json:"channel_id" gorm:"index"`
	ContentType string `json:"content_type" gorm:"type:varchar(1024);"`
	Url         string `json:"url" gorm:"type:varchar(1024);index"`
	FileId      string `json:"file_id" gorm:"type:varchar(1024);"`
	ExpiredTime int64  `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
}

func (file *Files) SaveFile() (error, int) {
	//文件2天有效
	file.ExpiredTime = helper.GetTimestamp() + (60 * 60 * 24 * 2)
	err := DB.Create(file).Error
	if err != nil {
		return err, 0
	}
	return nil, file.Id
}

// 获取文件
func GetFile(model string, url string) (*Files, error) {
	var file *Files
	err := DB.Order("id desc").Where("model = ? AND url = ?", model, url).Find(&file).Error
	return file, err
}

// 删除key对应的文件列表
func DelFileByChannelId(channelId int) (err error) {
	result := DB.Where("channel_id = ?", channelId).Delete(&Files{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// 删除key对应的某个文件
func DelFileByFileId(fileId string) (err error) {
	result := DB.Where("file_id = ?", fileId).Delete(&Files{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func DelExpiredFile() ([]int, error) {
	var fileIDs []int
	err := DB.Model(&Files{}).Where("expired_time <= ?", helper.GetTimestamp()).Pluck("id", &fileIDs).Error
	if err != nil {
		logger.SysError(fmt.Sprintf("DelExpiredFile faild: %s ", err.Error()))
	}
	result := DB.Where("id IN ?", fileIDs).Delete(&Files{})
	return fileIDs, result.Error
}
