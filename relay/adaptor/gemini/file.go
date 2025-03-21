package gemini

import (
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/generative-ai-go/genai"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/media"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/meta"
	"google.golang.org/api/option"
)

// 文件上传处理
func FileHandler(c *gin.Context, fieldUrl string, url string, contentType string, fileName string) (string, string, error) {
	meta := meta.GetByContext(c)
	//判断文件是否已经存在
	fileOld, err := model.GetFile(fieldUrl)
	if err != nil {
		return "", "", fmt.Errorf("get old file error: %s", err.Error())
	}
	currentKey := c.GetString("x-new-api-key")
	if currentKey == "" {
		currentKey = meta.APIKey
	}
	if fileOld.FileId != "" {
		//如果已存在当前句柄的x-new-api-key变量, 说明同个请求多个文件,例如A文件对应账号T1, 此时查询存在B文件但是对应key与A文件不同, 则需要重新上传
		//如果不存在缓存key或者 不同文件的key相同, 则可以用旧的值, 否则都需要重新上传
		newKey := c.GetString("x-new-api-key")
		if newKey == "" || newKey == fileOld.Key {
			meta.APIKey = fileOld.Key
			c.Set("x-new-api-key", fileOld.Key)
			c.Set("FileUri", fileOld.FileId)
			return fileOld.ContentType, fileOld.FileId, nil
		}
	}

	//1. 保存文件
	if contentType == "" && fileName == "" {
		err, contentType, fileName = media.SaveMediaByUrl(url)
		if err != nil {
			return "", "", fmt.Errorf("upload file error: %w", err)
		}
	}

	//2. 检查文件是否支持的类型
	if _, err := media.CheckLegalUrl(meta.APIType, contentType); err != nil {
		return "", "", err
	}
	//3.初始化gemini客户端
	client, err := genai.NewClient(c, option.WithAPIKey(currentKey))
	if err != nil {
		return "", "", fmt.Errorf("init genai error: %s", err.Error())
	}
	defer client.Close()

	//4. 创建文件并上传
	f, err := os.Open(fileName)
	if err != nil {
		return "", "", fmt.Errorf("os.Open error: %s", err.Error())
	}
	opts := genai.UploadFileOptions{
		MIMEType:    contentType,
		DisplayName: random.GetRandomString(10),
	}
	file, err := client.UploadFile(c, "", f, &opts)
	if err != nil {
		return "", "", fmt.Errorf("upload file error: %s", err.Error())
	}
	defer f.Close()
	// defer os.Remove(fileName) // 确保在程序结束时删除临时文件

	//5. 循环获取文件上传状态
	retryNum := 10
	for file.State == genai.FileStateProcessing {
		if retryNum <= 0 {
			return "", "", fmt.Errorf("Error: getting file state but timeout")
		}
		retryNum--
		time.Sleep(2 * time.Second)
		var err error
		if file, err = client.GetFile(c, file.Name); err != nil {
			return "", "", fmt.Errorf("Error in getting file state: %s", err.Error())
		}
	}
	if file.State != genai.FileStateActive {
		return "", "", fmt.Errorf("state %s: we can't process your file because it's failed when upload to Google server, you should check your files if it's legally", file.State)
	}
	//6. 保存文件数据
	fileModel := model.Files{
		TokenId:     meta.TokenId,
		Key:         currentKey,
		ContentType: contentType,
		ChannelId:   meta.ChannelId,
		Url:         fieldUrl,
		FileId:      file.URI,
	}
	err, fileId := fileModel.SaveFile()
	if err != nil {
		return "", "", fmt.Errorf("Error: saving file failed: %s", err.Error())
	}
	logger.SysLogf("[Upload File] API Key: %s | Url: %s | FileId: %d", currentKey, file.URI, fileId)
	c.Set("FileUri", file.URI)

	return contentType, file.URI, nil
}
