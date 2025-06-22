package gemini

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func ConvertImageRequest(request relaymodel.ImageRequest) (*ImageRequest, error) {
	var contents []ChatContent
	if len(request.Image) > 0 {
		logger.SysLogf("有图片! %d 张", len(request.Image))
		var parts []Part
		for _, img := range request.Image {
			//图片编辑
			mimeType, fileData, err := image.GetImageFromUrl(img, false)
			if err != nil {
				return nil, err
			}
			parts = append(parts, Part{
				InlineData: &InlineData{
					MimeType: mimeType,
					Data:     fileData,
				},
			})
		}
		parts = append(parts, Part{
			Text: request.Prompt,
		})
		contents = append(contents, ChatContent{
			Role:  "user",
			Parts: parts,
		})
	} else {
		//图片创建
		if request.N > 1 {
			contents = append(contents, ChatContent{
				Role: "user",
				Parts: []Part{
					{
						Text: fmt.Sprintf("I will send you a prompt, please generate pictures according to the prompts, and you need to generate %d different pictures", request.N),
					},
				},
			}, ChatContent{
				Role: "model",
				Parts: []Part{
					{
						Text: "Ok",
					},
				},
			})
		}
		contents = append(contents, ChatContent{
			Role: "user",
			Parts: []Part{
				{
					Text: request.Prompt,
				},
			},
		})
	}

	imageRequest := ImageRequest{
		Contents: contents,
		GenerationConfig: ChatGenerationConfig{
			ResponseModalities: []string{"text", "image"},
		},
	}

	return &imageRequest, nil
}
func ImageStreamHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.ErrorWithStatusCode, string, *relaymodel.Usage) {
	responseText := ""
	var usage *relaymodel.Usage
	scanner := bufio.NewScanner(resp.Body)
	maxBufferSize := 1024 * 1024 * 6                  // 6MB
	scanner.Buffer(make([]byte, 4096), maxBufferSize) // 初始 4KB，最大扩展到 1MB
	scanner.Split(bufio.ScanLines)

	common.SetEventStreamHeaders(c)
	var content []ImageResponse2ChatContent
	imgNum := 0
	for scanner.Scan() {
		adaptor.StartingStream(c, meta)
		data := scanner.Text()
		data = strings.TrimSpace(data)
		if !strings.HasPrefix(data, "data: ") {
			continue
		}
		data = strings.TrimPrefix(data, "data: ")
		data = strings.TrimSuffix(data, "\"")

		if config.DebugEnabled {
			logger.SysLogf("Body: %s", data)
		}
		var geminiResponse ChatResponse
		err := json.Unmarshal([]byte(data), &geminiResponse)
		if err != nil {
			logger.SysError("error unmarshalling stream response: " + err.Error())
			continue
		}
		if len(geminiResponse.Candidates) > 0 {
			if geminiResponse.Candidates[0].FinishReason == "IMAGE_SAFETY" {
				return &relaymodel.ErrorWithStatusCode{
					Error: relaymodel.Error{
						Message: "Unable to generate image that is an unsafe image, such as graphically violent or gruesome",
						Type:    "request_forbidden ",
						Param:   "",
						Code:    403,
					},
					StatusCode: 403,
				}, "", nil
			}
		}

		//请求画图模型, 但以聊天接口访问的, 按聊天接口的格式返回
		response, isStop, num := responseGemini2OpenAIChatStream(c, &geminiResponse, &content)
		imgNum += num
		if response == nil {
			logger.SysErrorf("error responseGemini2OpenAIChat: response is empty ->%s ", data)
			continue
		}
		response.Model = meta.ActualModelName
		responseText += response.Choices[0].Delta.StringContent()
		usage = &relaymodel.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
		}
		response.Usage = usage

		if len(content) > 0 && isStop {
			var prompt = ImageResponse2Chat{
				Role: "assistant",
			}
			prompt.Content = content
			response.SystemPrompt = prompt
		}

		err = render.ObjectData(c, response)
		if err != nil {
			logger.SysError(err.Error())
		}
	}

	if err := scanner.Err(); err != nil {
		logger.SysError("error reading stream: " + err.Error())
	}

	render.Done(c)

	err := resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), "", nil
	}

	return nil, responseText, usage
}

func responseGemini2OpenAIChatStream(c *gin.Context, geminiResponse *ChatResponse, promptContent *[]ImageResponse2ChatContent) (*openai.ChatCompletionsStreamResponse, bool, int) {
	var choice openai.ChatCompletionsStreamResponseChoice
	var response openai.ChatCompletionsStreamResponse
	response.Id = fmt.Sprintf("chatcmpl-%s", random.GetUUID())
	response.Created = helper.GetTimestamp()
	response.Object = "chat.completion.chunk"
	text := ""
	isStop := false
	imgNum := 0
	if len(geminiResponse.Candidates) > 0 {
		for i, candidate := range geminiResponse.Candidates {
			if candidate.FinishReason != "" {
				choice.FinishReason = &candidate.FinishReason
			}
			if strings.ToUpper(candidate.FinishReason) == "STOP" {
				isStop = true
			}
			if len(candidate.Content.Parts) > 0 {
				for _, item := range candidate.Content.Parts {
					if item.InlineData != nil {
						imgNum++
						// 这里是chat聊天模型, 直接将返回的b64转url, 不返回b64格式
						//url格式需要上传图床
						url, fileName, err := image.StreamUploadByB64(item.InlineData.Data, item.InlineData.MimeType)
						if err != nil {
							//上传失败, 仍然返回base64
							text = fmt.Sprintf(`%s![Image_%d](data:%s;base64,%s)`, text, i, item.InlineData.MimeType, item.InlineData.Data)
						} else {
							text = fmt.Sprintf(`%s![Image_%d](%s)`, text, i, url)
						}
						//这里, 将图片再次异步上传给gemini, 方便下次使用
						syncUploadImg2Gemini(c, item.InlineData.MimeType, fileName, url)
						*promptContent = append(*promptContent, ImageResponse2ChatContent{
							Type: "image_url",
							ImageUrl: ImageResponse2ChatImageUrl{
								Url:    url,
								Detail: item.InlineData.MimeType,
							},
						})
					} else {
						text = fmt.Sprintf("%s%s", text, item.Text)
						*promptContent = append(*promptContent, ImageResponse2ChatContent{
							Type: "text",
							Text: text,
						})
					}
				}
			}
		}
	}
	choice.Delta.Content = text
	response.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}
	return &response, isStop, imgNum
}

func ImageHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
	responseFormat := c.GetString("response_format")
	var geminiResponse ChatResponse
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

	// if config.DebugEnabled {
	// 	logger.SysLogf("Body: %s", string(responseBody))
	// }

	if len(geminiResponse.Candidates) == 0 {
		return &relaymodel.ErrorWithStatusCode{
			Error: relaymodel.Error{
				Message: "No candidates returned. Check your parameter of max_tokens",
				Type:    "server_error",
				Param:   "",
				Code:    500,
			},
			StatusCode: 400,
		}, nil
	}

	var usage relaymodel.Usage
	var jsonResponse []byte
	if meta.Image2Chat {
		//请求画图模型, 以chat接口访问的, 按chat接口的格式返回
		fullResponse, _ := responseGemini2OpenAIChat(c, &geminiResponse)
		usage = relaymodel.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
		}
		fullResponse.Usage = usage
		jsonResponse, err = json.Marshal(fullResponse)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
		}
	} else {
		//请求画图模型, 以画图接口访问的, 按画图接口的格式返回
		fullResponse, jerr := responseGemini2OpenAIImage(&geminiResponse, responseFormat)
		if jerr != nil {
			return jerr, nil
		}
		usage = relaymodel.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
		}
		fullResponse.Usage = usage
		jsonResponse, err = json.Marshal(fullResponse)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
		}
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, &usage
}

func responseGemini2OpenAIChat(c *gin.Context, response *ChatResponse) (*openai.TextResponse, int) {
	fullTextResponse := openai.TextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", random.GetUUID()),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: make([]openai.TextResponseChoice, 0, len(response.Candidates)),
	}
	var prompt = ImageResponse2Chat{
		Role: "assistant",
	}
	imgNum := 0
	var promptContent []ImageResponse2ChatContent
	for i, candidate := range response.Candidates {
		choice := openai.TextResponseChoice{
			Index: i,
			Message: relaymodel.Message{
				Role:    "assistant",
				Content: "",
			},
			FinishReason: constant.StopFinishReason,
		}
		if len(candidate.Content.Parts) > 0 {
			if candidate.Content.Parts[0].FunctionCall != nil {
				choice.Message.ToolCalls = getToolCalls(&candidate)
			} else {
				for i, item := range candidate.Content.Parts {
					content := ""
					if item.InlineData != nil {
						imgNum++
						// 这里是chat聊天模型, 直接将返回的b64转url, 不返回b64格式
						//url格式需要上传图床
						url, fileName, err := image.StreamUploadByB64(item.InlineData.Data, item.InlineData.MimeType)
						if err != nil {
							//上传失败, 仍然返回base64
							content = fmt.Sprintf(`%s![Image_%d](data:%s;base64,%s)`, choice.Message.Content, i, item.InlineData.MimeType, item.InlineData.Data)
						} else {
							content = fmt.Sprintf(`%s![Image_%d](%s)`, choice.Message.Content, i, url)
						}
						//这里, 将图片再次异步上传给gemini, 方便下次使用
						syncUploadImg2Gemini(c, item.InlineData.MimeType, fileName, url)
						promptContent = append(promptContent, ImageResponse2ChatContent{
							Type: "image_url",
							ImageUrl: ImageResponse2ChatImageUrl{
								Url:    url,
								Detail: item.InlineData.MimeType,
							},
						})
					} else {
						if i == 0 {
							content = item.Text
						} else {
							content = fmt.Sprintf("%s\n%s", choice.Message.Content, item.Text)
						}
						promptContent = append(promptContent, ImageResponse2ChatContent{
							Type: "text",
							Text: content,
						})
					}
					choice.Message.Content = content
				}
			}
		} else {
			choice.Message.Content = ""
			choice.FinishReason = candidate.FinishReason
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	prompt.Content = promptContent
	fullTextResponse.SystemPrompt = prompt
	return &fullTextResponse, imgNum
}

func responseGemini2OpenAIImage(response *ChatResponse, respType string) (*ImageResponse, *relaymodel.ErrorWithStatusCode) {
	var imgList []ImageData
	text := ""
	for _, candidate := range response.Candidates {
		if len(candidate.Content.Parts) > 0 {
			for _, item := range candidate.Content.Parts {
				if item.InlineData != nil {
					if respType == "b64_json" {
						imgList = append(imgList, ImageData{
							B64Json: item.InlineData.Data,
						})
					} else {
						//url格式需要上传图床
						url, _, err := image.StreamUploadByB64(item.InlineData.Data, item.InlineData.MimeType)
						if err != nil {
							return nil, openai.ErrorWrapper(err, "upload_image", http.StatusBadRequest)
						}
						imgList = append(imgList, ImageData{
							Url: url,
						})
					}
				} else {
					text = fmt.Sprintf("%s\n%s", text, item.Text)
				}
			}
		}
	}
	if len(imgList) == 0 {
		return nil, openai.ErrorWrapper(fmt.Errorf("Your prompt cannot generate an image, please adjust the prompt"), "invalid_prompt", http.StatusBadRequest)
	}
	image := ImageResponse{
		Created:       helper.GetTimestamp(),
		Data:          imgList,
		RevisedPrompt: text,
	}
	return &image, nil
}

// 异步上传文件到gemini, 这里是针对chat模型做的优化, 可以加快聊天响应速度
func syncUploadImg2Gemini(c *gin.Context, mimeType string, filePath string, url string) {
	go func(newContext *gin.Context) {
		_, fileData, err := FileHandler(newContext, url, url, mimeType, filePath)
		if err != nil {
			meta := meta.GetByContext(newContext)
			logger.SysErrorf("syncUploadImg2Gemini - FileHandler err: %s, api-key: %s", err.Error(), meta.APIKey)
			return
		}
		logger.SysLogf("sync upload image to gemini success : %s", fileData)
	}(c.Copy())
}
