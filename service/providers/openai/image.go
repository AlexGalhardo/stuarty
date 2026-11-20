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

// answerSlack is what an answer of images may hold besides the images, in bytes.
const answerSlack = 64 << 10

// capabilityOf is the hint of the capability of a model from the naming OpenAI documents (SDD-016 section
// 2.2). It only orders what a picker shows: a request names the model it wants.
func capabilityOf(id string) string {
	switch {
	case strings.HasPrefix(id, "dall-e"), strings.HasPrefix(id, "gpt-image"), strings.HasPrefix(id, "chatgpt-image"):
		return provider.CapabilityImage
	case strings.HasPrefix(id, "tts-"), strings.HasSuffix(id, "-tts"):
		return provider.CapabilityVoice
	case strings.HasPrefix(id, "gpt-"), strings.HasPrefix(id, "o1"), strings.HasPrefix(id, "o3"),
		strings.HasPrefix(id, "o4"), strings.HasPrefix(id, "chatgpt-"):
		return provider.CapabilityText
	}

	return ""
}

// ImageLimits implements providers.ImageGenerator with the sizes and counts the documentation of OpenAI gives
// for each family of model; a model it does not know has no documented limit and the provider is the judge.
func (*Adapter) ImageLimits(model string) provider.ImageLimits {
	switch {
	case strings.HasPrefix(model, "dall-e-3"):
		return provider.ImageLimits{Sizes: []string{"1024x1024", "1792x1024", "1024x1792"}, MaxCount: 1}
	case strings.HasPrefix(model, "dall-e-2"):
		return provider.ImageLimits{Sizes: []string{"256x256", "512x512", "1024x1024"}, MaxCount: 10}
	case strings.HasPrefix(model, "gpt-image"):
		return provider.ImageLimits{Sizes: []string{"1024x1024", "1536x1024", "1024x1536"}, MaxCount: 10}
	}

	return provider.ImageLimits{}
}

type imageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n"`
	Size   string `json:"size"`
}

type imageAnswer struct {
	Data []struct {
		B64           string `json:"b64_json"`
		URL           string `json:"url"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
}

// GenerateImages implements providers.ImageGenerator: POST /images/generations. An image comes back as base64
// or as a URL, by model; a URL is fetched with the gated client, so a host the connection does not list is
// refused. The images that arrive are kept when others do not.
func (*Adapter) GenerateImages(
	ctx context.Context,
	c connections.Credential,
	req provider.ImageRequest,
	maxBytes int64,
) (providers.ImageResult, error) {
	// The token of a sign-in is for the text of a plan only (SDD-016 section 12.2); the service refuses
	// before a job exists, and this keeps it off the image endpoint whatever the caller.
	if c.SignedIn {
		return providers.ImageResult{}, errSignInTextOnly
	}

	var ans imageAnswer

	call := wire.Call{
		Method: http.MethodPost, Path: "/images/generations", Header: headers(c),
		Body:      imageRequest{Model: req.Model, Prompt: req.Prompt, N: req.Count, Size: req.Size},
		MaxAnswer: int64(req.Count)*(maxBytes/3*4+8) + answerSlack,
	}

	if err := wire.JSON(ctx, c, call, &ans); err != nil {
		return providers.ImageResult{}, err
	}

	if len(ans.Data) == 0 {
		return providers.ImageResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
	}

	var res providers.ImageResult

	// Only the images asked for are fetched and held: the disk was checked, and the person approved, for that
	// count, and a server that lists more must not make the product download or keep them.
	for i := range min(len(ans.Data), req.Count) {
		d := &ans.Data[i]

		data, err := imageBytes(ctx, c, d.B64, d.URL, maxBytes)
		if err != nil {
			// A cancelled or expired context ends the whole request; any other failure of one file (a host the
			// connection does not list, a bad status, a file too large) leaves a partial batch.
			if ctx.Err() != nil {
				return providers.ImageResult{}, err
			}

			res.Failed++

			if res.Err == nil {
				res.Err = err
			}

			continue
		}

		res.Files = append(res.Files, providers.GeneratedFile{Data: data, RevisedPrompt: d.RevisedPrompt})
	}

	if len(res.Files) == 0 {
		return providers.ImageResult{}, res.Err
	}

	return res, nil
}

func imageBytes(ctx context.Context, c connections.Credential, b64, rawURL string, maxBytes int64) ([]byte, error) {
	switch {
	case b64 != "":
		if int64(len(b64)) > maxBytes/3*4+8 {
			return nil, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
		}

		// The length check above lets through up to a few bytes over maxBytes; the decoded file is held to it.
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || int64(len(data)) > maxBytes {
			return nil, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
		}

		return data, nil
	case rawURL != "":
		return wire.Download(ctx, c, rawURL, maxBytes)
	}

	return nil, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
}
