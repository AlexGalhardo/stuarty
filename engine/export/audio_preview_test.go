package export

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/audio"
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

// The time to render the window of two seconds at the playhead, with the "voice" chain, on the noisy tone of the
// fixture (SDD-004 section 7.1; acceptance criterion 1 of epic 51 is three seconds). On a loaded machine the number is
// only logged against the budget, never judged by a tighter one.
func TestAudioPreviewWindowRenderLatency(t *testing.T) {
	doc := chainDoc(t, withItemChain(timeline.PresetChain("voice")...))
	job := newJob(t, doc, "preview-latency", "unused.wav")
	fps := doc.Settings.FPS
	window := (2*fps.Num + fps.Den - 1) / fps.Den
	src := previewSourceOf(t, doc, 0, window)
	out := filepath.Join(shared.dir, "preview-latency.wav")

	_ = os.Remove(out)

	start := time.Now()

	if err := RenderAudioPreview(context.Background(), job, src, out); err != nil {
		t.Fatal(err)
	}

	took := time.Since(start)
	t.Logf("window of %d frames (2 s): %v", window, took)

	if took > 3*time.Second {
		t.Errorf("the window took %v, the budget is 3s", took)
	}
}

// What the timeline plays for a whole item is what the export mixes: the processed file times the gain of the plan
// (volume and track volume) equals the export's float mix before the loudness gain, within 2 steps of the 16-bit
// scale (SDD-004 section 7.1; the fades are per frame in the plan and per sample in the mix, so they are left out).
func TestAudioChainExportEqualsPreviewWithTheEnvelope(t *testing.T) {
	doc := chainDoc(t, func(d *timeline.Document) {
		withItemChain(timeline.PresetChain("voice")...)(d)

		it := d.Items[chainItem]
		it.Volume = 0.6
		d.Items[it.ID] = it

		tr := d.Tracks[chainTrack]
		v := 0.5
		tr.Volume = &v
		d.Tracks[chainTrack] = tr
	})

	end := doc.Items[chainItem].Duration
	job := newJob(t, doc, "env-export", "unused.wav")
	s := testState(t, job, "env-export")

	var mixed []float32

	srcs := plan.Audio(doc, 0, end)
	total := samplesAt(doc.Settings.FPS, end)

	err := audio.Mix(context.Background(), srcs, total, &audioOpener{s: s}, func(block []float32) error {
		mixed = append(mixed, block...)

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	file := renderPreview(t, doc, 0, end, "envelope")
	gains := plan.Window(doc, 0, end).Audio[0].Gain

	if int64(len(mixed)/2) != total || len(file) != 44+int(total)*4 {
		t.Fatalf("lengths: mix %d, file %d, total %d", len(mixed)/2, len(file), total)
	}

	fps := doc.Settings.FPS
	worst := 0.0

	for i := range int(total) {
		g := gains[i*fps.Num/(fps.Den*audio.SampleRate)]

		for ch := range 2 {
			pcm := int16(binary.LittleEndian.Uint16(file[44+i*4+ch*2:]))
			got := float64(pcm) / 32768 * g
			worst = max(worst, math.Abs(got-float64(mixed[i*2+ch]))*32768)
		}
	}

	if worst > 2 {
		t.Errorf("the preview differs from the export mix by %.2f steps of the 16-bit scale, want at most 2", worst)
	}

	t.Logf("largest difference between the preview and the export mix: %.3f steps of the 16-bit scale", worst)
}
