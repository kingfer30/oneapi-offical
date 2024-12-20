package image

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/random"

	"github.com/songquanpeng/one-api/common/logger"
	_ "golang.org/x/image/bmp"  // 导入BMP编解码器
	_ "golang.org/x/image/tiff" // 导入TIFF编解码器
	_ "golang.org/x/image/webp" // 导入WebP编解码器
)

var CacheSecond int64 = 600

type ImageCache struct {
	IsURL       bool   `json:"is_url"`
	ContentType string `json:"content_type"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

func setImageCache(url string, isUrl bool, contentType string, width int, height int) {
	var cache *ImageCache
	result, err := common.RedisHashGet("image_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err != nil {
			cache = &ImageCache{
				IsURL: isUrl,
			}
		}
	} else {
		cache = &ImageCache{
			IsURL: isUrl,
		}
	}
	if contentType != "" && cache.ContentType != contentType {
		cache.ContentType = contentType
	}
	if width != 0 && cache.Width != width {
		cache.Width = width
	}
	if height != 0 && cache.Height != height {
		cache.Height = height
	}
	common.RedisHashSet("image_url", random.StrToMd5(url), cache, CacheSecond)
}
func IsImageUrl(url string) (bool, error) {
	var cache *ImageCache
	result, err := common.RedisHashGet("image_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		log.Printf("err: %v, %s, %s", err, random.StrToMd5(url), result)
		if err == nil {
			return cache.IsURL, nil
		}
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = client.HTTPClient.Get(url)
		if err != nil {
			logger.SysLog(fmt.Sprintf("HTTPClient报错: %s", err.Error()))
			setImageCache(url, false, "", 0, 0)
			return false, err
		}
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		logger.SysLog(fmt.Sprintf("Content-Type错误: %s", resp.Header.Get("Content-Type")))
		setImageCache(url, false, "", 0, 0)
		return false, nil
	}
	setImageCache(url, true, resp.Header.Get("Content-Type"), 0, 0)
	return true, nil
}

func GetImageSizeFromUrl(url string) (width int, height int, err error) {
	var cache *ImageCache
	result, err := common.RedisHashGet("image_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err == nil {
			return cache.Width, cache.Height, nil
		}
	}
	isImage, err := IsImageUrl(url)
	if !isImage {
		return
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	img, _, err := image.DecodeConfig(resp.Body)
	if err != nil {
		return
	}
	setImageCache(url, true, "", img.Width, img.Height)
	return img.Width, img.Height, nil
}

func getImageFormat(input string) (string, string, error) {
	if strings.HasPrefix(input, "http") || strings.HasPrefix(input, "https") {
		//url no check
		return "", "", nil
	}
	if strings.HasPrefix(input, "data:image/png;base64,") {
		input = strings.TrimPrefix(input, "data:image/png;base64,")
	} else if strings.HasPrefix(input, "data:image/jpeg;base64,") {
		input = strings.TrimPrefix(input, "data:image/jpeg;base64,")
	} else if strings.HasPrefix(input, "data:image/jpg;base64,") {
		input = strings.TrimPrefix(input, "data:image/jpg;base64,")
	} else if strings.HasPrefix(input, "data:image/webp;base64,") {
		input = strings.TrimPrefix(input, "data:image/webp;base64,")
	} else if strings.HasPrefix(input, "data:image/gif;base64,") {
		input = strings.TrimPrefix(input, "data:image/gif;base64,")
	}
	input = strings.TrimSpace(input)

	var imageData []byte
	var err error
	imageData, err = base64.StdEncoding.DecodeString(input)
	if err != nil {
		logger.SysLog(fmt.Sprintf("Vision-Base64方式-DecodeString报错: %s->%s", input, err.Error()))
		return "", "", err
	}

	// 如果图像数据小于512字节，使用实际长度的数据。
	dataToCheck := imageData
	if len(imageData) > 512 {
		dataToCheck = imageData[:512]
	}

	contentType := http.DetectContentType(dataToCheck)

	switch contentType {
	case "image/jpeg":
		return "jpeg", input, nil
	case "image/png":
		return "png", input, nil
	case "image/webp":
		return "webp", input, nil
	case "image/gif":
		return "gif", input, nil
	default:
		return "", "", fmt.Errorf("unsupported image format: %s", contentType)
	}
}

func GetImageFromUrl(url string) (mimeType string, data string, err error) {
	// Check if the URL is a base64
	logger.SysLog("Vision-Image-Format Checking...")
	imgType, imgData, err := getImageFormat(url)

	if err == nil && imgType != "" {
		// URL is a data URL
		logger.SysLog(fmt.Sprintf("Vision-Base64 Yes ! %s", imgType))
		mimeType = "image/" + imgType
		data = imgData
		return mimeType, data, nil
	}
	logger.SysLog(fmt.Sprintf("Vision-Url Yes ! %s", url))
	isImage, err := IsImageUrl(url)
	if !isImage {
		return "", "", fmt.Errorf("failed to get this url : it may not an image")
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		resp, err = client.HTTPClient.Head(url)
		if err != nil {
			return "", "", fmt.Errorf("failed to get this url : %s, status : %s, err: %s", url, resp.Status, err)
		}
	}
	defer resp.Body.Close()
	buffer := bytes.NewBuffer(nil)
	_, err = buffer.ReadFrom(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to readFrom this url : %s, status : %s, err: %s", url, resp.Status, err)
	}
	mimeType = resp.Header.Get("Content-Type")
	data = base64.StdEncoding.EncodeToString(buffer.Bytes())
	return mimeType, data, nil
}

var (
	reg = regexp.MustCompile(`data:image/([^;]+);base64,`)
)

var readerPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Reader{}
	},
}

func GetImageSizeFromBase64(encoded string) (width int, height int, err error) {
	decoded, err := base64.StdEncoding.DecodeString(reg.ReplaceAllString(encoded, ""))
	if err != nil {
		return 0, 0, err
	}

	reader := readerPool.Get().(*bytes.Reader)
	defer readerPool.Put(reader)
	reader.Reset(decoded)

	img, _, err := image.DecodeConfig(reader)
	if err != nil {
		return 0, 0, err
	}

	return img.Width, img.Height, nil
}

func GetImageSize(image string) (width int, height int, err error) {
	if strings.HasPrefix(image, "http") || strings.HasPrefix(image, "https") {
		return GetImageSizeFromUrl(image)
	}
	return GetImageSizeFromBase64(image)
}
