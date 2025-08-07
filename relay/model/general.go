package model

type ResponseFormat struct {
	Type   string  `json:"type,omitempty"`
	Schema *Schema `json:"schema,omitempty"`
}

type JSONSchema struct {
	Description string `json:"description,omitempty"`
	Name        string `json:"name"`
	Schema      Schema `json:"schema,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`
}
type Schema struct {
	Type        int32    `json:"type"`
	Format      string   `json:"format,omitempty"`
	Description string   `json:"description,omitempty"`
	Nullable    bool     `json:"nullable,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	MaxItems    int64    `json:"maxItems,omitempty"`
	MinItems    int64    `json:"minItems,omitempty"`
	Required    []string `json:"required,omitempty"`
}

type Audio struct {
	Voice  string `json:"voice,omitempty"`
	Format string `json:"format,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type Thinking struct {
	Type            string       `json:"type"`
	ThinkingBudget  int          `json:"thinking_budget,omitempty"`
	IncludeThinking bool         `json:"include_thinking,omitempty"`
	ThinkingTag     *ThinkingTag `json:"thinking_tag,omitempty"`
}
type ThinkingTag struct {
	BlockTag bool   `json:"block_tag,omitempty"`
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
}

type GeneralOpenAIRequest struct {
	// https://platform.openai.com/docs/api-reference/chat/create
	Messages            []Message       `json:"messages,omitempty"`
	Model               string          `json:"model,omitempty"`
	Store               *bool           `json:"store,omitempty"`
	Metadata            any             `json:"metadata,omitempty"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	LogitBias           any             `json:"logit_bias,omitempty"`
	Logprobs            *bool           `json:"logprobs,omitempty"`
	TopLogprobs         *int            `json:"top_logprobs,omitempty"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	N                   int             `json:"n,omitempty"`
	Modalities          []string        `json:"modalities,omitempty"`
	Prediction          any             `json:"prediction,omitempty"`
	Audio               *Audio          `json:"audio,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	Seed                float64         `json:"seed,omitempty"`
	ServiceTier         *string         `json:"service_tier,omitempty"`
	Stop                any             `json:"stop,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	TopK                int             `json:"top_k,omitempty"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	ParallelTooCalls    *bool           `json:"parallel_tool_calls,omitempty"`
	User                string          `json:"user,omitempty"`
	FunctionCall        any             `json:"function_call,omitempty"`
	Functions           any             `json:"functions,omitempty"`
	// https://platform.openai.com/docs/api-reference/embeddings/create
	Input          any    `json:"input,omitempty"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
	// https://platform.openai.com/docs/api-reference/images/create
	Prompt  any     `json:"prompt,omitempty"`
	Quality *string `json:"quality,omitempty"`
	Size    string  `json:"size,omitempty"`
	Style   *string `json:"style,omitempty"`
	// Others
	Instruction string    `json:"instruction,omitempty"`
	NumCtx      int       `json:"num_ctx,omitempty"`
	Thinking    *Thinking `json:"thinking,omitempty"`
	Provider    any       `json:"provider,omitempty"`
	Reasoning    any       `json:"reasoning,omitempty"`
}

func (r GeneralOpenAIRequest) ParseInput() []string {
	if r.Input == nil {
		return nil
	}
	var input []string
	switch r.Input.(type) {
	case string:
		input = []string{r.Input.(string)}
	case []any:
		input = make([]string, 0, len(r.Input.([]any)))
		for _, item := range r.Input.([]any) {
			if str, ok := item.(string); ok {
				input = append(input, str)
			}
		}
	}
	return input
}
