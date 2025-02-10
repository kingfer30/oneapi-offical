package model

import (
	"encoding/json"
	"fmt"
	"strings"

	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
)

type Group struct {
	Id          int    `json:"id"`
	Type        string `json:"type" gorm:"default:''"`
	Name        string `json:"name" gorm:"index"`
	Models      string `json:"models"`
	Ratio       string `json:"ratio"`
	ActiveNum   int64  `json:"active_num" gorm:"default:0"`
	Status      int    `json:"status" gorm:"default:1;index:idx_status"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
}

var GroupModels = make(map[string]string)
var GroupInfo = make(map[string]*Group)

func InitGroupInfo() {
	groups, _ := GetAllGroups()
	for _, group := range groups {
		GroupModels[group.Name] = fmt.Sprintf(",%s,", group.Models)
		GroupInfo[group.Name] = &Group{
			Id:        group.Id,
			Type:      group.Type,
			ActiveNum: group.ActiveNum,
		}
		tmp := make(map[string]float64)
		err := json.Unmarshal([]byte(group.Ratio), &tmp)
		if err == nil {
			billingratio.GroupModelsRatio[group.Name] = tmp
		}
	}
}

func GetAllGroups() ([]*Group, error) {
	var groups []*Group
	err := DB.Where("status = 1").Order("id desc").Find(&groups).Error
	return groups, err
}

func GetGroupModels(group string) ([]string, error) {
	var models string
	err := DB.Model(&Group{}).Where("name = ?", group).Select("models").Find(&models).Error
	return strings.Split(models, ","), err
}
