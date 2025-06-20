package model

import "mime/multipart"

type VideoRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt" binding:"omitempty"`
	NegativePrompt string `json:"negative_prompt" binding:"omitempty"`
	Size           string `json:"size,omitempty"`
	N              int    `json:"n,omitempty"`
	Duration       int    `json:"duration,omitempty"`
	Image          string `json:"image,omitempty"`
}
type VideoFormRequest struct {
	Model          string                `form:"model"`
	Prompt         string                `form:"prompt"`
	NegativePrompt string                `form:"negative_prompt" binding:"omitempty"`
	Size           string                `form:"size"`
	N              int                   `form:"n,omitempty"`
	Image          *multipart.FileHeader `form:"image"`
}
