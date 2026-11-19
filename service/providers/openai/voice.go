package openai

import (
	"context"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

// speechChars is the longest text the speech endpoint takes (SDD-016 section 2.2).
const speechChars = 4096

// voices are the voices the documentation of OpenAI lists: it has no endpoint for them (SDD-016 section 2.2).
var voices = []string{
	"alloy", "ash", "ballad", "cedar", "coral", "echo", "fable", "marin", "nova", "onyx", "sage", "shimmer", "verse",
}

// VoiceLimits implements providers.VoiceGenerator.
func (*Adapter) VoiceLimits(string) provider.VoiceLimits {
	return provider.VoiceLimits{MaxChars: speechChars, Formats: provider.VoiceFormats}
}

// Voices implements providers.VoiceGenerator: the voices of the documentation, with no request.
func (*Adapter) Voices(context.Context, connections.Credential) ([]providers.Voice, error) {
	out := make([]providers.Voice, 0, len(voices))

	for _, v := range voices {
		out = append(out, providers.Voice{
			ID: v, Name: strings.ToUpper(v[:1]) + v[1:], Source: providers.SourceDocumented,
		})
	}

	return out, nil
}

type speechRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

// GenerateVoice implements providers.VoiceGenerator: POST /audio/speech, which answers with the audio. The
// language is not sent: the endpoint reads it from the text.
func (*Adapter) GenerateVoice(
	ctx context.Context,
	c connections.Credential,
	req provider.VoiceRequest,
	maxBytes int64,
) (providers.VoiceResult, error) {
	header := headers(c)
	header["Accept"] = "*/*"

	data, err := wire.Bytes(ctx, c, wire.Call{
		Method: http.MethodPost, Path: "/audio/speech", Header: header,
		Body: speechRequest{Model: req.Model, Input: req.Text, Voice: req.Voice, ResponseFormat: req.Format},
	}, maxBytes)
	if err != nil {
		return providers.VoiceResult{}, err
	}

	return providers.VoiceResult{File: providers.GeneratedFile{Data: data}}, nil
}
