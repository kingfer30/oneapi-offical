package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func ConvertImageRequest(request relaymodel.ImageRequest) (*ImageRequest, error) {
	var contents []ChatContent
	if request.Image != "" {
		//图片编辑
		mimeType, fileData, err := image.GetImageFromUrl(request.Image, false)
		if err != nil {
			return nil, err
		}
		contents = append(contents, ChatContent{
			Role: "user",
			Parts: []Part{
				{
					InlineData: &InlineData{
						MimeType: mimeType,
						Data:     fileData,
					},
				},
				{
					Text: request.Prompt,
				},
			},
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
func ImageHandler(c *gin.Context, meta *meta.Meta, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
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
	log.Print(string(responseBody))
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

	var usage relaymodel.Usage
	if geminiResponse.UsageMetadata != nil {
		usage = relaymodel.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
		}
	}
	var jsonResponse []byte
	if meta.Image2Chat {
		//请求画图模型, 但以聊天接口访问的, 按聊天接口的格式返回
		fullResponse, jerr := responseGemini2OpenAIChat(&geminiResponse, responseFormat)
		if jerr != nil {
			return jerr, nil
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

func responseGemini2OpenAIChat(response *ChatResponse, respType string) (*openai.TextResponse, *relaymodel.ErrorWithStatusCode) {
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
			text := ""
			for _, item := range candidate.Content.Parts {
				text = ""
				if item.InlineData != nil {
					if respType == "b64_json" {
						text = fmt.Sprintf(`%s\n<img src="data:%s;base64,%s">`, text, item.InlineData.MimeType, item.InlineData.Data)
					} else {
						//undo url格式需要上传图床
						text = fmt.Sprintf(`%s\n<img src="data:%s;base64,%s">`, text, item.InlineData.MimeType, item.InlineData.Data)
						// text = fmt.Sprintf(`%s\n<img src="%s">`, text, item.InlineData.Data)
					}
				} else {
					text = fmt.Sprintf("%s\n%s", text, item.Text)
				}
			}
			choice.Message.Content = text
		} else {
			choice.Message.Content = ""
			choice.FinishReason = candidate.FinishReason
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}

	return &fullTextResponse, nil
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
						//undo url格式需要上传图床
						imgList = append(imgList, ImageData{
							Url: item.InlineData.Data,
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
