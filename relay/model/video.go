package model

import "mime/multipart"

type VideoRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt" binding:"required"`
	Size   string `json:"size,omitempty"`
	Image  string `json:"image,omitempty"`
}
type VideoFormRequest struct {
	Model  string                `form:"model"`
	Prompt string                `form:"prompt"`
	Size   string                `form:"size"`
	Image  *multipart.FileHeader `form:"image"`
}
