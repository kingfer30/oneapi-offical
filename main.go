package main

import (
	"embed"
	"fmt"
	"os"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	_ "github.com/joho/godotenv/autoload"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/middleware"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/monitor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/router"

	"net/http"
	_ "net/http/pprof"
)

//go:embed web/build/*
var buildFS embed.FS

func main() {
	common.Init()
	logger.SetupLogger()
	logger.SysLogf("One API %s started", common.Version)

	if os.Getenv("GIN_MODE") != gin.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}
	if config.DebugEnabled {
		logger.SysLog("running in debug mode")
	}

	// Initialize SQL Database
	model.InitDB()
	model.InitLogDB()

	var err error
	err = model.CreateRootAccountIfNeed()
	if err != nil {
		logger.FatalLog("database init error: " + err.Error())
	}
	defer func() {
		err := model.CloseDB()
		if err != nil {
			logger.FatalLog("failed to close database: " + err.Error())
		}
	}()

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		logger.FatalLog("failed to initialize Redis: " + err.Error())
	}

	// Initialize options
	model.InitOptionMap()
	model.InitGroupInfo()
	// model.InitModelsInfo()
	logger.SysLog(fmt.Sprintf("using theme %s", config.Theme))
	if common.RedisEnabled {
		config.MemoryCacheEnabled = true
		config.RootUserEmail = model.GetRootUserEmail()
	}
	if config.MemoryCacheEnabled {
		logger.SysLog("memory cache enabled")
		model.InitChannelCache()
	}
	if os.Getenv("SYNC_CHANNEL_FREQUENCY") != "" {
		//渠道缓存
		frequency, err := strconv.Atoi(os.Getenv("SYNC_CHANNEL_FREQUENCY"))
		logger.SysLog(fmt.Sprintf("sync frequency: %d seconds", frequency))
		if err != nil {
			logger.FatalLog("failed to parse SYNC_CHANNEL_FREQUENCY: " + err.Error())
		}
		go model.SyncChannelCache(frequency)
	}
	//渠道唤醒
	if os.Getenv("SYNC_CHANNEL_WAKEUP") != "" {
		frequency, err := strconv.Atoi(os.Getenv("SYNC_CHANNEL_WAKEUP"))
		if err != nil {
			logger.FatalLog("failed to parse SYNC_CHANNEL_WAKEUP: " + err.Error())
		}
		go monitor.WakeupChannel(frequency)
	}
	if os.Getenv("SYNC_CHANNEL_SOFTLIMIT") != "" {
		//超软限制关闭
		frequency, err := strconv.Atoi(os.Getenv("SYNC_CHANNEL_SOFTLIMIT"))
		if err != nil {
			logger.FatalLog("failed to parse SYNC_CHANNEL_SOFTLIMIT: " + err.Error())
		}
		go model.SyncCloseSoftLimitChannel(frequency)
	}
	if os.Getenv("SYNC_TOKEN_ALERT") != "" {
		//token余额预警
		frequency, err := strconv.Atoi(os.Getenv("SYNC_TOKEN_ALERT"))
		if err != nil {
			logger.FatalLog("failed to parse SYNC_TOKEN_ALERT: " + err.Error())
		}
		go model.SyncTokenAlert(frequency)
	}
	if os.Getenv("SYNC_OPTIONS_FREQUENCY") != "" {
		//配置缓存
		frequency, err := strconv.Atoi(os.Getenv("SYNC_OPTIONS_FREQUENCY"))
		if err != nil {
			logger.FatalLog("failed to parse SYNC_OPTIONS_FREQUENCY: " + err.Error())
		}
		go model.SyncOptions(frequency)
	}
	if os.Getenv("TOKEN_UPDATE_FREQUENCY") != "" {
		//更新token
		frequency, err := strconv.Atoi(os.Getenv("TOKEN_UPDATE_FREQUENCY"))
		if err != nil {
			logger.FatalLog("failed to parse TOKEN_UPDATE_FREQUENCY: " + err.Error())
		}
		go model.UpdateAllTokensStatus(frequency)
	}
	if os.Getenv("BATCH_UPDATE_ENABLED") == "true" {
		config.BatchUpdateEnabled = true
		logger.SysLog("batch update enabled with interval " + strconv.Itoa(config.BatchUpdateInterval) + "s")
		model.InitBatchUpdater()
	}
	if config.EnableMetric {
		logger.SysLog("metric enabled, will disable channel if too much request failed")
	}
	if os.Getenv("AUTO_ACTIVATE_CHANNEL") == "true" {
		go monitor.AutoActivate(10)
	}
	go monitor.AutoDelFile(config.SyncFrequency)
	openai.InitTokenEncoders()
	client.Init()

	if os.Getenv("PPROF_DEBUG") == "true" {
		pprofAddr := "0.0.0.0:6060"
		go func(addr string) {
			if err := http.ListenAndServe(addr, nil); err != http.ErrServerClosed {
				logger.FatalLog("Pprof server ListenAndServe: " + err.Error())
			}
		}(pprofAddr)
		logger.SysLog(fmt.Sprintf("HTTP Pprof start at : %s", pprofAddr))
	}
	// Initialize HTTP server
	server := gin.New()
	server.Use(gin.Recovery())
	// This will cause SSE not to work!!!
	//server.Use(gzip.Gzip(gzip.DefaultCompression))
	server.Use(middleware.RequestId())
	middleware.SetUpLogger(server)
	// Initialize session store
	store := cookie.NewStore([]byte(config.SessionSecret))
	server.Use(sessions.Sessions("session", store))

	router.SetRouter(server, buildFS)
	var port = os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	logger.SysLogf("server started on http://localhost:%s", port)
	err = server.Run(":" + port)
	if err != nil {
		logger.FatalLog("failed to start HTTP server: " + err.Error())
	}
}
