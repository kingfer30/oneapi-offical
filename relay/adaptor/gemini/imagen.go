package gemini

import (
	"github.com/songquanpeng/one-api/common/image"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func ConvertImagenRequest(request relaymodel.ImageRequest) (*ImageRequest, error) {
	var contents []ChatContent
	if len(request.Image) > 0 {
		for _, img := range request.Image {
			//图片编辑
			mimeType, fileData, err := image.GetImageFromUrl(img, false)
			if err != nil {
				return nil, err
			}
			contents = append(contents, ChatContent{
				Role: "user",
				Parts: []Part{
					{
						InlineData: &InlineData{
							MimeType: mimeType,
							Data:     fileData,
						},
					},
					{
						Text: request.Prompt,
					},
				},
			})
		}
	} else {
		//图片创建
		if request.N > 1 {
			if request.N > 4 {
				request.N = 4
			}

		}
		contents = append(contents, ChatContent{
			Role: "user",
			Parts: []Part{
				{
					Text: request.Prompt,
				},
			},
		})
	}

	imageRequest := ImageRequest{
		Contents: contents,
		GenerationConfig: ChatGenerationConfig{
			ResponseModalities: []string{"text", "image"},
		},
	}

	return &imageRequest, nil
}
