package model

type ChannelConfig struct {
	Region            string `json:"region,omitempty"`
	SK                string `json:"sk,omitempty"`
	AK                string `json:"ak,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	APIVersion        string `json:"api_version,omitempty"`
	LibraryID         string `json:"library_id,omitempty"`
	Plugin            string `json:"plugin,omitempty"`
	VertexAIProjectID string `json:"vertex_ai_project_id,omitempty"`
	VertexAIADC       string `json:"vertex_ai_adc,omitempty"`
}
