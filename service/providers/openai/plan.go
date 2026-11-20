package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

// codeUsageLimit is the failure of a plan that has used up its share, one of the two a stream can report
// (SDD-016 section 12.1); the other, subscription_sharing_usage_unavailable, is an unavailable provider.
const codeUsageLimit = "subscription_sharing_usage_limit_exceeded"

// maxLine is the longest line of a stream that is read.
const maxLine = 1 << 20

type planModelsAnswer struct {
	Models []struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"display_name"`
		Visibility  string `json:"visibility"`
	} `json:"models"`
}

// planModels is GET /models with a plan token: `models`, of which the ones the person may pick are listed
// (visibility list). A plan has no price per token, so the catalogue publishes zero (SDD-016 section 12.2).
func planModels(ctx context.Context, c connections.Credential) ([]providers.Model, error) {
	var ans planModelsAnswer

	err := wire.JSON(ctx, c, wire.Call{Method: http.MethodGet, Path: "/models", Header: headers(c)}, &ans)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Model, 0, len(ans.Models))

	for _, m := range ans.Models {
		if m.Visibility != "list" {
			continue
		}

		name := m.DisplayName
		if name == "" {
			name = m.Slug
		}

		out = append(out, providers.Model{
			ID: m.Slug, Name: name, Price: &provider.Price{Currency: "USD"},
		})
	}

	return out, nil
}

type responsesRequest struct {
	Model           string   `json:"model"`
	Instructions    string   `json:"instructions,omitempty"`
	Input           string   `json:"input"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	Temperature     *float64 `json:"temperature,omitempty"`
	Store           bool     `json:"store"`
	Stream          bool     `json:"stream"`
}

type streamEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
	Code  string `json:"code"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
	Response struct {
		Status string `json:"status"`
		Error  *struct {
			Code string `json:"code"`
		} `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"response"`
}

// planGenerate is POST /responses with store false and stream true, read to its end. The request succeeds
// only after the event response.completed (or response.incomplete for the output limit); a stream that ends
// before, an error event and any other end are errors built from the kind of event and a safe code.
func planGenerate(
	ctx context.Context,
	c connections.Credential,
	req provider.TextRequest,
) (providers.TextResult, error) {
	raw, err := json.Marshal(responsesRequest{
		Model: req.Model, Instructions: req.System, Input: req.Prompt, MaxOutputTokens: req.MaxTokens,
		Temperature: req.Temperature, Store: false, Stream: true,
	})
	if err != nil {
		return providers.TextResult{}, err
	}

	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, wire.URL(c, "/responses"), bytes.NewReader(raw))
	if err != nil {
		return providers.TextResult{}, err
	}

	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "text/event-stream")
	hr.Header.Set("Authorization", headers(c)["Authorization"])

	res, err := c.Client.Do(hr)
	if err != nil {
		return providers.TextResult{}, transportError(ctx, err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, wire.MaxErrorBody))

		return providers.TextResult{}, wire.StatusError(res.StatusCode, res.Header.Get("Retry-After"), body)
	}

	return readStream(ctx, res.Body, res.StatusCode)
}

// readStream reads the events of a Responses stream.
func readStream(ctx context.Context, body io.Reader, status int) (providers.TextResult, error) {
	var text strings.Builder

	sc := bufio.NewScanner(io.LimitReader(body, wire.MaxAnswer))
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)

	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}

		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			continue
		}

		var ev streamEvent
		if json.Unmarshal([]byte(data), &ev) != nil {
			return providers.TextResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: status}
		}

		switch ev.Type {
		case "response.output_text.delta":
			text.WriteString(ev.Delta)
		case "response.completed":
			return finishStream(&text, &ev, providers.FinishStop, status)
		case "response.incomplete":
			if r := ev.Response.IncompleteDetails; r != nil && r.Reason == "max_output_tokens" {
				return finishStream(&text, &ev, providers.FinishLength, status)
			}

			return providers.TextResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: status}
		case "response.failed", "error":
			return providers.TextResult{}, failedStream(&ev, status)
		}
	}

	if ctx.Err() != nil {
		return providers.TextResult{}, ctx.Err()
	}

	// The stream ended with no terminal event: whatever text came is not a finished answer.
	return providers.TextResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: status}
}

func finishStream(text *strings.Builder, ev *streamEvent, reason string, status int) (providers.TextResult, error) {
	u := ev.Response.Usage
	if u == nil {
		return providers.TextResult{}, &providers.Error{Kind: providers.KindUnreadable, Status: status}
	}

	return providers.TextResult{
		Text: text.String(), FinishReason: reason,
		Usage: provider.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens},
	}, nil
}

// failedStream maps a failure event to an error built from its safe code.
func failedStream(ev *streamEvent, status int) error {
	code := ev.Code
	if ev.Error != nil && ev.Error.Code != "" {
		code = ev.Error.Code
	}

	if ev.Response.Error != nil && ev.Response.Error.Code != "" {
		code = ev.Response.Error.Code
	}

	e := &providers.Error{Kind: providers.KindUnavailable, Status: status, Code: providers.SafeCode(code)}
	if code == codeUsageLimit {
		e.Kind = providers.KindRateLimited
	}

	return e
}

// transportError keeps what the layers above must see as it is and turns the rest into "unavailable".
func transportError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, outbound.ErrNoConnection),
		errors.Is(err, outbound.ErrDestination),
		errors.Is(err, outbound.ErrNotPublic):
		return err
	case ctx.Err() != nil:
		return ctx.Err()
	}

	return &providers.Error{Kind: providers.KindUnavailable}
}
