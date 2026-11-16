package export

import (
	"bytes"
	"context"
	"testing"
)

// A renderer reused for several frames (a contact sheet) returns what a fresh one returns: a
// single-frame decoder that already delivered its frame must restart for the next source frame.
func TestSingleFrameRendererFollowsTheSourceFrame(t *testing.T) {
	doc := loadScene(t, "clip-caption")
	const scale = 0.25

	render := func(id string, frames ...int) []byte {
		t.Helper()

		rd, err := NewRenderer(context.Background(), RenderConfig{
			Runner: shared.runner, Doc: doc, JobID: id, ArtifactsDir: artifactsDir(),
		})
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = rd.Close() }()

		var pix []byte

		for _, f := range frames {
			img, err := rd.Render(context.Background(), f, scale)
			if err != nil {
				t.Fatal(err)
			}

			pix = bytes.Clone(img.Pix)
		}

		return pix
	}

	const a, b = 0, 30

	got := render("single-reused", a, b)
	want := render("single-fresh-b", b)

	if bytes.Equal(render("single-fresh-a", a), want) {
		t.Fatal("frames a and b look the same; pick frames where the clip changes")
	}

	if !bytes.Equal(got, want) {
		t.Error("a reused renderer returned a different picture for frame b than a fresh one")
	}
}
