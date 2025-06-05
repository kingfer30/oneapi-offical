package gemini

import "strings"

// https://ai.google.dev/models/gemini

var ModelList = []string{
	"text-embedding-004",
	"gemini-1.0-pro-vision",
	"gemini-1.0-pro-vision-latest",
	"gemini-1.5-pro",
	"gemini-1.5-pro-001",
	"gemini-1.5-pro-002",
	"gemini-1.5-pro-latest",
	"gemini-1.5-flash",
	"gemini-1.5-flash-001",
	"gemini-1.5-flash-001-tuning",
	"gemini-1.5-flash-002",
	"gemini-1.5-flash-latest",
	"gemini-1.5-flash-exp-0827",
	"gemini-1.5-flash-8b",
	"gemini-1.5-flash-8b-001",
	"gemini-1.5-flash-8b-latest",
	"gemini-1.5-flash-8b-exp-0924",
	"gemini-exp-1206",
	"learnlm-1.5-pro-experimental",
	"gemini-2.0-flash",
	"gemini-2.0-flash-001",
	"gemini-2.0-flash-exp",
	"gemini-2.0-flash-thinking-exp",
	"gemini-2.0-flash-thinking-exp-01-21",
	"gemini-2.0-flash-lite",
	"gemini-2.0-flash-lite-001",
	"gemini-2.0-flash-lite-preview-02-05",
	"gemini-2.0-flash-lite-preview",
	"gemini-2.0-pro-exp-02-05",
	"gemini-2.0-pro-exp",
	"gemini-2.0-flash-exp-image-generation",
	"gemini-2.0-flash-preview-image-generation",
	"gemma-3-27b-it",
	"gemini-embedding-exp-03-07",
	"gemini-embedding-exp",
	"embedding-001",
	"gemini-2.5-pro-exp-03-25",
	"gemini-2.5-flash-preview-04-17",
	"gemini-2.5-flash-preview-04-17-thinking",
	"gemini-2.5-flash-preview-05-20",
	"gemini-2.5-flash-preview-tts",
	"gemini-2.5-pro-preview-tts",
	"gemini-2.5-pro-preview-03-25",
	"gemini-2.5-pro-preview-05-06",
	"gemini-2.5-pro-preview-06-05",
	"imagen-3.0-generate-002",
	"veo-2.0-generate-001",
}

//定义支持画图的模型
var ImageModelList = []string{
	"gemini-2.0-flash-exp-image-generation",
	"gemini-2.0-flash-exp",
}

//定义低TPM的模型
var LowTPMModelList = []string{
	"gemini-2.5-pro-exp-03-25",
}

//定义低TPM的模型映射
var LowTPMModelMapping = map[string]int{
	"gemini-2.5-pro-exp-03-25": 250000,
}

//定义支持思考的模型
var ThinkingModelList = []string{
	"gemini-2.0-flash-thinking-exp",
	"gemini-2.0-flash-thinking-exp-01-21",
	"gemini-2.5-flash-preview-04-17",
	"gemini-2.5-flash-preview-04-17-thinking",
	"gemini-2.5-flash-preview-05-20",
	"gemini-2.5-pro-preview-05-06",
}

var BlockReasonList = map[string]string{
	"BLOCK_REASON_UNSPECIFIED": "Prompt was blocked.",
	"SAFETY":                   "Prompt was blocked due to safety reasons. Inspect safetyRatings to understand which safety category blocked it.",
	"OTHER":                    "Prompt was blocked due to unknown reasons.",
	"BLOCKLIST":                "Prompt was blocked due to the terms which are included from the terminology blocklist.",
	"PROHIBITED_CONTENT":       "Prompt was blocked due to prohibited content.",
	"IMAGE_SAFETY":             "Candidates blocked due to unsafe image generation content.",
}

func IsImageModel(name string) bool {
	for _, model := range ImageModelList {
		if strings.Contains(name, model) {
			return true
		}
	}
	return false
}

func IsLowTpmModel(name string) bool {
	for _, model := range LowTPMModelList {
		if strings.Contains(name, model) {
			return true
		}
	}
	return false
}

func IsThinkingModel(name string) bool {
	for _, model := range ThinkingModelList {
		if strings.Contains(name, model) {
			return true
		}
	}
	return false
}

func IsNeedToAddRandomModel(model string) bool {
	if strings.Contains(model, "flash") {
		return true
	}
	return false
}
