package mcp

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// The library through the media tools (SDD-011 section 14.5, epic 08): an import with an origin and tags, the
// listing by tag and reuse mark, the reuse by hash, the edit of an entry, and the refusals.
func TestMediaToolsReachTheLibrary(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")
	mk := func(name string) string {
		id := createProject(t, cs, name)

		return id
	}
	one, two := mk("One"), mk("Two")
	clip := env.WriteFile(t, "clip.mov", "not really a video")

	refused := call(t, cs, "media_import", map[string]any{
		"project_id": one,
		"paths":      []any{clip},
		"origin":     map[string]any{"kind": "fetched_by_agent", "source": "https://x.test/a"},
	})
	if !refused.IsError || !strings.Contains(textOf(t, refused), "rights_note") {
		t.Fatalf("a fetched file with no rights note was accepted: %s", textOf(t, refused))
	}

	unknown := call(t, cs, "media_import", map[string]any{
		"project_id": one, "paths": []any{clip}, "origin": map[string]any{"kind": "imported", "colour": "red"},
	})
	if !unknown.IsError {
		t.Error("an unknown member of the origin was accepted")
	}

	imp := ok(t, call(t, cs, "media_import", map[string]any{
		"project_id": one, "paths": []any{clip}, "wait_ms": 10000,
		"origin": map[string]any{"kind": "fetched_by_agent", "source": "https://x.test/a", "rights_note": "CC0"},
		"info":   map[string]any{"tags": []any{"B-Roll"}, "reusable": true},
	}))

	assets, _ := structured(t, imp)["assets"].([]any)
	hash, _ := assets[0].(map[string]any)["hash"].(string)

	if hash == "" {
		t.Fatalf("no hash in %v", assets)
	}

	list := ok(t, call(t, cs, "media_inspect", map[string]any{
		"library": map[string]any{"tag": "b-roll", "reusable": true},
	}))

	found, _ := structured(t, list)["assets"].([]any)
	if len(found) != 1 || found[0].(map[string]any)["hash"] != hash {
		t.Fatalf("library list = %v", structured(t, list))
	}

	if bad := call(t, cs, "media_inspect", map[string]any{"library": map[string]any{"tagg": "x"}}); !bad.IsError {
		t.Error("an unknown library filter was accepted")
	}

	reuse := ok(
		t,
		call(t, cs, "media_import", map[string]any{"project_id": two, "paths": []any{hash}, "wait_ms": 10000}),
	)
	if r, _ := structured(t, reuse)["assets"].([]any); len(r) != 1 || r[0].(map[string]any)["state"] != "ready" {
		t.Fatalf("reuse = %v", structured(t, reuse))
	}

	edit := ok(t, call(t, cs, "media_import", map[string]any{
		"hash": hash, "info": map[string]any{"tags": []any{"harbour"}, "description": "boats"},
	}))
	if tags, _ := structured(t, edit)["tags"].([]any); len(tags) != 2 {
		t.Errorf("edit = %v", structured(t, edit))
	}

	detail := ok(t, call(t, cs, "media_inspect", map[string]any{"hash": hash}))
	txt := textOf(t, detail)

	if uses, _ := structured(t, detail)["uses"].([]any); len(uses) != 2 {
		t.Errorf("the detail names %d uses, want 2", len(uses))
	}

	if !strings.Contains(txt, `field="rights_note"`) || !strings.Contains(txt, `field="source"`) {
		t.Errorf("source and rights note are not delimited:\n%s", txt)
	}

	// With no project and no filter the whole library is listed.
	all := ok(t, call(t, cs, "media_inspect", map[string]any{}))
	if f, _ := structured(t, all)["assets"].([]any); len(f) != 1 {
		t.Errorf("the whole library = %v", structured(t, all))
	}
}
