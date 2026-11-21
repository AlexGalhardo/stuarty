package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// golden compares text with testdata/untrusted/<name>; UPDATE_GOLDEN=1 rewrites
// the file after a deliberate change of a format of SDD-003 section 3.
func golden(t *testing.T, name, text string) {
	t.Helper()

	path := filepath.Join("testdata", "untrusted", name)
	text = strings.ReplaceAll(text, "\r\n", "\n") + "\n"

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (UPDATE_GOLDEN=1 go test ./web/app/mcp/ records it)", err)
	}

	if got := text; got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

// TestInspectionTextGolden: the inspection text with every section, the media
// list, and the history list match their golden files.
func TestInspectionTextGolden(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	env := testenv.New(t, testenv.Options{Clock: func() time.Time { return at }})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project := createProjectWith(t, cs, "Golden")

	imported := ok(t, call(t, cs, "media_import", map[string]any{
		"project_id": project, "paths": []any{env.WriteFile(t, "clip.mov", strings.Repeat("x", 64))},
		"asset_ids": []any{"ast_3c5v8n2w6d"}, "wait_ms": 20000,
	}))

	golden(t, "media_import.golden", textOf(t, imported))

	inspected := ok(t, call(t, cs, "media_inspect", map[string]any{
		"project_id": project, "asset_id": "ast_3c5v8n2w6d",
		"sections": []any{"probe", "transcript", "silences", "loudness"},
	}))
	golden(t, "media_inspect.golden", textOf(t, inspected))

	listed := ok(t, call(t, cs, "media_inspect", map[string]any{"project_id": project}))
	golden(t, "media_list.golden", textOf(t, listed))

	ok(t, call(t, cs, "edit", map[string]any{
		"project_id": project, "base_revision": 1,
		"operations": []any{map[string]any{"op": "track.add", "args": map[string]any{
			"kind": "visual", "name": "Video", "id": "trk_f6g7h8j9k0",
		}}},
	}))

	golden(t, "history.golden", textOf(t, ok(t, call(t, cs, "history", map[string]any{
		"project_id": project, "action": "list",
	}))))
}

// metadataGoldenTags is the benign metadata of the metadata golden; the CLI test reads
// the same golden through its own fixture.
var metadataGoldenTags = map[string]string{
	"artist": "Jane Roe", "comment": "shot on a phone", "creation_time": "2026-11-14T12:00:00Z",
	"encoder": "Lavf61.7.100", "title": "Beach day",
}

// TestInspectionMetadataGolden: media_inspect of a file with metadata tags
// matches its golden file, the count line and the untrusted block included.
func TestInspectionMetadataGolden(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project := createProjectWith(t, cs, "Golden")

	clip := env.WriteFile(t, "clip.mov", testenv.FakeMediaTags(2, 320, 240, metadataGoldenTags))
	ok(t, call(t, cs, "media_import", map[string]any{
		"project_id": project, "paths": []any{clip}, "asset_ids": []any{"ast_3c5v8n2w6d"}, "wait_ms": 20000,
	}))

	golden(t, "media_inspect_metadata.golden", textOf(t, ok(t, call(t, cs, "media_inspect", map[string]any{
		"project_id": project, "asset_id": "ast_3c5v8n2w6d",
	}))))
}

// TestMediaListPaging: the pages of a paged media list read in order are the
// unpaged list (§9 check 7). A media cursor is an offset and has no revision to
// go stale; the project view cursor is tested in TestProjectViewCursorGoesStale.
func TestMediaListPaging(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project := createProject(t, cs, "Paged")

	const total = 7

	paths := make([]any, 0, total)
	for i := range total {
		paths = append(paths, env.WriteFile(t, fmt.Sprintf("clip-%02d.mov", i), strings.Repeat("y", 64+i)))
	}

	ok(t, call(t, cs, "media_import", map[string]any{"project_id": project, "paths": paths, "wait_ms": 20000}))

	ids := func(res map[string]any) []string {
		var out []string

		list, _ := res["assets"].([]any)
		for _, a := range list {
			m, _ := a.(map[string]any)
			id, _ := m["asset_id"].(string)
			out = append(out, id)
		}

		return out
	}

	all := ids(structured(t, ok(t, call(t, cs, "media_inspect", map[string]any{"project_id": project}))))
	if len(all) != total {
		t.Fatalf("the unpaged list has %d assets, want %d", len(all), total)
	}

	var paged []string

	cursor := ""

	for range total + 1 {
		args := map[string]any{"project_id": project, "limit": 3}
		if cursor != "" {
			args["cursor"] = cursor
		}

		page := structured(t, ok(t, call(t, cs, "media_inspect", args)))
		paged = append(paged, ids(page)...)

		next, _ := page["next_cursor"].(string)
		if next == "" {
			break
		}

		cursor = next
	}

	if strings.Join(paged, ",") != strings.Join(all, ",") {
		t.Errorf("the pages are %v, the unpaged list is %v", paged, all)
	}
}

// TestProjectViewCursorGoesStale: the concatenated pages of the concise view are
// the whole project, and a cursor issued before an edit is E_VALIDATION with
// details.reason stale (§9 check 7).
func TestProjectViewCursorGoesStale(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	id := createProject(t, cs, "Big")

	ops := []any{map[string]any{
		"op": "track.add", "args": map[string]any{"kind": "visual", "name": "Text", "id": "trk_0000000099"},
	}}

	for i := range 199 {
		ops = append(ops, map[string]any{"op": "text.add", "args": map[string]any{
			"track_id": "trk_0000000099", "start": fmt.Sprintf("%df", i*30), "duration": "30f",
			"text": fmt.Sprintf("Caption number %d", i),
		}})
	}

	ok(t, call(t, cs, "edit", map[string]any{"project_id": id, "base_revision": 0, "operations": ops}))

	first := ok(t, call(t, cs, "project_get", map[string]any{"project_id": id, "limit": 50}))
	cursor, _ := structured(t, first)["next_cursor"].(string)

	if cursor == "" {
		t.Fatal("a 200 item project did not page")
	}

	ok(t, call(t, cs, "edit", map[string]any{
		"project_id": id, "base_revision": 1,
		"operations": []any{map[string]any{"op": "project.rename", "args": map[string]any{"name": "Renamed"}}},
	}))

	res := call(t, cs, "project_get", map[string]any{"project_id": id, "limit": 50, "cursor": cursor})
	text := textOf(t, res)
	if !res.IsError || !strings.Contains(text, "E_VALIDATION") || !strings.Contains(text, "stale") {
		t.Errorf("a stale cursor: %s", text)
	}
}
