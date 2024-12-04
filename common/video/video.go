package video

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/relay/apitype"
)

func IsVideoUrl(url string) (bool, error) {
	videoRegex := regexp.MustCompile(`(mp4|mov|mpeg|mpg|webm|wmv|3gpp|avi|x-flv)$`)
	if videoRegex.MatchString(url) {
		return true, nil
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = client.HTTPClient.Get(url)
		if err != nil {
			logger.SysLogf("IsVideoUrl - faild again:  %s", err)
			return false, fmt.Errorf("failed to get this url : %s, status : %s, err: %s", url, resp.Status, err)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to get this url : %s, status : %s", url, resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	return videoRegex.MatchString(contentType), nil
}

// 保存客户上传的多媒体文件
func SaveMediaByUrl(url string) (error, string, string) {
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		//先改为正常请求, 再次报错再进行异常抛出
		resp, err = client.HTTPClient.Get(url)
		if err != nil {
			logger.SysLogf("GetVideoUrl - faild again:  %s", err)
			return fmt.Errorf("failed to get this url : %s, status : %s, err: %s", url, resp.Status, err), "", ""
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
