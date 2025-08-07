package openrouter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/render"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

const (
	dataPrefix       = "data: "
	done             = "[DONE]"
	dataPrefixLength = len(dataPrefix)
)

func ConvertRequest(c *gin.Context, request model.GeneralOpenAIRequest) (*model.GeneralOpenAIRequest, error) {
	provider := ""
	//更改为openroute实际模型名称
	if ModelMappingList[request.Model] != "" {
		request.Model = ModelMappingList[request.Model]
		parts := strings.Split(request.Model, "/")
		provider = parts[0]
		request.Provider = map[string][]string{
			"order": {
				provider,
			},
		}
	}
	c.Set("provider", provider)
	if provider == "anthropic" {
		//claude模式下的参数对接
		if request.Thinking != nil && request.Thinking.Type == "enabled" {

			if request.Thinking.ThinkingBudget == 0 {
				request.Thinking.ThinkingBudget = 2000
			}

			request.Reasoning = map[string]any{
				"max_tokens": request.Thinking.ThinkingBudget,
			}
			if request.Thinking.IncludeThinking != false {
				c.Set("include_think", true)
				if request.Thinking != nil && request.Thinking.Type == "enabled" && request.Thinking.ThinkingTag != nil {
					c.Set("thinking_tag_start", request.Thinking.ThinkingTag.Start)
					c.Set("thinking_tag_end", request.Thinking.ThinkingTag.End)
					if request.Thinking.ThinkingTag.BlockTag {
						c.Set("thinking_tag_block", true)
					}
				} else {
					c.Set("thinking_tag_start", "<think>")
					c.Set("thinking_tag_end", "</think>")
				}
			}
		}
	}
	return &request, nil
}

func StreamHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.ErrorWithStatusCode, string, *model.Usage) {
	scanner := bufio.NewScanner(resp.Body)
	maxBufferSize := 1024 * 1024 * 6                  // 6MB
	scanner.Buffer(make([]byte, 4096), maxBufferSize) // 初始 4KB，最大扩展到 1MB
	scanner.Split(bufio.ScanLines)
	var usage *model.Usage

	common.SetEventStreamHeaders(c)
	var responseText string

	debugText := ""
	for scanner.Scan() {
		adaptor.StartingStream(c, meta)
		data := scanner.Text()
		debugText += data + "----\n"
		if config.DebugEnabled {
			logger.SysLogf("Body: %s", data)
		}
		if len(data) < dataPrefixLength || data[:dataPrefixLength] != dataPrefix { // ignore blank line or wrong format
			continue
		}

		data = strings.TrimPrefix(data, "data:")
		data = strings.TrimSpace(data)
		if data == done {
			continue
		}

		var streamResponse openai.ChatCompletionsStreamResponse
		err := json.Unmarshal([]byte(data), &streamResponse)
		if err != nil {
			logger.SysError("error unmarshalling stream response: " + err.Error())
			render.StringData(c, data) // if error happened, pass the data to client
			continue                   // just ignore the error
		}
		if len(streamResponse.Choices) == 0 {
			// but for empty choice and no usage, we should not pass it to client, this is for azure
			continue // just ignore empty choice
		}
		tempText, response := streamResponseChat2OpenAI(&streamResponse, meta)
		if tempText != "" {
			responseText += tempText
		}
		if streamResponse.Usage != nil {
			usage = streamResponse.Usage
			response.Usage = streamResponse.Usage
			if usage.CompletionTokens == 0 {
				logger.SysLogf("CompletionTokens is zero: %s", debugText)
			}
		}

		render.ObjectData(c, response)
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

func streamResponseChat2OpenAI(resp *openai.ChatCompletionsStreamResponse, meta *meta.Meta) (string, openai.ChatCompletionsStreamResponse) {
	var choice openai.ChatCompletionsStreamResponseChoice
	var responseText string
	var reasoningContent string
	for _, item := range resp.Choices {
		if item.Delta.Reasoning != "" {
			reasoningContent = fmt.Sprintf("%s%s", reasoningContent, item.Delta.Reasoning)
			if meta.IncludeThinking {
				if meta.EnableBlockTag {
					reasoningContent = fmt.Sprintf("%s%s%s", meta.ThinkingTagStart, reasoningContent, meta.ThinkingTagEnd)
				} else {
					if !meta.StartThinking {
						reasoningContent = fmt.Sprintf("%s%s", meta.ThinkingTagStart, reasoningContent)
						meta.StartThinking = true
					} else {
						reasoningContent = reasoningContent
					}
				}
			}
		}
		if item.Delta.Content != "" {
			if meta.StartThinking && !meta.EndThinking {
				meta.EndThinking = true
				responseText = fmt.Sprintf("%s%s", meta.ThinkingTagEnd, item.Delta.Content)
			}
			responseText = fmt.Sprintf("%s%s", responseText, item.Delta.Content)
		}
		if meta.IncludeThinking {
			if reasoningContent != "" {
				responseText = reasoningContent
				reasoningContent = ""
			} else {
				responseText = responseText
			}
		}
	}
	choice.Delta.Content = responseText
	choice.Delta.ReasoningContent = &reasoningContent
	var response openai.ChatCompletionsStreamResponse
	response.Id = fmt.Sprintf("chatcmpl-%s", random.GetUUID())
	response.Created = helper.GetTimestamp()
	response.Object = "chat.completion.chunk"
	response.Model = meta.OriginModelName
	response.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}
	return responseText, response
}

func Handler(c *gin.Context, resp *http.Response, promptTokens int, meta *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
	var textResponse ChatResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &textResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	if config.DebugEnabled {
		logger.SysLogf("body: %s", string(responseBody))
	}
	if textResponse.Error.Type != "" {
		return &model.ErrorWithStatusCode{
			Error:      textResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	fullTextResponse := responseChat2OpenAI(textResponse, meta)

	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, &textResponse.Usage
}

func responseChat2OpenAI(textResponse ChatResponse, meta *meta.Meta) openai.TextResponse {
	var responseText string
	var stopReason string
	ReasoningContent := ""
	if len(textResponse.Choices) > 0 {
		for _, item := range textResponse.Choices {
			if item.FinishReason != "" {
				stopReason = item.FinishReason
			}
			if item.Reasoning != "" {
				ReasoningContent = item.Reasoning
				if meta.IncludeThinking {
					responseText = fmt.Sprintf("%s%s%s", meta.ThinkingTagStart, item.Reasoning, meta.ThinkingTagEnd)
				}
			}
			if responseText != "" {
				responseText += "\n"
			}
			responseText = fmt.Sprintf("%s%s", responseText, item.Content)
		}
	}
	msg := model.Message{
		Role:    "assistant",
		Content: responseText,
		Name:    nil,
	}
	if ReasoningContent != "" && !meta.IncludeThinking {
		msg.ReasoningContent = &ReasoningContent
	}
	choice := openai.TextResponseChoice{
		Index:        0,
		Message:      msg,
		FinishReason: stopReason,
	}

	fullTextResponse := openai.TextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", random.GetUUID()),
		Object:  "chat.completion",
		Model:   meta.OriginModelName,
		Created: helper.GetTimestamp(),
		Choices: []openai.TextResponseChoice{choice},
		Usage:   textResponse.Usage,
	}
	return fullTextResponse
}
