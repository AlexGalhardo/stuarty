// Package google is the adapter of Google for video, through the Gemini API with the user's own API key
// (SDD-016 section 2.3). It speaks the Interactions API of the Gemini API: the model gemini-omni-1.2-flash,
// started in the background, read by the id of the interaction and downloaded from the finished interaction.
// It uses the standard library through the gated client of a connection and no vendor SDK.
package google

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

// ID is the provider id and the kind of its connections.
const ID = "google"

// DefaultEndpoint is the base URL of the Gemini API.
const DefaultEndpoint = "https://generativelanguage.googleapis.com/v1beta"

// Adapter is the Google provider.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

// Descriptor implements providers.Provider.
func (*Adapter) Descriptor() providers.Descriptor {
	return providers.Descriptor{
		ID: ID, Name: "Google (Gemini API)",
		Capabilities:    []string{provider.CapabilityVideo},
		CredentialKinds: []string{provider.CredentialAPIKey},
		DefaultEndpoint: DefaultEndpoint,
		DocsURL:         "https://aistudio.google.com/apikey",
	}
}

// headers carry the key in the header the Gemini API documents, never in the query string, where a log of
// a proxy or of the gate could keep it.
func headers(c connections.Credential) map[string]string {
	return map[string]string{"x-goog-api-key": string(c.Secret)}
}

type modelsAnswer struct {
	Models []struct {
		Name             string `json:"name"`
		DisplayName      string `json:"displayName"`
		InputTokenLimit  int    `json:"inputTokenLimit"`
		OutputTokenLimit int    `json:"outputTokenLimit"`
	} `json:"models"`
}

// Models implements providers.Provider: GET /models, the list of the Gemini API (the names carry a
// "models/" prefix the id does not). It publishes no price, so the price comes from the table of the person.
func (*Adapter) Models(ctx context.Context, c connections.Credential) ([]providers.Model, error) {
	var ans modelsAnswer

	call := wire.Call{
		Method: http.MethodGet, Path: "/models?" + url.Values{"pageSize": {"1000"}}.Encode(), Header: headers(c),
	}

	if err := wire.JSON(ctx, c, call, &ans); err != nil {
		return nil, err
	}

	out := make([]providers.Model, 0, len(ans.Models))

	for i := range ans.Models {
		m := &ans.Models[i]
		id := strings.TrimPrefix(m.Name, "models/")

		name := m.DisplayName
		if name == "" {
			name = id
		}

		out = append(out, providers.Model{
			ID: id, Name: name, ContextTokens: m.InputTokenLimit, OutputTokens: m.OutputTokenLimit,
			Capability: capabilityOf(id),
		})
	}

	return out, nil
}

// capabilityOf is the hint of the capability of a model from its documented naming: the Omni models make
// video. It only orders what a picker shows.
func capabilityOf(id string) string {
	if strings.HasPrefix(id, "gemini-omni") {
		return provider.CapabilityVideo
	}

	return ""
}
