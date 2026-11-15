package mcp

import (
	"bytes"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/vukem-video/web/contract/testenv"
)

// textProject is a project with one text item of 1 s, so that a frame has
// something to show.
func textProject(t *testing.T, cs *sdk.ClientSession) string {
	t.Helper()

	project, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{"name": "V"})))["project_id"].(string)

	tr := ok(t, call(t, cs, "edit", map[string]any{
		"project_id": project, "base_revision": 0,
		"operations": []any{map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual", "name": "V"}}},
	}))

	created, _ := structured(t, tr)["created"].([]any)
	if len(created) != 1 {
		t.Fatalf("created = %v", created)
	}

	ok(t, call(t, cs, "edit", map[string]any{
		"project_id": project, "base_revision": 1,
		"operations": []any{map[string]any{
			"op":   "text.add",
			"args": map[string]any{"track_id": created[0], "start": "0s", "duration": "1s", "text": "hello"},
		}},
	}))

	return project
}

func imagesOf(res *sdk.CallToolResult) []*sdk.ImageContent {
	var out []*sdk.ImageContent

	for _, c := range res.Content {
		if im, isImage := c.(*sdk.ImageContent); isImage {
			out = append(out, im)
		}
	}

	return out
}

func TestViewReturnsAnImageOnlyFromView(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{Frames: true})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project := textProject(t, cs)

	frame := ok(t, call(t, cs, "view", map[string]any{"project_id": project, "at": "0.5s"}))

	imgs := imagesOf(frame)
	if len(frame.Content) != 2 || len(imgs) != 1 || imgs[0].MIMEType != "image/jpeg" ||
		!bytes.HasPrefix(imgs[0].Data, []byte{0xff, 0xd8}) {
		t.Fatalf("a frame is a text block and one jpeg image block: %+v", frame.Content)
	}

	sc := structured(t, frame)
	if sc["mode"] != "frame" || sc["revision"] != float64(2) || sc["path"] == "" {
		t.Errorf("structured content = %v", sc)
	}

	for _, k := range []string{"image_base64", "text"} {
		if _, has := sc[k]; has {
			t.Errorf("structured content must not carry %s", k)
		}
	}

	sheet := ok(t, call(t, cs, "view", map[string]any{
		"project_id": project, "mode": "sheet", "times": []any{"0s", "0.5s"},
	}))
	if len(imagesOf(sheet)) != 1 || structured(t, sheet)["mode"] != "sheet" {
		t.Errorf("a sheet is one image: %+v", sheet.Content)
	}

	bad := call(t, cs, "view", map[string]any{"project_id": project, "mode": "sheet", "at": "0s"})
	if !bad.IsError || len(imagesOf(bad)) != 0 {
		t.Errorf("at in sheet mode must be an error without an image")
	}

	// No other tool returns an image.
	for _, name := range []string{"project_get", "lint", "history"} {
		if got := imagesOf(ok(t, call(t, cs, name, map[string]any{"project_id": project}))); len(got) != 0 {
			t.Errorf("%s returned an image", name)
		}
	}
}
