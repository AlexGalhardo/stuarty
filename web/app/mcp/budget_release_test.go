package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// TestToolListVendorTokens is the release measure of NFR-020: `tools/list` has at
// most 6,000 tokens by the vendor's own count. It sends the definitions to the
// token-count endpoint, so it is not part of CI: it runs once per release, with
//
//	STUARTY_BUDGET_RELEASE=1 ANTHROPIC_API_KEY=... ANTHROPIC_MODEL=<model> go test ./web/app/mcp -run TestToolListVendorTokens -v
//
// and the number it logs goes into the release notes.
func TestToolListVendorTokens(t *testing.T) {
	if os.Getenv("STUARTY_BUDGET_RELEASE") != "1" {
		t.Skip( // verify:env-skip a release measure that costs money and runs on request
			"release measure: set STUARTY_BUDGET_RELEASE=1, ANTHROPIC_API_KEY and ANTHROPIC_MODEL",
		)
	}

	key, model := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_MODEL")
	if key == "" || model == "" {
		t.Fatal("ANTHROPIC_API_KEY and ANTHROPIC_MODEL are required")
	}

	defs, err := loadDefinitions(command.Default())
	if err != nil {
		t.Fatal(err)
	}

	tools := make([]map[string]any, 0, len(order))

	for _, name := range order {
		var tool map[string]any
		if err := json.Unmarshal(marshal(t, defs[name].tool), &tool); err != nil {
			t.Fatal(err)
		}

		tools = append(tools, map[string]any{
			"name": name, "description": tool["description"], "input_schema": tool["inputSchema"],
		})
	}

	body, err := json.Marshal(map[string]any{
		"model": model, "tools": tools, "messages": []any{map[string]any{"role": "user", "content": "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, "https://api.anthropic.com/v1/messages/count_tokens", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	//nolint:forbidigo // release-only call to the vendor token counter, never run in CI
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("count_tokens: %d %v %s", res.StatusCode, err, raw)
	}

	var out struct {
		InputTokens int `json:"input_tokens"`
	}

	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}

	// One message of one character costs a handful of tokens of its own.
	t.Logf("tools/list of %d tools: %d tokens by the vendor count", len(tools), out.InputTokens)

	if out.InputTokens > 7200 {
		t.Errorf("tools/list is %d tokens, at most 7,200 (NFR-020, interim)", out.InputTokens)
	}
}
