package mcp

import (
	"bytes"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// textProject is a project with one text item of 1 s, so that a frame has
// something to show.
func textProject(tb testing.TB, cs *sdk.ClientSession) string {
	tb.Helper()

	project, _ := structured(tb, ok(tb, call(tb, cs, "project_create", map[string]any{"name": "V"})))["project_id"].(string)

	tr := ok(tb, call(tb, cs, "edit", map[string]any{
		"project_id": project, "base_revision": 0,
		"operations": []any{map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual", "name": "V"}}},
	}))

	created, _ := structured(tb, tr)["created"].([]any)
	if len(created) != 1 {
		tb.Fatalf("created = %v", created)
	}

	ok(tb, call(tb, cs, "edit", map[string]any{
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

// TestViewImageSizeTable is SDD-003 section 3.11 through the tool (§9 check 8):
// the default frame and the default sheet of the four presets have the size and
// the image cost of the table, under the 800 and 1,500 tokens of NFR-022. The
// function itself is pinned by engine/frame TestImageSizeTable; this is the tool
// reporting what the service rendered.
func TestViewImageSizeTable(t *testing.T) {
	t.Parallel()

	presets := []struct {
		name          string
		width, height int
		frame, sheet  [3]float64
	}{
		{"9:16", 1080, 1920, [3]float64{582, 1036, 777}, [3]float64{804, 1428, 1479}},
		{"1:1", 1080, 1080, [3]float64{784, 784, 784}, [3]float64{1064, 1064, 1444}},
		{"16:9", 1920, 1080, [3]float64{1036, 582, 777}, [3]float64{1428, 804, 1479}},
		{"4:5", 1080, 1350, [3]float64{694, 868, 775}, [3]float64{940, 1176, 1428}},
	}

	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()

			env := testenv.New(t, testenv.Options{Frames: true})
			cs := connect(t, env.Service, Options{}, "claude-code")
			project := textProject(t, cs)

			ok(t, call(t, cs, "edit", map[string]any{
				"project_id": project, "base_revision": 2,
				"operations": []any{map[string]any{
					"op": "settings.set", "args": map[string]any{"width": p.width, "height": p.height},
				}},
			}))

			frame := structured(t, ok(t, call(t, cs, "view", map[string]any{"project_id": project, "at": "0.5s"})))
			sheet := structured(t, ok(t, call(t, cs, "view", map[string]any{
				"project_id": project, "mode": "sheet", "from": "0s", "until": "0.9s", "count": 9,
			})))

			for name, got := range map[string]map[string]any{"frame": frame, "sheet": sheet} {
				want := p.frame
				if name == "sheet" {
					want = p.sheet
				}

				if got["height"] != want[1] || got["width"] != want[0] || got["image_tokens"] != want[2] {
					t.Errorf("%s of %s: %vx%v, %v tokens, want %vx%v, %v", name, p.name,
						got["width"], got["height"], got["image_tokens"], want[0], want[1], want[2])
				}
			}
		})
	}
}
