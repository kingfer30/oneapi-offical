package openrouter

var ModelList = []string{
	"claude-3-haiku-20240307",
	"claude-3-sonnet-20240229",
	"claude-3-opus-20240229",
	"claude-3-5-sonnet-20240620",
	"claude-3-5-haiku-20241022",
	"claude-3-5-sonnet-20241022",
	"claude-3-5-sonnet-latest",
	"claude-3-7-sonnet-20250219",
	"claude-3-7-sonnet-latest",
	"claude-opus-4-20250514",
	"claude-sonnet-4-20250514",
	"claude-opus-4-1-20250805",
}

var ModelMappingList = map[string]string{
	"claude-3-haiku-20240307":    "anthropic/claude-3-5-haiku-20241022",
	"claude-3-sonnet-20240229":   "anthropic/claude-3-5-sonnet-20241022",
	"claude-3-opus-20240229":     "anthropic/claude-3-opus-20240229",
	"claude-3-5-haiku-20241022":  "anthropic/claude-3-5-haiku-20241022",
	"claude-3-5-sonnet-20241022": "anthropic/claude-3-5-sonnet-20241022",
	"claude-3-5-sonnet-latest":   "anthropic/claude-3-5-sonnet-20241022",
	"claude-3-7-sonnet-20250219": "anthropic/claude-3-7-sonnet-20250219",
	"claude-3-7-sonnet-latest":   "anthropic/claude-3-7-sonnet-20250219",
	"claude-opus-4-20250514":     "anthropic/claude-opus-4",
	"claude-sonnet-4-20250514":   "anthropic/claude-sonnet-4",
	"claude-opus-4-1-20250805":   "anthropic/claude-opus-4.1",
}
