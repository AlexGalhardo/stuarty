package mcp

import (
	"strings"
	"testing"
)

// TestLintWithAPresetChecksTheRange: lint with a preset is ExportCheck, and start and end check a range of the
// timeline, as export:check and GET /projects/{id}/export-check do, so an agent can find the part that fits a
// network before it exports it. A range without a preset is refused, never ignored.
func TestLintWithAPresetChecksTheRange(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	id, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{"name": "Long"})))["project_id"].(string)

	ok(t, call(t, cs, "edit", map[string]any{"project_id": id, "base_revision": 0, "operations": []any{
		map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual", "id": "trk_0000000099"}},
		map[string]any{"op": "text.add", "args": map[string]any{
			"track_id": "trk_0000000099", "start": "0s", "duration": "700s", "text": "long",
		}},
	}}))

	whole := structured(t, ok(t, call(t, cs, "lint", map[string]any{"project_id": id, "preset": "tiktok"})))
	if whole["ok"] != false || !strings.Contains(textOf(t, call(t, cs, "lint", map[string]any{
		"project_id": id, "preset": "tiktok",
	})), "X_DURATION_TOO_LONG") {
		t.Fatalf("the whole 700 s timeline against tiktok (600 s): %v", whole)
	}

	part := structured(t, ok(t, call(t, cs, "lint", map[string]any{
		"project_id": id, "preset": "tiktok", "start": "0s", "end": "60s",
	})))
	if part["ok"] != true || part["duration_ms"] != float64(60000) {
		t.Fatalf("the first minute against tiktok: %v", part)
	}

	if res := call(t, cs, "lint", map[string]any{"project_id": id, "start": "0s"}); !res.IsError ||
		!strings.Contains(textOf(t, res), "E_VALIDATION") {
		t.Errorf("a range without a preset: %s", textOf(t, res))
	}
}
