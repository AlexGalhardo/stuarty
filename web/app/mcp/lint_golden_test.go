package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// TestLintReportGolden: the lint report of a project with an overlap of sound
// and a timeline longer than 180 s matches its golden file, with the measurer
// of the owner (the 320x240 clips cover the frame, so L_OFF_FRAME fires). The
// rules that read a stored artifact are in TestLintArtifactRulesGolden.
func TestLintReportGolden(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	env := testenv.New(t, testenv.Options{Clock: func() time.Time { return at }, DurationMS: 300000})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{
		"name": "Lint", "id": "prj_7k2m9x4q1b",
	})))["project_id"].(string)
	imported := ok(t, call(t, cs, "media_import", map[string]any{
		"project_id": project, "paths": []any{env.WriteFile(t, "clip.mov", strings.Repeat("x", 64))},
		"asset_ids": []any{"ast_3c5v8n2w6d"}, "wait_ms": 20000,
	}))

	if assets, _ := structured(t, imported)["assets"].([]any); len(assets) != 1 {
		t.Fatalf("import: %s", textOf(t, imported))
	}

	clip := func(id, track, start, duration string) any {
		return map[string]any{"op": "clip.add", "args": map[string]any{
			"track_id": track, "asset_id": "ast_3c5v8n2w6d", "start": start, "duration": duration, "id": id,
		}}
	}

	ok(t, call(t, cs, "edit", map[string]any{
		"project_id": project, "base_revision": 1,
		"operations": []any{
			map[string]any{
				"op":   "track.add",
				"args": map[string]any{"kind": "visual", "name": "Base", "id": "trk_f6g7h8j9k0"},
			},
			map[string]any{
				"op":   "track.add",
				"args": map[string]any{"kind": "visual", "name": "Top", "id": "trk_m1n2p3q4r5"},
			},
			clip("itm_a1b2c3d4e5", "trk_f6g7h8j9k0", "0s", "190s"),
			clip("itm_f6g7h8j9k0", "trk_m1n2p3q4r5", "10s", "5s"),
		},
	}))

	report := textOf(t, ok(t, call(t, cs, "lint", map[string]any{"project_id": project})))

	for _, code := range []string{"L_AUDIO_OVERLAP", "L_DURATION_LIMIT"} {
		if !strings.Contains(report, code) {
			t.Errorf("the report lacks %s:\n%s", code, report)
		}
	}

	golden(t, "lint.golden", report)
}
