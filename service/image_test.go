package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/library"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// An in-memory store of the image tables: the sqlite one lives in web/app/store, which imports this package.

type memImages struct {
	mu   sync.Mutex
	rows map[string]*ImageRow
	ops  map[string][]ImageOpRow
}

func (m *memImages) Create(_ context.Context, row ImageRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.rows[row.ID]; ok {
		return ErrExists
	}

	r := row
	m.rows[row.ID] = &r

	return nil
}

func (m *memImages) Get(_ context.Context, id string) (ImageRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[id]
	if !ok {
		return ImageRow{}, ErrNotFound
	}

	return *r, nil
}

func (m *memImages) List(_ context.Context, q ImageQuery) ([]ImageHead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []ImageHead

	for _, r := range m.rows {
		if q.Templates == "only" && !r.Template || q.Templates == "none" && r.Template {
			continue
		}

		out = append(out, r.ImageHead)
	}

	slices.SortFunc(out, func(a, b ImageHead) int { return strings.Compare(a.ID, b.ID) })

	if q.Offset >= len(out) {
		return nil, nil
	}

	out = out[q.Offset:]

	return out[:min(len(out), q.Limit+1)], nil
}

func (m *memImages) Commit(_ context.Context, id string, from int64, u ImageUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r := m.rows[id]
	if r == nil || r.Revision != from {
		return ErrStale
	}

	r.Name, r.Template, r.Revision, r.SchemaVersion, r.Document = u.Name, u.Template, from+1, u.SchemaVersion, u.Document
	r.DocSHA256, r.Width, r.Height, r.Layers, r.UpdatedMs = u.DocSHA256, u.Width, u.Height, u.Layers, u.UpdatedMs

	return nil
}

func (m *memImages) AppendOp(_ context.Context, op ImageOpRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ops[op.ImageID] = append(m.ops[op.ImageID], op)

	return nil
}

func (m *memImages) OpsSince(_ context.Context, id string, revision int64) ([]ImageOpRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []ImageOpRow

	for i := range m.ops[id] {
		if m.ops[id][i].Revision > revision {
			out = append(out, m.ops[id][i])
		}
	}

	return out, nil
}

func (m *memImages) LastOpen(_ context.Context, id, actor string, kinds []OperationKind) (ImageOpRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ops := m.ops[id]
	for i := range slices.Backward(ops) {
		if o := &ops[i]; o.Actor == actor && o.Revision > 1 && o.ClosedBy == nil && slices.Contains(kinds, o.Kind) {
			return *o, nil
		}
	}

	return ImageOpRow{}, ErrNotFound
}

func (m *memImages) Close(_ context.Context, id string, revision, by int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.ops[id] {
		if o := &m.ops[id][i]; o.Revision == revision && o.ClosedBy == nil {
			o.ClosedBy = &by

			return nil
		}
	}

	return ErrStale
}

func (m *memImages) CloseOpenRedo(_ context.Context, id, actor string, by int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.ops[id] {
		if o := &m.ops[id][i]; o.Actor == actor && o.Kind == OpUndo && o.ClosedBy == nil {
			o.ClosedBy = &by
		}
	}

	return nil
}

func (m *memImages) ByKey(_ context.Context, id, actor, key string) (ImageOpRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.ops[id] {
		if o := &m.ops[id][i]; o.Actor == actor && o.IdempotencyKey != nil && *o.IdempotencyKey == key {
			return *o, nil
		}
	}

	return ImageOpRow{}, ErrNotFound
}

type memAssets struct {
	AssetRepo
	rows     map[string]AssetRow
	projects map[string][]ProjectAssetRow // project id to the files it holds
}

func (m memAssets) ListProjectAssets(_ context.Context, id string, _ ...ProjectAssetState) ([]ProjectAssetRow, error) {
	return m.projects[id], nil
}

type memProjects struct {
	ProjectRepo
	heads []ProjectHead
}

func (m memProjects) List(_ context.Context, q ProjectQuery) ([]ProjectHead, string, error) {
	var out []ProjectHead

	for i := range m.heads {
		if q.OwnerUserID == "" || m.heads[i].OwnerUserID != nil && *m.heads[i].OwnerUserID == q.OwnerUserID {
			out = append(out, m.heads[i])
		}
	}

	return out, "", nil
}

func (m memAssets) Get(_ context.Context, hash string) (AssetRow, error) {
	r, ok := m.rows[hash]
	if !ok {
		return AssetRow{}, ErrNotFound
	}

	return r, nil
}

type memTx struct {
	Tx
	st *memStore
}

func (t memTx) Images() ImageRepo  { return t.st.images }
func (t memTx) Assets() AssetRepo  { return t.st.assets }
func (t memTx) AfterCommit(func()) {}

type memStore struct {
	Store
	images   *memImages
	assets   memAssets
	projects memProjects
}

func (m *memStore) Images() ImageRepo { return m.images }

// No test here reads a connection; the manager the service builds only needs a store to hold.
func (m *memStore) Connections() connections.Store { return nil }

// Nor does any test here use the library; its manager only needs an index to hold.
func (m *memStore) Library() library.Index { return nil }

func (m *memStore) Projects() ProjectRepo { return m.projects }
func (m *memStore) Assets() AssetRepo     { return m.assets }
func (m *memStore) Write(ctx context.Context, fn func(context.Context, Tx) error) error {
	return fn(ctx, memTx{st: m})
}

const imgHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// newImageService is a service over the memory store and a workspace with one prepared image: 64 by 48.
func newImageService(t *testing.T) (*svc, string) {
	t.Helper()

	ws := t.TempDir()
	w, h := 64, 48

	st := &memStore{
		images: &memImages{rows: map[string]*ImageRow{}, ops: map[string][]ImageOpRow{}},
		assets: memAssets{projects: map[string][]ProjectAssetRow{}, rows: map[string]AssetRow{imgHash: {
			Hash: imgHash, Kind: "image", OriginalName: "/home/me/face.png", IngestState: IngestReady,
			Facts: AssetFacts{Width: &w, Height: &h},
		}}},
	}

	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 5), B: 128, A: 255})
		}
	}

	dir := filepath.Join(ws, "artifacts", strings.TrimPrefix(imgHash, "sha256:"))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(dir, ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "png")),
		buf.Bytes(),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	n := 0

	s, err := New(Options{Store: st, Workspace: ws, NewIDs: func(prefix string) string {
		n++

		return fmt.Sprintf("%s_%010x", prefix, n)[:14]
	}})
	if err != nil {
		t.Fatal(err)
	}

	return s.(*svc), ws
}

func asActor(actor string) context.Context {
	return WithCaller(
		context.Background(),
		Caller{Actor: actor, Scopes: []string{ScopeRead, ScopeEdit, ScopeMedia, ScopeRender}},
	)
}

func ops(pairs ...string) []command.Operation {
	var out []command.Operation
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, command.Operation{Op: pairs[i], Args: json.RawMessage(pairs[i+1])})
	}

	return out
}

func mustCode(t *testing.T, err error, code string) *Error {
	t.Helper()

	var se *Error
	if !errors.As(err, &se) || se.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}

	return se
}

func TestImageFlow(t *testing.T) {
	s, ws := newImageService(t)
	ctx := asActor("local:agent")

	created, err := s.ImageCreate(
		ctx,
		ImageCreateRequest{Preset: "video_thumbnail", Name: "Episode 12", Background: "#101820"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if created.Revision != 1 || created.Width != 1280 ||
		!strings.HasPrefix(created.Text, textview.Note+"\nimage img_") {
		t.Fatalf("%+v", created)
	}

	id := created.ImageID

	edit, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: ops(
		"layer.add_image",
		`{"asset": "`+imgHash+`", "fit": "cover", "id": "lyr_aaaaaaaaaa"}`,
		"layer.add_text",
		`{"text": "HOW IT WORKS", "style": {"size": 120, "color": "#FFE600"}, "id": "lyr_bbbbbbbbbb"}`,
		"layer.add_shape",
		`{"shape": "rounded_rect", "w": 400, "h": 80, "radius": 20, "fill": "#E63946", "id": "lyr_cccccccccc"}`,
		"layer.align",
		`{"layer_ids": ["lyr_bbbbbbbbbb", "lyr_cccccccccc"], "to": "canvas", "horizontal": "center"}`,
	)})
	if err != nil {
		t.Fatal(err)
	}

	if !edit.Applied || edit.Revision != 2 || len(edit.Created) != 3 || len(edit.Effective) != 4 {
		t.Fatalf("%+v", edit)
	}

	if !strings.Contains(edit.Description, "added text") {
		t.Errorf("description %q", edit.Description)
	}

	desc, err := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: id})
	if err != nil || desc.Revision != 2 || len(desc.Layers) != 3 ||
		!strings.Contains(desc.Text, "lyr_bbbbbbbbbb text") {
		t.Fatalf("%v %+v", err, desc)
	}

	undone, err := s.ImageUndo(ctx, ImageStepRequest{ImageID: id})
	if err != nil || !undone.Applied || undone.Revision != 3 || undone.Step != 2 || undone.Kind != "undo" {
		t.Fatalf("%v %+v", err, undone)
	}

	if d, _ := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: id}); len(d.Layers) != 0 {
		t.Fatalf("the undo removed the layers: %d left", len(d.Layers))
	}

	redone, err := s.ImageRedo(ctx, ImageStepRequest{ImageID: id})
	if err != nil || !redone.Applied || redone.Kind != "redo" {
		t.Fatalf("%v %+v", err, redone)
	}

	_, err = s.ImageRedo(ctx, ImageStepRequest{ImageID: id})
	mustCode(t, err, CodeNothingToRedo)

	exported, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id})
	if err != nil {
		t.Fatal(err)
	}

	raw, rerr := os.ReadFile(exported.Path)
	if rerr != nil || !strings.HasPrefix(exported.Path, filepath.Join(ws, "exports", "images", id)) {
		t.Fatalf("%v %s", rerr, exported.Path)
	}

	if cfg, err := png.DecodeConfig(bytes.NewReader(raw)); err != nil || cfg.Width != 1280 || cfg.Height != 720 {
		t.Fatalf("%v %+v", err, cfg)
	}

	again, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id})
	if err != nil || again.SHA256 != exported.SHA256 || again.Path != exported.Path {
		t.Fatalf("the same document exports to the same file: %v", err)
	}

	small, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id, Format: "jpeg", Preset: "square_post"})
	if err != nil || small.Width != 1080 || small.Height != 1080 || !strings.HasSuffix(small.Path, ".jpg") {
		t.Fatalf("%v %+v", err, small)
	}

	_, err = s.ImageExport(ctx, ImageExportRequest{ImageID: id, Format: "webp"})
	if se := mustCode(t, err, CodeExportFailed); se.Details["reason"] != "webp_unavailable" {
		t.Fatalf("%+v", se)
	}

	_, err = s.ImageExport(ctx, ImageExportRequest{ImageID: id, Width: -1})
	mustCode(t, err, command.CodeValidation)

	// a quality out of range is the caller's mistake with a preset as without one
	_, err = s.ImageExport(ctx, ImageExportRequest{ImageID: id, Format: "jpeg", Preset: "square_post", Quality: 101})
	if se := mustCode(t, err, command.CodeValidation); se.Path != "quality" {
		t.Fatalf("%+v", se)
	}
}

func TestImageCreationIsNotUndone(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:agent")

	c, _ := s.ImageCreate(ctx, ImageCreateRequest{Width: 100, Height: 100})
	_, err := s.ImageUndo(ctx, ImageStepRequest{ImageID: c.ImageID})
	mustCode(t, err, CodeNothingToUndo)
}

func TestImageEditRules(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:agent")
	c, _ := s.ImageCreate(ctx, ImageCreateRequest{Width: 100, Height: 100})
	id := c.ImageID

	add := ops("layer.add_fill", `{"color": "#000000", "id": "lyr_aaaaaaaaaa"}`)

	dry, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: add, DryRun: true})
	if err != nil || dry.Applied || !dry.DryRun || dry.Revision != 2 {
		t.Fatalf("%v %+v", err, dry)
	}

	if d, _ := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: id}); d.Revision != 1 {
		t.Fatal("a dry run writes nothing")
	}

	first, err := s.ImageEdit(
		ctx,
		ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: add, IdempotencyKey: "key-12345678"},
	)
	if err != nil || !first.Applied {
		t.Fatal(err)
	}

	replay, err := s.ImageEdit(
		ctx,
		ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: add, IdempotencyKey: "key-12345678"},
	)
	if err != nil || !replay.Replayed || replay.Revision != 2 {
		t.Fatalf("%v %+v", err, replay)
	}

	_, err = s.ImageEdit(ctx, ImageEditRequest{
		ImageID: id, BaseRevision: 2, IdempotencyKey: "key-12345678",
		Operations: ops("layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "name": "x"}`),
	})
	mustCode(t, err, command.CodeIdempotencyMismatch)

	noop, err := s.ImageEdit(
		ctx,
		ImageEditRequest{
			ImageID:      id,
			BaseRevision: 2,
			Operations:   ops("layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "name": "Fill"}`),
		},
	)
	if err != nil || noop.Applied {
		t.Fatalf("a batch that changes nothing commits nothing: %v %+v", err, noop)
	}

	_, err = s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 9, Operations: add})
	mustCode(t, err, command.CodeValidation)

	_, err = s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops("clip.add", `{}`)})
	mustCode(t, err, command.CodeUnknownCommand)

	_, err = s.ImageEdit(
		ctx,
		ImageEditRequest{
			ImageID:      id,
			BaseRevision: 2,
			Operations:   ops("layer.remove", `{"layer_id": "lyr_zzzzzzzzzz"}`),
		},
	)
	if se := mustCode(t, err, command.CodeNotFound); se.OpIndex == nil || *se.OpIndex != 0 {
		t.Fatalf("%+v", se)
	}

	_, err = s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: "img_zzzzzzzzzz"})
	mustCode(t, err, command.CodeNotFound)

	readOnly := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: []string{ScopeRead}})
	_, err = s.ImageEdit(readOnly, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: add})
	mustCode(t, err, CodeForbidden)

	if _, err := s.ImageDescribe(readOnly, ImageDescribeRequest{ImageID: id}); err != nil {
		t.Fatalf("read is enough to describe: %v", err)
	}

	_, err = s.ImageEdit(context.Background(), ImageEditRequest{ImageID: id, Operations: add})
	mustCode(t, err, CodeUnauthorized)
}

func TestImageConflictAndUndoOfAnotherActor(t *testing.T) {
	s, _ := newImageService(t)
	a, b := asActor("local:a"), asActor("local:b")
	c, _ := s.ImageCreate(a, ImageCreateRequest{Width: 200, Height: 200})
	id := c.ImageID

	if _, err := s.ImageEdit(a, ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: ops(
		"layer.add_shape", `{"shape": "rect", "w": 50, "h": 50, "id": "lyr_aaaaaaaaaa"}`,
		"layer.add_shape", `{"shape": "rect", "w": 50, "h": 50, "id": "lyr_bbbbbbbbbb"}`,
	)}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(
		a,
		ImageEditRequest{
			ImageID:      id,
			BaseRevision: 2,
			Operations:   ops("layer.transform", `{"layer_id": "lyr_aaaaaaaaaa", "x": 5, "y": 6}`),
		},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(
		b,
		ImageEditRequest{
			ImageID:      id,
			BaseRevision: 3,
			Operations:   ops("layer.transform", `{"layer_id": "lyr_aaaaaaaaaa", "x": 70}`),
		},
	); err != nil {
		t.Fatal(err)
	}

	// a planned against revision 3 and b changed the same layer at revision 4: a conflict, and nothing is written
	_, err := s.ImageEdit(
		a,
		ImageEditRequest{
			ImageID:      id,
			BaseRevision: 3,
			Operations:   ops("layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "name": "mine"}`),
		},
	)
	if se := mustCode(t, err, command.CodeRevisionConflict); se.Details["conflict"] == nil {
		t.Fatalf("%+v", se)
	}

	// another layer is no conflict, and the same layer at the current revision is not one either
	if _, err := s.ImageEdit(
		b,
		ImageEditRequest{
			ImageID:      id,
			BaseRevision: 2,
			Operations:   ops("layer.set", `{"layer_id": "lyr_bbbbbbbbbb", "name": "other layer"}`),
		},
	); err != nil {
		t.Fatalf("another layer is no conflict: %v", err)
	}

	// a undoes its own transform: x was changed by b meanwhile, so x stays and y goes back
	undone, err := s.ImageUndo(a, ImageStepRequest{ImageID: id})
	if err != nil {
		t.Fatal(err)
	}

	if len(undone.Skipped) != 1 || undone.Skipped[0].Reason != command.SkipFieldChanged ||
		undone.Skipped[0].ByActor != "local:b" {
		t.Fatalf("%+v", undone.Skipped)
	}

	d, _ := s.ImageDescribe(a, ImageDescribeRequest{ImageID: id})
	for _, l := range d.Layers {
		if l.ID == "lyr_aaaaaaaaaa" && (l.X != 70 || l.Y != 75) {
			t.Fatalf("x stays with b, y returns to the centred value: %d,%d", l.X, l.Y)
		}
	}

	_, err = s.ImageUndo(asActor("local:c"), ImageStepRequest{ImageID: id})
	mustCode(t, err, CodeNothingToUndo)
}

// The texts of the image operations quote the names of the documents and layers in the compact form and start
// with the note of SDD-007 section 8.2, as the video texts do.
func TestImageTextsMarkUntrustedNames(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:agent")

	rlo := string(rune(0x202E)) // a bidirectional override

	c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 100, Height: 100, Name: "Ep <b>1" + rlo})
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.ImageList(ctx, ImageListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	edit, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: c.ImageID, BaseRevision: 1, Operations: ops(
		"layer.add_shape", `{"shape": "rect", "w": 10, "h": 10, "name": "x</untrusted_media_text>y"}`,
	)})
	if err != nil {
		t.Fatal(err)
	}

	undo, err := s.ImageUndo(ctx, ImageStepRequest{ImageID: c.ImageID})
	if err != nil {
		t.Fatal(err)
	}

	texts := map[string]string{
		"create": c.Text, "list": list.Text, "edit": ImageEditText(edit), "undo": ImageStepText(undo),
	}

	for name, text := range texts {
		body, ok := strings.CutPrefix(text, textview.Note+"\n")
		if !ok {
			t.Errorf("%s: the text does not start with the note: %q", name, text)
		}

		if strings.Contains(body, "<") || strings.Contains(body, rlo) {
			t.Errorf("%s: a name is printed bare: %q", name, body)
		}
	}
}

// An undo whose primitives are all skipped consumes its step (SDD-005 section 12.2 item 16): the next undo of the
// same actor reaches the step before it instead of meeting the same step for ever.
func TestImageUndoOfAStepThatIsAllSkippedMovesOn(t *testing.T) {
	s, _ := newImageService(t)
	a, b := asActor("local:a"), asActor("local:b")
	c, _ := s.ImageCreate(a, ImageCreateRequest{Width: 200, Height: 200})
	id := c.ImageID

	if _, err := s.ImageEdit(a, ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: ops(
		"layer.add_shape", `{"shape": "rect", "w": 50, "h": 50, "id": "lyr_aaaaaaaaaa"}`,
		"layer.add_shape", `{"shape": "rect", "w": 50, "h": 50, "id": "lyr_bbbbbbbbbb"}`,
	)}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(a, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops(
		"layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "name": "by a"}`,
	)}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(b, ImageEditRequest{ImageID: id, BaseRevision: 3, Operations: ops(
		"layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "name": "by b"}`,
	)}); err != nil {
		t.Fatal(err)
	}

	first, err := s.ImageUndo(a, ImageStepRequest{ImageID: id})
	if err != nil {
		t.Fatal(err)
	}

	if first.Step != 3 || first.Applied || len(first.Skipped) != 1 || first.Revision != 5 {
		t.Fatalf("the rename of a is skipped, and the step is consumed by revision 5: %+v", first)
	}

	second, err := s.ImageUndo(a, ImageStepRequest{ImageID: id})
	if err != nil {
		t.Fatal(err)
	}

	if second.Step != 2 || !second.Applied {
		t.Fatalf("the next undo reaches the step before: step %d applied %t", second.Step, second.Applied)
	}

	// the shape b renamed stays with b; the other shape of the step goes
	if d, _ := s.ImageDescribe(a, ImageDescribeRequest{ImageID: id}); len(d.Layers) != 1 ||
		d.Layers[0].ID != "lyr_aaaaaaaaaa" {
		t.Fatalf("only the shape b changed is left: %+v", d.Layers)
	}
}

func TestImageTemplates(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:agent")

	tpl, err := s.ImageCreate(ctx, ImageCreateRequest{Preset: "video_thumbnail", Template: true, Name: "Thumbnail"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: tpl.ImageID, BaseRevision: 1, Operations: ops(
		"layer.add_image", `{"asset": "`+imgHash+`", "slot": "photo", "id": "lyr_aaaaaaaaaa"}`,
		"layer.add_text", `{"text": "", "slot": "title", "id": "lyr_bbbbbbbbbb"}`,
	)}); err != nil {
		t.Fatal(err)
	}

	// the template keeps its own file as the default of the photo, so only the title is required
	_, err = s.ImageCreate(ctx, ImageCreateRequest{TemplateID: tpl.ImageID})
	se := mustCode(t, err, command.CodeSlot)

	if missing, _ := se.Details["missing"].([]string); !slices.Equal(missing, []string{"title"}) {
		t.Fatalf("%+v", se.Details)
	}

	title := "Episode 12"
	made, err := s.ImageCreate(
		ctx,
		ImageCreateRequest{TemplateID: tpl.ImageID, Values: map[string]coreimage.SlotValue{"title": {Text: &title}}},
	)
	if err != nil {
		t.Fatal(err)
	}

	if made.ImageID == tpl.ImageID || made.Revision != 1 {
		t.Fatalf("%+v", made)
	}

	exp, err := s.ImageExport(ctx, ImageExportRequest{ImageID: made.ImageID})
	if err != nil || exp.Width != 1280 {
		t.Fatalf("a filled template exports: %v", err)
	}

	// the template itself still has an empty slot: it does not export
	_, err = s.ImageExport(ctx, ImageExportRequest{ImageID: tpl.ImageID})
	if se := mustCode(t, err, CodeLintBlocking); se.Details["findings"] == nil {
		t.Fatal("the findings are in the error")
	}

	_, err = s.ImageCreate(ctx, ImageCreateRequest{TemplateID: made.ImageID})
	mustCode(t, err, command.CodeInvalidState)

	_, err = s.ImageCreate(ctx, ImageCreateRequest{TemplateID: tpl.ImageID, Preset: "square_post"})
	mustCode(t, err, command.CodeValidation)

	list, err := s.ImageList(ctx, ImageListRequest{Templates: "only"})
	if err != nil || len(list.Images) != 1 || list.Images[0].ImageID != tpl.ImageID {
		t.Fatalf("%v %+v", err, list)
	}

	all, _ := s.ImageList(ctx, ImageListRequest{Limit: 1})
	if len(all.Images) != 1 || all.NextCursor == nil {
		t.Fatalf("%+v", all)
	}

	rest, err := s.ImageList(ctx, ImageListRequest{Limit: 1, Cursor: *all.NextCursor})
	if err != nil || len(rest.Images) != 1 || rest.NextCursor != nil {
		t.Fatalf("%v %+v", err, rest)
	}

	_, err = s.ImageList(ctx, ImageListRequest{Templates: "x"})
	mustCode(t, err, command.CodeValidation)
}

func TestImageDescribeStaysInsideTheBudget(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:agent")
	c, _ := s.ImageCreate(ctx, ImageCreateRequest{Width: 800, Height: 600})

	var batch []command.Operation

	for range 120 {
		batch = append(
			batch,
			command.Operation{
				Op: "layer.add_text",
				Args: json.RawMessage(
					`{"text": "A fairly long text for a layer that is one of very many", "name": "A layer with a name that is long too"}`,
				),
			},
		)
	}

	if _, err := s.ImageEdit(
		ctx,
		ImageEditRequest{ImageID: c.ImageID, BaseRevision: 1, Operations: batch},
	); err != nil {
		t.Fatal(err)
	}

	d, err := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: c.ImageID})
	if err != nil || len(d.Text) > coreimage.DescribeBudget || !strings.Contains(d.Text, "more layers") ||
		len(d.Layers) != 120 {
		t.Fatalf("%v %d", err, len(d.Text))
	}
}

func networkUser(id string) context.Context {
	return WithCaller(context.Background(), Caller{
		Actor: "user:" + id, OwnerUserID: id, Scopes: []string{ScopeRead, ScopeEdit, ScopeMedia, ScopeRender},
	})
}

// holdAsset makes a project of the owner hold the prepared image.
func holdAsset(s *svc, owner string) {
	st := s.store.(*memStore)
	st.projects.heads = append(st.projects.heads, ProjectHead{ID: "prj_" + owner, OwnerUserID: &owner})
	st.assets.projects["prj_"+owner] = []ProjectAssetRow{
		{ProjectID: "prj_" + owner, AssetHash: imgHash, Name: "face.png"},
	}
}

// In network mode an image can only be used by a user who holds it in a project: another user could otherwise
// put a file they never imported into a document, export it and learn its name.
func TestImageAssetOfAnotherUserIsNotUsable(t *testing.T) {
	s, _ := newImageService(t)
	holdAsset(s, "alice")

	st0 := s.store.(*memStore)
	row := st0.assets.rows[imgHash]
	row.OriginalName = "/home/bob/private-name.png"
	st0.assets.rows[imgHash] = row

	alice, bob := networkUser("alice"), networkUser("bob")
	add := ops("layer.add_image", `{"asset": "`+imgHash+`", "id": "lyr_aaaaaaaaaa"}`)

	ca, err := s.ImageCreate(alice, ImageCreateRequest{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}

	cb, err := s.ImageCreate(bob, ImageCreateRequest{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.ImageEdit(bob, ImageEditRequest{ImageID: cb.ImageID, BaseRevision: 1, Operations: add})
	if se := mustCode(t, err, command.CodeNotFound); strings.Contains(se.Message+se.Hint, "face.png") {
		t.Fatalf("the name of a file of another user is not returned: %+v", se)
	}

	if _, err := s.ImageEdit(
		alice,
		ImageEditRequest{ImageID: ca.ImageID, BaseRevision: 1, Operations: add},
	); err != nil {
		t.Fatalf("the user who holds the file may use it: %v", err)
	}

	// another user imported the same bytes under a name of their own: the global row has it, the caller must not
	if d, _ := s.ImageDescribe(
		alice,
		ImageDescribeRequest{ImageID: ca.ImageID},
	); len(d.Layers) != 1 ||
		d.Layers[0].Name != "face.png" {
		t.Fatalf("the layer carries the name of the project entry of the caller, not the global one: %+v", d.Layers)
	}

	// a document that already names the file (a template shared by an administrator, an old revision) does not
	// export it for a user who does not hold it
	st := s.store.(*memStore)
	st.images.rows[cb.ImageID].Document = st.images.rows[ca.ImageID].Document

	_, err = s.ImageExport(bob, ImageExportRequest{ImageID: cb.ImageID})
	if err == nil {
		t.Fatal("bob exported the file of alice")
	}
}

func TestImageRefusesAProjectLimitedKeyEverywhere(t *testing.T) {
	s, _ := newImageService(t)
	limited := WithCaller(context.Background(), Caller{
		Actor: "key:k", ProjectID: "prj_aaaaaaaaaa", Scopes: []string{ScopeRead, ScopeEdit},
	})

	c, _ := s.ImageCreate(asActor("local:a"), ImageCreateRequest{Width: 10, Height: 10})
	add := ops("layer.add_fill", `{"color": "#000000"}`)

	calls := map[string]func() error{
		"create":   func() error { _, err := s.ImageCreate(limited, ImageCreateRequest{Width: 10, Height: 10}); return err },
		"list":     func() error { _, err := s.ImageList(limited, ImageListRequest{}); return err },
		"describe": func() error { _, err := s.ImageDescribe(limited, ImageDescribeRequest{ImageID: c.ImageID}); return err },
		"edit": func() error {
			_, err := s.ImageEdit(limited, ImageEditRequest{ImageID: c.ImageID, BaseRevision: 1, Operations: add})
			return err
		},
		"undo":   func() error { _, err := s.ImageUndo(limited, ImageStepRequest{ImageID: c.ImageID}); return err },
		"redo":   func() error { _, err := s.ImageRedo(limited, ImageStepRequest{ImageID: c.ImageID}); return err },
		"export": func() error { _, err := s.ImageExport(limited, ImageExportRequest{ImageID: c.ImageID}); return err },
	}

	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: a key limited to one project reached an image", name)
		} else {
			mustCode(t, err, CodeForbidden)
		}
	}
}

func TestImageCreateRefusesAnIDOfTheCallerInNetworkMode(t *testing.T) {
	s, _ := newImageService(t)

	if _, err := s.ImageCreate(
		asActor("local:a"),
		ImageCreateRequest{Width: 10, Height: 10, ID: "img_aaaaaaaaaa"},
	); err != nil {
		t.Fatalf("local mode takes an id: %v", err)
	}

	_, err := s.ImageCreate(networkUser("bob"), ImageCreateRequest{Width: 10, Height: 10, ID: "img_aaaaaaaaaa"})
	mustCode(t, err, command.CodeValidation)

	_, err = s.ImageCreate(networkUser("bob"), ImageCreateRequest{Width: 10, Height: 10, ID: "img_bbbbbbbbbb"})
	mustCode(t, err, command.CodeValidation)
}

// A project entry with no name must not fall back to the global name, which another user may have set.
func TestImageEmptyScopedNameDoesNotFallBackToTheGlobalOne(t *testing.T) {
	s, _ := newImageService(t)
	holdAsset(s, "alice")

	st := s.store.(*memStore)
	st.assets.projects["prj_alice"][0].Name = ""

	row := st.assets.rows[imgHash]
	row.OriginalName = "/home/bob/private-name.png"
	st.assets.rows[imgHash] = row

	alice := networkUser("alice")
	c, _ := s.ImageCreate(alice, ImageCreateRequest{Width: 100, Height: 100})

	if _, err := s.ImageEdit(alice, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1, Operations: ops("layer.add_image", `{"asset": "`+imgHash+`"}`),
	}); err != nil {
		t.Fatal(err)
	}

	d, _ := s.ImageDescribe(alice, ImageDescribeRequest{ImageID: c.ImageID})
	if len(d.Layers) != 1 || strings.Contains(d.Layers[0].Name, "private") || d.Layers[0].Name != "Image" {
		t.Fatalf("the name is neutral: %+v", d.Layers)
	}
}
