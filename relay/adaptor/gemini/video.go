package gemini

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func ConvertVideoRequest(request relaymodel.VideoRequest) (*VideoRequest, error) {
	parameters := VideoParameters{
		PersonGeneration: "allow_adult",
	}
	ins := VideoInstances{
		Prompt: request.Prompt,
	}
	if request.Size != "" {
		parameters.AspectRatio = request.Size
	}
	if request.Image != "" {
		ins.Image = VideoImage{
			BytesBase64Encoded: request.Image,
		}
	}

	videoRequest := VideoRequest{
		Instances:  ins,
		Parameters: parameters,
	}

	return &videoRequest, nil
}
func VideoHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
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
