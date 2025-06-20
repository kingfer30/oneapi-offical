package model

import "mime/multipart"

type ImageRequest struct {
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt" binding:"required"`
	N              int      `json:"n,omitempty"`
	Size           string   `json:"size,omitempty"`
	Quality        string   `json:"quality,omitempty"`
	ResponseFormat string   `json:"response_format,omitempty"`
	Style          string   `json:"style,omitempty"`
	User           string   `json:"user,omitempty"`
	Image          []string `json:"image,omitempty"`
}
type ImageFormRequest struct {
	Model          string                  `form:"model"`
	Prompt         string                  `form:"prompt"`
	N              int                     `form:"n"`
	Size           string                  `form:"size"`
	Quality        string                  `form:"quality"`
	ResponseFormat string                  `form:"response_format"`
	Style          string                  `form:"style"`
	User           string                  `form:"user"`
	Image          []*multipart.FileHeader `form:"image"`
}
