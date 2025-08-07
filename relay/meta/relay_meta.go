package meta

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/channeltype"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Meta struct {
	Mode         int
	ChannelType  int
	ChannelId    int
	TokenId      int
	TokenName    string
	UserId       int
	Group        string
	ModelMapping map[string]string
	// BaseURL is the proxy url set in the channel config
	BaseURL  string
	APIKey   string
	APIType  int
	Config   relaymodel.ChannelConfig
	IsStream bool
	// OriginModelName is the model name from the raw user request
	OriginModelName string
	// ActualModelName is the model name after mapping
	ActualModelName   string
	RequestURLPath    string
	PromptTokens      int // only for DoResponse
	SystemPrompt      string
	IncludeThinking   bool
	StartThinking     bool
	EndThinking       bool
	EnableBlockTag    bool
	ThinkingTagStart  string
	ThinkingTagEnd    string
	UseThinking       bool
	SelfImplement     bool
	IsImageModel      bool
	Image2Chat        bool
	TextRequest       *relaymodel.GeneralOpenAIRequest
	TxtRequestCount   int
	CalcPrompt        bool
	isFirstResponse   bool
	StartTime         time.Time
	FirstResponseTime time.Time
	Provider    string
}

func GetByContext(c *gin.Context) *Meta {
	meta := Meta{
		Mode:              relaymode.GetByPath(c.Request.URL.Path),
		ChannelType:       c.GetInt(ctxkey.Channel),
		ChannelId:         c.GetInt(ctxkey.ChannelId),
		TokenId:           c.GetInt(ctxkey.TokenId),
		TokenName:         c.GetString(ctxkey.TokenName),
		UserId:            c.GetInt(ctxkey.Id),
		Group:             c.GetString(ctxkey.Group),
		ModelMapping:      c.GetStringMapString(ctxkey.ModelMapping),
		OriginModelName:   c.GetString(ctxkey.RequestModel),
		BaseURL:           c.GetString(ctxkey.BaseURL),
		APIKey:            strings.TrimPrefix(c.Request.Header.Get("Authorization"), "Bearer "),
		RequestURLPath:    c.Request.URL.String(),
		SystemPrompt:      c.GetString(ctxkey.SystemPrompt),
		SelfImplement:     false,
		CalcPrompt:        c.GetBool(ctxkey.CalcPrompt),
		StartTime:         c.GetTime(ctxkey.RequestStartTime),
		FirstResponseTime: c.GetTime(ctxkey.RequestStartTime).Add(-time.Second),
	}
	cfg, ok := c.Get(ctxkey.Config)
	if ok {
		meta.Config = cfg.(relaymodel.ChannelConfig)
	}
	if meta.BaseURL == "" {
		meta.BaseURL = channeltype.ChannelBaseURLs[meta.ChannelType]
	}
	meta.APIType = channeltype.ToAPIType(meta.ChannelType)
	return &meta
}

func (meta *Meta) SetFirstResponseTime() {
	if meta.isFirstResponse {
		meta.FirstResponseTime = time.Now()
		meta.isFirstResponse = false
	}
}
