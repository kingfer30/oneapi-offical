package gemini

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/media"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/model"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

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
	"github.com/google/generative-ai-go/genai"
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
	geminiRequest := ChatRequest{
		Contents: make([]ChatContent, 0, len(textRequest.Messages)),
		GenerationConfig: ChatGenerationConfig{
			Temperature:     textRequest.Temperature,
			TopP:            textRequest.TopP,
			MaxOutputTokens: textRequest.MaxTokens,
			StopSequences:   textRequest.Stop,
			// ThinkingConfig: ThinkingConfig{
			// 	IncludeThoughts: true,
			// },
		},
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
		functions := make([]relaymodel.Function, 0, len(textRequest.Tools))
		for _, tool := range textRequest.Tools {
			functions = append(functions, tool.Function)
		}
		geminiRequest.Tools = []ChatTools{
			{
				FunctionDeclarations: functions,
			},
		}
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
			b, jerr := json.Marshal(textRequest)
			if jerr == nil {
				logger.SysLog(fmt.Sprintf("Gemini-Text-Empty: %s", string(b)))
			}
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
					mimeType, fileData, err = FileHandler(c, part.ImageURL.Url, "", "")
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
					if config.GeminiUploadImageEnabled {
						mimeType, fileName, err := image.GetImageFromUrl(part.ImageURL.Url, true)
						if err != nil {
							return nil, err
						}
						mimeType, fileData, err = FileHandler(c, random.StrToMd5(part.ImageURL.Url), mimeType, fileName)
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

	return &geminiRequest, nil
}

func ConvertEmbeddingRequest(request relaymodel.GeneralOpenAIRequest) *BatchEmbeddingRequest {
	inputs := request.ParseInput()
	requests := make([]EmbeddingRequest, len(inputs))
	model := fmt.Sprintf("models/%s", request.Model)

	for i, input := range inputs {
		requests[i] = EmbeddingRequest{
			Model: model,
			Content: ChatContent{
				Parts: []Part{
					{
						Text: input,
					},
				},
			},
		}
	}

	return &BatchEmbeddingRequest{
		Requests: requests,
	}
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

func embeddingResponseGemini2OpenAI(response *EmbeddingResponse) *openai.EmbeddingResponse {
	openAIEmbeddingResponse := openai.EmbeddingResponse{
		Object: "list",
		Data:   make([]openai.EmbeddingResponseItem, 0, len(response.Embeddings)),
		Model:  "gemini-embedding",
		Usage:  relaymodel.Usage{TotalTokens: 0},
	}
	for _, item := range response.Embeddings {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, openai.EmbeddingResponseItem{
			Object:    `embedding`,
			Index:     0,
			Embedding: item.Values,
		})
	}
	return &openAIEmbeddingResponse
}

func StreamHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.ErrorWithStatusCode, string, *relaymodel.Usage) {
	responseText := ""
	var usage *relaymodel.Usage
	scanner := bufio.NewScanner(resp.Body)
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

		response := streamResponseGeminiChat2OpenAI(&geminiResponse, meta)
		if response == nil {
			continue
		}

		responseText += response.Choices[0].Delta.StringContent()
		usage = &relaymodel.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
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
	fullTextResponse := responseGeminiChat2OpenAI(&geminiResponse, meta)
	fullTextResponse.Model = meta.ActualModelName
	var usage relaymodel.Usage
	if geminiResponse.UsageMetadata != nil {
		usage = relaymodel.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
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

func EmbeddingHandler(c *gin.Context, resp *http.Response) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	var geminiEmbeddingResponse EmbeddingResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &geminiEmbeddingResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if geminiEmbeddingResponse.Error != nil {
		return &relaymodel.ErrorWithStatusCode{
			Error: relaymodel.Error{
				Message: geminiEmbeddingResponse.Error.Message,
				Type:    "gemini_error",
				Param:   "",
				Code:    geminiEmbeddingResponse.Error.Code,
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := embeddingResponseGemini2OpenAI(&geminiEmbeddingResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

// 文件上传处理
func FileHandler(c *gin.Context, url string, contentType string, fileName string) (string, string, error) {
	meta := meta.GetByContext(c)
	//判断文件是否已经存在
	fileOld, err := model.GetFile(url)
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
		Url:         url,
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
				mimeType, fileData, err = FileHandler(c, part.ImageURL.Url, "", "")
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
					if strings.Contains(meta.ActualModelName, "think") && meta.Thinking {
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
					if strings.Contains(meta.ActualModelName, "think") && meta.Thinking {
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
