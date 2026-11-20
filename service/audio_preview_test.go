package service

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/cache"
	"github.com/AlexGalhardo/stuarty/engine/frame"
)

const (
	previewItemA = "itm_aaaaaaaaa1"
	previewItemB = "itm_aaaaaaaaa2"
	previewItemC = "itm_aaaaaaaaa3"
)

// fakePreviewer renders nothing: it writes a few bytes, and records what it was asked.
type fakePreviewer struct {
	Exporter

	mu      sync.Mutex
	samples []int64
	fail    bool
	gate    chan struct{} // when set, a render waits for it to be closed
}

func (f *fakePreviewer) RenderAudioPreview(_ context.Context, in AudioPreviewInput) error {
	if f.gate != nil {
		<-f.gate
	}

	f.mu.Lock()
	f.samples = append(f.samples, in.Source.Samples)
	fail := f.fail
	f.mu.Unlock()

	if fail {
		return errors.New("decoder failed")
	}

	return os.WriteFile(in.OutPath, []byte("RIFF....WAVEfake"), 0o600)
}

func (f *fakePreviewer) renders() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]int64(nil), f.samples...)
}

var voiceChain = []timeline.Processor{{Kind: timeline.ProcEQ, Params: map[string]float64{"hp_hz": 90}}}

// previewEnv is the project of measureEnv with items A and B on one chain over the same range of one asset, and C
// without a chain, and a renderer that writes small files into a cache under the workspace.
func previewEnv(t *testing.T, fake *fakePreviewer) *batchEnv {
	t.Helper()

	env := measureEnv(t, nil)
	store, _ := env.s.store.(*batchStore)

	doc, err := timeline.Decode(store.projects.row.Document)
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{previewItemA, previewItemB, previewItemC} {
		it := timeline.NewItem(id, timeline.ItemAudio, measureTrack, 30, 150)
		it.Asset = "ast_bbbbbbbbbb"

		if id != previewItemC {
			it.Processors = voiceChain
		}

		doc.Items[id] = it
	}

	delete(doc.Items, measureAudioItem)

	store.projects.row.Document = timeline.Canonical(doc)

	dir := t.TempDir()

	cs, err := cache.Open(dir, 0)
	if err != nil {
		t.Fatal(err)
	}

	// The background renders end before the folder is removed (cleanups run last in, first out).
	t.Cleanup(func() {
		deadline := time.Now().Add(20 * time.Second)

		for time.Now().Before(deadline) {
			env.s.previews.mu.Lock()
			idle := env.s.previews.queued == 0
			env.s.previews.mu.Unlock()

			if idle {
				return
			}

			runtime.Gosched()
		}
	})

	env.s.workspace = dir
	env.s.media.Frames = &frame.Frames{Cache: cs}
	fake.Exporter = env.s.exporter
	env.s.exporter = fake

	return env
}

func previewKeys(t *testing.T, env *batchEnv) map[string]*plan.ProcessedAudio {
	t.Helper()

	res, err := env.s.PlanWindow(renderCtx(), PlanWindowRequest{ProjectID: batchProject, From: 0, Until: 300})
	if err != nil {
		t.Fatal(err)
	}

	keys := map[string]*plan.ProcessedAudio{}
	for i := range res.Audio {
		keys[res.Audio[i].Item] = res.Audio[i].Processed
	}

	return keys
}

func openPreview(env *batchEnv, project, key, from string) (*File, error) {
	return env.s.OpenFile(renderCtx(), FileRef{Kind: "audio_preview", ProjectID: project, ID: key, Variant: from})
}

// waitReady asks again until the file is there: the render runs in the background.
func waitReady(t *testing.T, env *batchEnv, key, from string) *File {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)

	for {
		f, err := openPreview(env, batchProject, key, from)
		if err != nil {
			t.Fatal(err)
		}

		if !f.Pending {
			return f
		}

		if time.Now().After(deadline) {
			t.Fatal("the file was not rendered in 20 seconds")
		}

		runtime.Gosched()
	}
}

// A window is rendered first, then the whole item by itself; each is stored once, by its key, and asked for again it
// is served with no new render.
func TestAudioPreviewCache(t *testing.T) {
	t.Parallel()

	fake := &fakePreviewer{}
	env := previewEnv(t, fake)
	keys := previewKeys(t, env)

	a := keys[previewItemA]
	if a == nil || keys[previewItemC] != nil {
		t.Fatalf("processed: %+v", keys)
	}

	// The playhead is at frame 60 of an item that starts at 30: the window is the two seconds from there.
	win := waitReady(t, env, a.Key, "60")

	if win.ContentType != "audio/wav" || win.Header["X-Stuarty-Window"] != "60-119" || !win.Immutable {
		t.Errorf("window file: %+v", win)
	}

	whole := waitReady(t, env, a.Key, "")
	if whole.Header != nil || whole.ETag != `"ap-`+a.Key+`"` {
		t.Errorf("whole file: %+v", whole)
	}

	// The render of the whole item followed the window by itself, with nothing asked for it.
	got := fake.renders()
	if len(got) != 2 || got[0] != 2*48000 || got[1] != 150*48000/30 {
		t.Fatalf("renders (samples) = %v, want the window of 96000 then the item of 240000", got)
	}

	// Asked again, both are served from the store.
	waitReady(t, env, a.Key, "60")
	waitReady(t, env, a.Key, "")

	if n := len(fake.renders()); n != 2 {
		t.Errorf("a stored file was rendered again: %d renders", n)
	}

	// Past the end of the item, before it, or not a number: a validation error, and no render.
	for _, from := range []string{"29", "180", "x", "-1"} {
		_, err := openPreview(env, batchProject, a.Key, from)
		if se, ok := errors.AsType[*Error](err); !ok || se.Code != command.CodeValidation {
			t.Errorf("from %q: %v", from, err)
		}
	}
}

// The window is ready within three seconds of the first request (acceptance criterion 1). The render of this test is
// a fake, so it measures what the service adds (the queue, the store, the answer); the render of the real 2 seconds
// of the reference clip is measured by TestAudioPreviewWindowRenderLatency in engine/export.
func TestAudioPreviewWindowLatency(t *testing.T) {
	t.Parallel()

	fake := &fakePreviewer{}
	env := previewEnv(t, fake)
	a := previewKeys(t, env)[previewItemA]

	start := time.Now()
	waitReady(t, env, a.Key, "60")

	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("the window took %v, the budget is 3s", took)
	}
}

// A window near the end of the item is cut to it.
func TestAudioPreviewCacheWindowIsCutAtTheEndOfTheItem(t *testing.T) {
	t.Parallel()

	fake := &fakePreviewer{}
	env := previewEnv(t, fake)
	a := previewKeys(t, env)[previewItemA]

	win := waitReady(t, env, a.Key, "170")
	if win.Header["X-Stuarty-Window"] != "170-179" {
		t.Errorf("window = %q", win.Header["X-Stuarty-Window"])
	}
}

// A render that fails is an error, is not tried again at once, and leaves no file.
func TestAudioPreviewCacheFailure(t *testing.T) {
	t.Parallel()

	fake := &fakePreviewer{fail: true}
	env := previewEnv(t, fake)
	a := previewKeys(t, env)[previewItemA]

	deadline := time.Now().Add(20 * time.Second)

	for {
		_, err := openPreview(env, batchProject, a.Key, "")
		if se, ok := errors.AsType[*Error](err); ok && se.Code == command.CodeInternal {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("the failed render was not reported: %v", err)
		}

		runtime.Gosched()
	}

	if _, err := openPreview(env, batchProject, a.Key, ""); err == nil {
		t.Error("a failed render was asked again at once")
	}

	if n := len(fake.renders()); n != 1 {
		t.Errorf("renders = %d, want 1", n)
	}
}

// Two items with one chain over one range share one file; an edit of another item, of a volume or of a position
// makes none; a changed parameter makes another (acceptance criterion 2).
func TestAudioPreviewKeyIsPerClip(t *testing.T) {
	t.Parallel()

	fake := &fakePreviewer{}
	env := previewEnv(t, fake)
	keys := previewKeys(t, env)

	a, b := keys[previewItemA], keys[previewItemB]
	if a == nil || b == nil || a.Key != b.Key {
		t.Fatalf("A and B on one chain have two keys: %+v %+v", a, b)
	}

	waitReady(t, env, a.Key, "")
	waitReady(t, env, b.Key, "")

	if n := len(fake.renders()); n != 1 {
		t.Fatalf("two items, one file: renders = %d", n)
	}

	// Edit other items and the volume of A: the keys stay and nothing is rendered.
	store, _ := env.s.store.(*batchStore)

	doc, err := timeline.Decode(store.projects.row.Document)
	if err != nil {
		t.Fatal(err)
	}

	c := doc.Items[previewItemC]
	c.Volume, c.Start = 0.2, 40
	doc.Items[c.ID] = c

	va := doc.Items[previewItemA]
	va.Volume = 0.5
	va.FadeIn = 6
	doc.Items[va.ID] = va

	store.projects.row.Document = timeline.Canonical(doc)

	again := previewKeys(t, env)
	if again[previewItemA].Key != a.Key || again[previewItemB].Key != a.Key {
		t.Error("an edit of another item or of a volume moved a key")
	}

	if _, err := openPreview(env, batchProject, a.Key, ""); err != nil {
		t.Fatal(err)
	}

	if n := len(fake.renders()); n != 1 {
		t.Errorf("an edit outside the entry made a render: %d", n)
	}

	// A parameter of A is another key, and another file.
	vb := doc.Items[previewItemA]
	vb.Processors = []timeline.Processor{{Kind: timeline.ProcEQ, Params: map[string]float64{"hp_hz": 91}}}
	doc.Items[vb.ID] = vb
	store.projects.row.Document = timeline.Canonical(doc)

	changed := previewKeys(t, env)
	if changed[previewItemA].Key == a.Key || changed[previewItemB].Key != a.Key {
		t.Fatalf("a changed parameter: %+v", changed)
	}

	waitReady(t, env, changed[previewItemA].Key, "")

	if n := len(fake.renders()); n != 2 {
		t.Errorf("renders = %d, want 2", n)
	}
}

// A key that the project's plan did not hand out, or one of another project, is not found, and nothing is rendered.
func TestAudioPreviewCacheKeyBelongsToTheProject(t *testing.T) {
	t.Parallel()

	fake := &fakePreviewer{}
	env := previewEnv(t, fake)
	a := previewKeys(t, env)[previewItemA]

	bad := []string{
		"", "short", a.Key[:63], a.Key + "0", "../" + a.Key[3:], a.Key[:63] + "G",
		"0000000000000000000000000000000000000000000000000000000000000000",
	}

	for _, key := range bad {
		_, err := openPreview(env, batchProject, key, "")
		if se, ok := errors.AsType[*Error](err); !ok || se.Code != command.CodeNotFound {
			t.Errorf("key %q: %v", key, err)
		}
	}

	if _, err := openPreview(env, "prj_zzzzzzzzzz", a.Key, ""); err == nil {
		t.Error("a key was served through another project")
	}

	if n := len(fake.renders()); n != 0 {
		t.Errorf("renders = %d", n)
	}
}

// T-90: the key opens the sound only through the project whose plan handed it out, and no crafted key or frame reaches
// another file or starts a render.
func TestSecurity_T90_AudioPreviewKeyIsTheProjects(t *testing.T) {
	t.Parallel()

	fake := &fakePreviewer{}
	env := previewEnv(t, fake)
	a := previewKeys(t, env)[previewItemA]

	for _, key := range []string{
		"../" + a.Key, "..\\" + a.Key, "/" + a.Key[1:], "C:/" + a.Key[3:], a.Key + "/../x", strings.ToUpper(a.Key),
		a.Key[:32] + "/" + a.Key[33:], "audio-previews/" + a.Key[:48], strings.Repeat("0", 64),
	} {
		if _, err := openPreview(env, batchProject, key, ""); err == nil {
			t.Errorf("key %q was served", key)
		}
	}

	for _, from := range []string{"../x", "60/../1", "1e2", " 60", "0x3c", "99999999999999999999"} {
		if _, err := openPreview(env, batchProject, a.Key, from); err == nil {
			t.Errorf("from %q was served", from)
		}
	}

	if _, err := openPreview(env, "prj_zzzzzzzzzz", a.Key, ""); err == nil {
		t.Error("the key of this project opened a sound through another project")
	}

	if n := len(fake.renders()); n != 0 {
		t.Errorf("a refused request rendered: %d", n)
	}
}

// At most sixteen renders wait; one file is rendered once however many ask for it (SDD-007 T-91).
func TestSecurity_T91_AudioPreviewRendersAreBounded(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	fake := &fakePreviewer{gate: gate}
	env := previewEnv(t, fake)
	a := previewKeys(t, env)[previewItemA]

	// The same window asked for many times is one render.
	for range 50 {
		if _, err := openPreview(env, batchProject, a.Key, "60"); err != nil {
			t.Fatal(err)
		}
	}

	var full *Error

	for from := 31; from < 170 && full == nil; from++ {
		_, err := openPreview(env, batchProject, a.Key, strconv.Itoa(from))
		if se, ok := errors.AsType[*Error](err); ok {
			full = se
		}
	}

	if full == nil || full.Code != CodeQueueFull {
		t.Fatalf("the queue never filled: %v", full)
	}

	env.s.previews.mu.Lock()
	q := env.s.previews.queued
	env.s.previews.mu.Unlock()

	if q != maxQueuedPreviews {
		t.Errorf("queued = %d, want %d", q, maxQueuedPreviews)
	}

	// Released, the waiting renders all end (the cleanup of previewEnv waits for them).
	close(gate)
}

// A source with its asset's content hash in the key: the plan's key is the one of plan.PreviewKey.
func TestAudioPreviewPlanKeyIsThePlansKey(t *testing.T) {
	t.Parallel()

	env := previewEnv(t, &fakePreviewer{})
	store, _ := env.s.store.(*batchStore)

	doc, err := timeline.Decode(store.projects.row.Document)
	if err != nil {
		t.Fatal(err)
	}

	src, ok := plan.PreviewSource(doc, previewItemA, 30, 180)
	if !ok {
		t.Fatal("no source")
	}

	if got := previewKeys(t, env)[previewItemA].Key; got != plan.PreviewKey(doc, &src) {
		t.Errorf("key %s, want %s", got, plan.PreviewKey(doc, &src))
	}
}
