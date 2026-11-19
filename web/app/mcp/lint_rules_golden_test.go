package mcp

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// TestLintArtifactRulesGolden: with the rasterizer measurer and the stored
// peaks, loudness and transcript artifacts wired as the owner wires them, the
// lint tool reports L_OCCLUDED, L_SAFE_AREA, L_OFF_FRAME, L_AUDIO_CLIP,
// L_LOUDNESS, L_CAPTION_DESYNC, L_TRANSITION_EMPTY and L_SLOT_UNFILLED (and
// L_AUDIO_OVERLAP) in the text of its golden file. L_DURATION_LIMIT is in
// TestLintReportGolden.
func TestLintArtifactRulesGolden(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{
		"name": "Rules", "id": "prj_7k2m9x4q1b",
	})))["project_id"].(string)
	imported := ok(t, call(t, cs, "media_import", map[string]any{
		"project_id": project, "paths": []any{env.WriteFile(t, "speech.mov", strings.Repeat("x", 64))},
		"asset_ids": []any{"ast_3c5v8n2w6d"}, "wait_ms": 20000,
	}))

	if assets, _ := structured(t, imported)["assets"].([]any); len(assets) != 1 {
		t.Fatalf("import: %s", textOf(t, imported))
	}

	env.WriteLoudPeaks(t, project, "ast_3c5v8n2w6d")
	env.PutTranscript(t, project, "ast_3c5v8n2w6d")

	revision := 1

	edit := func(ops ...any) map[string]any {
		t.Helper()

		out := structured(t, ok(t, call(t, cs, "edit", map[string]any{
			"project_id": project, "base_revision": revision, "operations": ops,
		})))
		if r, isNum := out["revision"].(float64); isNum {
			revision = int(r)
		}

		return out
	}
	op := func(name string, args map[string]any) any { return map[string]any{"op": name, "args": args} }
	track := func(id, name string) any {
		return op("track.add", map[string]any{"kind": "visual", "name": name, "id": id})
	}
	clip := func(id, trk string) any {
		return op("clip.add", map[string]any{
			"track_id": trk, "asset_id": "ast_3c5v8n2w6d", "start": "0s", "duration": "2s", "id": id,
		})
	}
	shape := func(id, trk string, size int) any {
		return op("shape.add", map[string]any{
			"track_id": trk, "start": "2s", "duration": "2s", "shape": "rect", "width": size, "height": size, "id": id,
		})
	}

	edit(
		track("trk_a1b2c3d4e5", "Voice"), track("trk_f6g7h8j9k0", "Voice 2"),
		track("trk_m1n2p3q4r5", "Shapes"), track("trk_s6t7v8w9x0", "Overlay"), track("trk_c1d2e3f4g5", "Captions"),
		clip("itm_a1b2c3d4e5", "trk_a1b2c3d4e5"), clip("itm_f6g7h8j9k0", "trk_f6g7h8j9k0"),
		shape("itm_m1n2p3q4r5", "trk_m1n2p3q4r5", 100), shape("itm_s6t7v8w9x0", "trk_s6t7v8w9x0", 9999),
		op("text.add", map[string]any{
			"track_id": "trk_s6t7v8w9x0", "start": "5s", "duration": "1s", "text": "Edge of the frame",
			"x": 20, "y": 20, "id": "itm_b2c3d4e5f6",
		}),
		op("text.add", map[string]any{
			"track_id": "trk_s6t7v8w9x0",
			"start":    "7s",
			"duration": "1s",
			"text":     "Needs a title",
			"id":       "itm_c3d4e5f6g7",
		}),
		op("settings.set", map[string]any{"loudness_target": -30}),
	)
	made := edit(
		op("item.set_transition", map[string]any{"item_id": "itm_c3d4e5f6g7", "edge": "in", "kind": "fade"}),
		op("slot.mark", map[string]any{"item_id": "itm_c3d4e5f6g7", "name": "title"}),
		op("caption.from_transcript", map[string]any{"clip_item_id": "itm_a1b2c3d4e5", "track_id": "trk_c1d2e3f4g5"}),
	)

	first := firstCaption(t, made)

	edit(op("item.move", map[string]any{"item_id": first, "start": "10f"}))

	report := textOf(t, ok(t, call(t, cs, "lint", map[string]any{"project_id": project, "limit": 100})))

	for _, code := range []string{
		"L_OCCLUDED", "L_SAFE_AREA", "L_OFF_FRAME", "L_AUDIO_CLIP", "L_LOUDNESS", "L_CAPTION_DESYNC",
		"L_TRANSITION_EMPTY", "L_SLOT_UNFILLED", "L_AUDIO_OVERLAP",
	} {
		if !strings.Contains(report, code) {
			t.Errorf("the report lacks %s:\n%s", code, report)
		}
	}

	golden(t, "lint_rules.golden", report)
}

// firstCaption is the first item the caption.from_transcript operation made.
func firstCaption(t *testing.T, edited map[string]any) string {
	t.Helper()

	created, _ := edited["created"].([]any)

	for _, c := range created {
		if id, _ := c.(string); strings.HasPrefix(id, "itm_") {
			return id
		}
	}

	t.Fatalf("the edit created no caption: %v", edited)

	return ""
}

// TestInspectTranscriptUnavailableGolden: without a transcription provider the
// transcript section is the one line `transcript: unavailable
// (E_TRANSCRIBE_UNAVAILABLE): <hint>` under the note, and nothing else.
func TestInspectTranscriptUnavailableGolden(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{
		"name": "Silent", "id": "prj_7k2m9x4q1b",
	})))["project_id"].(string)
	ok(t, call(t, cs, "media_import", map[string]any{
		"project_id": project, "paths": []any{env.WriteFile(t, "clip.mov", strings.Repeat("x", 64))},
		"asset_ids": []any{"ast_3c5v8n2w6d"}, "wait_ms": 20000,
	}))

	text := textOf(t, ok(t, call(t, cs, "media_inspect", map[string]any{
		"project_id": project, "asset_id": "ast_3c5v8n2w6d", "sections": []any{"transcript"},
	})))

	if !strings.Contains(text, "transcript: unavailable (E_TRANSCRIBE_UNAVAILABLE): ") {
		t.Errorf("the transcript line is missing:\n%s", text)
	}

	golden(t, "media_inspect_transcript_unavailable.golden", text)
}
