package gemini

import "strings"

// https://ai.google.dev/models/gemini

var ModelList = []string{
	"text-embedding-004",
	"gemini-pro",
	"gemini-pro-vision",
	"gemini-1.0-pro",
	"gemini-1.0-pro-001",
	"gemini-1.0-pro-vision-001",
	"gemini-1.0-pro-latest",
	"gemini-1.5-pro",
	"gemini-1.5-pro-001",
	"gemini-1.5-pro-002",
	"gemini-1.5-pro-latest",
	"gemini-1.5-flash",
	"gemini-1.5-flash-001",
	"gemini-1.5-flash-002",
	"gemini-1.5-flash-latest",
	"gemini-1.5-flash-exp-0827",
	"gemini-1.5-flash-8b",
	"gemini-1.5-flash-8b-001",
	"gemini-1.5-flash-8b-latest",
	"gemini-exp-1206",
	"learnlm-1.5-pro-experimental",
	"gemini-2.0-flash",
	"gemini-2.0-flash-001",
	"gemini-2.0-flash-exp",
	"gemini-2.0-flash-thinking-exp",
	"gemini-2.0-flash-thinking-exp-01-21",
	"gemini-2.0-flash-lite-preview-02-05",
	"gemini-2.0-pro-exp-02-05",
	"gemini-2.0-flash-exp-image-generation",
}

//定义支持画图的模型
var ImageModelList = []string{
	"gemini-2.0-flash-exp-image-generation",
	"gemini-2.0-flash-exp",
}

func IsImageModel(name string) bool {
	for _, model := range ImageModelList {
		if strings.Contains(name, model) {
			return true
		}
	}
	return false
}
