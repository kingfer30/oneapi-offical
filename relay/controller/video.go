package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func getVideoRequest(c *gin.Context, _ int) (*relaymodel.VideoRequest, error) {
	videoRequest := &relaymodel.VideoRequest{}
	if strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json") {
		err := common.UnmarshalBodyReusable(c, videoRequest)
		if err != nil {
			return nil, err
		}
	} else {
		//非json格式转为带form的结构体
		var videoFormRequest relaymodel.VideoFormRequest
		err := common.UnmarshalBodyReusable(c, &videoFormRequest)
		if err != nil {
			return nil, err
		}
		logger.SysLogf("转换成功, %s, %s, %d, %s", videoFormRequest.Model, videoFormRequest.Prompt, videoFormRequest.Size)
		videoFormRequest.Model = videoFormRequest.Model
		videoFormRequest.Prompt = videoFormRequest.Prompt
		videoFormRequest.Size = videoFormRequest.Size
		//将上传图片转为b64
		file, err := videoFormRequest.Image.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()
		var encodedBuilder strings.Builder
		encoder := base64.NewEncoder(base64.StdEncoding, &encodedBuilder)
		defer encoder.Close()

		// 设置内存安全限制（示例设为20MB）
		const maxSize = 20 << 20 // 20MB
		limitedReader := io.LimitReader(file, maxSize)

		bytesCopied, err := io.Copy(encoder, limitedReader) // 流式处理
		if err != nil {
			return nil, fmt.Errorf("video reading error: %v", err)
		}
		// 检查是否超过大小限制
		if bytesCopied >= maxSize {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			logger.SysLogf("Memory usage: HeapInuse=%v MiB", m.HeapInuse/1024/1024)
			logger.SysLogf("videos is too large: %s,", videoFormRequest.Image.Filename)
			return nil, fmt.Errorf("video exceeds maximum allowed size")
		}

		// 确保所有数据刷新到builder
		if err := encoder.Close(); err != nil {
			return nil, fmt.Errorf("base64 close error: %w", err)
		}
		videoRequest.Image = encodedBuilder.String()
	}
	if videoRequest.Size == "" {
		videoRequest.Size = "1024x1024"
	}
	if videoRequest.Model == "" {
		return nil, fmt.Errorf("empty Model: %s", videoRequest.Model)
	}
	return videoRequest, nil
}

func validateVideoRequest(videoRequest *relaymodel.VideoRequest, meta *meta.Meta) *relaymodel.ErrorWithStatusCode {
	// check prompt length
	if videoRequest.Prompt == "" {
		return openai.ErrorWrapper(errors.New("prompt is required"), "prompt_missing", http.StatusBadRequest)
	}

	if meta.Mode == relaymode.ImagesEdit {
		if videoRequest.Image == "" {
			return openai.ErrorWrapper(errors.New("Image is required"), "video_missing", http.StatusBadRequest)
		}
	} else {
		//图片创建的清楚图片编辑
		if videoRequest.Image != "" {
			videoRequest.Image = ""
		}
	}
	return nil
}

func getVideoCostRatio(videoRequest *relaymodel.VideoRequest) (float64, error) {
	if videoRequest == nil {
		return 0, errors.New("videoRequest is nil")
	}
	videoCostRatio := getImageSizeRatio(videoRequest.Model, videoRequest.Size)
	return videoCostRatio, nil
}

func RelayVideoHelper(c *gin.Context, relayMode int) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	meta := meta.GetByContext(c)
	videoRequest, err := getVideoRequest(c, meta.Mode)
	if err != nil {
		logger.Errorf(ctx, "getImageRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "invalid_video_request", http.StatusBadRequest)
	}

	// map model name
	var isModelMapped bool
	meta.OriginModelName = videoRequest.Model
	videoRequest.Model, isModelMapped = getMappedModelName(videoRequest.Model, meta.ModelMapping)
	meta.ActualModelName = videoRequest.Model

	// model validation
	bizErr := validateVideoRequest(videoRequest, meta)
	if bizErr != nil {
		return bizErr
	}

	videoCostRatio, err := getVideoCostRatio(videoRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "get_video_cost_ratio_failed", http.StatusInternalServerError)
	}

	videoModel := videoRequest.Model
	// Convert the original video model
	videoRequest.Model, _ = getMappedModelName(videoRequest.Model, billingratio.ImageOriginModelName)

	var requestBody io.Reader
	var jsonStr []byte
	if isModelMapped || meta.ChannelType == channeltype.Azure { // make Azure channel request body
		jsonStr, err = json.Marshal(videoRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_video_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	} else {
		requestBody = c.Request.Body
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(fmt.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	// these adaptors need to convert the request
	switch meta.ChannelType {
	case channeltype.Zhipu,
		channeltype.Ali,
		channeltype.Replicate,
		channeltype.Gemini,
		channeltype.Baidu:
		finalRequest, err := adaptor.ConvertVideoRequest(videoRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "convert_video_request_failed", http.StatusInternalServerError)
		}
		jsonStr, err = json.Marshal(finalRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_video_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	}
	if len(jsonStr) == 0 {
		jsonStr, err = json.Marshal(requestBody)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_video_request_failed", http.StatusInternalServerError)
		}
	}

	logger.Debugf(c.Request.Context(), "converted request: \n%s", string(jsonStr))

	modelRatio := billingratio.GetModelRatio(videoModel, meta.ChannelType, meta.Group)
	groupRatio := billingratio.GetGroupRatio(meta.Group)
	completionRatio := billingratio.GetCompletionRatio(videoModel, meta.ChannelType)
	ratio := modelRatio * groupRatio
	userQuota, _ := model.CacheGetUserQuota(ctx, meta.UserId)

	quota := int64(ratio * videoCostRatio * 1000)

	if userQuota-quota < 0 {
		return openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}

	// do request
	resp, err := adaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		logger.Errorf(ctx, "DoRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	// do response
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		logger.Errorf(ctx, "respErr is not nil: %+v", respErr)
		return respErr
	}

	defer func(ctx context.Context) {
		if resp != nil &&
			resp.StatusCode != http.StatusCreated && // replicate returns 201
			resp.StatusCode != http.StatusOK {
			return
		}

		//如果返回的token比计算的大, 则使用它的
		prompt := 0
		completion := 0
		if usage != nil && usage.TotalTokens > int(quota) {
			prompt = usage.PromptTokens
			completion = usage.CompletionTokens
			quota = int64(math.Ceil((float64(prompt) + float64(completion)*completionRatio) * ratio))
			log.Printf("quota:%d", quota)
			if usage.TotalTokens > int(quota) {
				quota = int64(usage.TotalTokens)
			}
		}
		err := model.PostConsumeTokenQuota(meta.TokenId, quota)
		if err != nil {
			logger.SysError("error consuming token remain quota: " + err.Error())
		}
		err = model.CacheUpdateUserQuota(ctx, meta.UserId)
		if err != nil {
			logger.SysError("error update user quota cache: " + err.Error())
		}
		if quota != 0 {
			tokenName := c.GetString(ctxkey.TokenName)
			logContent := fmt.Sprintf("模型倍率 %.2f，分组倍率 %.2f", modelRatio, groupRatio)
			model.RecordConsumeLog(ctx, meta.UserId, meta.ChannelId, prompt, completion, videoRequest.Model, tokenName, quota, logContent)
			model.UpdateUserUsedQuotaAndRequestCount(meta.UserId, quota)
			channelId := c.GetInt(ctxkey.ChannelId)
			model.UpdateChannelUsedQuota(channelId, quota)
		}
	}(c.Request.Context())

	return nil
}
