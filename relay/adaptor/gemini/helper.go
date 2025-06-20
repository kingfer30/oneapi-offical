package gemini

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/config"
)

func DoRequest(c *gin.Context, req *http.Request) (*http.Response, error) {
	var client *http.Client
	if config.HttpProxy == "" {
		var transport http.RoundTripper
		client = &http.Client{
			Timeout:   time.Duration(config.RelayGeminiTimeout) * time.Second,
			Transport: transport,
		}
	} else {
		url, err := url.Parse(config.HttpProxy)
		if err != nil {
			return nil, fmt.Errorf("url.Parse failed: %w", err)
		}
		client = &http.Client{
			Timeout: time.Duration(config.RelayGeminiTimeout) * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(url),
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("resp is nil")
	}
	_ = req.Body.Close()
	_ = c.Request.Body.Close()
	return resp, nil
}
