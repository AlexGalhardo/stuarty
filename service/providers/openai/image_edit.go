package openai

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

// removeBackgroundPrompt is what the adapter asks of the model. OpenAI has no operation of this name: the edit
// of a picture with a prompt and a transparent background is the documented way (SDD-016 section 2.4).
const removeBackgroundPrompt = "Remove the background of this picture. Keep the subject exactly as it is, with clean " +
	"edges and unchanged colours, and make everything behind it transparent."

// EditLimits implements providers.ImageEditor with what the documentation of OpenAI gives for the models of the
// GPT Image family, which do the edit with a transparent background; another model has no documented limit and
// the provider is the judge.
func (*Adapter) EditLimits(model string) provider.EditLimits {
	if strings.HasPrefix(model, "gpt-image") || strings.HasPrefix(model, "chatgpt-image") {
		return provider.EditLimits{
			Operations: []string{provider.OperationRemoveBackground},
			Sizes:      []string{"1024x1024", "1536x1024", "1024x1536"},
		}
	}

	return provider.EditLimits{}
}

type imageSource struct {
	ImageURL string `json:"image_url"`
}

type editRequest struct {
	Model        string        `json:"model"`
	Prompt       string        `json:"prompt"`
	Images       []imageSource `json:"images"`
	Background   string        `json:"background"`
	OutputFormat string        `json:"output_format"`
	N            int           `json:"n"`
	Size         string        `json:"size,omitempty"`
}

// RemoveBackground implements providers.ImageEditor: POST /images/edits with the picture as a base64 data URL,
// a prompt that asks for the background to go and background transparent. The answer is read as that of
// GenerateImages: base64, or a URL fetched with the gated client.
func (*Adapter) RemoveBackground(
	ctx context.Context,
	c connections.Credential,
	req provider.EditRequest,
	src providers.SourceImage,
	maxBytes int64,
) (providers.ImageResult, error) {
	if len(src.Data) == 0 || src.Format == "" {
		return providers.ImageResult{}, &providers.Error{Kind: providers.KindBadRequest}
	}

	body := editRequest{
		Model:  req.Model,
		Prompt: removeBackgroundPrompt,
		Images: []imageSource{{
			ImageURL: "data:image/" + src.Format + ";base64," + base64.StdEncoding.EncodeToString(src.Data),
		}},
		Background: "transparent", OutputFormat: "png", N: 1, Size: req.Size,
	}

	var ans imageAnswer

	call := wire.Call{
		Method: http.MethodPost, Path: "/images/edits", Header: headers(c), Body: body,
		MaxAnswer: maxBytes/3*4 + 8 + answerSlack,
	}

	if err := wire.JSON(ctx, c, call, &ans); err != nil {
		return providers.ImageResult{}, err
	}

	if len(ans.Data) == 0 {
		return providers.ImageResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
	}

	d := &ans.Data[0]

	data, err := imageBytes(ctx, c, d.B64, d.URL, maxBytes)
	if err != nil {
		return providers.ImageResult{}, err
	}

	return providers.ImageResult{Files: []providers.GeneratedFile{{Data: data, RevisedPrompt: d.RevisedPrompt}}}, nil
}
