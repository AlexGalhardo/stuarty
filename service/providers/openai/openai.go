// Package openai is the adapter of OpenAI for text (the Chat Completions API), images and speech, with an API
// key (SDD-016 sections 2.1 and 2.2). It uses the standard library through the gated client of a connection and no
// vendor SDK. The same wire is spoken by many servers, so the connection may name another base URL.
package openai

import (
	"context"
	"net/http"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

// ID is the provider id and the kind of its connections.
const ID = "openai"

// framing is the tokens the chat format adds to the input: a few per message, for two messages and the
// start of the answer, rounded up.
const framing = 16

// Adapter is the OpenAI provider.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

// Descriptor implements providers.Provider.
func (*Adapter) Descriptor() providers.Descriptor {
	return providers.Descriptor{
		ID: ID, Name: "OpenAI",
		Capabilities:    []string{provider.CapabilityText},
		CredentialKinds: []string{provider.CredentialAPIKey, provider.CredentialOAuth},
		DefaultEndpoint: "https://api.openai.com/v1",
		DocsURL:         "https://platform.openai.com/api-keys",
	}
}

// Framing implements providers.TextGenerator.
func (*Adapter) Framing() int { return framing }

func headers(c connections.Credential) map[string]string {
	return map[string]string{"Authorization": "Bearer " + string(c.Secret)}
}

type modelsAnswer struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// Models implements providers.Provider: GET /models. The catalogue of OpenAI names models and nothing
// else: no limits and no price, so the limits are unknown and the price comes from the table of the person.
func (*Adapter) Models(ctx context.Context, c connections.Credential) ([]providers.Model, error) {
	if c.SignedIn {
		return planModels(ctx, c)
	}

	var ans modelsAnswer

	err := wire.JSON(ctx, c, wire.Call{Method: http.MethodGet, Path: "/models", Header: headers(c)}, &ans)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Model, 0, len(ans.Data))
	for _, m := range ans.Data {
		out = append(out, providers.Model{ID: m.ID, Name: m.ID, Capability: capabilityOf(m.ID)})
	}

	return out, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model               string    `json:"model"`
	Messages            []message `json:"messages"`
	MaxCompletionTokens int       `json:"max_completion_tokens"`
	Temperature         *float64  `json:"temperature,omitempty"`
}

type chatAnswer struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// GenerateText implements providers.TextGenerator: POST /chat/completions, one request, no streaming.
func (*Adapter) GenerateText(
	ctx context.Context,
	c connections.Credential,
	req provider.TextRequest,
) (providers.TextResult, error) {
	if c.SignedIn {
		return planGenerate(ctx, c, req)
	}

	body := chatRequest{Model: req.Model, MaxCompletionTokens: req.MaxTokens, Temperature: req.Temperature}

	if req.System != "" {
		body.Messages = append(body.Messages, message{Role: "system", Content: req.System})
	}

	body.Messages = append(body.Messages, message{Role: "user", Content: req.Prompt})

	var ans chatAnswer

	err := wire.JSON(
		ctx,
		c,
		wire.Call{Method: http.MethodPost, Path: "/chat/completions", Header: headers(c), Body: body},
		&ans,
	)
	if err != nil {
		return providers.TextResult{}, err
	}

	if len(ans.Choices) == 0 || ans.Usage == nil {
		return providers.TextResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
	}

	// A refusal or a tool call has no content: an empty text that was still billed.
	var text string
	if c := ans.Choices[0].Message.Content; c != nil {
		text = *c
	}

	return providers.TextResult{
		Text:         text,
		FinishReason: finish(ans.Choices[0].FinishReason),
		Usage:        provider.Usage{InputTokens: ans.Usage.PromptTokens, OutputTokens: ans.Usage.CompletionTokens},
		Model:        ans.Model,
	}, nil
}

func finish(reason string) string {
	switch reason {
	case "stop":
		return providers.FinishStop
	case "length":
		return providers.FinishLength
	}

	return providers.FinishOther
}
