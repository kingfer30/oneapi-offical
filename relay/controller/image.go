package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"strings"
	"time"

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

func getImageRequest(c *gin.Context, _ int) (*relaymodel.ImageRequest, error) {
	imageRequest := &relaymodel.ImageRequest{}
	if strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json") {
		err := common.UnmarshalBodyReusable(c, imageRequest)
		if err != nil {
			return nil, err
		}
	} else {
		//非json格式转为带form的结构体
		var imageFormRequest relaymodel.ImageFormRequest
		err := common.UnmarshalBodyReusable(c, &imageFormRequest)
		if err != nil {
			return nil, err
		}
		imageRequest.Model = imageFormRequest.Model
		imageRequest.N = imageFormRequest.N
		imageRequest.Prompt = imageFormRequest.Prompt
		imageRequest.Quality = imageFormRequest.Quality
		imageRequest.Size = imageFormRequest.Size
		if len(imageFormRequest.Image) > 0 {
			for _, img := range imageFormRequest.Image {
				//将上传图片转为b64
				file, err := img.Open()
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
					return nil, fmt.Errorf("image reading error: %v", err)
				}
				// 检查是否超过大小限制
				if bytesCopied >= maxSize {
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					logger.SysLogf("Memory usage: HeapInuse=%v MiB", m.HeapInuse/1024/1024)
					logger.SysLogf("images is too large: %s,", img.Filename)
					return nil, fmt.Errorf("image exceeds maximum allowed size")
				}

				// 确保所有数据刷新到builder
				if err := encoder.Close(); err != nil {
					return nil, fmt.Errorf("base64 close error: %w", err)
				}
				imageRequest.Image = append(imageRequest.Image, encodedBuilder.String())
			}
		}

	}
	if imageRequest.N == 0 {
		imageRequest.N = 1
	}
	if imageRequest.Size == "" {
		imageRequest.Size = "1024x1024"
	}
	if imageRequest.Model == "" {
		imageRequest.Model = "dall-e-2"
	}
	return imageRequest, nil
}

func isValidImageSize(model string, size string) bool {
	if model == "cogview-3" || billingratio.ImageSizeRatios[model] == nil {
		return true
	}
	_, ok := billingratio.ImageSizeRatios[model][size]
	return ok
}

func isValidImagePromptLength(model string, promptLength int) bool {
	maxPromptLength, ok := billingratio.ImagePromptLengthLimitations[model]
	return !ok || promptLength <= maxPromptLength
}

func isWithinRange(element string, value int) bool {
	amounts, ok := billingratio.ImageGenerationAmounts[element]
	return !ok || (value >= amounts[0] && value <= amounts[1])
}

func getImageSizeRatio(model string, size string) float64 {
	if ratio, ok := billingratio.ImageSizeRatios[model][size]; ok {
		return ratio
	}
	return 1
}

func validateImageRequest(imageRequest *relaymodel.ImageRequest, meta *meta.Meta) *relaymodel.ErrorWithStatusCode {
	// check prompt length
	if imageRequest.Prompt == "" {
		return openai.ErrorWrapper(errors.New("prompt is required"), "prompt_missing", http.StatusBadRequest)
	}

	// model validation
	if !isValidImageSize(imageRequest.Model, imageRequest.Size) {
		return openai.ErrorWrapper(errors.New("size not supported for this image model"), "size_not_supported", http.StatusBadRequest)
	}

	if !isValidImagePromptLength(imageRequest.Model, len(imageRequest.Prompt)) {
		return openai.ErrorWrapper(errors.New("prompt is too long"), "prompt_too_long", http.StatusBadRequest)
	}

	// Number of generated images validation
	if !isWithinRange(imageRequest.Model, imageRequest.N) {
		return openai.ErrorWrapper(errors.New("invalid value of n"), "n_not_within_range", http.StatusBadRequest)
	}

	if meta.Mode == relaymode.ImagesEdit {
		if len(imageRequest.Image) == 0 {
			return openai.ErrorWrapper(errors.New("image is required"), "image_missing", http.StatusBadRequest)
		}
	} else {
		//图片创建的清除图片编辑
		if len(imageRequest.Image) > 0 {
			imageRequest.Image = make([]string, 0)
		}
	}
	return nil
}

func getImageCostRatio(imageRequest *relaymodel.ImageRequest) (float64, string, error) {
	if imageRequest == nil {
		return 0, "", errors.New("imageRequest is nil")
	}
	var imageMode = "standard"
	imageCostRatio := 1.0
	if imageRequest.Model == "gpt-image-1" {
		imageMode = "auto"
		if strings.ToLower(imageRequest.Quality) == "high" {
			imageMode = "high"
			//hd mode
			if imageRequest.Size == "1024x1024" {
				imageCostRatio = 15.1818
			} else if imageRequest.Size == "1024x1536" || imageRequest.Size == "1536x1024" {
				imageCostRatio = 22.7272
			} else {
				imageCostRatio = 1
			}
		} else if strings.ToLower(imageRequest.Quality) == "low" {
			imageMode = "low"
			if imageRequest.Size == "1024x1536" || imageRequest.Size == "1536x1024" {
				imageCostRatio = 1.4545
			} else {
				imageCostRatio = 1
			}
		} else if strings.ToLower(imageRequest.Quality) == "medium" {
			imageMode = "medium"
			//standard mode
			if imageRequest.Size == "1024x1024" {
				imageCostRatio = 3.8181
			} else if imageRequest.Size == "1024x1536" || imageRequest.Size == "1536x1024" {
				imageCostRatio = 5.7272
			} else {
				imageCostRatio = 1
			}
		}
	} else if imageRequest.Model == "dall-e-3" {
		if imageRequest.Quality == "hd" {
			if imageRequest.Size == "1024x1024" {
				imageCostRatio = 2
			} else if imageRequest.Size == "1024x1792" || imageRequest.Size == "1792x1024" {
				imageCostRatio = 3
			} else {
				imageCostRatio = 2
			}
		} else {
			if imageRequest.Size == "1024x1024" {
				imageCostRatio = 1
			} else if imageRequest.Size == "1024x1792" || imageRequest.Size == "1792x1024" {
				imageCostRatio = 2
			} else {
				imageCostRatio = 1
			}
		}
	} else {
		imageCostRatio *= getImageSizeRatio(imageRequest.Model, imageRequest.Size)
	}
	return imageCostRatio, imageMode, nil
}

func RelayImageHelper(c *gin.Context, relayMode int) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	meta := meta.GetByContext(c)
	imageRequest, err := getImageRequest(c, meta.Mode)
	if err != nil {
		logger.Errorf(ctx, "getImageRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
	}

	// map model name
	var isModelMapped bool
	meta.OriginModelName = imageRequest.Model
	imageRequest.Model, isModelMapped = getMappedModelName(imageRequest.Model, meta.ModelMapping)
	meta.ActualModelName = imageRequest.Model

	// model validation
	bizErr := validateImageRequest(imageRequest, meta)
	if bizErr != nil {
		return bizErr
	}

	imageCostRatio, imageMode, err := getImageCostRatio(imageRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "get_image_cost_ratio_failed", http.StatusInternalServerError)
	}

	imageModel := imageRequest.Model
	// Convert the original image model
	imageRequest.Model, _ = getMappedModelName(imageRequest.Model, billingratio.ImageOriginModelName)
	c.Set("response_format", imageRequest.ResponseFormat)

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(fmt.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	var requestBody io.Reader
	var jsonStr []byte
	if isModelMapped || meta.ChannelType == channeltype.Azure { // make Azure channel request body
		jsonStr, err = json.Marshal(imageRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	} else {
		bodyByte, _ := common.GetRequestBody(c)
		jsonStr = bodyByte
		requestBody = bytes.NewBuffer(bodyByte)
	}

	// these adaptors need to convert the request
	switch meta.ChannelType {
	case channeltype.Zhipu,
		channeltype.Ali,
		channeltype.Replicate,
		channeltype.Gemini,
		channeltype.Baidu:
		finalRequest, err := adaptor.ConvertImageRequest(imageRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "convert_image_request_failed", http.StatusInternalServerError)
		}
		jsonStr, err = json.Marshal(finalRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	}
	logger.Debugf(c.Request.Context(), "converted request: \n%s", string(jsonStr))

	modelRatio := billingratio.GetModelRatio(imageModel, meta.ChannelType, meta.Group)
	groupRatio := billingratio.GetGroupRatio(meta.Group)
	completionRatio := billingratio.GetCompletionRatio(imageModel, meta.ChannelType)
	ratio := modelRatio * groupRatio
	userQuota, _ := model.CacheGetUserQuota(ctx, meta.UserId)

	var quota int64
	switch meta.ChannelType {
	case channeltype.Replicate:
		// replicate always return 1 image
		quota = int64(ratio * imageCostRatio * 1000)
	default:
		quota = int64(ratio*imageCostRatio*1000) * int64(imageRequest.N)
	}

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

	prompt := 0
	completion := 0
	useTimeSeconds := time.Now().Unix() - meta.StartTime.Unix()
	defer func(ctx *gin.Context) {
		if resp != nil &&
			resp.StatusCode != http.StatusCreated && // replicate returns 201
			resp.StatusCode != http.StatusOK {
			return
		}
		//有返回usage的, 按照它的计算
		if usage != nil {
			prompt = usage.PromptTokens
			completion = usage.CompletionTokens
			if meta.ChannelType == channeltype.Gemini {
				//gemini按照正常chat计费
				quota = int64(math.Ceil((float64(prompt) + float64(completion)*completionRatio) * ratio))
			}
			if imageRequest.Model == "gpt-image-1" {
				prompt = usage.InputTokens
				completion = usage.OutputTokens
				quota = int64(prompt) + int64(float64(completion)*completionRatio)
				quota = int64(float64(quota) * modelRatio)
			}
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
			logContent := fmt.Sprintf("model rate %.2f, size radio %.2f, group rate %.2f (%s-%s)", modelRatio, imageCostRatio, groupRatio, imageRequest.Size, imageMode)
			model.RecordConsumeLog(c, meta.IsStream, meta.FirstResponseTime, int(useTimeSeconds), meta.UserId, meta.ChannelId, prompt, completion, imageRequest.Model, tokenName, quota, logContent, meta.TokenId)
			model.UpdateUserUsedQuotaAndRequestCount(meta.UserId, quota)
			channelId := c.GetInt(ctxkey.ChannelId)
			model.UpdateChannelUsedQuota(channelId, quota)
		}
	}(c)

	return nil
}
