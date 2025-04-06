package client

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
)

var HTTPClient *http.Client
var ImpatientHTTPClient *http.Client
var Ipv4Client *http.Client
var UserContentRequestHTTPClient *http.Client

func Init() {
	if config.UserContentRequestProxy != "" {
		logger.SysLog(fmt.Sprintf("using %s as proxy to fetch user content", config.UserContentRequestProxy))
		proxyURL, err := url.Parse(config.UserContentRequestProxy)
		if err != nil {
			logger.FatalLog(fmt.Sprintf("USER_CONTENT_REQUEST_PROXY set but invalid: %s", config.UserContentRequestProxy))
		}
		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
		UserContentRequestHTTPClient = &http.Client{
			Transport: transport,
			Timeout:   time.Second * time.Duration(config.UserContentRequestTimeout),
		}
	} else {
		UserContentRequestHTTPClient = &http.Client{}
	}
	var transport http.RoundTripper
	if config.RelayProxy != "" {
		logger.SysLog(fmt.Sprintf("using %s as api relay proxy", config.RelayProxy))
		proxyURL, err := url.Parse(config.RelayProxy)
		if err != nil {
			logger.FatalLog(fmt.Sprintf("USER_CONTENT_REQUEST_PROXY set but invalid: %s", config.UserContentRequestProxy))
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
	}

	if config.RelayTimeout == 0 {
		HTTPClient = &http.Client{
			Transport: transport,
		}
	} else {
		HTTPClient = &http.Client{
			Timeout:   time.Duration(config.RelayTimeout) * time.Second,
			Transport: transport,
		}
	}
	if config.RelayIPv4Proxy == "" {
		Ipv4Client = &http.Client{
			Transport: transport,
		}
	} else {
		proxyUrl, err := url.Parse(config.RelayIPv4Proxy)
		if err != nil {
			logger.FatalLog(fmt.Sprintf("RELAY_IPV4_PROXY set but invalid: %s", config.RelayIPv4Proxy))
		}
		Ipv4Client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyUrl),
			},
		}
	}

	ImpatientHTTPClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
}

func GetMediaClient(urlStr string) *http.Client {
	u, err := url.Parse(urlStr)
	if err != nil {
		logger.SysLogf("getImageClient - 解析URL(%s)出错: %s", urlStr, err)
		return nil
	}
	// 获取主机名（自动处理端口和IPv6）
	host := u.Hostname()
	// 判断是否为IP地址
	if ip := net.ParseIP(host); ip != nil {
		return Ipv4Client
	} else {
		return UserContentRequestHTTPClient
	}
}
