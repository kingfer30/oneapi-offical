package media

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/relay/apitype"
)

var CacheSecond int64 = 600

type MediaCache struct {
	IsMedia     bool   `json:"is_media"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
}

func setMediaCache(url string, IsMedia bool, contentType string, path string) {
	var cache *MediaCache
	result, err := common.RedisHashGet("media_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err != nil {
			cache = &MediaCache{
				IsMedia: IsMedia,
			}
		}
	} else {
		cache = &MediaCache{
			IsMedia: IsMedia,
		}
	}
	if contentType != "" && cache.ContentType != contentType {
		cache.ContentType = contentType
	}
	if path != "" && cache.Path != path {
		cache.Path = path
	}
	common.RedisHashSet("media_url", random.StrToMd5(url), cache, CacheSecond)
}

func IsMediaUrl(url string) (bool, error) {
	if !strings.HasPrefix(url, "http") && !strings.HasPrefix(url, "https") {
		//url no check
		return false, nil
	}
	mediaRegex := regexp.MustCompile(`(mp4|mov|mpeg|mpg|webm|wmv|3gpp|avi|x-flv|pdf|wav|mp3|aiff|aac|ogg|flac)$`)
	if mediaRegex.MatchString(url) {
		return true, nil
	}
	var cache *MediaCache
	result, err := common.RedisHashGet("media_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err == nil {
			return cache.IsMedia, nil
		}
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = client.HTTPClient.Get(url)
		if err != nil {
			logger.SysLogf("IsMediaUrl - faild again:  %s", err)
			setMediaCache(url, false, "", "")
			return false, fmt.Errorf("failed to get this url : %s, err: %s", url, err)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		setMediaCache(url, false, "", "")
		return false, fmt.Errorf("failed to get this url : %s, status : %s", url, resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	setMediaCache(url, mediaRegex.MatchString(contentType), contentType, "")
	return mediaRegex.MatchString(contentType), nil
}

// 保存客户上传的多媒体文件
func SaveMediaByUrl(url string) (error, string, string) {
	var cache *MediaCache
	result, err := common.RedisHashGet("media_url", random.StrToMd5(url))
	if err == nil {
		err = json.Unmarshal([]byte(result), &cache)
		if err == nil {
			return nil, cache.ContentType, cache.Path
		}
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = client.HTTPClient.Get(url)
		if err != nil {
			logger.SysLogf("GetMediaUrl - faild again:  %s", err)
			if resp == nil {
				return fmt.Errorf("failed to get this url : %s, resp为空, err: %s", url, err), "", ""
			} else {
				return fmt.Errorf("failed to get this url : %s, status : %s, err: %s", url, resp.Status, err), "", ""
			}
		}
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		logger.SysLogf("SaveMediaByUrl - Error: received non-200 response status: %s\n", resp.Status)
		return err, "", ""
	}
	extension := filepath.Ext(url)
	if extension == "" {
		if contentType != "" {
			list := strings.Split(contentType, "/")
			if len(list) > 1 {
				extension = list[1]
			}
		} else {
			logger.SysLogf("SaveMediaByUrl - Error: url extension is empty: %s\n", url)
			return err, "", ""
		}
	} else {
		list := strings.Split(extension, "?")
		if len(list) > 1 {
			extension = list[0]
		}
	}
	if contentType == "audio/mpeg" {
		contentType = "audio/mp3"
	}

	// 创建临时文件
	tmp_name := fmt.Sprintf("tmpfile_%s%s", random.GetRandomNumberString(16), extension)
	tempFile, err := os.CreateTemp("", tmp_name)
	if err != nil {
		logger.SysLogf("SaveMediaByUrl - Error: creating temporary file: %s => %s", url, tmp_name)
		return err, "", ""
	}

	// 使用bufio进行高效读写
	writer := bufio.NewWriter(tempFile)
	defer tempFile.Close()

	// 分块读取
	const blockSize = 1024 * 1024 // 1MB
	buf := make([]byte, blockSize)
	total := 0
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			total += n
		}
		if err != nil {
			if n == 0 || err == io.EOF {
				// 如果是EOF，说明已经读取完毕，可以正常退出循环
				if n > 0 {
					res, err := writer.Write(buf[:n])
					if err != nil {
						logger.SysLogf("SaveMediaByUrl - Error: writer.Write file: %s => %s", url, err.Error())
						return err, "", ""
					}
					logger.SysLogf("done - %v - %v - %v", n, res, total)
				}
				break
			}
			logger.SysLogf("SaveMediaByUrl - Error: resp.Body.Read: %s => %s", url, err.Error())
			return err, "", ""
		}
		if _, err := writer.Write(buf[:n]); err != nil {
			logger.SysLogf("SaveMediaByUrl - Error: writer.Write file: %s => %s", url, err.Error())
			return err, "", ""
		}
	}
	if err := writer.Flush(); err != nil {
		logger.SysLogf("SaveMediaByUrl - Error: flushing writer: %s", err)
	}
	// 获取文件的真实大小
	fileInfo, err := tempFile.Stat()
	if err != nil {
		logger.SysLogf("SaveMediaByUrl - Error getting file info: %s", err)
		return err, "", ""
	}
	logger.SysLogf("SaveMediaByUrl - url: %s, save-path: %s, file_name: %s, content-type: %s,file-size: %d", url, tmp_name, tempFile.Name(), contentType, fileInfo.Size())
	setMediaCache(url, true, contentType, tempFile.Name())
	return nil, contentType, tempFile.Name()
}

// 检查多媒体文件是否合法(api是否支持上传)
func CheckLegalUrl(apiType int, contentType string) (string, error) {
	if apiType == apitype.Gemini {
		switch contentType {
		//pdf
		case "application/pdf":
			return "pdf", nil
		//音频
		case "audio/wav":
			return "wav", nil
		case "audio/mp3":
			return "mp3", nil
		case "audio/aiff":
			return "aiff", nil
		case "audio/aac":
			return "aac", nil
		case "audio/ogg":
			return "ogg", nil
		case "audio/flac":
			return "flac", nil
		//视频
		case "video/mp4":
			return "mp4", nil
		case "video/mpeg":
			return "mpeg", nil
		case "video/mov":
			return "mov", nil
		case "video/avi":
			return "avi", nil
		case "video/x-flv":
			return "x-flv", nil
		case "video/mpg":
			return "mpg", nil
		case "video/webm":
			return "webm", nil
		case "video/wmv":
			return "wmv", nil
		case "video/3gpp":
			return "3gpp", nil
		}
	}
	return "", fmt.Errorf("unsupport media type: %s", contentType)
}
