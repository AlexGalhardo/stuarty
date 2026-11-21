package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// charsPerToken is the estimate of SDD-003 section 3.10: tokens are characters
// divided by 3.2 everywhere.
const charsPerToken = 3.2

func tokens(s string) int { return int(float64(len([]rune(s))) / charsPerToken) }

// TestToolDefinitionBudget is NFR-020 as revised by epic 13: the definitions total at most MaxToolListBytes
// bytes of minified JSON (7,000 tokens), no tool is over MaxToolBytes, and the list has at most MaxTools tools
// (SDD-003 section 4.1). internal/tools/surfacebudget measures the list a client receives; this measures the
// definitions themselves.
func TestToolDefinitionBudget(t *testing.T) {
	t.Parallel()

	defs, err := loadDefinitions()
	if err != nil {
		t.Fatal(err)
	}

	total := 0

	for _, name := range order {
		raw := marshal(t, defs[name].tool)
		total += len(raw)

		if len(raw) > MaxToolBytes {
			t.Errorf("%s is %d bytes, at most %d", name, len(raw), MaxToolBytes)
		}
	}

	if len(order) > MaxTools {
		t.Errorf(
			"%d tools, at most %d: a domain adds operations, not tools (SDD-003 section 4.8)",
			len(order),
			MaxTools,
		)
	}

	if total > MaxToolListBytes {
		t.Errorf("the definitions are %d bytes, at most %d", total, MaxToolListBytes)
	}

	if tk := tokens(strings.Repeat("x", total)); tk > 7300 {
		t.Errorf("the definitions are about %d tokens, at most 7,300 (NFR-020)", tk)
	}

	t.Logf("%d Prototype definitions: %d bytes, about %d tokens", len(order), total, tokens(strings.Repeat("x", total)))
}

// TestConciseViewOfAnItemHeavyProjectFitsTwoPages is NFR-021: the concise view of
// a project of 200 items takes two pages of at most 4,000 tokens each.
func TestConciseViewOfAnItemHeavyProjectFitsTwoPages(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "x")

	id := createProject(t, cs, "Big")

	ops := []any{
		map[string]any{
			"op":   "track.add",
			"args": map[string]any{"kind": "visual", "name": "Text", "id": "trk_0000000099"},
		},
	}
	for i := range 199 {
		ops = append(ops, map[string]any{"op": "text.add", "args": map[string]any{
			"track_id": "trk_0000000099",
			"start":    fmt.Sprintf("%df", i*30),
			"duration": "30f",
			"text":     fmt.Sprintf("Caption number %d", i),
		}})
	}

	ok(t, call(t, cs, "edit", map[string]any{"project_id": id, "base_revision": 0, "operations": ops}))

	pages, items := 0, 0
	cursor := ""

	for {
		args := map[string]any{"project_id": id}
		if cursor != "" {
			args["cursor"] = cursor
		}

		res := ok(t, call(t, cs, "project_get", args))
		text := textOf(t, res)
		pages++

		if tk := tokens(text); tk > 4000 {
			t.Errorf("page %d is about %d tokens, at most 4,000", pages, tk)
		}

		items += strings.Count(text, "\nitm_")

		next, _ := structured(t, res)["next_cursor"].(string)
		if next == "" {
			break
		}

		cursor = next

		if pages > 5 {
			t.Fatal("too many pages")
		}
	}

	if pages > 2 || items != 199 {
		t.Fatalf("%d pages and %d items, want at most 2 pages and 199 items", pages, items)
	}
}

// D-088: project_get takes detail document over MCP as the CLI and the HTTP API do, and the description says what it
// costs, because that one call is outside the per-call caps of SDD-003 section 3.10.
func TestProjectGetDescriptionStatesTheCostOfTheDocument(t *testing.T) {
	t.Parallel()

	defs, err := loadDefinitions()
	if err != nil {
		t.Fatal(err)
	}

	d := defs["project_get"].tool.Description
	for _, want := range []string{"document", "unbounded", "tokens"} {
		if !strings.Contains(d, want) {
			t.Errorf("the description of project_get does not say %q: %s", want, d)
		}
	}
}
