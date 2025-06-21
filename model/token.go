package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/billing"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/message"
	"gorm.io/gorm"
)

const (
	TokenStatusEnabled   = 1 // don't use 0, 0 is the default value!
	TokenStatusDisabled  = 2 // also don't use 0
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

type Token struct {
	Id                  int     `json:"id"`
	UserId              int     `json:"user_id"`
	Key                 string  `json:"key" gorm:"type:char(48);uniqueIndex"`
	Status              int     `json:"status" gorm:"default:1"`
	Name                string  `json:"name" gorm:"index" `
	CreatedTime         int64   `json:"created_time" gorm:"bigint"`
	AccessedTime        int64   `json:"accessed_time" gorm:"bigint"`
	ExpiredTime         int64   `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	HardLimitUsd        int64   `json:"hard_limit_usd" gorm:"default:0"`
	RemainQuota         int64   `json:"remain_quota" gorm:"bigint;default:0"`
	UnlimitedQuota      bool    `json:"unlimited_quota" gorm:"default:false"`
	UsedQuota           int64   `json:"used_quota" gorm:"bigint;default:0"` // used quota
	Models              *string `json:"models" gorm:"type:text"`            // allowed models
	Subnet              *string `json:"subnet" gorm:"default:''"`           // allowed subnet
	RpmLimit            int     `json:"rpm_limit" gorm:"default:0"`
	DpmLimit            int     `json:"dpm_limit" gorm:"default:0"`
	TpmLimit            int     `json:"tpm_limit" gorm:"default:0"`
	Email               string  `json:"email"`
	WebhookType         int     `json:"webhook_type" gorm:"default:1"`
	Webhook             string  `json:"webhook"`
	ExpiredAlert        int     `json:"expired_alert" gorm:"default:0"`
	ExhaustedAlert      int     `json:"exhausted_alert" gorm:"default:0"`
	CustomContact       string  `json:"custom_contact"`
	ModerationsEnable   bool    `json:"moderations_enable" gorm:"default:false"`
	ModerationsNum      int     `json:"moderations_num" gorm:"default:0"`
	LastModerationsTime int64   `json:"last_moderations_time" gorm:"bigint"`

	//标记为忽略数据库
	BatchNumber   int `json:"batch_number" gorm:"-"`
	RechargeQuota int `json:"recharge_quota"  gorm:"-"`
}

func GetAllUserTokens(userId int, startIdx int, num int, order string) ([]*Token, error) {
	var tokens []*Token
	var err error
	query := DB.Where("user_id = ?", userId)

	switch order {
	case "remain_quota":
		query = query.Order("unlimited_quota desc, remain_quota desc")
	case "used_quota":
		query = query.Order("used_quota desc")
	default:
		query = query.Order("id desc")
	}

	err = query.Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

func SearchUserTokens(userId int, keyword string) (tokens []*Token, err error) {
	err = DB.Where("user_id = ?", userId).Where("name LIKE ?", keyword+"%").Find(&tokens).Error
	return tokens, err
}

func ValidateUserToken(c *gin.Context, key string) (token *Token, err error) {
	if key == "" {
		return nil, errors.New("no key provided")
	}
	token, err = CacheGetTokenByKey(key, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid token")
		}
		logger.SysError("CacheGetTokenByKey failed: " + err.Error())
		return nil, errors.New("failed to check key.")
	}
	var keyText = helper.EncryptKey(token.Key)
	if token.Status == TokenStatusExhausted {
		return nil, helper.GetCustomReturnError(c, fmt.Sprintf("API Key: %s, You exceeded your current quota, please check your quota", keyText))
	} else if token.Status == TokenStatusExpired {
		return nil, helper.GetCustomReturnError(c, fmt.Sprintf("API Key: %s, Your key was expired, please check your key status", keyText))
	}
	if token.Status != TokenStatusEnabled {
		return nil, helper.GetCustomReturnError(c, fmt.Sprintf("API Key: %s, You cann't access our system, please check your key status", keyText))
	}
	if token.ExpiredTime != -1 && token.ExpiredTime < helper.GetTimestamp() {
		if !common.RedisEnabled {
			token.Status = TokenStatusExpired
			err := token.SelectUpdate()
			if err != nil {
				logger.SysError("failed to update token status" + err.Error())
			}
		}
		return nil, helper.GetCustomReturnError(c, fmt.Sprintf("API Key: %s, Your key was expired, please check your key status", keyText))
	}
	if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		if !common.RedisEnabled {
			// in this case, we can make sure the token is exhausted
			token.Status = TokenStatusExhausted
			err := token.SelectUpdate()
			if err != nil {
				logger.SysError("failed to update token status" + err.Error())
			}
		}
		return nil, helper.GetCustomReturnError(c, fmt.Sprintf("API Key: %s, You exceeded your current quota, please check your quota", keyText))
	}
	return token, nil
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id or userId is empty")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id is empty")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	return &token, err
}

func (t *Token) Insert() error {
	var err error
	err = DB.Create(t).Error
	return err
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (t *Token) Update() error {
	err := DB.Model(t).Select("name", "status", "expired_time", "remain_quota", "hard_limit_usd", "unlimited_quota", "rpm_limit", "dpm_limit", "tpm_limit",
		"custom_contact", "email", "moderations_enable", "expired_alert", "exhausted_alert", "models", "subnet").Updates(t).Error
	if common.RedisEnabled {
		common.RedisDel(fmt.Sprintf("Auth_Error:sk-%s", t.Key))
		common.RedisDel(fmt.Sprintf("token:%s", t.Key))
	}
	return err
}

func (t *Token) SelectUpdate() error {
	// This can update zero values
	return DB.Model(t).Select("accessed_time", "status").Updates(t).Error
}

func (token *Token) UpdateAlertTime() error {
	// This can update zero values
	return DB.Model(token).Select("expired_alert", "exhausted_alert").Updates(token).Error
}

func (token *Token) UpdateModeration() error {
	// This can update zero values
	return DB.Model(token).Select("moderations_num", "last_moderations_time", "status").Updates(token).Error
}

func (t *Token) Delete() error {
	var err error
	err = DB.Delete(t).Error
	return err
}

func (t *Token) GetModels() string {
	if t == nil {
		return ""
	}
	if t.Models == nil {
		return ""
	}
	return *t.Models
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id or userId is empty")
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(id int, quota int64) (err error) {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, quota)
		return nil
	}
	return increaseTokenQuota(id, quota)
}

func increaseTokenQuota(id int, quota int64) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": helper.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, quota int64) (err error) {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		return nil
	}
	return decreaseTokenQuota(id, quota)
}

func decreaseTokenQuota(id int, quota int64) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": helper.GetTimestamp(),
		},
	).Error
	return err
}

func PreConsumeTokenQuota(tokenId int, quota int64) (err error) {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return errors.New("insufficient token quota")
	}
	userQuota, err := GetUserQuota(token.UserId)
	if err != nil {
		return err
	}
	if userQuota < quota {
		return errors.New("insufficient user quota")
	}
	quotaTooLow := userQuota >= config.QuotaRemindThreshold && userQuota-quota < config.QuotaRemindThreshold
	noMoreQuota := userQuota-quota <= 0
	if quotaTooLow || noMoreQuota {
		go func() {
			email, err := GetUserEmail(token.UserId)
			if err != nil {
				logger.SysError("failed to fetch user email: " + err.Error())
			}
			prompt := "Your quota is about to run out"
			if noMoreQuota {
				prompt = "Your quota has been used up"
			}
			if email != "" {
				topUpLink := fmt.Sprintf("%s/topup", config.ServerAddress)
				err = message.SendEmail(prompt, email,
					fmt.Sprintf("%s, the current remaining quota is %d, in order not to affect your use, please recharge in time. <br/> Recharge link: <a href='%s'>%s</a>", prompt, userQuota, topUpLink, topUpLink))
				if err != nil {
					logger.SysError("failed to send email" + err.Error())
				}
			}
		}()
	}
	if !token.UnlimitedQuota {
		err = DecreaseTokenQuota(tokenId, quota)
		if err != nil {
			return err
		}
	}
	err = DecreaseUserQuota(token.UserId, quota)
	return err
}

func PostConsumeTokenQuota(tokenId int, quota int64) (err error) {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	if quota > 0 {
		err = DecreaseUserQuota(token.UserId, quota)
	} else {
		err = IncreaseUserQuota(token.UserId, -quota)
	}
	if !token.UnlimitedQuota {
		if quota > 0 {
			err = DecreaseTokenQuota(tokenId, quota)
		} else {
			err = IncreaseTokenQuota(tokenId, -quota)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func UpdateAllTokensStatus(frequency int) error {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		logger.SysLog("begining update tokens")
		BatchUpdateTokens()
		logger.SysLog("update tokens done")
	}
}

func BatchInsertToken(tokens *[]Token) error {
	return DB.Create(&tokens).Error
}

func BatchUpdateTokens() error {
	var tokens []*Token
	err := DB.Where("`status` = ? and ( `remain_quota` < 0 or ( `expired_time` != -1 and `expired_time` < ? ))", TokenStatusEnabled, helper.GetTimestamp()).Find(&tokens).Error
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if token.ExpiredTime != -1 && token.ExpiredTime < helper.GetTimestamp() {
			t := time.Unix(token.ExpiredTime, 0)
			// 格式化为具体日期
			fdate := t.Format("2006-01-02")
			subject := "GuoGuo API - Key has expired"
			content := fmt.Sprintf("Hi there,<br/><br/>Your Key : %s has expired at %s<br/><br/>We are start rejecting your API requests now. If you still need the API access, please contact us or buy a new Key. Each Key has one free renewal opportunity. The renewal period is determined based on the remaining balance of the key, up to 15 days. If the validity period expires again and the balance is not exhausted, you will need to buy again.<br/><br/><a href='https://t.me/aiguoguo199' target='_blank'>Contact us</a><br/><br/>Best,<br/>AI GuoGuo",
				helper.EncryptKey(token.Key), fdate)

			message.SendMailASync(token.Email, subject, content)

			token.ExpiredAlert = 2

			token.Status = TokenStatusExpired
			err := token.SelectUpdate()
			if err != nil {
				logger.SysError("failed to update token status -> expired" + err.Error())
			}
			if common.RedisEnabled {
				common.RedisDel(fmt.Sprintf("token:%s", token.Key))
			}
			logger.SysLogf("Token【(%d)%s】Expired, time at: %d", token.Id, token.Name, token.ExpiredTime)

		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			//余额耗尽触发提醒
			subject := "GuoGuo API - Hard Limit Notice"
			content := fmt.Sprintf("Hi there,<br/><br/>You've reached your API usage hard limit of $%.2f for this Key: %s.<br/><br/>Your API requests will be rejected until you increase your hard limit.<br/><br/>To have your quota increased, please contact us. <br/><br/><a href='https://t.me/aiguoguo199' target='_blank'>Contact us</a><br/><br/>Best,<br/>AI GuoGuo",
				float64(token.HardLimitUsd)/500000, helper.EncryptKey(token.Key))

			message.SendMailASync(token.Email, subject, content)

			token.ExhaustedAlert = 2

			token.Status = TokenStatusExhausted
			err := token.SelectUpdate()
			if err != nil {
				logger.SysError("failed to update token status -> exhausted" + err.Error())
			}
			if common.RedisEnabled {
				common.RedisDel(fmt.Sprintf("token:%s", token.Key))
			}
			logger.SysLogf("Token【(%d)%s】Exhausted, Balance: %d", token.Id, token.Name, token.RemainQuota)
		}
	}
	return nil
}

func GetTokenLog(token_id int) (*[]billing.OpenAIUsageDetailCost, error) {
	var result *[]billing.OpenAIUsageDetailCost
	endTimestamp := time.Now()
	startTimestamp := endTimestamp.AddDate(0, 0, -7)
	err := DB.Table("logs").Where("token_id = ? and  created_at >= ? and created_at < ?", token_id, startTimestamp.Unix(), endTimestamp.Unix()).
		Select("model_name as model, quota / 500000 as quota, FROM_UNIXTIME(created_at,'%Y-%m-%d %H:%i:%s') as request_at").
		Order("id desc").
		Scan(&result).Error
	if err != nil {
		logger.SysError(fmt.Sprintf("BatchInsertTokenStatisticLog Error: %s", err.Error()))
		return nil, err
	}
	return result, nil
}

func SyncTokenAlert(sleepTime int) {
	for {
		logger.SysLog("Start Token Alert..")
		var tokens []*Token
		err := DB.Where("email !='' and status = ? and ((remain_quota / hard_limit_usd <= 0.2 and exhausted_alert =0) or (`expired_time` != -1 and `expired_time` <= ? and expired_alert = 0))",
			TokenStatusEnabled, helper.GetTimestamp()+(5*86400)).Find(&tokens).Error
		if err == nil {
			if len(tokens) > 0 {
				subject := ""
				content := ""
				needSend := false
				logger.SysLogf("Find Tokens :%d", len(tokens))
				for _, token := range tokens {
					if float64(token.RemainQuota)/float64(token.HardLimitUsd) <= 0.2 && token.ExhaustedAlert == 0 {
						//余额不足触发提醒
						subject = "GuoGuo API - Soft Limit Notice"
						content = fmt.Sprintf("Hi there,<br/><br/>You've reached your API usage soft limit of $%.2f for this Key: %s, which has triggered this friendly notification email.<br/><br/>Don't worry, you still have API access! Your current hard limit is set to $%.2f. If you reach this amount we'll start rejecting your API requests. <br/><br/><a href='https://t.me/aiguoguo199' target='_blank'>Contact us</a><br/><br/>Best,<br/>AI GuoGuo",
							float64(token.RemainQuota)/500000, helper.EncryptKey(token.Key), float64(token.HardLimitUsd)/500000)
						token.ExhaustedAlert = 1
						needSend = true
					} else if token.ExpiredTime != -1 && token.ExpiredTime <= helper.GetTimestamp()+(5*86400) && token.ExpiredAlert == 0 {
						t := time.Unix(token.ExpiredTime, 0)
						// 格式化为具体日期
						fdate := t.Format("2006-01-02")
						subject = "GuoGuo API - Key Expiring Soon Notice"
						content = fmt.Sprintf("Hi there,<br/><br/>Your API Key: %s, will expire on %s, please use it as soon as possible. After this date, your Key will not be usable. <br/><br/>Don't worry, you still have API access! If you don't have time to use it, please contact us. Each Key has one free renewal opportunity. The renewal period is determined based on the remaining balance of the key, up to 15 days. If the validity period expires again and the balance is not exhausted, you will need to buy again. <br/><br/><a href='https://t.me/aiguoguo199' target='_blank'>Contact us</a><br/><br/>Best,<br/>AI GuoGuo",
							helper.EncryptKey(token.Key), fdate)
						token.ExpiredAlert = 1
						needSend = true
					}
					if needSend {
						message.SendMailASync(token.Email, subject, content)
						token.UpdateAlertTime()
						logger.SysLogf("[%s][%s] 触发告警 : %s", token.Name, token.Email, content)
						needSend = false
					}
				}
			} else {
				logger.SysLog("Non Token Need Alert")
			}
		} else {
			logger.SysLogf("Fetch fail :%s", err)
		}
		logger.SysLog("End Token Alert..")
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
}
func TokenAlert(id int, alertType string) error {
	var token *Token
	err := DB.Where("id =?", id).Find(&token).Error
	if err == nil {
		subject := ""
		content := ""
		if alertType == "yue" {
			//余额不足触发提醒
			subject = "GuoGuo API - Soft Limit Notice"
			content = fmt.Sprintf("Hi there,<br/><br/>You've reached your API usage soft limit of $%.2f for this Key: %s, which has triggered this friendly notification email.<br/><br/>Don't worry, you still have API access! Your current hard limit is set to $%.2f. If you reach this amount we'll start rejecting your API requests. <br/><br/><a href='https://t.me/aiguoguo199' target='_blank'>Contact us</a><br/><br/>Best,<br/>AI GuoGuo",
				float64(token.RemainQuota)/500000, helper.EncryptKey(token.Key), float64(token.HardLimitUsd)/500000)
		} else if alertType == "expired" {
			t := time.Unix(token.ExpiredTime, 0)
			// 格式化为具体日期
			fdate := t.Format("2006-01-02")
			subject = "GuoGuo API - Key Expiring Soon Notice"
			content = fmt.Sprintf("Hi there,<br/><br/>Your API Key: %s, will expire on %s, please use it as soon as possible. After this date, your Key will not be usable. <br/><br/>Don't worry, you still have API access! If you don't have time to use it, please contact us. Each Key has one free renewal opportunity. The renewal period is determined based on the remaining balance of the key, up to 15 days. If the validity period expires again and the balance is not exhausted, you will need to buy again. <br/><br/><a href='https://t.me/aiguoguo199' target='_blank'>Contact us</a><br/><br/>Best,<br/>AI GuoGuo",
				helper.EncryptKey(token.Key), fdate)
		}
		if subject != "" {
			message.SendMailASync(token.Email, subject, content)
			logger.SysLog(fmt.Sprintf("[%s][%s] 手动触发告警 : %s", token.Name, token.Email, content))
			content = ""
		}
		return nil
	}
	return err
}
