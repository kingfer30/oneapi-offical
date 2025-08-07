package openrouter

import "github.com/songquanpeng/one-api/relay/model"

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type ChatResponse struct {
	Choices     []TextResponseChoice `json:"choices"`
	model.Usage `json:"usage"`
	Error       model.Error `json:"error"`
}

type TextResponseChoice struct {
	Index         int `json:"index"`
	model.Message `json:"message"`
	FinishReason  string `json:"finish_reason"`
}

// type StreamResponse struct {
// 	Type         string    `json:"type"`
// 	Message      *Response `json:"message"`
// 	Index        int       `json:"index"`
// 	ContentBlock *Content  `json:"content_block"`
// 	Delta        *Delta    `json:"delta"`
// 	Usage        *Usage    `json:"usage"`
// }
