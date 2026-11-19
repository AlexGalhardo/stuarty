// Package anthropic is the adapter of Anthropic for text: the Messages API with an API key
// (SDD-016 section 2.1). It is reached with an API key only: Stuarty never offers a Claude.ai login and
// never routes a request through subscription credentials (ADR-0036 item 3, RF-365). It uses the
// standard library through the gated client of a connection and no vendor SDK.
package anthropic

import (
	"context"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

// ID is the provider id and the kind of its connections.
const ID = "anthropic"

// Version is the value of the anthropic-version header: the version of the API the adapter speaks.
const Version = "2023-06-01"

// framing is the tokens the message format adds to the input, rounded up.
const framing = 16

// Adapter is the Anthropic provider.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

// Descriptor implements providers.Provider.
func (*Adapter) Descriptor() providers.Descriptor {
	return providers.Descriptor{
		ID: ID, Name: "Anthropic",
		Capabilities:    []string{provider.CapabilityText},
		CredentialKinds: []string{provider.CredentialAPIKey},
		DefaultEndpoint: "https://api.anthropic.com/v1",
		DocsURL:         "https://console.anthropic.com/settings/keys",
	}
}

// Framing implements providers.TextGenerator.
func (*Adapter) Framing() int { return framing }

func headers(c connections.Credential) map[string]string {
	return map[string]string{"x-api-key": string(c.Secret), "anthropic-version": Version}
}

type modelsAnswer struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		MaxInput    int    `json:"max_input_tokens"`
		MaxOutput   int    `json:"max_tokens"`
	} `json:"data"`
}

// Models implements providers.Provider: GET /models. The limits are used when the catalogue gives
// them; it gives no price, so the price comes from the table of the person.
func (*Adapter) Models(ctx context.Context, c connections.Credential) ([]providers.Model, error) {
	var ans modelsAnswer

	err := wire.JSON(ctx, c, wire.Call{Method: http.MethodGet, Path: "/models?limit=1000", Header: headers(c)}, &ans)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Model, 0, len(ans.Data))
	for _, m := range ans.Data {
		out = append(out, providers.Model{
			ID: m.ID, Name: m.DisplayName, ContextTokens: m.MaxInput, OutputTokens: m.MaxOutput,
		})
	}

	return out, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	System      string    `json:"system,omitempty"`
	Messages    []message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
}

type messagesAnswer struct {
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// GenerateText implements providers.TextGenerator: POST /messages, one request, no streaming.
func (*Adapter) GenerateText(
	ctx context.Context,
	c connections.Credential,
	req provider.TextRequest,
) (providers.TextResult, error) {
	body := messagesRequest{
		Model: req.Model, MaxTokens: req.MaxTokens, System: req.System, Temperature: req.Temperature,
		Messages: []message{{Role: "user", Content: req.Prompt}},
	}

	var ans messagesAnswer

	err := wire.JSON(
		ctx,
		c,
		wire.Call{Method: http.MethodPost, Path: "/messages", Header: headers(c), Body: body},
		&ans,
	)
	if err != nil {
		return providers.TextResult{}, err
	}

	if ans.Usage == nil {
		return providers.TextResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
	}

	var text strings.Builder

	for _, block := range ans.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	return providers.TextResult{
		Text:         text.String(),
		FinishReason: finish(ans.StopReason),
		Usage:        provider.Usage{InputTokens: ans.Usage.InputTokens, OutputTokens: ans.Usage.OutputTokens},
		Model:        ans.Model,
	}, nil
}

func finish(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return providers.FinishStop
	case "max_tokens":
		return providers.FinishLength
	}

	return providers.FinishOther
}
