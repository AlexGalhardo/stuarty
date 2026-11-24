package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	assetlib "github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/library"
)

// fakeLifecycle stands for the account manager: it keeps the functions the Service registers.
type fakeLifecycle struct{ onLock, onUnlock []func() }

func (f *fakeLifecycle) OnLock(fn func())   { f.onLock = append(f.onLock, fn) }
func (f *fakeLifecycle) OnUnlock(fn func()) { f.onUnlock = append(f.onUnlock, fn) }

func (f *fakeLifecycle) lock() {
	for _, fn := range f.onLock {
		fn()
	}
}

func (f *fakeLifecycle) unlock() {
	for _, fn := range f.onUnlock {
		fn()
	}
}

func lifecycleService(t *testing.T) (*svc, *fakeLifecycle, string) {
	t.Helper()

	ws := t.TempDir()
	life := &fakeLifecycle{}
	st := &memStore{
		library:     memLibrary{rows: map[string]assetlib.Row{}},
		images:      &memImages{rows: map[string]*ImageRow{}, ops: map[string][]ImageOpRow{}},
		checkpoints: &memCheckpoints{rows: map[string]CheckpointRow{}},
		assets:      memAssets{projects: map[string][]ProjectAssetRow{}, rows: map[string]AssetRow{}},
	}

	s, err := New(Options{Store: st, Workspace: ws, Limits: Limits{MinFreeBytes: -1}, Lifecycle: life})
	if err != nil {
		t.Fatal(err)
	}

	return s.(*svc), life, ws
}

// After the lock no cache of the Service holds content and the index of the library is empty (ADR-0053 item 7,
// SDD-007 T-134): the pictures and prepared layers of the image editor, the specs of the bitmaps a plan handed out,
// the snap points and the previews of a timeline, and the index of the library.
func TestSecurity_T134_NoIndexAndNoCacheOutlivesTheLock(t *testing.T) {
	s, life, ws := lifecycleService(t)
	ctx := asActor("local:person")

	if len(life.onLock) == 0 || len(life.onUnlock) == 0 {
		t.Fatal("the Service did not register for the lock and the unlock")
	}

	// A note in the library, indexed.
	mem := filepath.Join(ws, "library", "memory", "note.md")
	if err := os.MkdirAll(filepath.Dir(mem), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(mem, []byte("---\nname: note\ndescription: a note\nscope: workspace\nactor: local:ui\nkept: true\n"+
		"created: 2026-11-19T10:00:00Z\nupdated: 2026-11-19T10:00:00Z\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	life.unlock()

	if rows, err := s.lib.List(ctx, "memory", library.Filter{IncludeUnreviewed: true}); err != nil || len(rows) != 1 {
		t.Fatalf("the unlock must build the index: %v %v", rows, err)
	}

	// An image frame fills the cache of the editor; the other caches get a content-bearing entry.
	c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 200, Height: 100, Background: "#102030"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(ctx, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1,
		Operations: ops("layer.add_fill", `{"color": "#335577", "id": "lyr_aaaaaaaaaa"}`,
			"layer.add_shape", `{"shape": "ellipse", "w": 60, "h": 40, "fill": "#FF8800", "id": "lyr_bbbbbbbbbb"}`),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: c.ImageID}); err != nil {
		t.Fatal(err)
	}

	if st := s.imageCache().Stats(); st.Pictures == 0 {
		t.Fatalf("the control: the frame left nothing in the cache of the editor: %+v", st)
	}

	s.bitmaps.specs = map[bitmapID]knownBitmap{{project: "prj_x", key: "k"}: {}}
	s.snaps.m = map[snapKey]plan.Snap{{project: "prj_x"}: {}}
	s.previews.known = map[bitmapID]knownPreview{{project: "prj_x", key: "k"}: {}}

	life.lock()

	if st := s.imageCache().Stats(); st.Pictures != 0 || st.Bytes != 0 || st.PreparedLayers != 0 {
		t.Errorf("the cache of the image editor survived the lock: %+v", st)
	}

	if len(s.bitmaps.specs) != 0 || s.bitmaps.cache != nil || len(s.bitmaps.caches) != 0 {
		t.Error("the bitmaps of a plan survived the lock")
	}

	if len(s.snaps.m) != 0 {
		t.Error("the snap points survived the lock")
	}

	if len(s.previews.known) != 0 {
		t.Error("the previews survived the lock")
	}

	// The index of the library is empty: List asks the files, so it needs the index rebuilt at the next unlock.
	if n, err := s.lib.IndexSize(context.Background()); err != nil || n != 0 {
		t.Errorf("the index of the library survived the lock: %d %v", n, err)
	}
}
