package gemini

import relaymodel "github.com/songquanpeng/one-api/relay/model"

type ChatRequest struct {
	Contents         []ChatContent        `json:"contents"`
	SafetySettings   []ChatSafetySettings `json:"safety_settings,omitempty"`
	GenerationConfig ChatGenerationConfig `json:"generation_config,omitempty"`
	Tools            []ChatTools          `json:"tools,omitempty"`
}

type EmbeddingRequest struct {
	Model                string      `json:"model"`
	Content              ChatContent `json:"content"`
	TaskType             string      `json:"taskType,omitempty"`
	Title                string      `json:"title,omitempty"`
	OutputDimensionality int         `json:"outputDimensionality,omitempty"`
}

type BatchEmbeddingRequest struct {
	Requests []EmbeddingRequest `json:"requests"`
}

type EmbeddingData struct {
	Values []float64 `json:"values"`
}

type EmbeddingResponse struct {
	Embeddings []EmbeddingData `json:"embeddings"`
	Error      *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int           `json:"code,omitempty"`
	Message string        `json:"message,omitempty"`
	Status  string        `json:"status,omitempty"`
	Details []ErrorDetail `json:"details,omitempty"`
}

type ErrorDetail struct {
	Type       string `json:"@type,omitempty"`
	RetryDelay string `json:"retryDelay,omitempty"`
}

type GeminiErrorResponse struct {
	Error *Error `json:"error,omitempty"`
}

type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type FunctionCall struct {
	FunctionName string `json:"name"`
	Arguments    any    `json:"args"`
}

type FileData struct {
	MimeType string `json:"mimeType"`
	Uri      string `json:"fileUri"`
}

type Part struct {
	Text         string        `json:"text,omitempty"`
	InlineData   *InlineData   `json:"inlineData,omitempty"`
	FileData     *FileData     `json:"fileData,omitempty"`
	FunctionCall *FunctionCall `json:"functionCall,omitempty"`
	Thought      bool          `json:"thought,omitempty"`
}

type ChatContent struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}
type ChatSafetySettings struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type ChatTools struct {
	FunctionDeclarations  any          `json:"function_declarations,omitempty"`
	GoogleSearchRetrieval any          `json:"google_search_retrieval,omitempty"`
	CodeExecution         any          `json:"code_execution,omitempty"`
	GoogleSearch          GoogleSearch `json:"google_search,omitempty"`
}
type GoogleSearch struct {
}

type ChatGenerationConfig struct {
	ResponseMimeType   string          `json:"responseMimeType,omitempty"`
	ResponseModalities []string        `json:"responseModalities,omitempty"`
	ResponseSchema     any             `json:"responseSchema,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	TopP               *float64        `json:"topP,omitempty"`
	TopK               float64         `json:"topK,omitempty"`
	MaxOutputTokens    int             `json:"maxOutputTokens,omitempty"`
	CandidateCount     int             `json:"candidateCount,omitempty"`
	StopSequences      any             `json:"stopSequences,omitempty"`
	ThinkingConfig     *ThinkingConfig `json:"thinkingConfig,omitempty"`
}
type ThinkingConfig struct {
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

type ChatCandidate struct {
	Content       ChatContent        `json:"content"`
	FinishReason  string             `json:"finishReason"`
	Index         int64              `json:"index"`
	SafetyRatings []ChatSafetyRating `json:"safetyRatings"`
}

type ChatSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type ChatPromptFeedback struct {
	BlockReason   string             `json:"blockReason,omitempty"`
	SafetyRatings []ChatSafetyRating `json:"safetyRatings"`
}

type ChatResponse struct {
	Candidates     []ChatCandidate     `json:"candidates"`
	PromptFeedback *ChatPromptFeedback `json:"promptFeedback"`
	UsageMetadata  *UsageMetaData      `json:"usageMetadata"`
	ModelVersion   string              `json:"modelVersion"`
}
type UsageMetaData struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type ImageRequest struct {
	Contents         []ChatContent        `json:"contents"`
	GenerationConfig ChatGenerationConfig `json:"generation_config,omitempty"`
}

type ImageData struct {
	Url           string `json:"url,omitempty"`
	B64Json       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type ImageResponse struct {
	Created       int64            `json:"created"`
	Data          []ImageData      `json:"data"`
	RevisedPrompt string           `json:"revised_prompt,omitempty"`
	Usage         relaymodel.Usage `json:"usage"`
}

type ImageResponse2Chat struct {
	Role    string                      `json:"role,omitempty"`
	Content []ImageResponse2ChatContent `json:"content,omitempty"`
}
type ImageResponse2ChatContent struct {
	Type     string                     `json:"type,omitempty"`
	Text     string                     `json:"text,omitempty"`
	ImageUrl ImageResponse2ChatImageUrl `json:"image_url,omitempty"`
}
type ImageResponse2ChatImageUrl struct {
	Url    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type VideoRequest struct {
	Instances  VideoInstances  `json:"instances"`
	Parameters VideoParameters `json:"parameters,omitempty"`
}
type VideoInstances struct {
	Prompt string     `json:"prompt"`
	Image  VideoImage `json:"image"`
}

type VideoImage struct {
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	GcsUri             string `json:"gcsUri"`
}

type VideoParameters struct {
	SampleCount      int           `json:"sampleCount"`
	Seed             int           `json:"seed,omitempty"`
	EnhancePrompt    bool          `json:"enhancePrompt,omitempty"`
	NegativePrompt   string        `json:"negativePrompt,omitempty"`
	AspectRatio      string        `json:"aspectRatio,omitempty"`
	OutputOptions    OutputOptions `json:"outputOptions,omitempty"`
	SampleImageStyle string        `json:"sampleImageStyle,omitempty"`
	PersonGeneration string        `json:"personGeneration,omitempty"`
	SafetySetting    string        `json:"safetySetting,omitempty"`
	AddWatermark     bool          `json:"addWatermark,omitempty"`
	StorageUri       string        `json:"storageUri,omitempty"`
	DurationSeconds  int           `json:"durationSeconds,omitempty"`
	Mode             string        `json:"mode,omitempty"`
	UpscaleConfig    UpscaleConfig `json:"upscaleConfig,omitempty"`
}
type UpscaleConfig struct {
	UpscaleFactor string `json:"upscaleFactor"`
}

type OutputOptions struct {
	MimeType           string `json:"mimeType,omitempty"`
	CompressionQuality int    `json:"compressionQuality,omitempty"`
}

type RunningResultResponse struct {
	Name string `json:"name,omitempty"`
}

type VideoResultResponse struct {
	Name     string            `json:"name,omitempty"`
	Done     bool              `json:"done,omitempty"`
	Response *VideoJobResponse `json:"response,omitempty"`
}
type VideoJobResponse struct {
	GenerateVideoResponse GenerateVideoResponse `json:"generateVideoResponse,omitempty"`
}
type GenerateVideoResponse struct {
	GeneratedSamples []GeneratedSamples `json:"generatedSamples,omitempty"`
}

type GeneratedSamples struct {
	Video VideoUrl `json:"video,omitempty"`
}
type VideoUrl struct {
	Uri string `json:"uri,omitempty"`
}
