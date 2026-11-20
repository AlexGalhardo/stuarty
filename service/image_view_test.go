package service

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The reads of the image editor screen (SDD-012 section 24.3).

func TestImageFrameDrawsTheCurrentRevision(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:person")

	c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 200, Height: 100, Background: "#102030"})
	if err != nil {
		t.Fatal(err)
	}

	whole, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: c.ImageID})
	if err != nil || whole.Revision != 1 || whole.Width != 200 || whole.Height != 100 || whole.MIME != "image/png" {
		t.Fatalf("%v %+v", err, whole)
	}

	if cfg, derr := png.DecodeConfig(bytes.NewReader(whole.Image)); derr != nil || cfg.Width != 200 {
		t.Fatalf("%v %+v", derr, cfg)
	}

	small, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: c.ImageID, Width: 50})
	if err != nil || small.Width != 50 || small.Height != 25 {
		t.Fatalf("%v %+v", err, small)
	}

	if cfg, derr := png.DecodeConfig(bytes.NewReader(small.Image)); derr != nil || cfg.Width != 50 || cfg.Height != 25 {
		t.Fatalf("%v %+v", derr, cfg)
	}

	// An empty slot stops an export, never a picture: the screen shows what there is.
	if _, err := s.ImageEdit(ctx, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1,
		Operations: ops("layer.add_text", `{"text": "", "slot": "title"}`),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: c.ImageID}); err != nil {
		t.Fatalf("a blocking finding stopped the picture: %v", err)
	}

	for _, w := range []int{-1, 15, 4097} {
		_, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: c.ImageID, Width: w})
		if se := mustCode(t, err, "E_VALIDATION"); se.Path != "width" {
			t.Fatalf("width %d: %+v", w, se)
		}
	}

	_, err = s.ImageFrame(ctx, ImageFrameRequest{ImageID: "img_nope000000"})
	mustCode(t, err, "E_NOT_FOUND")
}

// The size the result names is the size of the PNG, also for a picture so wide that its height rounds to zero.
func TestImageFrameNamesTheSizeOfThePicture(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:person")

	c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 4096, Height: 16})
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: c.ImageID, Width: 16})
	if err != nil {
		t.Fatal(err)
	}

	cfg, derr := png.DecodeConfig(bytes.NewReader(f.Image))
	if derr != nil || cfg.Width != f.Width || cfg.Height != f.Height {
		t.Fatalf("the result says %dx%d, the picture is %+v (%v)", f.Width, f.Height, cfg, derr)
	}
}

// TestImageFrameStartsFromTheCacheOfItsOwnUser: the frame of the editor is drawn from the pictures an earlier
// frame of the same user left in the cache, and in network mode never from those of another user, even for a
// document with the same content (SDD-021 section 6.1, T-89).
func TestImageFrameStartsFromTheCacheOfItsOwnUser(t *testing.T) {
	s, _ := newImageService(t)
	alice, bob := networkUser("alice"), networkUser("bob")

	same := func(ctx context.Context) string {
		t.Helper()

		c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 200, Height: 100, Background: "#102030"})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := s.ImageEdit(ctx, ImageEditRequest{
			ImageID: c.ImageID, BaseRevision: 1,
			Operations: ops(
				"layer.add_fill", `{"color": "#335577", "id": "lyr_aaaaaaaaaa"}`,
				"layer.add_shape", `{"shape": "ellipse", "w": 60, "h": 40, "fill": "#FF8800", "id": "lyr_bbbbbbbbbb"}`,
			),
		}); err != nil {
			t.Fatal(err)
		}

		return c.ImageID
	}

	frame := func(ctx context.Context, id string) []byte {
		t.Helper()

		f, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: id})
		if err != nil {
			t.Fatal(err)
		}

		return f.Image
	}

	mine, theirs := same(alice), same(bob)

	first := frame(alice, mine)
	if h := s.imageFrames.Stats().Hits; h != 0 {
		t.Fatalf("the first frame found %d pictures in an empty cache", h)
	}

	if again := frame(alice, mine); !bytes.Equal(again, first) || s.imageFrames.Stats().Hits != 1 {
		t.Fatalf("the same frame again must be the same picture, from the cache: %+v", s.imageFrames.Stats())
	}

	if other := frame(bob, theirs); !bytes.Equal(other, first) || s.imageFrames.Stats().Hits != 1 {
		t.Fatalf("bob started from a picture of alice: %+v", s.imageFrames.Stats())
	}

	if frame(bob, theirs); s.imageFrames.Stats().Hits != 2 {
		t.Fatalf("bob does not find his own picture: %+v", s.imageFrames.Stats())
	}
}

func TestImageHistoryNamesTheActors(t *testing.T) {
	s, _ := newImageService(t)
	person := asActor("local:person")
	agent := asActor("agent:claude")

	c, err := s.ImageCreate(person, ImageCreateRequest{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(person, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1,
		Operations: ops("layer.add_fill", `{"color": "#000000"}`),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(agent, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 2,
		Operations: ops("layer.add_shape", `{"shape": "ellipse", "w": 10, "h": 10}`),
	}); err != nil {
		t.Fatal(err)
	}

	// The person undoes their own step; the agent's later step stays.
	if _, err := s.ImageUndo(person, ImageStepRequest{ImageID: c.ImageID}); err != nil {
		t.Fatal(err)
	}

	h, err := s.ImageHistory(person, ImageHistoryRequest{ImageID: c.ImageID})
	if err != nil || h.Revision != 4 || len(h.Entries) != 4 {
		t.Fatalf("%v %+v", err, h)
	}

	if e := h.Entries[0]; e.Revision != 4 || e.Kind != "undo" || !e.Mine || e.UndoOf == nil || *e.UndoOf != 2 {
		t.Fatalf("newest first, the undo of the person: %+v", e)
	}

	if e := h.Entries[1]; e.Revision != 3 || e.Actor != "agent:claude" || e.Mine || e.Ops[0] != "layer.add_shape" {
		t.Fatalf("the agent step is not mine: %+v", e)
	}

	if e := h.Entries[2]; e.Revision != 2 || e.UndoneBy == nil || *e.UndoneBy != 4 {
		t.Fatalf("the undone step says by whom: %+v", e)
	}

	one, err := s.ImageHistory(person, ImageHistoryRequest{ImageID: c.ImageID, Limit: 1})
	if err != nil || len(one.Entries) != 1 || one.Entries[0].Revision != 4 {
		t.Fatalf("%v %+v", err, one)
	}

	_, err = s.ImageHistory(person, ImageHistoryRequest{ImageID: c.ImageID, Limit: 101})
	mustCode(t, err, "E_VALIDATION")
}

func TestImageFileServesOnlyWhatAnExportWrote(t *testing.T) {
	s, ws := newImageService(t)
	ctx := asActor("local:person")

	c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 40, Height: 40})
	if err != nil {
		t.Fatal(err)
	}

	exp, err := s.ImageExport(ctx, ImageExportRequest{ImageID: c.ImageID})
	if err != nil {
		t.Fatal(err)
	}

	name := filepath.Base(exp.Path)

	f, err := s.ImageFile(ctx, ImageFileRequest{ImageID: c.ImageID, Name: name})
	if err != nil || f.ContentType != "image/png" || f.Size != exp.Bytes ||
		f.Path != "exports/images/"+c.ImageID+"/"+name {
		t.Fatalf("%v %+v", err, f)
	}

	raw, rerr := os.ReadFile(filepath.Join(ws, filepath.FromSlash(f.Path)))
	if rerr != nil || int64(len(raw)) != exp.Bytes {
		t.Fatalf("%v", rerr)
	}

	// a name that is not the shape an export gives, a traversal, a file that does not exist, another document
	for _, bad := range []string{"../../../token", "..%2f..%2ftoken", "1-abc.png", "x.png", strings.ToUpper(name), "9-000000000000.png"} {
		_, err := s.ImageFile(ctx, ImageFileRequest{ImageID: c.ImageID, Name: bad})
		mustCode(t, err, "E_NOT_FOUND")
	}

	_, err = s.ImageFile(ctx, ImageFileRequest{ImageID: "img_nope000000", Name: name})
	mustCode(t, err, "E_NOT_FOUND")
}

func TestImageReadsRefuseAProjectLimitedKey(t *testing.T) {
	s, _ := newImageService(t)
	limited := WithCaller(context.Background(), Caller{
		Actor: "key:k", ProjectID: "prj_aaaaaaaaaa", Scopes: []string{ScopeRead},
	})

	c, _ := s.ImageCreate(asActor("local:a"), ImageCreateRequest{Width: 10, Height: 10})

	_, err := s.ImageFrame(limited, ImageFrameRequest{ImageID: c.ImageID})
	mustCode(t, err, CodeForbidden)

	_, err = s.ImageHistory(limited, ImageHistoryRequest{ImageID: c.ImageID})
	mustCode(t, err, CodeForbidden)

	_, err = s.ImageFile(limited, ImageFileRequest{ImageID: c.ImageID, Name: "1-000000000000.png"})
	mustCode(t, err, CodeForbidden)
}

// VV-279: a row of the history says in words what the step did, with the summaries of the core, as the history of a
// timeline does.
func TestImageHistoryDescribesEachStep(t *testing.T) {
	s, _ := newImageService(t)
	person := asActor("local:person")

	c, err := s.ImageCreate(person, ImageCreateRequest{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(person, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1,
		Operations: ops(
			"layer.add_fill", `{"color": "#000000"}`,
			"layer.add_shape", `{"shape": "ellipse", "w": 10, "h": 10}`,
		),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageUndo(person, ImageStepRequest{ImageID: c.ImageID}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageRedo(person, ImageStepRequest{ImageID: c.ImageID}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageRevert(person, ImageRevertRequest{ImageID: c.ImageID, Revision: 1}); err != nil {
		t.Fatal(err)
	}

	h, err := s.ImageHistory(person, ImageHistoryRequest{ImageID: c.ImageID})
	if err != nil || len(h.Entries) != 5 {
		t.Fatalf("%v %+v", err, h)
	}

	want := []string{
		"Reverted to r1.",
		"Redid r3.",
		"Undid r2.",
		"Adds a solid colour over the whole canvas. Adds a rectangle, a rounded rectangle or an ellipse.",
		"Sets the canvas, name and background of a new image document.",
	}

	for i, w := range want {
		if got := h.Entries[i].Description; got != w {
			t.Errorf("entry %d description = %q, want %q", i, got, w)
		}
	}
}

// VV-279: the words of a row hold no text of the user: the text and the name of a layer stay out, and a command run
// twice in one step is said once.
func TestImageHistoryDescriptionHoldsNoLayerText(t *testing.T) {
	s, _ := newImageService(t)
	person := asActor("local:person")

	c, err := s.ImageCreate(person, ImageCreateRequest{Width: 100, Height: 100, Name: "Private cover"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(person, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1,
		Operations: ops(
			"layer.add_text", `{"text": "SECRET WORDS", "id": "lyr_aaaaaaaaaa"}`,
			"layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "name": "Hidden name"}`,
			"layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 50}`,
		),
	}); err != nil {
		t.Fatal(err)
	}

	h, err := s.ImageHistory(person, ImageHistoryRequest{ImageID: c.ImageID})
	if err != nil || len(h.Entries) != 2 {
		t.Fatalf("%v %+v", err, h)
	}

	for _, e := range h.Entries {
		for _, private := range []string{"SECRET", "Hidden", "Private", "lyr_"} {
			if strings.Contains(e.Description, private) {
				t.Errorf("r%d description %q holds %q", e.Revision, e.Description, private)
			}
		}
	}

	if got, want := h.Entries[0].Description, "Adds a text layer. "+
		"Sets the name, opacity, visibility, lock or slot of a layer."; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}
