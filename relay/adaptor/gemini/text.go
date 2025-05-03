package gemini

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/media"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/model"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"

	"github.com/gin-gonic/gin"
)

// https://ai.google.dev/docs/gemini_api_overview?hl=zh-cn

const (
	VisionMaxImageNum = 16
)

var mimeTypeMap = map[string]string{
	"json_object": "application/json",
	"text":        "text/plain",
}

// Setting safety to the lowest possible values since Gemini is already powerless enough
func ConvertRequest(c *gin.Context, textRequest relaymodel.GeneralOpenAIRequest) (*ChatRequest, error) {
	generationConfig := ChatGenerationConfig{
		Temperature:     textRequest.Temperature,
		TopP:            textRequest.TopP,
		MaxOutputTokens: textRequest.MaxTokens,
		StopSequences:   textRequest.Stop,
	}
	if IsImageModel(textRequest.Model) {
		generationConfig.ResponseModalities = []string{"text", "image"}
	}
	if textRequest.ThinkingBudget != nil {
		generationConfig.ThinkingConfig = &ThinkingConfig{
			ThinkingBudget: textRequest.ThinkingBudget,
		}
	}
	geminiRequest := ChatRequest{
		Contents:         make([]ChatContent, 0, len(textRequest.Messages)),
		GenerationConfig: generationConfig,
	}
	if textRequest.Modalities != nil {
		geminiRequest.GenerationConfig.ResponseModalities = textRequest.Modalities
	}
	if textRequest.ResponseFormat != nil {
		if mimeType, ok := mimeTypeMap[textRequest.ResponseFormat.Type]; ok {
			geminiRequest.GenerationConfig.ResponseMimeType = mimeType
		}
		if textRequest.ResponseFormat.Schema != nil {
			geminiRequest.GenerationConfig.ResponseSchema = textRequest.ResponseFormat.Schema
			geminiRequest.GenerationConfig.ResponseMimeType = mimeTypeMap["json_object"]
		}
	}
	if textRequest.Tools != nil {
		var tools []ChatTools
		var functions []relaymodel.Function
		for _, tool := range textRequest.Tools {
			if tool.Type == "google_search_tool" {
				tools = append(tools, ChatTools{
					GoogleSearch: GoogleSearch{},
				})
				continue
			}
			functions = append(functions, tool.Function)
		}
		if len(functions) > 0 {
			tools = append(tools, ChatTools{
				FunctionDeclarations: functions,
			})
		}
		geminiRequest.Tools = tools
	} else if textRequest.Functions != nil {
		geminiRequest.Tools = []ChatTools{
			{
				FunctionDeclarations: textRequest.Functions,
			},
		}
	}
	nextRole := "user"
	for k, message := range textRequest.Messages {
		msg := message.StringContent()
		if msg == "" {
			msg = "Hi"
		}
		content := ChatContent{
			Role: message.Role,
			Parts: []Part{
				{
					Text: msg,
				},
			},
		}
		openaiContent := message.ParseContent()
		var parts []Part
		imageNum := 0
		for _, part := range openaiContent {
			if part.Type == relaymodel.ContentTypeText {
				msg = part.Text
				if msg == "" {
					msg = "Hi"
				}
				parts = append(parts, Part{
					Text: msg,
				})
			} else if part.Type == relaymodel.ContentTypeImageURL {
				imageNum += 1
				if imageNum > VisionMaxImageNum {
					continue
				}
				mimeType := ""
				fileData := ""
				var err error
				ok, err := media.IsMediaUrl(part.ImageURL.Url)
				if err != nil {
					return nil, err
				}
				if ok {
					mimeType, fileData, err = FileHandler(c, part.ImageURL.Url, part.ImageURL.Url, "", "")
					if err != nil {
						return nil, err
					}
					parts = append(parts, Part{
						FileData: &FileData{
							MimeType: mimeType,
							Uri:      fileData,
						},
					})
				} else {
					// 这里图片统一转为File, 因为base64经常报错
					// 以下的情形会强制开启图片上传gemini:
					// 1. 后台统一开启,
					// 2. chat对话中使用了画图模型, 对话角色=系统的也强制开启, 用户的不管
					// 3. 用户图片的, 但是第一次请求报错429的, 会改为这种
					if config.GeminiUploadImageEnabled || (IsImageModel(textRequest.Model) && content.Role != "user") ||
						(image.GetImageCacheWithGeminiFile(part.ImageURL.Url) != "") {
						fileName := ""
						fieldUrl := ""
						if strings.HasPrefix(part.ImageURL.Url, "http") || strings.HasPrefix(part.ImageURL.Url, "https") {
							fieldUrl = part.ImageURL.Url
						} else {
							fieldUrl = random.StrToMd5(part.ImageURL.Url)
						}
						fileOld, err := model.GetFile(fieldUrl)
						if err != nil || fileOld.Id == 0 {
							//为空则 重新获取
							mimeType, fileName, err = image.GetImageFromUrl(part.ImageURL.Url, true)
							if err != nil {
								return nil, err
							}
						}

						mimeType, fileData, err = FileHandler(c, fieldUrl, part.ImageURL.Url, mimeType, fileName)
						if err != nil {
							return nil, err
						}
						parts = append(parts, Part{
							FileData: &FileData{
								MimeType: mimeType,
								Uri:      fileData,
							},
						})
					} else {
						mimeType, fileData, err = image.GetImageFromUrl(part.ImageURL.Url, false)
						if err != nil {
							return nil, err
						}
						//这里走原始的图片逻辑, 是取b64传给gemini, 保存起来是用于有些prompt请求一直会429, 外层判断429后会改为上面那种方式
						c.Set("gemini-img-url", part.ImageURL.Url)
						parts = append(parts, Part{
							InlineData: &InlineData{
								MimeType: mimeType,
								Data:     fileData,
							},
						})
					}
				}

			}
		}
		content.Parts = parts

		// there's cannt start with assistant role
		if content.Role == "assistant" && k == 0 {
			geminiRequest.Contents = append(geminiRequest.Contents, ChatContent{
				Role: "user",
				Parts: []Part{
					{
						Text: "Hello",
					},
				},
			})
			nextRole = "model"
		}

		// there's no assistant role in gemini and API shall vomit if Role is not user or model
		if content.Role == "assistant" || content.Role == "system" {
			content.Role = "model"
		}
		// per chat need gemini-user
		if (content.Role == "model" || content.Role == "system" || content.Role == "assistant") && nextRole == "user" {
			geminiRequest.Contents = append(geminiRequest.Contents, ChatContent{
				Role: "user",
				Parts: []Part{
					{
						Text: "Hello",
					},
				},
			})
			nextRole = "model"
		} else if content.Role == "user" && nextRole == "model" {
			geminiRequest.Contents = append(geminiRequest.Contents, ChatContent{
				Role: "model",
				Parts: []Part{
					{
						Text: "Hi",
					},
				},
			})
			nextRole = "user"
		}

		geminiRequest.Contents = append(geminiRequest.Contents, content)

		if content.Role == "user" {
			nextRole = "model"
		} else {
			nextRole = "user"
		}
	}
	if nextRole == "user" {
		geminiRequest.Contents = append(geminiRequest.Contents, ChatContent{
			Role: "user",
			Parts: []Part{
				{
					Text: "Hello",
				},
			},
		})
	}

	if IsNeedToAddRandomModel(textRequest.Model) && len(geminiRequest.Contents) > 0 {
		name := random.GetRandomString(10)
		//需要添加随机字符以减少被gemini识别为自动程序
		if geminiRequest.Contents[0].Role == "user" {
			geminiRequest.Contents[0].Parts[0].Text = fmt.Sprintf("I'm %s, dont say my name\n%s", name, geminiRequest.Contents[0].Parts[0].Text)
		} else {
			geminiRequest.Contents = append([]ChatContent{
				{
					Role: "user",
					Parts: []Part{
						{
							Text: "Hello",
						},
					},
				},
			}, geminiRequest.Contents...)
		}
		if len(geminiRequest.Contents) > 1 {
			geminiRequest.Contents[len(geminiRequest.Contents)-1].Parts[0].Text = fmt.Sprintf("I'm %s, dont say my name\n%s", name, geminiRequest.Contents[len(geminiRequest.Contents)-1].Parts[0].Text)
		}
	}

	return &geminiRequest, nil
}

func (g *ChatResponse) GetResponseText(meta *meta.Meta) string {
	if g == nil {
		return ""
	}
	if len(g.Candidates) > 0 && len(g.Candidates[0].Content.Parts) > 0 {
		if strings.Contains(meta.ActualModelName, "think") && meta.Thinking {
			//thinkingStart:\n%s\nthinkingEnd\n%s
			if len(g.Candidates[0].Content.Parts) > 1 {
				think := "thinkingStart\n" + g.Candidates[0].Content.Parts[0].Text
				think += "\nthinkingEnd\n"
				think += g.Candidates[0].Content.Parts[1].Text
				meta.EndThinking = true
				return think
			}
			if meta.EndThinking {
				return g.Candidates[0].Content.Parts[0].Text
			} else {
				return fmt.Sprintf("thinkingStart\n%s\nthinkingEnd\n", g.Candidates[0].Content.Parts[0].Text)
			}
		} else {
			if len(g.Candidates[0].Content.Parts) > 1 {
				think := g.Candidates[0].Content.Parts[0].Text
				think += "\n"
				think += g.Candidates[0].Content.Parts[1].Text
				return think
			}
			return g.Candidates[0].Content.Parts[0].Text
		}
	}
	return ""
}

func getToolCalls(candidate *ChatCandidate) []relaymodel.Tool {
	var toolCalls []relaymodel.Tool

	item := candidate.Content.Parts[0]
	if item.FunctionCall == nil {
		return toolCalls
	}
	argsBytes, err := json.Marshal(item.FunctionCall.Arguments)
	if err != nil {
		logger.FatalLog("getToolCalls failed: " + err.Error())
		return toolCalls
	}
	toolCall := relaymodel.Tool{
		Id:   fmt.Sprintf("call_%s", random.GetUUID()),
		Type: "function",
		Function: relaymodel.Function{
			Arguments: string(argsBytes),
			Name:      item.FunctionCall.FunctionName,
		},
	}
	toolCalls = append(toolCalls, toolCall)
	return toolCalls
}

func responseGeminiChat2OpenAI(response *ChatResponse, meta *meta.Meta) *openai.TextResponse {
	fullTextResponse := openai.TextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", random.GetUUID()),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: make([]openai.TextResponseChoice, 0, len(response.Candidates)),
	}
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
					if item.InlineData != nil {
						choice.Message.Content = fmt.Sprintf("%s\ndata:%s;base64,%s", choice.Message.Content, item.InlineData.MimeType, item.InlineData.Data)
					} else {
						if strings.Contains(meta.ActualModelName, "think") && meta.Thinking {
							if i == 0 {
								choice.Message.Content = fmt.Sprintf("thinkingStart:\n%s", item.Text)
							} else {
								choice.Message.Content = fmt.Sprintf("%s\nthinkingEnd\n%s", choice.Message.Content, item.Text)
							}
						} else {
							if i == 0 {
								choice.Message.Content = item.Text
							} else {
								choice.Message.Content = fmt.Sprintf("%s\n%s", choice.Message.Content, item.Text)
							}
						}
					}
				}
			}
		} else {
			choice.Message.Content = ""
			choice.FinishReason = candidate.FinishReason
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponseGeminiChat2OpenAI(geminiResponse *ChatResponse, meta *meta.Meta) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Content = geminiResponse.GetResponseText(meta)
	//choice.FinishReason = &constant.StopFinishReason
	var response openai.ChatCompletionsStreamResponse
	response.Id = fmt.Sprintf("chatcmpl-%s", random.GetUUID())
	response.Created = helper.GetTimestamp()
	response.Object = "chat.completion.chunk"
	response.Model = "gemini"
	response.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}
	return &response
}

func StreamHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.ErrorWithStatusCode, string, *relaymodel.Usage) {
	responseText := ""
	var usage *relaymodel.Usage
	scanner := bufio.NewScanner(resp.Body)
	maxBufferSize := 1024 * 1024 * 6                  // 6MB
	scanner.Buffer(make([]byte, 4096), maxBufferSize) // 初始 4KB，最大扩展到 1MB
	scanner.Split(bufio.ScanLines)

	common.SetEventStreamHeaders(c)
	for scanner.Scan() {
		data := scanner.Text()
		data = strings.TrimSpace(data)
		if !strings.HasPrefix(data, "data: ") {
			continue
		}
		data = strings.TrimPrefix(data, "data: ")
		data = strings.TrimSuffix(data, "\"")
		var geminiResponse ChatResponse
		err := json.Unmarshal([]byte(data), &geminiResponse)
		if err != nil {
			logger.SysError("error unmarshalling stream response: " + err.Error())
			continue
		}
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != "" {
			reason := BlockReasonList[geminiResponse.PromptFeedback.BlockReason]
			return &relaymodel.ErrorWithStatusCode{
				Error: relaymodel.Error{
					Message: reason,
					Type:    "prompt_error",
					Param:   "",
					Code:    403,
				},
				StatusCode: 403,
			}, "", nil
		}
		if len(geminiResponse.Candidates) > 0 && strings.ToUpper(geminiResponse.Candidates[0].FinishReason) == "MAX_TOKENS" {
			return &relaymodel.ErrorWithStatusCode{
				Error: relaymodel.Error{
					Message: "No candidates returned. Check your parameter of max_tokens",
					Type:    "prompt_error",
					Param:   "",
					Code:    400,
				},
				StatusCode: 400,
			}, "", nil
		}
		response := streamResponseGeminiChat2OpenAI(&geminiResponse, meta)
		if response == nil {
			continue
		}

		responseText += response.Choices[0].Delta.StringContent()
		prompt, completion, quota := ResetChatQuota(
			geminiResponse.UsageMetadata.PromptTokenCount,
			geminiResponse.UsageMetadata.CandidatesTokenCount,
			geminiResponse.UsageMetadata.ThoughtsTokenCount,
			geminiResponse.UsageMetadata.TotalTokenCount,
			true,
			meta,
		)
		usage = &relaymodel.Usage{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			ThoughtsTokens:   geminiResponse.UsageMetadata.ThoughtsTokenCount,
			TotalTokens:      quota,
		}
		response.Usage = usage

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

func Handler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	var geminiResponse ChatResponse
	err = json.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if len(geminiResponse.Candidates) == 0 {
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != "" {
			reason := BlockReasonList[geminiResponse.PromptFeedback.BlockReason]
			return &relaymodel.ErrorWithStatusCode{
				Error: relaymodel.Error{
					Message: reason,
					Type:    "prompt_error",
					Param:   "",
					Code:    403,
				},
				StatusCode: 403,
			}, nil
		}
		return &relaymodel.ErrorWithStatusCode{
			Error: relaymodel.Error{
				Message: "No candidates returned. Check your parameter of max_tokens",
				Type:    "server_error",
				Param:   "",
				Code:    400,
			},
			StatusCode: 400,
		}, nil
	}
	if len(geminiResponse.Candidates) > 0 && strings.ToUpper(geminiResponse.Candidates[0].FinishReason) == "MAX_TOKENS" {
		return &relaymodel.ErrorWithStatusCode{
			Error: relaymodel.Error{
				Message: "No candidates returned. Check your parameter of max_tokens",
				Type:    "prompt_error",
				Param:   "",
				Code:    400,
			},
			StatusCode: 400,
		}, nil
	}
	fullTextResponse := responseGeminiChat2OpenAI(&geminiResponse, meta)
	fullTextResponse.Model = meta.ActualModelName
	var usage relaymodel.Usage
	if geminiResponse.UsageMetadata != nil {
		prompt, completion, quota := ResetChatQuota(
			geminiResponse.UsageMetadata.PromptTokenCount,
			geminiResponse.UsageMetadata.CandidatesTokenCount,
			geminiResponse.UsageMetadata.ThoughtsTokenCount,
			geminiResponse.UsageMetadata.TotalTokenCount,
			false,
			meta,
		)
		usage = relaymodel.Usage{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			ThoughtsTokens:   geminiResponse.UsageMetadata.ThoughtsTokenCount,
			TotalTokens:      quota,
		}
	} else {
		completionTokens := openai.CountTokenText(geminiResponse.GetResponseText(meta), meta.ActualModelName)
		usage = relaymodel.Usage{
			PromptTokens:     meta.PromptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      meta.PromptTokens + completionTokens,
		}
	}

	fullTextResponse.Usage = usage
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, &usage
}
func ChangeChat2TxtRequest(c *gin.Context, textRequest relaymodel.GeneralOpenAIRequest) (*ChatRequest, error) {
	generationConfig := ChatGenerationConfig{
		Temperature:     textRequest.Temperature,
		TopP:            textRequest.TopP,
		MaxOutputTokens: textRequest.MaxTokens,
		StopSequences:   textRequest.Stop,
	}
	if textRequest.ThinkingBudget != nil {
		generationConfig.ThinkingConfig = &ThinkingConfig{
			ThinkingBudget: textRequest.ThinkingBudget,
		}
	}
	geminiRequest := ChatRequest{
		Contents:         make([]ChatContent, 0),
		GenerationConfig: generationConfig,
	}
	if textRequest.ResponseFormat != nil {
		if mimeType, ok := mimeTypeMap[textRequest.ResponseFormat.Type]; ok {
			geminiRequest.GenerationConfig.ResponseMimeType = mimeType
		}
		if textRequest.ResponseFormat.Schema != nil {
			geminiRequest.GenerationConfig.ResponseSchema = textRequest.ResponseFormat.Schema
			geminiRequest.GenerationConfig.ResponseMimeType = mimeTypeMap["json_object"]
		}
	}
	userContent := ""
	var parts []Part
	for _, message := range textRequest.Messages {
		msg := message.StringContent()
		if msg != "" {
			userContent = userContent + msg + "\r\n"
		}
		openaiContent := message.ParseContent()
		imageNum := 0
		for _, part := range openaiContent {
			if part.Type == relaymodel.ContentTypeText {
				msg = part.Text
				if msg != "" {
					userContent = userContent + msg + "\r\n"
				}
			} else if part.Type == relaymodel.ContentTypeImageURL {
				imageNum += 1
				if imageNum > VisionMaxImageNum {
					continue
				}
				mimeType := ""
				fileData := ""
				var err error
				ok, err := media.IsMediaUrl(part.ImageURL.Url)
				if err != nil {
					return nil, err
				}
				if ok {
					mimeType, fileData, err = FileHandler(c, part.ImageURL.Url, part.ImageURL.Url, "", "")
					if err != nil {
						return nil, err
					}
					parts = append(parts, Part{
						FileData: &FileData{
							MimeType: mimeType,
							Uri:      fileData,
						},
					})
				} else {
					mimeType, fileData, err = image.GetImageFromUrl(part.ImageURL.Url, false)
					if err != nil {
						return nil, err
					}
					//这里走原始的图片逻辑, 是取b64传给gemini, 保存起来是用于有些prompt请求一直会429, 外层判断429后会改为上面那种方式
					c.Set("gemini-img-url", part.ImageURL.Url)
					parts = append(parts, Part{
						InlineData: &InlineData{
							MimeType: mimeType,
							Data:     fileData,
						},
					})
				}
			}
		}
	}

	//内容转入文本文件, 并向gemini提问: 请打开我上传的txt文件, 回答文件内容的问题, 注意参考我上传的所有文件来作答
	// 判断文件夹是否存在
	dirPath := "/mnt/tpm_file"
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// 文件夹不存在，创建新的文件夹
		err := os.MkdirAll(dirPath, 0755) // 0755 是文件夹的权限设置
		if err != nil {
			logger.SysLogf("ChangeChat2TxtRequest - Error: MkdirAll temporary file: =>create dic failed: %s", err)
			return nil, err
		}
	} else if err != nil {
		// 其他错误
		logger.SysLogf("ChangeChat2TxtRequest - Error: MkdirAll temporary file: =>create dic error failed : %s", err)
		return nil, err
	}
	tmp_name := fmt.Sprintf("%s/tmpfile_%s.txt", dirPath, random.GetRandomNumberString(16))
	err := os.WriteFile(tmp_name, []byte(userContent), 0755)
	if err != nil {
		logger.SysLogf("ChangeChat2TxtRequest - Error:os.WriteFile : %s", err)
		return nil, err
	}
	mimeType, fileData, err := FileHandler(c, random.StrToMd5(userContent), "", "text/plain", tmp_name)
	if err != nil {
		return nil, err
	}
	parts = append(parts, Part{
		FileData: &FileData{
			MimeType: mimeType,
			Uri:      fileData,
		},
	})

	parts = append(parts, Part{
		Text: "Please open the txt file I uploaded and answer the questions in the file. Please refer to all the files I uploaded to answer the questions.",
	})
	content := ChatContent{
		Role:  "User",
		Parts: parts,
	}

	geminiRequest.Contents = append(geminiRequest.Contents, content)
	//删除文件
	if err := os.Remove(tmp_name); err != nil {
		logger.SysLogf("ChangeChat2TxtRequest - Error:os.Remove : %s", err)
	}

	return &geminiRequest, nil
}
