package service

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// memCheckpoints is the checkpoints of the image tests: the rules of the sqlite repository, in memory.
type memCheckpoints struct {
	mu   sync.Mutex
	rows map[string]CheckpointRow
}

func (m *memCheckpoints) key(id, name string) string { return id + "|" + name }

func (m *memCheckpoints) Create(_ context.Context, row CheckpointRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.rows[m.key(row.ProjectID, row.Name)]; ok {
		return ErrExists
	}

	m.rows[m.key(row.ProjectID, row.Name)] = row

	return nil
}

func (m *memCheckpoints) Get(_ context.Context, id, name string) (CheckpointRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	row, ok := m.rows[m.key(id, name)]
	if !ok {
		return CheckpointRow{}, ErrNotFound
	}

	return row, nil
}

func (m *memCheckpoints) List(_ context.Context, id string) ([]CheckpointRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []CheckpointRow

	for _, r := range m.rows {
		if r.ProjectID == id {
			out = append(out, r)
		}
	}

	slices.SortFunc(out, func(a, b CheckpointRow) int {
		if a.Revision != b.Revision {
			return int(b.Revision - a.Revision)
		}

		return strings.Compare(a.Name, b.Name)
	})

	return out, nil
}

func (m *memCheckpoints) Rename(_ context.Context, id, name, newName string, note *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	row, ok := m.rows[m.key(id, name)]
	if !ok {
		return ErrNotFound
	}

	if _, taken := m.rows[m.key(id, newName)]; taken && newName != name {
		return ErrExists
	}

	delete(m.rows, m.key(id, name))

	row.Name, row.Auto = newName, false
	if note != nil {
		row.Note = *note
	}

	m.rows[m.key(id, newName)] = row

	return nil
}

func (m *memCheckpoints) Remove(_ context.Context, id, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.rows[m.key(id, name)]; !ok {
		return ErrNotFound
	}

	delete(m.rows, m.key(id, name))

	return nil
}

func (m *memCheckpoints) TrimAuto(_ context.Context, id string, keep int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var auto []CheckpointRow

	for _, r := range m.rows {
		if r.ProjectID == id && r.Auto {
			auto = append(auto, r)
		}
	}

	slices.SortFunc(auto, func(a, b CheckpointRow) int {
		if a.AtMs != b.AtMs {
			return int(b.AtMs - a.AtMs)
		}

		if a.Revision != b.Revision {
			return int(b.Revision - a.Revision)
		}

		return strings.Compare(b.Name, a.Name)
	})

	for i := keep; i < len(auto); i++ {
		delete(m.rows, m.key(id, auto[i].Name))
	}

	return nil
}

// imageHistoryEnv is an image with one revision per step of a short scenario, and the canonical bytes of the
// document at each of them, so that a revert can be compared with the document it must give back.
type imageHistoryEnv struct {
	t     *testing.T
	s     *svc
	id    string
	docs  map[int64][]byte
	clock time.Time
}

func newImageHistoryEnv(t *testing.T, actor string) *imageHistoryEnv {
	t.Helper()

	s, _ := newImageService(t)
	e := &imageHistoryEnv{t: t, s: s, docs: map[int64][]byte{}, clock: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	s.now = func() time.Time { return e.clock }

	created, err := s.ImageCreate(asActor(actor), ImageCreateRequest{Preset: "video_thumbnail", Name: "Episode 12"})
	if err != nil {
		t.Fatal(err)
	}

	e.id = created.ImageID
	e.remember(1)

	return e
}

func (e *imageHistoryEnv) remember(rev int64) {
	e.t.Helper()

	row, err := e.s.store.Images().Get(context.Background(), e.id)
	if err != nil || row.Revision != rev {
		e.t.Fatalf("the image is at r%d (%v), want r%d", row.Revision, err, rev)
	}

	e.docs[rev] = bytes.Clone(row.Document)
}

func (e *imageHistoryEnv) document() []byte {
	e.t.Helper()

	row, err := e.s.store.Images().Get(context.Background(), e.id)
	if err != nil {
		e.t.Fatal(err)
	}

	return row.Document
}

func (e *imageHistoryEnv) revision() int64 {
	e.t.Helper()

	row, err := e.s.store.Images().Get(context.Background(), e.id)
	if err != nil {
		e.t.Fatal(err)
	}

	return row.Revision
}

func (e *imageHistoryEnv) edit(actor string, base int64, pairs ...string) *ImageEditResult {
	e.t.Helper()

	res, err := e.s.ImageEdit(
		asActor(actor),
		ImageEditRequest{ImageID: e.id, BaseRevision: base, Operations: ops(pairs...)},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	if res.Applied {
		e.remember(res.Revision)
	}

	return res
}

// scenario makes r2 to r5: a text, a shape, a change of the text and the removal of the shape.
func (e *imageHistoryEnv) scenario(actor string) {
	e.t.Helper()

	e.edit(actor, 1, "layer.add_text", `{"text": "HOW IT WORKS", "id": "lyr_aaaaaaaaaa"}`)
	e.edit(
		actor,
		2,
		"layer.add_shape",
		`{"shape": "rect", "w": 300, "h": 60, "fill": "#E63946", "id": "lyr_bbbbbbbbbb"}`,
	)
	e.edit(actor, 3, "layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 60, "name": "Headline"}`)
	e.edit(actor, 4, "layer.remove", `{"layer_id": "lyr_bbbbbbbbbb"}`)
}

func (e *imageHistoryEnv) opCount() int {
	e.t.Helper()

	rows, err := e.s.store.Images().OpsSince(context.Background(), e.id, 0)
	if err != nil {
		e.t.Fatal(err)
	}

	return len(rows)
}

// Criterion 1: the revert gives a document of the canonical bytes of the revision, as a new revision that undo
// takes back, and the log keeps every row.
func TestImageRevertThroughTheService(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:ui")
	e.scenario("local:ui") // r1..r5
	ctx := asActor("local:ui")

	res, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Revision: 3})
	if err != nil {
		t.Fatal(err)
	}

	if !res.Applied || res.Revision != 6 || res.BaseRevision != 5 || res.Target != 3 || res.Kind != "revert" ||
		res.Same {
		t.Fatalf("result = %+v", res.ImageEditResult)
	}

	if !bytes.Equal(e.document(), e.docs[3]) {
		t.Fatal("the document after the revert is not the document of r3")
	}

	// The note about quoted text comes first, then the state with the target at the end of its line.
	lines := strings.Split(res.Text, "\n")
	if len(res.Diff) != 2 || !strings.HasPrefix(lines[1], "reverted r6 (base r5) by local:ui: changes=2") ||
		!strings.HasSuffix(lines[1], ", to r3") {
		t.Errorf("diff %d, text:\n%s", len(res.Diff), res.Text)
	}

	// The log kept everything and the revert is an edit of the actor with its own inverse.
	if e.opCount() != 6 {
		t.Fatalf("the log has %d rows, want 6", e.opCount())
	}

	rows, _ := e.s.store.Images().OpsSince(context.Background(), e.id, 5)
	if rows[0].Kind != OpEdit || !strings.Contains(string(rows[0].Ops), `"image.revert"`) ||
		!strings.Contains(string(rows[0].Ops), `"revision":3`) {
		t.Errorf("the row of the revert: %s %s", rows[0].Kind, rows[0].Ops)
	}

	// The way back from a revert is the undo of the same actor, and it gives the document before the revert.
	undone, err := e.s.ImageUndo(ctx, ImageStepRequest{ImageID: e.id})
	if err != nil || !undone.Applied || undone.Step != 6 {
		t.Fatalf("undo of the revert: %v %+v", err, undone)
	}

	if !bytes.Equal(e.document(), e.docs[5]) {
		t.Fatal("revert then undo does not give the document before the revert")
	}

	// A revert to a revision is the document of the revision whatever came between: back to r1 from r7.
	if _, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Revision: 1}); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(e.document(), e.docs[1]) {
		t.Fatal("the document after the revert is not the document of r1")
	}
}

// The revert to a revision that holds the current document writes nothing, and the arguments are checked.
func TestImageRevertThroughTheServiceRefusesWhatItCannotDo(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:ui")
	e.scenario("local:ui")
	ctx := asActor("local:ui")

	// r4 and r5 differ (r5 removed a layer): undo r5 makes r6 equal to r4, so a revert to r4 says nothing to do.
	if _, err := e.s.ImageUndo(ctx, ImageStepRequest{ImageID: e.id}); err != nil {
		t.Fatal(err)
	}

	before := e.opCount()

	same, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Revision: 4})
	if err != nil || same.Applied || !same.Same || e.opCount() != before ||
		!strings.Contains(same.Text, "reverted nothing: the image already equals r4") {
		t.Fatalf("same document: %v %+v", err, same)
	}

	for name, req := range map[string]ImageRevertRequest{
		"both":                   {Revision: 2, Checkpoint: "x"},
		"neither":                {},
		"a revision of the past": {Revision: -1},
		"a revision ahead":       {Revision: 99},
		"a negative base":        {Revision: 2, BaseRevision: -1},
	} {
		req.ImageID = e.id

		if _, err := e.s.ImageRevert(ctx, req); err == nil {
			t.Errorf("%s was accepted", name)
		} else {
			mustCode(t, err, command.CodeValidation)
		}
	}

	_, err = e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Checkpoint: "nope"})
	if se := mustCode(t, err, command.CodeNotFound); se.Details["checkpoint"] != "nope" {
		t.Errorf("details = %v", se.Details)
	}

	_, err = e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: "img_zzzzzzzzzz", Revision: 1})
	mustCode(t, err, command.CodeNotFound)
}

// Criterion 2: a dry run returns the diff and writes nothing.
func TestImageRevertDryRun(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:claude-code")
	e.scenario("local:claude-code")

	before, rows := e.document(), e.opCount()

	cps, _ := e.s.store.ImageCheckpoints().List(context.Background(), e.id)

	res, err := e.s.ImageRevert(
		asActor("local:claude-code"),
		ImageRevertRequest{ImageID: e.id, Revision: 2, DryRun: true},
	)
	if err != nil {
		t.Fatal(err)
	}

	if res.Applied || !res.DryRun || res.Revision != 6 || len(res.Diff) == 0 || !strings.HasPrefix(res.Text, "note:") ||
		!strings.Contains(res.Text, "dry run, would revert as r6 (base r5)") {
		t.Fatalf("result = %+v\n%s", res.ImageEditResult, res.Text)
	}

	if !bytes.Equal(e.document(), before) || e.opCount() != rows || e.revision() != 5 {
		t.Fatal("a dry run wrote something")
	}

	after, _ := e.s.store.ImageCheckpoints().List(context.Background(), e.id)
	if len(after) != len(cps) {
		t.Fatalf("a dry run made a checkpoint: %d, was %d", len(after), len(cps))
	}
}

// The base revision is the conflict check of SDD-002 A.7: another actor changed a layer the revert changes.
func TestImageRevertConflictsWithAnotherActor(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:ui")
	e.scenario("local:ui") // r5
	e.edit("local:agent", 5, "layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "name": "Agent title"}`)

	// The person read r5 and goes back to r2; the agent changed the layer the revert changes.
	_, err := e.s.ImageRevert(asActor("local:ui"), ImageRevertRequest{ImageID: e.id, Revision: 2, BaseRevision: 5})
	mustCode(t, err, command.CodeRevisionConflict)

	if e.revision() != 6 {
		t.Fatalf("a conflict wrote: r%d", e.revision())
	}

	// Without a base the revert is on the current revision and goes through.
	if _, err := e.s.ImageRevert(asActor("local:ui"), ImageRevertRequest{ImageID: e.id, Revision: 2}); err != nil {
		t.Fatal(err)
	}
}

// The log and the document must agree: a row that cannot be crossed, a walk that is too long and a digest that
// does not match are each an error and never a document to continue from.
func TestImageRevertRefusesALogItCannotRebuild(t *testing.T) {
	t.Parallel()

	ctx := asActor("local:ui")

	t.Run("a step of another kind", func(t *testing.T) {
		t.Parallel()

		e := newImageHistoryEnv(t, "local:ui")
		e.scenario("local:ui")

		mem := e.s.store.(*memStore).images
		mem.ops[e.id][3].Kind = OpImport

		_, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Revision: 2})
		mustCode(t, err, command.CodeInvalidState)

		// Past that row the revert still works.
		if _, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Revision: 4}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a walk of more than 5000 rows", func(t *testing.T) {
		t.Parallel()

		e := newImageHistoryEnv(t, "local:ui")
		e.s.store.(*memStore).images.rows[e.id].Revision = maxRebuildSteps + 10

		_, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Revision: 1})
		se := mustCode(t, err, command.CodeLimit)

		if !strings.Contains(se.Hint, "checkpoint") {
			t.Errorf("hint = %q", se.Hint)
		}
	})

	t.Run("a digest that does not match", func(t *testing.T) {
		t.Parallel()

		e := newImageHistoryEnv(t, "local:ui")
		e.scenario("local:ui")
		e.s.store.(*memStore).images.ops[e.id][1].DocSHA256 = strings.Repeat("0", 64)

		_, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Revision: 2})
		se := mustCode(t, err, command.CodeInternal)

		if se.Details["image_id"] != e.id || se.Details["revision"] != int64(2) {
			t.Errorf("details = %v", se.Details)
		}

		if e.revision() != 5 {
			t.Fatal("a log that does not match was continued from")
		}
	})
}

// Criterion 3: a named checkpoint survives later edits and a revert goes to it by its name.
func TestImageCheckpoint(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:ui")
	ctx := asActor("local:ui")

	e.edit("local:ui", 1, "layer.add_text", `{"text": "First", "id": "lyr_aaaaaaaaaa"}`)

	cp, err := e.s.ImageCheckpointCreate(ctx, ImageCheckpointCreateRequest{
		ImageID: e.id, Name: "  before the agent \x07", Note: "the title is right",
	})
	if err != nil || cp.Name != "before the agent" || cp.Revision != 2 || cp.Auto || cp.Actor != "local:ui" {
		t.Fatalf("create = %v %+v", err, cp)
	}

	e.edit("local:ui", 2, "layer.add_shape", `{"shape": "rect", "w": 100, "h": 100, "id": "lyr_bbbbbbbbbb"}`)
	e.edit("local:ui", 3, "layer.remove", `{"layer_id": "lyr_aaaaaaaaaa"}`)

	list, err := e.s.ImageCheckpointList(ctx, ImageCheckpointListRequest{ImageID: e.id})
	if err != nil || len(list.Checkpoints) != 1 || list.Revision != 4 || list.Checkpoints[0].Revision != 2 ||
		!strings.Contains(list.Text, `r2 "before the agent" local:ui`) || !strings.HasPrefix(list.Text, "note:") ||
		!strings.Contains(list.Text, "checkpoints "+e.id+" rev 4") {
		t.Fatalf("list = %v %+v\n%s", err, list, list.Text)
	}

	res, err := e.s.ImageRevert(ctx, ImageRevertRequest{ImageID: e.id, Checkpoint: "before the agent"})
	if err != nil || res.Target != 2 || !bytes.Equal(e.document(), e.docs[2]) {
		t.Fatalf("revert by name = %v %+v", err, res)
	}

	// A name is unique in its image, an unknown one is not found, and no revision goes with a checkpoint.
	_, err = e.s.ImageCheckpointCreate(ctx, ImageCheckpointCreateRequest{ImageID: e.id, Name: "before the agent"})
	if se := mustCode(t, err, command.CodeIDExists); se.Path != "name" {
		t.Errorf("path = %q", se.Path)
	}

	for _, req := range []ImageCheckpointCreateRequest{
		{Name: ""}, {Name: "a/b"}, {Name: strings.Repeat("x", 61)}, {Name: "ahead", Revision: 99},
	} {
		req.ImageID = e.id

		_, err = e.s.ImageCheckpointCreate(ctx, req)
		mustCode(t, err, command.CodeValidation)
	}

	note := "the hero"

	renamed, err := e.s.ImageCheckpointRename(ctx, ImageCheckpointRenameRequest{
		ImageID: e.id, Name: "before the agent", NewName: "hero", Note: &note,
	})
	if err != nil || renamed.Name != "hero" || renamed.Note != "the hero" || renamed.Revision != 2 {
		t.Fatalf("rename = %v %+v", err, renamed)
	}

	_, err = e.s.ImageCheckpointRename(ctx, ImageCheckpointRenameRequest{ImageID: e.id, Name: "nope", NewName: "x"})
	mustCode(t, err, command.CodeNotFound)

	removed, err := e.s.ImageCheckpointRemove(ctx, ImageCheckpointRemoveRequest{ImageID: e.id, Name: "hero"})
	if err != nil || removed.Name != "hero" {
		t.Fatalf("remove = %v %+v", err, removed)
	}

	if e.opCount() != 5 {
		t.Fatalf("a checkpoint changed the log: %d rows, want 5", e.opCount())
	}

	_, err = e.s.ImageCheckpointRemove(ctx, ImageCheckpointRemoveRequest{ImageID: e.id, Name: "hero"})
	mustCode(t, err, command.CodeNotFound)

	_, err = e.s.ImageCheckpointList(ctx, ImageCheckpointListRequest{ImageID: "img_zzzzzzzzzz"})
	mustCode(t, err, command.CodeNotFound)

	// A checkpoint goes with its image, and another image has its own names.
	other, err := e.s.ImageCreate(ctx, ImageCreateRequest{Preset: "square_post"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.s.ImageCheckpointCreate(
		ctx,
		ImageCheckpointCreateRequest{ImageID: other.ImageID, Name: "hero"},
	); err != nil {
		t.Fatalf("the name of another image: %v", err)
	}
}

// Criterion 4: an agent's first edit of the day makes the automatic checkpoint at the revision the edit started
// from, a second edit that day does not make another, and a person's edit makes none.
func TestImageCheckpointAutomatic(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:ui")
	ctx := context.Background()
	repo := e.s.store.ImageCheckpoints()

	// A person's edit and a dry run of an agent's make none.
	e.edit("local:ui", 1, "layer.add_text", `{"text": "Mine", "id": "lyr_aaaaaaaaaa"}`)

	if _, err := e.s.ImageEdit(asActor("local:claude-code"), ImageEditRequest{
		ImageID: e.id, BaseRevision: 2, DryRun: true,
		Operations: ops("layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 50}`),
	}); err != nil {
		t.Fatal(err)
	}

	if rows, _ := repo.List(ctx, e.id); len(rows) != 0 {
		t.Fatalf("a person's edit and a dry run made %d checkpoints", len(rows))
	}

	// An edit that changes nothing makes none either.
	e.edit("local:claude-code", 2, "layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 100}`)

	if rows, _ := repo.List(ctx, e.id); len(rows) != 0 {
		t.Fatalf("an edit that changed nothing made %d checkpoints", len(rows))
	}

	// The first edit of the day of an agent.
	e.edit("local:claude-code", 2, "layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 70}`)

	rows, _ := repo.List(ctx, e.id)
	if len(rows) != 1 || rows[0].Name != "auto local:claude-code 2026-11-20" || rows[0].Revision != 2 ||
		!rows[0].Auto ||
		rows[0].Actor != "local:claude-code" {
		t.Fatalf("checkpoints = %+v", rows)
	}

	// The second edit of that day finds it.
	e.edit("local:claude-code", 3, "layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 40}`)

	if rows, _ = repo.List(ctx, e.id); len(rows) != 1 || rows[0].Revision != 2 {
		t.Fatalf("a second edit that day made another: %+v", rows)
	}

	// Another agent has its own, and another day another one, at the revision the edit starts from.
	e.edit("key:key_h8v2n5q7w3", 4, "layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 30}`)

	e.clock = e.clock.Add(24 * time.Hour)
	e.edit("local:claude-code", 5, "layer.set", `{"layer_id": "lyr_aaaaaaaaaa", "opacity": 20}`)

	names := map[string]int64{}

	rows, _ = repo.List(ctx, e.id)
	for _, r := range rows {
		names[r.Name] = r.Revision
	}

	if want := map[string]int64{
		"auto local:claude-code 2026-11-20": 2, "auto key:key_h8v2n5q7w3 2026-11-20": 4,
		"auto local:claude-code 2026-11-21": 5,
	}; len(names) != 3 || names["auto local:claude-code 2026-11-20"] != 2 ||
		names["auto key:key_h8v2n5q7w3 2026-11-20"] != 4 || names["auto local:claude-code 2026-11-21"] != 5 {
		t.Fatalf("checkpoints = %v, want %v", names, want)
	}

	// The automatic checkpoint is what a person reverts to after the agent worked.
	if _, err := e.s.ImageRevert(asActor("local:ui"), ImageRevertRequest{
		ImageID: e.id, Checkpoint: "auto local:claude-code 2026-11-20",
	}); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(e.document(), e.docs[2]) {
		t.Fatal("the revert to the automatic checkpoint is not the document the agent started from")
	}
}

// The ceiling: at most 30 automatic checkpoints are kept, and a named one, or an automatic one that was renamed,
// is never removed by it.
func TestImageCheckpointCeilingKeepsNamedOnes(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:ui")
	ctx := context.Background()
	repo := e.s.store.ImageCheckpoints()

	e.edit("local:ui", 1, "layer.add_text", `{"text": "x", "id": "lyr_aaaaaaaaaa"}`)

	if _, err := e.s.ImageCheckpointCreate(asActor("local:ui"), ImageCheckpointCreateRequest{
		ImageID: e.id, Name: "keep me", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}

	for day := range maxAutoCheckpoints + 5 {
		e.clock = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC).AddDate(0, 0, day)
		e.edit("local:claude-code", e.revision(), "layer.set",
			fmt.Sprintf(`{"layer_id": "lyr_aaaaaaaaaa", "opacity": %d}`, 1+day))

		if day == 0 {
			// The first automatic checkpoint is renamed: it is a named one from now on.
			if _, err := e.s.ImageCheckpointRename(asActor("local:ui"), ImageCheckpointRenameRequest{
				ImageID: e.id, Name: "auto local:claude-code 2026-01-01", NewName: "the first",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	rows, _ := repo.List(ctx, e.id)

	var auto, named []string

	for _, r := range rows {
		if r.Auto {
			auto = append(auto, r.Name)
		} else {
			named = append(named, r.Name)
		}
	}

	slices.Sort(named)

	if len(auto) != maxAutoCheckpoints || !slices.Equal(named, []string{"keep me", "the first"}) {
		t.Fatalf("%d automatic, named %v", len(auto), named)
	}

	// The oldest automatic ones went: the days 2 to 6 of January are gone, the newest stays.
	if slices.Contains(auto, "auto local:claude-code 2026-01-02") ||
		!slices.Contains(auto, "auto local:claude-code 2026-02-04") {
		t.Fatalf("the ceiling removed the wrong ones: %v", auto)
	}
}

// The diff is of two documents and not of the log: the text names each change, the entries carry them all, and an
// edit followed by its undo has nothing to say.
func TestImageDiffThroughTheService(t *testing.T) {
	t.Parallel()

	e := newImageHistoryEnv(t, "local:ui")
	e.scenario("local:ui") // r1..r5
	ctx := asActor("local:ui")

	res, err := e.s.ImageDiff(ctx, ImageDiffRequest{ImageID: e.id, From: 1, Until: 4})
	if err != nil || res.Revision != 5 || res.From != 1 || res.To != 4 || res.Count != 2 || len(res.Entries) != 2 {
		t.Fatalf("diff = %v %+v", err, res)
	}

	for _, want := range []string{
		textviewNote, "diff " + e.id + " r1 -> r4 (2 changes)", `added text "Headline" lyr_aaaaaaaaaa`,
		`added shape "Shape" lyr_bbbbbbbbbb`,
	} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("the text lacks %q:\n%s", want, res.Text)
		}
	}

	// From a checkpoint to the current revision; r5 removed the shape r4 had.
	if _, err := e.s.ImageCheckpointCreate(
		ctx,
		ImageCheckpointCreateRequest{ImageID: e.id, Name: "r4", Revision: 4},
	); err != nil {
		t.Fatal(err)
	}

	res, err = e.s.ImageDiff(ctx, ImageDiffRequest{ImageID: e.id, FromCheckpoint: "r4"})
	if err != nil || res.To != 5 || res.Count != 1 || res.Entries[0].Verb != "removed" ||
		res.Entries[0].Noun != "layer" {
		t.Fatalf("diff from a checkpoint = %v %+v", err, res)
	}

	// An undo and the revision before it are the same document.
	if _, err := e.s.ImageUndo(ctx, ImageStepRequest{ImageID: e.id}); err != nil {
		t.Fatal(err)
	}

	res, err = e.s.ImageDiff(ctx, ImageDiffRequest{ImageID: e.id, From: 4})
	if err != nil || res.Count != 0 || !strings.HasSuffix(res.Text, "(no changes)") {
		t.Fatalf("diff across an undo = %v %+v", err, res)
	}

	for name, req := range map[string]ImageDiffRequest{
		"neither": {}, "both": {From: 1, FromCheckpoint: "r4"}, "ahead": {From: 1, Until: 99}, "zero": {From: 99},
	} {
		req.ImageID = e.id

		if _, err := e.s.ImageDiff(ctx, req); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	_, err = e.s.ImageDiff(ctx, ImageDiffRequest{ImageID: e.id, FromCheckpoint: "nope"})
	mustCode(t, err, command.CodeNotFound)
}

const textviewNote = "note: text in double quotes"
