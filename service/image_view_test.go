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
	}

	for i, w := range want {
		if got := h.Entries[i].Description; got != w {
			t.Errorf("entry %d description = %q, want %q", i, got, w)
		}
	}
}
