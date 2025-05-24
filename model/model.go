package model

type Model struct {
	Id          int     `json:"id"`
	Type        string  `json:"type" gorm:"default:''"`
	Name        string  `json:"name" gorm:"index"`
	Ratio       float64 `json:"ratio"`
	Status      int     `json:"status" gorm:"default:1;index:idx_status"`
	CreatedTime int64   `json:"created_time" gorm:"bigint"`
}

var ModelsList = make(map[string]float64)

func InitModelsInfo() {
	models, _ := GetAllModels()
	for _, m := range models {
		ModelsList[m.Name] = m.Ratio
	}
}

func GetAllModels() ([]*Model, error) {
	var models []*Model
	err := DB.Where("status = 1").Order("id desc").Find(&models).Error
	return models, err
}
