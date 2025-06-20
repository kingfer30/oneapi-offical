package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func ConvertVideoRequest(c *gin.Context, request *relaymodel.VideoRequest) (*VideoRequest, error) {
	parameters := VideoParameters{
		PersonGeneration: "allow_adult",
	}
	ins := VideoInstances{
		Prompt: request.Prompt,
	}
	if request.Size != "" {
		parameters.AspectRatio = request.Size
	}
	if request.N > 0 {
		parameters.SampleCount = request.N
	}
	if request.Duration > 0 {
		parameters.DurationSeconds = request.Duration
	}
	if request.NegativePrompt != "" {
		parameters.NegativePrompt = request.NegativePrompt
	}
	if request.Image != "" {
		_, fileData, err := image.GetImageFromUrl(request.Image, false)
		if err != nil {
			return nil, err
		}
		ins.Image = VideoImage{
			BytesBase64Encoded: fileData,
		}
	}

	videoRequest := VideoRequest{
		Instances:  ins,
		Parameters: parameters,
	}

	return &videoRequest, nil
}
func VideoHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	var geminiResponse RunningResultResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if geminiResponse.Name == "" {
		return nil, nil
	}

	var urlList []string
	//视频模型需要循环来查询状态 , 每2秒查询一次 1分钟后超时
	for i := 0; i < 30; i++ {
		err, videoResult := getJobResult(c, meta, geminiResponse.Name)
		if err != nil {
			return err, nil
		}
		if videoResult.Response != nil {
			if len(videoResult.Response.GenerateVideoResponse.GeneratedSamples) > 0 {
				for _, v := range videoResult.Response.GenerateVideoResponse.GeneratedSamples {
					urlList = append(urlList, v.Video.Uri)
				}
				if len(urlList) > 0 {
					break
				}
			}
		}
		//每2秒查询一次
		time.Sleep(2 * time.Second)
	}
	if len(urlList) == 0 {
		return openai.ErrorWrapper(fmt.Errorf("Uri is empty"), "uri_is_empty", http.StatusInternalServerError),  nil
	}

	return nil,  nil
}

func getJobResult(c *gin.Context, meta *meta.Meta, path string) (*relaymodel.ErrorWithStatusCode, *VideoResultResponse) {
	defaultVersion := config.GeminiVersion
	version := helper.AssignOrDefault(meta.Config.APIVersion, defaultVersion)
	url := fmt.Sprintf("%s/%s/%s", meta.BaseURL, version, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return openai.ErrorWrapper(err, "new_request_failed", http.StatusInternalServerError), nil
	}
	req.Header.Set("x-goog-api-key", meta.APIKey)

	resp, err := DoRequest(c, req)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError), nil
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	var geminiResponse *VideoResultResponse
	err = json.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, geminiResponse
}
