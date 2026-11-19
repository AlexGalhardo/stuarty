// Package openrouter is the adapter of OpenRouter for text: the Chat Completions wire it speaks, with a
// key the person pasted or one the sign-in of OpenRouter issued (SDD-016 section 12). The key is the
// secret of the connection either way, and the catalogue publishes a price for each model.
package openrouter

import (
	"context"
	"math/big"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

// ID is the provider id and the kind of its connections.
const ID = "openrouter"

// framing is the tokens the chat format adds to the input, as for the other chat adapters.
const framing = 16

// Adapter is the OpenRouter provider.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

// Descriptor implements providers.Provider.
func (*Adapter) Descriptor() providers.Descriptor {
	return providers.Descriptor{
		ID: ID, Name: "OpenRouter",
		Capabilities:    []string{provider.CapabilityText},
		CredentialKinds: []string{provider.CredentialAPIKey, provider.CredentialOAuth},
		DefaultEndpoint: DefaultEndpoint,
		DocsURL:         "https://openrouter.ai/keys",
	}
}

// DefaultEndpoint is the base URL of the API of OpenRouter.
const DefaultEndpoint = "https://openrouter.ai/api/v1"

// Framing implements providers.TextGenerator.
func (*Adapter) Framing() int { return framing }

func headers(c connections.Credential) map[string]string {
	return map[string]string{"Authorization": "Bearer " + string(c.Secret)}
}

type modelsAnswer struct {
	Data []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
		TopProvider   struct {
			MaxCompletionTokens int `json:"max_completion_tokens"`
		} `json:"top_provider"`
		Pricing struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
	} `json:"data"`
}

// Models implements providers.Provider: GET /models, whose entries carry the limits and the price.
func (*Adapter) Models(ctx context.Context, c connections.Credential) ([]providers.Model, error) {
	var ans modelsAnswer

	err := wire.JSON(ctx, c, wire.Call{Method: http.MethodGet, Path: "/models", Header: headers(c)}, &ans)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Model, 0, len(ans.Data))

	for i := range ans.Data {
		m := &ans.Data[i]
		model := providers.Model{
			ID: m.ID, Name: m.Name, ContextTokens: m.ContextLength, OutputTokens: m.TopProvider.MaxCompletionTokens,
		}

		if model.Name == "" {
			model.Name = m.ID
		}

		if in, ok := microsPerMTok(m.Pricing.Prompt); ok {
			if outp, ok := microsPerMTok(m.Pricing.Completion); ok {
				model.Price = &provider.Price{Currency: "USD", InputMicrosPerMTok: in, OutputMicrosPerMTok: outp}
			}
		}

		out = append(out, model)
	}

	return out, nil
}

// perMTok is a million tokens times a million (the unit of a price is a millionth of a currency unit).
var perMTok = big.NewRat(1_000_000_000_000, 1)

// microsPerMTok turns the price of one token in dollars, written as a decimal ("0.000003"), into the
// millionths of a dollar for a million tokens, rounded up. A price that is not a non-negative decimal
// ("-1" is a router with a variable price) or is above the largest price has no value.
func microsPerMTok(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, "eExX/ ") {
		return 0, false
	}

	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return 0, false
	}

	r.Mul(r, perMTok)

	// Ceiling of a non-negative rational.
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}

	if !q.IsInt64() || q.Int64() > provider.MaxPriceMicros {
		return 0, false
	}

	return q.Int64(), true
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature *float64  `json:"temperature,omitempty"`
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
	body := chatRequest{Model: req.Model, MaxTokens: req.MaxTokens, Temperature: req.Temperature}

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
