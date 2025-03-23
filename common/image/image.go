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
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
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
func IsImageUrl(url string) (bool, string, error) {
	var cache *ImageCache
	result, err := common.RedisHashGet("image_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err == nil {
			return cache.IsURL, cache.ContentType, nil
		}
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = imageClient.Get(url)
		if err != nil {
			logger.SysLogf("IsImageUrl - HTTPClient报错: %s", err.Error())
			return false, "", err
		}
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		//读取响应体
		body, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			logger.SysLogf("IsImageUrl - io.ReadAll: %s", contentType)
			setImageCache(url, false, "", 0, 0, "")
			return false, "", nil
		}

		// 获取并解析Content-Type
		detectedType := http.DetectContentType(body)
		contentType, _, _ = mime.ParseMediaType(detectedType)
		if !strings.HasPrefix(contentType, "image/") {
			logger.SysLogf("IsImageUrl - Content-Type错误: %s - %s", contentType, url)
			setImageCache(url, false, "", 0, 0, "")
			return false, "", nil
		}
	}
	setImageCache(url, true, contentType, 0, 0, "")
	return true, contentType, nil
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
	isImage, _, err := IsImageUrl(url)
	if !isImage {
		return
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = imageClient.Get(url)
		if err != nil {
			logger.SysLogf("HTTPClient报错: %s", err.Error())
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
		logger.SysLogf("Vision-Base64方式-DecodeString报错: %s->%s", input, err.Error())
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
		file, err := SaveWithStream(input, t)
		if err != nil {
			return "", "", fmt.Errorf("fail to SaveWithStream: %s", err)
		}
		setImageCache(source, true, contentType, 0, 0, file)
		return contentType, file, nil
	}
	return contentType, input, nil
}

// 流式保存函数（内存安全版）
func SaveWithStream(base64Data string, ext string) (string, error) {
	// 1. 创建解码流（复用格式检测后的纯净Base64数据）
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(base64Data))

	// 2. 创建目标目录
	dirPath := "/mnt/tpm_file"
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			logger.SysLogf("SaveWithStream - Error: MkdirAll temporary file: %s =>create dic failed: %s", dirPath, err)
			return "", fmt.Errorf("SaveWithStream - Error: %v", err)
		}
	} else if err != nil {
		// 其他错误
		logger.SysLogf("SaveWithStream - Error: MkdirAll temporary file: %s =>create dic error failed : %s", dirPath, err)
		return "", err
	}

	// 3. 创建临时文件（避免写入过程中被读取）
	tmp_name := fmt.Sprintf("tmpfile_%s.%s", random.GetRandomNumberString(16), ext)
	tmpFile, err := os.CreateTemp(dirPath, tmp_name)
	if err != nil {
		logger.SysLogf("SaveWithStream - Error: creating temporary file: %s => %s", tmp_name, err)
		return "", fmt.Errorf("SaveWithStream - Error: %v", err)
	}
	defer tmpFile.Close()

	// 4. 使用带内存限制的流式拷贝
	bufferedWriter := bufio.NewWriterSize(tmpFile, 32*1024) // 32KB缓冲区
	if _, err := io.CopyN(bufferedWriter, decoder, 20*1024*1024); err != nil && err != io.EOF {
		logger.SysLogf("SaveWithStream - Error: io.CopyN: %s => %s", tmp_name, err)
		return "", fmt.Errorf("流式拷贝失败: %v", err)
	}
	if err := bufferedWriter.Flush(); err != nil {
		logger.SysLogf("SaveWithStream - Error: bufferedWriter.Flush: %s => %s", tmp_name, err)
		return "", fmt.Errorf("缓冲写入失败: %v", err)
	}

	// 5. 原子重命名确保完整性
	if err := os.Rename(tmpFile.Name(), (dirPath + "/" + tmp_name)); err != nil {
		logger.SysLogf("SaveWithStream - Error: os.Rename: %s => %s", (dirPath + "/" + tmp_name), err)
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
	isImage, contentType, err := IsImageUrl(url)
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

	parts := strings.SplitN(contentType, ";", 2)

	if saveLocal {
		t := strings.SplitN(parts[0], "/", 2)
		ext := ""
		if len(t) < 2 {
			logger.SysLogf("GetImageFromUrl - saveLocal: split Content-Type err: %s =>%s=>%v", contentType, parts[0], t)
			ext = "unknow"
		} else {
			ext = t[1]
		}
		//保存到本地文件
		file, err := SaveWithStream(data, ext)
		if err != nil {
			return "", "", fmt.Errorf("fail to SaveWithStream: %s", err)
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

type UploadResponse []struct {
	Src string `json:"src"`
}

// 流式上传图片到图床
func StreamUploadByB64(b64Data string, mimeType string) (string, string, error) {
	parts := strings.Split(mimeType, "/")
	extension := ""
	if len(parts) != 2 {
		extension = "png"
	}
	extension = parts[1]
	filePath, err := SaveWithStream(b64Data, extension)
	if err != nil {
		logger.SysErrorf("StreamUploadByB64 - SaveWithStream err: %s", err.Error())
		return "", "", err
	}

	filename := fmt.Sprintf("%s.%s", random.GetRandomString(16), extension)

	file, err := os.Open(filePath)
	if err != nil {
		logger.SysLogf("StreamUploadByB64 - os.Open err: %s", err.Error())
		return "", "", err
	}
	defer file.Close()

	// 重试机制
	var src string
	var maxRetries = 3 // 最大重试次数
	for i := 0; i < maxRetries; i++ {
		// 创建管道连接解码和上传
		pr, pw := io.Pipe()
		errChan := make(chan error, 1)
		writer := multipart.NewWriter(pw)

		defer pw.Close()
		go func() {
			defer pw.Close()
			defer writer.Close()
			part, _ := writer.CreateFormFile("file", filename)
			if err != nil {
				errChan <- err
				return
			}

			if _, err := io.Copy(part, file); err != nil {
				errChan <- err
				return
			}
			errChan <- nil
		}()

		// 在主goroutine中检查错误
		select {
		case err := <-errChan:
			if err != nil {
				logger.SysLogf("StreamUploadByB64 - io.Copy err: %s", err.Error())
				return "", "", err
			}
		default:
			// 正常继续执行
		}
		// 创建请求
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/upload", config.GeminiImgUploadDomain), pr)
		if err != nil {
			logger.SysLogf("StreamUploadByB64 - NewRequest err: %s", err.Error())
			return "", "", err
		}

		// 设置Content-Type
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := client.HTTPClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			// 只处理200状态码
			if resp.StatusCode != http.StatusOK {
				logger.SysLogf("StreamUploadByB64 - StatusCode err: %d", resp.StatusCode)
				return "", "", fmt.Errorf("file upload fail - status : %d", resp.StatusCode)
			}

			// 读取并解析响应
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.SysLogf("StreamUploadByB64 - ReadAll err: %s", err.Error())
				return "", "", fmt.Errorf("file upload fail - err: %s", err.Error())
			}
			logger.SysLogf("StreamUploadByB64 - done status: %s, detail: %s", resp.Status, string(body))

			var result UploadResponse
			if err := json.Unmarshal(body, &result); err != nil {
				logger.SysLogf("StreamUploadByB64 - Unmarshal err: %s", err.Error())
				return "", "", fmt.Errorf("JSON解析失败: %v", err.Error())
			}

			// 验证响应格式
			if len(result) == 0 || result[0].Src == "" {
				logger.SysLogf("StreamUploadByB64 - empty result: %s", string(body))
				return "", "", fmt.Errorf("file upload fail - empty response")
			}
			src = result[0].Src
			break
		} else {
			logger.SysLogf("StreamUploadByB64 - Post - %s error: , retrying", err)
			if resp != nil {
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err == nil {
					logger.SysLogf("StreamUploadByB64 - Post - %s error, status : %s, detail: %s", config.GeminiImgUploadDomain, resp.Status, string(body))
				}
			}
		}
	}

	if src == "" {
		logger.SysLog("StreamUploadByB64 - empty url, exceed maximum upload retries")
		return "", "", fmt.Errorf("file upload fail - exceed maximum upload retries")
	}
	return fmt.Sprintf("%s%s", config.GeminiImgUploadDomain, src), filePath, nil
}
