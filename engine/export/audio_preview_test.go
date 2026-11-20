package export

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const chainItem = "itm_chain00001"

func previewSourceOf(t *testing.T, doc *timeline.Document, from, to int) plan.AudioSource {
	t.Helper()

	src, ok := plan.PreviewSource(doc, chainItem, from, to)
	if !ok {
		t.Fatal("the item has no preview source")
	}

	return src
}

func previewKeyOf(t *testing.T, doc *timeline.Document) string {
	t.Helper()

	src := previewSourceOf(t, doc, 0, 1<<20)

	return plan.PreviewKey(doc, &src)
}

// The key is the content of the audio entry of the item and nothing else: the same chain gives the same key, a changed
// parameter gives another, and an edit of another item, of the volume or of the position leaves it.
func TestAudioPreviewKey(t *testing.T) {
	voice := withItemChain(timeline.PresetChain("voice")...)
	base := previewKeyOf(t, chainDoc(t, voice))

	if len(base) != 64 {
		t.Fatalf("the key %q is not 64 hex characters", base)
	}

	if again := previewKeyOf(t, chainDoc(t, voice)); again != base {
		t.Errorf("the same input gave two keys: %s and %s", base, again)
	}

	if none := previewKeyOf(t, chainDoc(t, nil)); none == base {
		t.Error("an item with no chain has the key of an item with one")
	}

	eq := func(hz float64) func(*timeline.Document) {
		return withItemChain(timeline.Processor{Kind: timeline.ProcEQ, Params: map[string]float64{"hp_hz": hz}})
	}

	if previewKeyOf(t, chainDoc(t, eq(80))) == previewKeyOf(t, chainDoc(t, eq(120))) {
		t.Error("a changed parameter kept the key")
	}

	same := previewKeyOf(t, chainDoc(t, func(d *timeline.Document) {
		voice(d)

		// An edit of another item, the volume and the fades of this one, its track volume, and a second track
		// with a chain of its own are not in the file.
		for id := range d.Items {
			if id != chainItem {
				it := d.Items[id]
				it.Start += 3
				d.Items[id] = it
			}
		}

		it := d.Items[chainItem]
		it.Volume = 0.5
		it.FadeIn = 10
		d.Items[it.ID] = it

		tr := d.Tracks[chainTrack]
		v := 0.3
		tr.Volume = &v
		tr.Processors = []timeline.Processor{{Kind: timeline.ProcLimiter, Params: map[string]float64{"ceiling_db": -3}}}
		d.Tracks[chainTrack] = tr
	}))
	if same != base {
		t.Error("an edit outside the audio entry changed the key")
	}

	doc := chainDoc(t, voice)
	whole := previewSourceOf(t, doc, 0, 1<<20)
	window := previewSourceOf(t, doc, 10, 40)

	if plan.PreviewKey(doc, &window) == plan.PreviewKey(doc, &whole) {
		t.Error("a window has the key of the whole item")
	}

	if window.StartSample != 0 || window.Gain != nil || window.BusChain != nil || window.Track != "" {
		t.Errorf("the preview source keeps the envelope or the place: %+v", window)
	}
}

func renderPreview(t *testing.T, doc *timeline.Document, from, to int, name string) []byte {
	t.Helper()

	job := newJob(t, doc, "preview-"+name, "unused.wav")
	src := previewSourceOf(t, doc, from, to)
	out := filepath.Join(shared.dir, "preview-"+name+".wav")

	_ = os.Remove(out)

	if err := RenderAudioPreview(context.Background(), job, src, out); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if want := 44 + int(src.Samples)*4; len(b) != want {
		t.Fatalf("the file has %d bytes, want %d", len(b), want)
	}

	return b
}

// The render of an item before the envelope and the loudness gain is deterministic, holds the processors, and a
// window of it is a file of its own.
func TestAudioChainPreviewRender(t *testing.T) {
	doc := chainDoc(t, withItemChain(timeline.PresetChain("voice")...))

	whole := renderPreview(t, doc, 0, 1<<20, "whole")

	if !bytes.Equal(whole, renderPreview(t, doc, 0, 1<<20, "whole2")) {
		t.Error("two renders of one source differ")
	}

	if bytes.Equal(whole, renderPreview(t, chainDoc(t, nil), 0, 1<<20, "plain")) {
		t.Error("the processors changed nothing")
	}

	window := renderPreview(t, doc, 15, 45, "window")
	if len(window) >= len(whole) {
		t.Errorf("the window (%d bytes) is not shorter than the item (%d)", len(window), len(whole))
	}
}
