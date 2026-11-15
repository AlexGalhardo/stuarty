package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicVersion = "2023-06-01"
	maxResponseBytes = 4 << 20
	maxAttempts      = 3
)

// Doer is the part of *http.Client the harness uses.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// block is one content block of a Messages API message. The same struct serves text, tool_use
// and tool_result blocks; unused members are omitted.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Tools     []toolDef `json:"tools"`
	Messages  []message `json:"messages"`
}

type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type response struct {
	Content    []block `json:"content"`
	StopReason string  `json:"stop_reason"`
	Usage      usage   `json:"usage"`
}

// client talks to the Messages API. The key is only ever placed in the request header.
type client struct {
	http    Doer
	url     string
	key     string
	sleep   func(time.Duration)
	backoff time.Duration
}

var errAPI = errors.New("anthropic API error")

func (c *client) send(ctx context.Context, body request) (response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return response{}, fmt.Errorf("encode request: %w", err)
	}

	var lastErr error

	for attempt := range maxAttempts {
		if attempt > 0 {
			c.sleep(c.backoff << (attempt - 1))
		}

		resp, retry, err := c.once(ctx, payload)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if !retry || ctx.Err() != nil {
			break
		}
	}

	return response{}, lastErr
}

func (c *client) once(ctx context.Context, payload []byte) (resp response, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return response{}, false, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.key)
	req.Header.Set("anthropic-version", anthropicVersion)

	res, err := c.http.Do(req)
	if err != nil {
		// The transport error text never carries request headers.
		return response{}, true, fmt.Errorf("send request: %w", err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return response{}, true, fmt.Errorf("read response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		retry = res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500

		return response{}, retry, fmt.Errorf("%w: status %d: %s", errAPI, res.StatusCode, clip(string(raw), 300))
	}

	if err := json.Unmarshal(raw, &resp); err != nil {
		return response{}, false, fmt.Errorf("decode response: %w", err)
	}

	return resp, false, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
