package billing

type OpenAIUsageDailyCost struct {
	Timestamp float64 `json:"timestamp"`
	LineItems []struct {
		Name string  `json:"name"`
		Cost float64 `json:"cost"`
	}
}
type OpenAIUsageResponse struct {
	Object     string                 `json:"object"`
	DailyCosts []OpenAIUsageDailyCost `json:"daily_costs,omitempty"`
	TotalUsage float64                `json:"total_usage"` // unit: 0.01 dollar
}
type OpenAIUsageDetailResponse struct {
	Object      string                   `json:"object"`
	DetailCosts *[]OpenAIUsageDetailCost `json:"daily_costs,omitempty"`
}

type OpenAIUsageDetailCost struct {
	Model     string  `json:"model"`
	Quota     float64 `json:"quota"`
	RequestAt string  `json:"request_at"`
}
