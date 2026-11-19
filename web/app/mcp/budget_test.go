package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// charsPerToken is the estimate of SDD-003 section 3.10: tokens are characters
// divided by 3.2 everywhere.
const charsPerToken = 3.2

func tokens(s string) int { return int(float64(len([]rune(s))) / charsPerToken) }

// TestToolDefinitionBudget is NFR-020: the tool definitions total at most 7,200
// tokens. The interim cap of SDD-003 section 4.1 is 21,500 bytes of minified JSON for
// all 16 tools of the ALPHA build, 2,700 for edit and 1,600 for any other; the
// 12 tools of the Prototype are measured here.
func TestToolDefinitionBudget(t *testing.T) {
	t.Parallel()

	defs, err := loadDefinitions(command.Default())
	if err != nil {
		t.Fatal(err)
	}

	total := 0

	for _, name := range order {
		raw := marshal(t, defs[name].tool)
		total += len(raw)

		limit := 1600
		if name == "edit" {
			limit = 2700
		}

		if len(raw) > limit {
			t.Errorf("%s is %d bytes, at most %d", name, len(raw), limit)
		}
	}

	if total > 21500 {
		t.Errorf("the definitions are %d bytes, at most 21,500", total)
	}

	if tk := tokens(strings.Repeat("x", total)); tk > 7200 {
		t.Errorf("the definitions are about %d tokens, at most 7,200 (NFR-020, interim)", tk)
	}

	t.Logf("%d Prototype definitions: %d bytes, about %d tokens", len(order), total, tokens(strings.Repeat("x", total)))
}

// TestConciseViewOfAnItemHeavyProjectFitsTwoPages is NFR-021: the concise view of
// a project of 200 items takes two pages of at most 4,000 tokens each.
func TestConciseViewOfAnItemHeavyProjectFitsTwoPages(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "x")

	id, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{"name": "Big"})))["project_id"].(string)

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
