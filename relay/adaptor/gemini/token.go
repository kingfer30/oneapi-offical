package gemini

import (
	"math"

	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/meta"
)

func ResetChatQuota(usePrompt int, useCompletion int, useThoughts int, useTotal int, stream bool, meta *meta.Meta) (int, int, int) {
	modelRatio := billingratio.GetModelRatio(meta.ActualModelName, meta.ChannelType, meta.Group)
	groupRatio := billingratio.GetGroupRatio(meta.Group)
	name := meta.ActualModelName
	if useThoughts > 0 {
		meta.UseThinking = true
		name = name + "-thinking"
	}
	completionRatio := billingratio.GetCompletionRatio(name, meta.ChannelType)
	ratio := modelRatio * groupRatio
	quota := 0
	//如果非流 且 没有补全token, 按照倍率*1000
	if !stream && useCompletion <= 0 {
		defaultToken := int(ratio * 1000)
		useCompletion += defaultToken
	}
	quota = int(math.Ceil((float64(usePrompt) + float64(useCompletion)*completionRatio) * ratio))
	if useTotal > int(quota) {
		quota = useTotal
	}
	return usePrompt, useCompletion, quota
}

func ResetImg2ChatQuota(usePrompt int, useCompletion int, useTotal int, imgN int, meta *meta.Meta) (int, int, int) {
	modelRatio := billingratio.GetModelRatio(meta.ActualModelName, meta.ChannelType, meta.Group)
	groupRatio := billingratio.GetGroupRatio(meta.Group)
	completionRatio := billingratio.GetCompletionRatio(meta.ActualModelName, meta.ChannelType)
	ratio := modelRatio * groupRatio
	quota := 0
	//如果 没有返回, 按照图片数量生成返回token
	imgToken := int(ratio*1000) * imgN
	useCompletion += imgToken
	quota = int(math.Ceil((float64(usePrompt) + float64(useCompletion)*completionRatio) * ratio))
	if useTotal > int(quota) {
		quota = useTotal
	}
	return usePrompt, useCompletion, quota
}
