package video

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/relay/apitype"
)

func IsVideoUrl(url string) bool {
	videoRegex := regexp.MustCompile(`\.(mp4|mov|mpeg|mpg|webm|wmv|3gpp|avi|x-flv)$`)
	return videoRegex.MatchString(url)
}

// 保存客户上传的多媒体文件
func SaveMediaByUrl(url string) (error, string, string) {
	logger.SysLogf("[Saving File] - %s", url)
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		return err, "", ""
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
		logger.SysLogf("SaveMediaByUrl - Error: url extension is empty: %s\n", url)
		return err, "", ""
	}

	// 创建临时文件
	tmp_name := fmt.Sprintf("tmp_%s%s", random.GetRandomNumberString(16), extension)
	tempFile, err := os.CreateTemp("", tmp_name)
	if err != nil {
		logger.SysLogf("SaveMediaByUrl - Error: creating temporary file: %s => %s", url, tmp_name)
		return err, "", ""
	}

	// 使用bufio进行高效读写
	writer := bufio.NewWriter(tempFile)
	defer writer.Flush()
	defer tempFile.Close()

	// 分块读取
	const blockSize = 1024 * 1024 // 1MB
	buf := make([]byte, blockSize)
	for {
		n, err := resp.Body.Read(buf)
		if n == 0 && err != nil {
			break
		}
		if err != nil {
			logger.SysLogf("SaveMediaByUrl - Error: resp.Body.Read: %s => %s", url, err.Error())
			return err, "", ""
		}
		if _, err := writer.Write(buf[:n]); err != nil {
			logger.SysLogf("SaveMediaByUrl - Error: writer.Write file: %s => %s", url, err.Error())
			return err, "", ""
		}
	}
	// 获取文件的真实大小
	fileInfo, err := tempFile.Stat()
	if err != nil {
		logger.SysLogf("SaveMediaByUrl - Error getting file info: %s", err)
		return err, "", ""
	}
	logger.SysLogf("SaveMediaByUrl - url: %s, save-path: %s, content-type: %s,file-size: %d", url, tmp_name, contentType, fileInfo.Size())
	return nil, contentType, tempFile.Name()
}

// 检查多媒体文件是否合法(api是否支持上传)
func CheckLegalUrl(apiType int, contentType string) (string, error) {
	if apiType == apitype.Gemini {
		switch contentType {
		case "image/jpeg":
			return "jpeg", nil
		case "image/png":
			return "png", nil
		case "image/webp":
			return "webp", nil
		case "image/heic":
			return "heic", nil
		case "image/heif":
			return "heif", nil
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
