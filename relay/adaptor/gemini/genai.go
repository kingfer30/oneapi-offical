package gemini

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/generative-ai-go/genai"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/media"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func DoChatByGenai(c *gin.Context, meta *meta.Meta) (*relaymodel.Usage, string, *relaymodel.ErrorWithStatusCode) {
	textRequest := meta.TextRequest
	// //初始化gemini客户端
	// var httpClient *http.Client
	// if config.HttpProxy == "" {
	// 	httpClient = commonClient.HTTPClient
	// } else {
	// 	logger.SysLogf("使用代理: %s", config.HttpProxy)
	// 	urlObj, err := url.Parse("http://199.119.138.75:1080")
	// 	urlObj.User = url.UserPassword("xiaoguo", "Ji6dft4Cqd9l_eX6h3")
	// 	logger.SysLogf("%v", urlObj)
	// 	if err != nil {
	// 		return nil, "", openai.ErrorWrapper(err, "http_proxy_url.parse_failed", http.StatusInternalServerError)
	// 	}
	// 	httpClient = &http.Client{
	// 		Transport: &http.Transport{
	// 			Proxy:           http.ProxyURL(urlObj),
	// 			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // CAUTION: Insecure!
	// 		},
	// 	}
	// }
	client, err := genai.NewClient(c, option.WithAPIKey(meta.APIKey))

	if err != nil {
		return nil, "", openai.ErrorWrapper(err, "init_genai_error", http.StatusInternalServerError)
	}
	defer client.Close()

	model := client.GenerativeModel(meta.OriginModelName)

	model.SafetySettings = []*genai.SafetySetting{
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryHarassment,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryHateSpeech,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategorySexuallyExplicit,
			Threshold: genai.HarmBlockNone,
		},
	}
	if textRequest.Temperature != nil {
		model.SetTemperature(float32(*textRequest.Temperature))
	}
	if textRequest.TopP != nil {
		model.SetTopP(float32(*textRequest.TopP))
	}
	if textRequest.Stop != nil {
		seq, ok := textRequest.Stop.([]string)
		if !ok {
			seq, ok := textRequest.Stop.(string)
			if ok {
				model.StopSequences = []string{
					seq,
				}
			}
		} else {
			model.StopSequences = seq
		}
	}
	if textRequest.MaxTokens != 0 {
		model.SetMaxOutputTokens(int32(textRequest.MaxTokens))
	}
	if textRequest.ResponseFormat != nil {
		if mimeType, ok := mimeTypeMap[textRequest.ResponseFormat.Type]; ok {
			model.ResponseMIMEType = mimeType
		}
		if textRequest.ResponseFormat.Schema != nil {
			model.ResponseSchema = &genai.Schema{
				Type:        genai.TypeArray,
				Format:      textRequest.ResponseFormat.Schema.Format,
				Description: textRequest.ResponseFormat.Schema.Description,
				Nullable:    textRequest.ResponseFormat.Schema.Nullable,
				Enum:        textRequest.ResponseFormat.Schema.Enum,
				Required:    textRequest.ResponseFormat.Schema.Required,
			}
			model.ResponseMIMEType = mimeTypeMap["json_object"]
		}
	}
	var tools []*genai.Tool
	if textRequest.Tools != nil {
		for _, tool := range textRequest.Tools {
			params, ok := tool.Function.Parameters.(*genai.Schema)
			if ok {
				tools = append(tools, &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{
					{
						Name:        tool.Function.Name,
						Description: tool.Function.Description,
						Parameters:  params,
					},
				}})
			} else {
				tools = append(tools, &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{
					{
						Name:        tool.Function.Name,
						Description: tool.Function.Description,
					},
				}})
			}
		}
	}
	model.Tools = tools
	var usage *relaymodel.Usage

	if len(textRequest.Messages) == 1 {
		parts, err := generateParts(c, textRequest.Messages[0])
		if err != nil {
			return nil, "", openai.ErrorWrapper(err, "generate_parts_error", http.StatusInternalServerError)
		}
		var jerr *relaymodel.ErrorWithStatusCode
		var fullText string
		var usage *relaymodel.Usage
		if meta.IsStream {
			iter := model.GenerateContentStream(c, parts...)
			jerr, fullText, usage = handleGenaiStream(c, meta, iter)
			if jerr != nil {
				return nil, "", jerr
			}
		} else {
			resp, err := model.GenerateContent(c, parts...)
			if err != nil {
				return nil, "", handleGenaiError(err, true, meta)
			}
			jerr, fullText, usage = handleGenaiUnStream(c, meta, resp)
			if jerr != nil {
				return nil, "", jerr
			}
		}
		return usage, fullText, nil
	}

	nextRole := "user"
	var content []*genai.Content
	for k, message := range textRequest.Messages {
		msgParts, err := generateParts(c, message)
		if err != nil {
			return nil, "", openai.ErrorWrapper(err, "generate_parts_error", http.StatusInternalServerError)
		}

		// there's cannt start with assistant role
		if message.Role == "assistant" && k == 0 {
			content = append(content, &genai.Content{
				Parts: []genai.Part{genai.Text("Hello")},
				Role:  "user",
			})
			nextRole = "model"
		}

		// there's no assistant role in gemini and API shall vomit if Role is not user or model
		if message.Role == "assistant" || message.Role == "system" {
			message.Role = "model"
		}
		// per chat need gemini-user
		if (message.Role == "model" || message.Role == "system" || message.Role == "assistant") && nextRole == "user" {
			content = append(content, &genai.Content{
				Parts: []genai.Part{genai.Text("Hello")},
				Role:  "user",
			})
			nextRole = "model"
		} else if message.Role == "user" && nextRole == "model" {
			content = append(content, &genai.Content{
				Parts: []genai.Part{genai.Text("Hi")},
				Role:  "model",
			})
			nextRole = "user"
		}
		if message.Role == "user" {
			nextRole = "model"
		} else {
			nextRole = "user"
		}

		content = append(content, &genai.Content{
			Parts: msgParts,
			Role:  message.Role,
		})
	}
	if nextRole == "user" {
		content = append(content, &genai.Content{
			Parts: []genai.Part{genai.Text("Ask my question")},
			Role:  "user",
		})
	}
	//这里理应为多条消息, 如果最后只有一条, 应该报错
	if len(content) == 1 {
		return nil, "", openai.ErrorWrapper(err, "error_genai_content_length", http.StatusInternalServerError)
	}
	//最后一条为发送内容, 其他为历史
	last := content[len(content)-1]
	content = content[:len(content)-1]
	cs := model.StartChat()
	cs.History = content
	var jerr *relaymodel.ErrorWithStatusCode
	var fullText string
	if !meta.IsStream {
		resp, err := cs.SendMessage(c, last.Parts...)
		if err != nil {
			// 处理错误
			return nil, "", handleGenaiError(err, false, meta)
		}
		jerr, fullText, usage = handleGenaiUnStream(c, meta, resp)
		if jerr != nil {
			return nil, "", jerr
		}
	} else {
		iter := cs.SendMessageStream(c, last.Parts...)
		jerr, fullText, usage = handleGenaiStream(c, meta, iter)
		if jerr != nil {
			return nil, "", jerr
		}
	}
	return usage, fullText, nil
}

func generateParts(c *gin.Context, message relaymodel.Message) ([]genai.Part, error) {
	var parts []genai.Part
	openaiContent := message.ParseContent()
	imageNum := 0
	for _, part := range openaiContent {
		if part.Type == relaymodel.ContentTypeText {
			msg := part.Text
			if msg == "" {
				msg = "Hi"
			}
			parts = append(parts, genai.Text(msg))
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
				parts = append(parts, genai.FileData{
					URI:      fileData,
					MIMEType: mimeType,
				})
			} else {
				var reg = regexp.MustCompile(`data:image/([^;]+);base64,`)
				mimeType, fileData, err = image.GetImageFromUrl(part.ImageURL.Url, true)
				if err != nil {
					return nil, err
				}
				format := strings.TrimPrefix(mimeType, "image/")
				dataBytes, err := base64.StdEncoding.DecodeString(reg.ReplaceAllString(fileData, ""))
				if err != nil {
					return nil, err
				}
				parts = append(parts, genai.ImageData(format, dataBytes))
			}
		}
	}
	return parts, nil
}
func handleGenaiStream(c *gin.Context, meta *meta.Meta, iter *genai.GenerateContentResponseIterator) (*relaymodel.ErrorWithStatusCode, string, *relaymodel.Usage) {
	msgCount := 0
	fullText := ""
	var usage *relaymodel.Usage
	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			if msgCount == 0 {
				return &relaymodel.ErrorWithStatusCode{
					Error: relaymodel.Error{
						Message: "No candidates returned. Check your parameter of max_tokens",
						Type:    "server_error",
						Param:   "",
						Code:    http.StatusInternalServerError,
					},
					StatusCode: http.StatusInternalServerError,
				}, "", nil
			}
			break
		}
		if err != nil {
			// 处理错误
			return handleGenaiError(err, false, meta), "", nil
		}
		var choice openai.ChatCompletionsStreamResponseChoice
		var response openai.ChatCompletionsStreamResponse
		response.Id = fmt.Sprintf("chatcmpl-%s", random.GetUUID())
		response.Created = helper.GetTimestamp()
		response.Object = "chat.completion.chunk"
		response.Model = meta.ActualModelName
		currentText := ""
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for index, part := range cand.Content.Parts {
					text := fmt.Sprint(part)
					if meta.IncludeThinking {
						//thinkingStart:\n%s\nthinkingEnd\n%s
						if index >= 1 {
							currentText := "thinkingStart\n" + text
							currentText += "\nthinkingEnd\n"
							currentText += text
							meta.EndThinking = true
						} else {
							if meta.EndThinking {
								currentText = text
							} else {
								currentText = fmt.Sprintf("thinkingStart\n%s\nthinkingEnd\n", text)
							}
						}
					} else {
						if index >= 1 {
							currentText += "\n"
							currentText += text
						} else {
							currentText = text
						}
					}
				}
			}
		}
		choice.Delta.Content = currentText
		response.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}
		usage = &relaymodel.Usage{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
		}
		response.Usage = usage
		err = render.ObjectData(c, response)
		if err != nil {
			logger.SysErrorf("render.ObjectData error:\n%s", err.Error())
		}
		fullText += currentText
		msgCount++
	}
	render.Done(c)
	return nil, fullText, usage
}
func handleGenaiUnStream(c *gin.Context, meta *meta.Meta, resp *genai.GenerateContentResponse) (*relaymodel.ErrorWithStatusCode, string, *relaymodel.Usage) {
	if len(resp.Candidates) == 0 {
		return &relaymodel.ErrorWithStatusCode{
			Error: relaymodel.Error{
				Message: "No candidates returned. Check your parameter of max_tokens",
				Type:    "server_error",
				Param:   "",
				Code:    500,
			},
			StatusCode: 400,
		}, "", nil
	}
	fullTextResponse := openai.TextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", random.GetUUID()),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: make([]openai.TextResponseChoice, 0, len(resp.Candidates)),
		Model:   meta.ActualModelName,
	}
	fullText := ""
	var toolCalls []relaymodel.Tool
	for i, candidate := range resp.Candidates {
		choice := openai.TextResponseChoice{
			Index: i,
			Message: relaymodel.Message{
				Role:    "assistant",
				Content: "",
			},
			FinishReason: constant.StopFinishReason,
		}
		if candidate.Content != nil {
			part := candidate.Content.Parts[0]
			funcall, ok := part.(genai.FunctionCall)
			if ok {
				toolCall := relaymodel.Tool{
					Id:   fmt.Sprintf("call_%s", random.GetUUID()),
					Type: "function",
					Function: relaymodel.Function{
						Arguments: funcall.Args,
						Name:      funcall.Name,
					},
				}
				toolCalls = append(toolCalls, toolCall)
				choice.Message.ToolCalls = toolCalls
			} else {
				for i, item := range candidate.Content.Parts {
					currentText := fmt.Sprint(item)
					if meta.IncludeThinking {
						if i == 0 {
							fullText = fmt.Sprintf("thinkingStart:\n%s", currentText)
						} else {
							fullText += fmt.Sprintf("%s\nthinkingEnd\n%s", choice.Message.Content, currentText)
						}
					} else {
						if i == 0 {
							fullText = currentText
						} else {
							fullText += "\n"
							fullText += currentText
						}
					}
				}
				choice.Message.Content = fullText
			}
		} else {
			choice.Message.Content = ""
			choice.FinishReason = candidate.FinishReason.String()
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	var usage relaymodel.Usage
	if resp.UsageMetadata != nil {
		usage = relaymodel.Usage{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
		}
	} else {
		completionTokens := openai.CountTokenText(fullText, meta.ActualModelName)
		usage = relaymodel.Usage{
			PromptTokens:     meta.PromptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      meta.PromptTokens + completionTokens,
		}
	}

	fullTextResponse.Usage = usage
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), "", nil
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, fullText, &usage
}
func handleGenaiError(err error, canAs bool, meta *meta.Meta) *relaymodel.ErrorWithStatusCode {
	// 处理错误
	apiError, ok := err.(*apierror.APIError)
	if ok {
		statusCode := apiError.HTTPCode()
		errorMessage := apiError.GRPCStatus().Message()
		if errorMessage == "" {
			errorMessage = "unknow error in apierror"
		}
		if statusCode == http.StatusTooManyRequests {
			errorMessage = "Resource has been exhausted (e.g. check quota)."
			if !canAs {
				//这里需要另起一个协程检查该渠道是否存在问题 是则禁用, 因为多文本使用stream, 无法识别错误(垃圾)
			}
		}
		return openai.ErrorWrapper(fmt.Errorf("%s", errorMessage), "agg_genai_error", statusCode)
	}
	gerr, ok := err.(*googleapi.Error)
	if ok {
		statusCode := gerr.Code
		errorMessage := gerr.Message
		if errorMessage == "" {
			errorMessage = "unknow error in googleapi"
		}
		if statusCode == http.StatusTooManyRequests {
			errorMessage = "Resource has been exhausted (e.g. check quota)."
		}
		return openai.ErrorWrapper(fmt.Errorf("%s", errorMessage), "agg_genai_error", statusCode)
	} else {
		logger.SysErrorf("faild to as googleapi error: %s\n", err.Error())
		return openai.ErrorWrapper(err, "agg_genai_error", http.StatusInternalServerError)
	}
}

func GetTokens(c *gin.Context, textRequest *relaymodel.GeneralOpenAIRequest, apiKey string) (int, error) {
	client, err := genai.NewClient(c, option.WithAPIKey(apiKey))

	if err != nil {
		return 0, err
	}
	defer client.Close()

	model := client.GenerativeModel(textRequest.Model)

	model.SafetySettings = []*genai.SafetySetting{
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryHarassment,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryHateSpeech,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategorySexuallyExplicit,
			Threshold: genai.HarmBlockNone,
		},
	}
	if textRequest.Temperature != nil {
		model.SetTemperature(float32(*textRequest.Temperature))
	}
	if textRequest.TopP != nil {
		model.SetTopP(float32(*textRequest.TopP))
	}
	if textRequest.Stop != nil {
		seq, ok := textRequest.Stop.([]string)
		if !ok {
			seq, ok := textRequest.Stop.(string)
			if ok {
				model.StopSequences = []string{
					seq,
				}
			}
		} else {
			model.StopSequences = seq
		}
	}
	if textRequest.MaxTokens != 0 {
		model.SetMaxOutputTokens(int32(textRequest.MaxTokens))
	}
	if textRequest.ResponseFormat != nil {
		if mimeType, ok := mimeTypeMap[textRequest.ResponseFormat.Type]; ok {
			model.ResponseMIMEType = mimeType
		}
		if textRequest.ResponseFormat.Schema != nil {
			model.ResponseSchema = &genai.Schema{
				Type:        genai.TypeArray,
				Format:      textRequest.ResponseFormat.Schema.Format,
				Description: textRequest.ResponseFormat.Schema.Description,
				Nullable:    textRequest.ResponseFormat.Schema.Nullable,
				Enum:        textRequest.ResponseFormat.Schema.Enum,
				Required:    textRequest.ResponseFormat.Schema.Required,
			}
			model.ResponseMIMEType = mimeTypeMap["json_object"]
		}
	}
	var tools []*genai.Tool
	if textRequest.Tools != nil {
		for _, tool := range textRequest.Tools {
			params, ok := tool.Function.Parameters.(*genai.Schema)
			if ok {
				tools = append(tools, &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{
					{
						Name:        tool.Function.Name,
						Description: tool.Function.Description,
						Parameters:  params,
					},
				}})
			} else {
				tools = append(tools, &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{
					{
						Name:        tool.Function.Name,
						Description: tool.Function.Description,
					},
				}})
			}
		}
	}
	model.Tools = tools

	var tokenResult *genai.CountTokensResponse
	var tokenErr error

	if len(textRequest.Messages) == 1 {
		parts, err := generateParts(c, textRequest.Messages[0])
		if err != nil {
			return 0, err
		}
		tokenResult, tokenErr = model.CountTokens(c, parts...)
	} else {
		nextRole := "user"
		var content []*genai.Content
		for k, message := range textRequest.Messages {
			msgParts, err := generateParts(c, message)
			if err != nil {
				return 0, err
			}

			// there's cannt start with assistant role
			if message.Role == "assistant" && k == 0 {
				content = append(content, &genai.Content{
					Parts: []genai.Part{genai.Text("Hello")},
					Role:  "user",
				})
				nextRole = "model"
			}

			// there's no assistant role in gemini and API shall vomit if Role is not user or model
			if message.Role == "assistant" || message.Role == "system" {
				message.Role = "model"
			}
			// per chat need gemini-user
			if (message.Role == "model" || message.Role == "system" || message.Role == "assistant") && nextRole == "user" {
				content = append(content, &genai.Content{
					Parts: []genai.Part{genai.Text("Hello")},
					Role:  "user",
				})
				nextRole = "model"
			} else if message.Role == "user" && nextRole == "model" {
				content = append(content, &genai.Content{
					Parts: []genai.Part{genai.Text("Hi")},
					Role:  "model",
				})
				nextRole = "user"
			}
			if message.Role == "user" {
				nextRole = "model"
			} else {
				nextRole = "user"
			}

			content = append(content, &genai.Content{
				Parts: msgParts,
				Role:  message.Role,
			})
		}
		if nextRole == "user" {
			content = append(content, &genai.Content{
				Parts: []genai.Part{genai.Text("Ask my question")},
				Role:  "user",
			})
		}
		//这里理应为多条消息, 如果最后只有一条, 应该报错
		if len(content) == 1 {
			return 0, fmt.Errorf("error_genai_content_length")
		}
		//最后一条为发送内容, 其他为历史
		last := content[len(content)-1]
		content = content[:len(content)-1]
		cs := model.StartChat()
		cs.History = content
		tokenResult, tokenErr = model.CountTokens(c, last.Parts...)
	}
	if tokenErr != nil {
		return 0, tokenErr
	}
	return int(tokenResult.TotalTokens), nil
}
