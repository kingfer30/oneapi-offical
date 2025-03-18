package image

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
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

var imageClient *http.Client

func init() {
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.Proxy = nil
	imageClient = &http.Client{
		Transport: customTransport,
	}
}

type ImageCache struct {
	IsURL       bool   `json:"is_url"`
	ContentType string `json:"content_type"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Path        string `json:"path"`
}

func setImageCache(url string, isUrl bool, contentType string, width int, height int, path string) {
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
	if path != "" && cache.Path != path {
		cache.Path = path
	}
	common.RedisHashSet("image_url", random.StrToMd5(url), cache, CacheSecond)
}
func IsImageUrl(url string) (bool, error) {
	var cache *ImageCache
	result, err := common.RedisHashGet("image_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err == nil {
			return cache.IsURL, nil
		}
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = imageClient.Get(url)
		if err != nil {
			logger.SysLog(fmt.Sprintf("HTTPClient报错: %s", err.Error()))
			return false, err
		}
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		logger.SysLog(fmt.Sprintf("Content-Type错误: %s", resp.Header.Get("Content-Type")))
		setImageCache(url, false, "", 0, 0, "")
		return false, nil
	}
	setImageCache(url, true, resp.Header.Get("Content-Type"), 0, 0, "")
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
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = imageClient.Get(url)
		if err != nil {
			logger.SysLog(fmt.Sprintf("HTTPClient报错: %s", err.Error()))
			return
		}
	}
	defer resp.Body.Close()
	img, _, err := image.DecodeConfig(resp.Body)
	if err != nil {
		return
	}
	setImageCache(url, true, "", img.Width, img.Height, "")
	return img.Width, img.Height, nil
}

func getImageFormat(input string, saveLocal bool) (string, string, error) {
	if strings.HasPrefix(input, "http") || strings.HasPrefix(input, "https") {
		//url no check
		return "", "", nil
	}
	source := input
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
	} else if strings.HasPrefix(input, "data:image/heic;base64,") {
		input = strings.TrimPrefix(input, "data:image/heic;base64,")
	} else if strings.HasPrefix(input, "data:image/heif;base64,") {
		input = strings.TrimPrefix(input, "data:image/;base64,")
	}
	input = strings.TrimSpace(input)

	var cache *ImageCache
	result, err := common.RedisHashGet("image_url", random.StrToMd5(input))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err == nil {
			if saveLocal {
				return cache.ContentType, cache.Path, nil
			} else {
				return cache.ContentType, input, nil
			}
		}
	}

	var imageData []byte
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

	t := ""
	switch contentType {
	case "image/jpeg":
		t = "jpeg"
	case "image/png":
		t = "png"
	case "image/webp":
		t = "webp"
	case "image/heic":
		t = "heic"
	case "image/heif":
		t = "image/heif"
	case "image/gif":
		t = "gif"
	}
	if t == "" {
		return "", "", fmt.Errorf("unsupported image format: %s", contentType)
	}
	if saveLocal {
		//保存到本地文件
		file, err := saveWithStream(input, t)
		if err != nil {
			return "", "", fmt.Errorf("fail to saveWithStream: %s", err)
		}
		setImageCache(source, true, contentType, 0, 0, file)
		return contentType, file, nil
	}
	return contentType, input, nil
}

// 流式保存函数（内存安全版）
func saveWithStream(base64Data string, ext string) (string, error) {
	// 1. 创建解码流（复用格式检测后的纯净Base64数据）
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(base64Data))

	// 2. 创建目标目录
	dirPath := "/mnt/tpm_file"
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			logger.SysLogf("saveWithStream - Error: MkdirAll temporary file: %s =>create dic failed: %s", dirPath, err)
			return "", fmt.Errorf("saveWithStream - Error: %v", err)
		}
	} else if err != nil {
		// 其他错误
		logger.SysLogf("saveWithStream - Error: MkdirAll temporary file: %s =>create dic error failed : %s", dirPath, err)
		return "", err
	}

	// 3. 创建临时文件（避免写入过程中被读取）
	tmp_name := fmt.Sprintf("tmpfile_%s.%s", random.GetRandomNumberString(16), ext)
	tmpFile, err := os.CreateTemp(dirPath, tmp_name)
	if err != nil {
		logger.SysLogf("saveWithStream - Error: creating temporary file: %s => %s", tmp_name, err)
		return "", fmt.Errorf("saveWithStream - Error: %v", err)
	}
	defer tmpFile.Close()

	// 4. 使用带内存限制的流式拷贝
	bufferedWriter := bufio.NewWriterSize(tmpFile, 32*1024) // 32KB缓冲区
	if _, err := io.CopyN(bufferedWriter, decoder, 20*1024*1024); err != nil && err != io.EOF {
		logger.SysLogf("saveWithStream - Error: io.CopyN: %s => %s", tmp_name, err)
		return "", fmt.Errorf("流式拷贝失败: %v", err)
	}
	if err := bufferedWriter.Flush(); err != nil {
		logger.SysLogf("saveWithStream - Error: bufferedWriter.Flush: %s => %s", tmp_name, err)
		return "", fmt.Errorf("缓冲写入失败: %v", err)
	}

	// 5. 原子重命名确保完整性
	if err := os.Rename(tmpFile.Name(), (dirPath + "/" + tmp_name)); err != nil {
		logger.SysLogf("saveWithStream - Error: os.Rename: %s => %s", (dirPath + "/" + tmp_name), err)
		return "", fmt.Errorf("文件重命名失败: %v", err)
	}
	return (dirPath + "/" + tmp_name), nil
}

func GetImageFromUrl(url string, saveLocal bool) (mimeType string, data string, err error) {
	// Check if the URL is a base64
	imgType, imgData, err := getImageFormat(url, saveLocal)

	if err == nil && imgType != "" {
		// URL is a data URL
		data = imgData
		return imgType, data, nil
	}
	isImage, err := IsImageUrl(url)
	if !isImage && err == nil {
		return "", "", fmt.Errorf("failed to get this url : it may not an image")
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		setImageCache(url, false, "", 0, 0, "")
		return "", "", fmt.Errorf("failed to get this url : %s, err: %s", url, err)
	}
	defer resp.Body.Close()

	var encodedBuilder strings.Builder
	encoder := base64.NewEncoder(base64.StdEncoding, &encodedBuilder)
	defer encoder.Close()

	// 设置内存安全限制（示例设为20MB）
	const maxSize = 20 << 20 // 20MB
	limitedReader := io.LimitReader(resp.Body, maxSize)

	bytesCopied, err := io.Copy(encoder, limitedReader) // 流式处理
	if err != nil {
		return "", "", fmt.Errorf("copy error: %v", err)
	}
	// 检查是否超过大小限制
	if bytesCopied >= maxSize {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		logger.SysLogf("Memory usage: HeapInuse=%v MiB", m.HeapInuse/1024/1024)
		logger.SysLogf("images is too large: %s,", url)
		return "", "", fmt.Errorf("image exceeds maximum allowed size")
	}

	// 确保所有数据刷新到builder
	if err := encoder.Close(); err != nil {
		return "", "", fmt.Errorf("base64 close error: %w", err)
	}
	data = encodedBuilder.String()

	mimeType = resp.Header.Get("Content-Type")
	parts := strings.SplitN(mimeType, ";", 2)

	if saveLocal {
		t := strings.SplitN(parts[0], "/", 2)
		ext := ""
		if len(t) < 2 {
			logger.SysLogf("GetImageFromUrl - saveLocal: split Content-Type err: %s =>%s=>%v", mimeType, parts[0], t)
			ext = "unknow"
		} else {
			ext = t[1]
		}
		//保存到本地文件
		file, err := saveWithStream(data, ext)
		if err != nil {
			return "", "", fmt.Errorf("fail to saveWithStream: %s", err)
		}
		setImageCache(url, true, parts[0], 0, 0, file)
		return parts[0], file, nil
	}
	return parts[0], data, nil
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
