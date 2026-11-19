package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

var ctx = context.Background()

func testStore(t *testing.T) *Store {
	t.Helper()
	return openStore(t, newDir(t), Options{})
}

func newProject(id string, updated int64) service.ProjectRow {
	return service.ProjectRow{
		ID: id, Name: "name " + id, State: service.ProjectActive, SchemaVersion: 1,
		Summary:   service.DocSummary{Width: 1080, Height: 1920, FPSNum: 30, FPSDen: 1},
		CreatedMs: updated, UpdatedMs: updated, CreatedBy: "local:ui",
		Document: []byte(`{"name":"` + id + `"}`), DocSHA256: "sha-" + id,
	}
}

func mustCreate(t *testing.T, s service.Repos, rows ...service.ProjectRow) {
	t.Helper()
	for i := range rows {
		if err := s.Projects().Create(ctx, rows[i]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProjectsCreateGetCommit(t *testing.T) {
	s := testStore(t)
	row := newProject("prj_a", 100)
	owner := "usr_1"
	row.OwnerUserID = &owner
	mustCreate(t, s, row)

	if err := s.Projects().Create(ctx, row); !errors.Is(err, service.ErrExists) {
		t.Errorf("duplicate Create = %v, want ErrExists", err)
	}
	got, err := s.Projects().Get(ctx, "prj_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 0 || string(got.Document) != `{"name":"prj_a"}` || got.DocSHA256 != "sha-prj_a" ||
		got.OwnerUserID == nil || *got.OwnerUserID != "usr_1" || got.TrashedMs != nil || got.Summary.Width != 1080 {
		t.Errorf("Get = %+v", got)
	}
	if _, err := s.Projects().Get(ctx, "prj_none"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("Get missing = %v", err)
	}

	floor := int64(1)
	upd := service.ProjectUpdate{
		Name:          "renamed",
		SchemaVersion: 1,
		Document:      []byte(`{"v":2}`),
		DocSHA256:     "sha2",
		UpdatedMs:     200,
		Summary: service.DocSummary{
			Width:          1,
			Height:         2,
			FPSNum:         24,
			FPSDen:         1,
			DurationFrames: 90,
			Tracks:         2,
			Items:          3,
			Assets:         1,
		},
	}
	if err := s.Projects().Commit(ctx, "prj_a", 0, upd); err != nil {
		t.Fatal(err)
	}
	if err := s.Projects().Commit(ctx, "prj_a", 0, upd); !errors.Is(err, service.ErrStale) {
		t.Errorf("Commit with a stale revision = %v, want ErrStale", err)
	}
	if err := s.Projects().Commit(ctx, "prj_missing", 0, upd); !errors.Is(err, service.ErrStale) {
		t.Errorf("Commit of a missing project = %v, want ErrStale", err)
	}
	got, _ = s.Projects().Get(ctx, "prj_a")
	if got.Revision != 1 || got.Name != "renamed" || got.Summary.Items != 3 || got.UpdatedMs != 200 ||
		got.ResyncFloor != 0 {
		t.Errorf("after Commit: %+v", got)
	}

	upd.ResyncFloor = &floor
	if err := s.Projects().Commit(ctx, "prj_a", 1, upd); err != nil {
		t.Fatal(err)
	}
	head, _ := s.Projects().Head(ctx, "prj_a")
	if head.Revision != 2 || head.ResyncFloor != 1 {
		t.Errorf("Head = %+v", head)
	}
}

func TestProjectsListPagingAndTrash(t *testing.T) {
	s := testStore(t)
	for i := range 5 {
		mustCreate(t, s, newProject(fmt.Sprintf("prj_%d", i), int64(100+i)))
	}
	mustCreate(t, s, newProject("prj_tie", 102)) // same updated_ms as prj_2

	var ids []string
	cursor := ""
	for {
		items, next, err := s.Projects().List(ctx, service.ProjectQuery{Limit: 2, After: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range items {
			ids = append(ids, h.ID)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	want := []string{"prj_4", "prj_3", "prj_tie", "prj_2", "prj_1", "prj_0"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("paged order = %v, want %v", ids, want)
	}

	if err := s.Projects().SetState(ctx, "prj_3", service.ProjectTrashed, "local:ui", 500); err != nil {
		t.Fatal(err)
	}
	active, _, _ := s.Projects().List(ctx, service.ProjectQuery{})
	all, _, _ := s.Projects().List(ctx, service.ProjectQuery{IncludeTrashed: true})
	trashed, _, _ := s.Projects().List(ctx, service.ProjectQuery{State: service.ProjectTrashed})
	if len(active) != 5 || len(all) != 6 || len(trashed) != 1 || trashed[0].ID != "prj_3" {
		t.Errorf("active %d, all %d, trashed %v", len(active), len(all), trashed)
	}
	h, _ := s.Projects().Head(ctx, "prj_3")
	if h.State != service.ProjectTrashed || h.TrashedBy == nil || *h.TrashedBy != "local:ui" || *h.TrashedMs != 500 ||
		h.UpdatedMs != 500 ||
		h.Revision != 0 {
		t.Errorf("trashed head = %+v", h)
	}
	if err := s.Projects().SetState(ctx, "prj_3", service.ProjectActive, "local:ui", 600); err != nil {
		t.Fatal(err)
	}
	h, _ = s.Projects().Head(ctx, "prj_3")
	if h.State != service.ProjectActive || h.TrashedMs != nil || h.TrashedBy != nil {
		t.Errorf("restored head = %+v", h)
	}
	if err := s.Projects().
		SetState(ctx, "prj_none", service.ProjectTrashed, "x", 1); !errors.Is(
		err,
		service.ErrNotFound,
	) {
		t.Errorf("SetState missing = %v", err)
	}
	if _, _, err := s.Projects().List(ctx, service.ProjectQuery{After: "!!"}); !errors.Is(err, ErrBadCursor) {
		t.Errorf("bad cursor = %v", err)
	}
}

func op(project string, rev int64, kind service.OperationKind, actor string) service.OperationRow {
	return service.OperationRow{
		ProjectID: project, Revision: rev, Kind: kind, Actor: actor, BaseRevision: rev - 1, AtMs: rev,
		Ops: json.RawMessage(`[]`), Inverse: json.RawMessage(`[]`), Diff: json.RawMessage(`[]`),
		SchemaVersion: 1, DocSHA256: fmt.Sprintf("sha-%d", rev),
	}
}

func TestOperationsAppendQueriesAndUndoChain(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	ops := s.Operations()

	// The worked example of SDD-005 section 12.1 for one actor.
	const a = "local:ui"
	target := func(n int64) *int64 { return &n }
	rows := []service.OperationRow{
		op("prj_a", 5, service.OpEdit, a), op("prj_a", 6, service.OpEdit, a),
	}
	rows[0].BaseRevision, rows[1].BaseRevision = 0, 5
	for _, r := range rows {
		if err := ops.Append(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if err := ops.Append(ctx, rows[0]); !errors.Is(err, service.ErrExists) {
		t.Errorf("duplicate revision = %v, want ErrExists", err)
	}

	candidate := func(redo bool) int64 {
		var r service.OperationRow
		var err error
		if redo {
			r, err = ops.RedoCandidate(ctx, "prj_a", a)
		} else {
			r, err = ops.UndoCandidate(ctx, "prj_a", a)
		}
		if errors.Is(err, service.ErrNotFound) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return r.Revision
	}
	undo := func(rev int64, tgt int64) {
		u := op("prj_a", rev, service.OpUndo, a)
		u.TargetRevision = target(tgt)
		if err := ops.Append(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err := ops.Close(ctx, "prj_a", tgt, rev); err != nil {
			t.Fatal(err)
		}
	}

	if candidate(false) != 6 || candidate(true) != 0 {
		t.Fatal("after 5, 6: undo candidate must be 6 and redo none")
	}
	undo(7, 6)
	if candidate(false) != 5 || candidate(true) != 7 {
		t.Errorf("after undo 7: undo %d redo %d, want 5 and 7", candidate(false), candidate(true))
	}
	undo(8, 5)
	if candidate(false) != 0 || candidate(true) != 8 {
		t.Errorf("after undo 8: undo %d redo %d, want 0 and 8", candidate(false), candidate(true))
	}
	// redo 9 consumes undo 8 and is itself undoable.
	r9 := op("prj_a", 9, service.OpRedo, a)
	r9.TargetRevision = target(8)
	if err := ops.Append(ctx, r9); err != nil {
		t.Fatal(err)
	}
	if err := ops.Close(ctx, "prj_a", 8, 9); err != nil {
		t.Fatal(err)
	}
	if candidate(false) != 9 || candidate(true) != 7 {
		t.Errorf("after redo 9: undo %d redo %d, want 9 and 7", candidate(false), candidate(true))
	}
	// A new edit ends the redo chain.
	if err := ops.Append(ctx, op("prj_a", 10, service.OpEdit, a)); err != nil {
		t.Fatal(err)
	}
	if err := ops.CloseOpenRedo(ctx, "prj_a", a, 10); err != nil {
		t.Fatal(err)
	}
	if candidate(false) != 10 || candidate(true) != 0 {
		t.Errorf("after edit 10: undo %d redo %d, want 10 and none", candidate(false), candidate(true))
	}

	// Another actor has an independent chain.
	if err := ops.Append(ctx, op("prj_a", 11, service.OpEdit, "local:claude-code")); err != nil {
		t.Fatal(err)
	}
	if candidate(false) != 10 {
		t.Error("the other actor's edit must not change this actor's chain")
	}
	other, err := ops.UndoCandidate(ctx, "prj_a", "local:claude-code")
	if err != nil || other.Revision != 11 {
		t.Errorf("other actor candidate = %v, %v", other.Revision, err)
	}

	if err := ops.Close(ctx, "prj_a", 10, 99); err != nil {
		t.Fatal(err)
	}
	if err := ops.Close(ctx, "prj_a", 10, 100); !errors.Is(err, service.ErrStale) {
		t.Errorf("closing a closed row = %v, want ErrStale", err)
	}

	// A migration ends every chain.
	if err := ops.Append(ctx, op("prj_a", 12, service.OpEdit, a)); err != nil {
		t.Fatal(err)
	}
	m := op("prj_a", 13, service.OpMigrate, "local:migrate")
	own := int64(13)
	m.ClosedBy = &own
	if err := ops.Append(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := ops.CloseAllOpen(ctx, "prj_a", 13); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{a, "local:claude-code", "local:migrate"} {
		if _, err := ops.UndoCandidate(ctx, "prj_a", actor); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("%s after migration: %v, want ErrNotFound", actor, err)
		}
	}
}

func TestOperationsSinceAndPage(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	skipped := json.RawMessage(`[{"reason":"object_missing"}]`)
	key := "key-1"
	for i := int64(1); i <= 6; i++ {
		actor := "local:ui"
		if i%2 == 0 {
			actor = "local:claude-code"
		}
		r := op("prj_a", i, service.OpEdit, actor)
		r.Diff = json.RawMessage(`[{"kind":"x","pad":"` + strings.Repeat("p", 100) + `"}]`)
		if i == 3 {
			r.Skipped, r.IdempotencyKey = skipped, &key
		}
		if err := s.Operations().Append(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Operations().Get(ctx, "prj_a", 3)
	if err != nil || string(got.Skipped) != string(skipped) || got.IdempotencyKey == nil ||
		*got.IdempotencyKey != key ||
		got.ClosedBy != nil {
		t.Errorf("Get = %+v, %v", got, err)
	}
	if _, err := s.Operations().Get(ctx, "prj_a", 99); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("Get missing = %v", err)
	}

	since, _ := s.Operations().Since(ctx, "prj_a", 2, service.OperationFilter{})
	if len(since) != 4 || since[0].Revision != 3 || since[3].Revision != 6 {
		t.Errorf("Since 2 = %d rows", len(since))
	}
	others, _ := s.Operations().Since(ctx, "prj_a", 0, service.OperationFilter{ExcludeActor: "local:ui"})
	if len(others) != 3 {
		t.Errorf("Since excluding local:ui = %d rows, want 3", len(others))
	}
	limited, _ := s.Operations().Since(ctx, "prj_a", 0, service.OperationFilter{Limit: 2})
	capped, _ := s.Operations().Since(ctx, "prj_a", 0, service.OperationFilter{MaxBytes: 250})
	if len(limited) != 2 || len(capped) != 2 {
		t.Errorf(
			"limit gave %d, MaxBytes gave %d rows; want 2 and 2 (stops after the row that crosses the limit)",
			len(limited),
			len(capped),
		)
	}

	page, more, err := s.Operations().Page(ctx, "prj_a", service.HistoryQuery{Limit: 4})
	if err != nil || !more || len(page) != 4 || page[0].Revision != 6 || page[3].Revision != 3 {
		t.Errorf("Page = %d rows more=%v %v", len(page), more, err)
	}
	page, more, _ = s.Operations().Page(ctx, "prj_a", service.HistoryQuery{Before: 3, Limit: 4})
	if more || len(page) != 2 || page[0].Revision != 2 {
		t.Errorf("Page before 3 = %d rows more=%v", len(page), more)
	}
	page, _, _ = s.Operations().Page(ctx, "prj_a", service.HistoryQuery{Actor: "local:claude-code", Limit: 10})
	if len(page) != 3 {
		t.Errorf("Page by actor = %d rows", len(page))
	}
}

func TestIdempotency(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	repo := s.Idempotency()
	row := service.IdempotencyRow{
		ProjectID: "prj_a", Actor: "local:ui", Key: "k-12345678", RequestHash: "h1",
		Revision: 1, Result: json.RawMessage(`{"revision":1}`), CreatedMs: 1000, ExpiresMs: 1000 + 86400000,
	}
	if err := repo.Put(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err := repo.Put(ctx, row); !errors.Is(err, service.ErrExists) {
		t.Errorf("Put over a live row = %v, want ErrExists", err)
	}
	got, err := repo.Get(ctx, "prj_a", "local:ui", "k-12345678", 2000)
	if err != nil || got.RequestHash != "h1" || string(got.Result) != `{"revision":1}` {
		t.Errorf("Get = %+v, %v", got, err)
	}
	if _, err := repo.Get(ctx, "prj_a", "local:claude-code", "k-12345678", 2000); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("another actor's key must not collide: %v", err)
	}
	expired := int64(1000 + 86400000 + 1000)
	if _, err := repo.Get(ctx, "prj_a", "local:ui", "k-12345678", expired); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expired Get = %v, want ErrNotFound", err)
	}

	row.RequestHash, row.Revision, row.CreatedMs, row.ExpiresMs = "h2", 2, expired, expired+86400000
	if err := repo.Put(ctx, row); err != nil {
		t.Fatalf("Put over an expired row: %v", err)
	}
	if got, _ := repo.Get(
		ctx,
		"prj_a",
		"local:ui",
		"k-12345678",
		expired,
	); got.RequestHash != "h2" ||
		got.Revision != 2 {
		t.Errorf("replaced row = %+v", got)
	}
	n, err := repo.PurgeExpired(ctx, expired+86400000)
	if err != nil || n != 1 {
		t.Errorf("PurgeExpired = %d, %v", n, err)
	}

	mustCreate(t, s, newProject("prj_b", 1))

	for _, project := range []string{"prj_a", "prj_b"} {
		row.ProjectID, row.ExpiresMs = project, expired
		if err := repo.Put(ctx, row); err != nil {
			t.Fatal(err)
		}
	}

	if n, err := repo.PurgeProjectExpired(ctx, "prj_a", expired+1); err != nil || n != 1 {
		t.Errorf("PurgeProjectExpired = %d, %v; want only the project's row", n, err)
	}

	if n, err := repo.PurgeExpired(ctx, expired+1); err != nil || n != 1 {
		t.Errorf("the other project's expired row was touched: PurgeExpired = %d, %v", n, err)
	}
}

func TestAssetsAndArtifacts(t *testing.T) {
	s := testStore(t)
	const hash = "sha256:" + "ab00000000000000000000000000000000000000000000000000000000000000"
	row := service.AssetRow{Hash: hash, SizeBytes: 10, OriginalName: "clip.mp4", CreatedMs: 1, UpdatedMs: 1}

	got, created, err := s.Assets().Upsert(ctx, row)
	if err != nil || !created || got.IngestState != service.IngestPending || got.Kind != "" ||
		got.Facts.DurationMs != nil {
		t.Fatalf("Upsert = %+v, %v, %v", got, created, err)
	}
	row.OriginalName = "other.mp4"
	got, created, err = s.Assets().Upsert(ctx, row)
	if err != nil || created || got.OriginalName != "clip.mp4" {
		t.Errorf("second Upsert = %+v, %v, %v; want the existing row", got, created, err)
	}

	if err := s.Assets().
		SetIngest(ctx, hash, service.IngestFailed, 0.5, json.RawMessage(`{"code":"E_INGEST_FAILED"}`), 5); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Assets().Get(ctx, hash)
	if got.IngestState != service.IngestFailed || got.Progress != 0.5 ||
		string(got.IngestError) != `{"code":"E_INGEST_FAILED"}` {
		t.Errorf("after SetIngest: %+v", got)
	}
	facts := service.AssetFacts{
		Kind:              "video",
		NormalizedVersion: new(1),
		DurationMs:        new(int64(5000)),
		Width:             new(1920),
		Height:            new(1080),
		HasAudio:          true,
		FrameRateNum:      new(30000),
		FrameRateDen:      new(1001),
		FrameCount:        new(int64(150)),
		SampleCount:       new(int64(240000)),
	}
	if err := s.Assets().SetFacts(ctx, hash, facts, 9); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Assets().Get(ctx, hash)
	if got.Kind != "video" || *got.Facts.DurationMs != 5000 || !got.Facts.HasAudio || *got.Facts.FrameRateDen != 1001 ||
		*got.Facts.NormalizedVersion != 1 ||
		got.UpdatedMs != 9 {
		t.Errorf("after SetFacts: %+v", got.Facts)
	}
	if err := s.Assets().
		SetIngest(ctx, "sha256:none", service.IngestReady, 1, nil, 1); !errors.Is(
		err,
		service.ErrNotFound,
	) {
		t.Errorf("SetIngest missing = %v", err)
	}

	art := service.ArtifactRow{
		AssetHash: hash, Kind: "transcript", Profile: 1, Variant: "abc123def456", Ext: "json",
		RelPath: "artifacts/x/transcript-1.json", SizeBytes: 3, SHA256: "s1", CreatedMs: 1,
	}
	if err := s.Assets().PutArtifact(ctx, art); err != nil {
		t.Fatal(err)
	}
	art.SizeBytes, art.Meta = 4, json.RawMessage(`{"lang":"en"}`)
	if err := s.Assets().PutArtifact(ctx, art); err != nil {
		t.Fatal(err)
	}
	arts, _ := s.Assets().Artifacts(ctx, hash)
	if len(arts) != 1 || arts[0].SizeBytes != 4 || string(arts[0].Meta) != `{"lang":"en"}` ||
		arts[0].Variant != "abc123def456" {
		t.Errorf("Artifacts = %+v", arts)
	}
}

func TestProjectAssetsTrashAndSweep(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1), newProject("prj_b", 1))
	const h1, h2 = "sha256:01", "sha256:02"
	for _, h := range []string{h1, h2} {
		if _, _, err := s.Assets().
			Upsert(ctx, service.AssetRow{Hash: h, SizeBytes: 1, OriginalName: "n", CreatedMs: 1, UpdatedMs: 1}); err != nil {
			t.Fatal(err)
		}
	}
	pa := service.ProjectAssetRow{
		ProjectID: "prj_a", AssetID: "ast_1", AssetHash: h1, State: service.ProjectAssetPending,
		Name: "clip", ImportedBy: "local:ui", ImportedMs: 10,
	}
	if err := s.Assets().PutProjectAsset(ctx, pa); err != nil {
		t.Fatal(err)
	}
	dup := pa
	dup.AssetID = "ast_2"
	if err := s.Assets().PutProjectAsset(ctx, dup); !errors.Is(err, service.ErrExists) {
		t.Errorf("same hash under a second asset id = %v, want ErrExists", err)
	}
	pa.State = service.ProjectAssetActive
	if err := s.Assets().PutProjectAsset(ctx, pa); err != nil { // replace
		t.Fatal(err)
	}
	if err := s.Assets().PutProjectAsset(ctx, service.ProjectAssetRow{
		ProjectID: "prj_b", AssetID: "ast_9", AssetHash: h1,
		State: service.ProjectAssetPending, Name: "clip", ImportedBy: "local:ui", ImportedMs: 11,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Assets().FindProjectAssetByHash(ctx, "prj_a", h1)
	if err != nil || got.AssetID != "ast_1" || got.State != service.ProjectAssetActive {
		t.Errorf("FindProjectAssetByHash = %+v, %v", got, err)
	}
	if _, err := s.Assets().GetProjectAsset(ctx, "prj_a", "ast_none"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("GetProjectAsset missing = %v", err)
	}
	refs, _ := s.Assets().ProjectsReferencing(ctx, h1)
	pending, _ := s.Assets().PendingForHash(ctx, h1)
	if strings.Join(refs, ",") != "prj_a,prj_b" || len(pending) != 1 || pending[0].ProjectID != "prj_b" {
		t.Errorf("referencing %v, pending %v", refs, pending)
	}

	snap := json.RawMessage(`{"id":"ast_1"}`)
	if err := s.Assets().
		SetProjectAssetState(ctx, "prj_a", "ast_1", service.ProjectAssetTrashed, "local:ui", 50, snap); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Assets().GetProjectAsset(ctx, "prj_a", "ast_1")
	if got.State != service.ProjectAssetTrashed || *got.TrashedBy != "local:ui" || *got.TrashedMs != 50 ||
		string(got.Snapshot) != `{"id":"ast_1"}` {
		t.Errorf("trashed = %+v", got)
	}
	if err := s.Assets().
		SetProjectAssetState(ctx, "prj_a", "ast_1", service.ProjectAssetActive, "local:ui", 60, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Assets().GetProjectAsset(ctx, "prj_a", "ast_1")
	if got.State != service.ProjectAssetActive || got.TrashedBy != nil || got.TrashedMs != nil ||
		string(got.Snapshot) != `{"id":"ast_1"}` {
		t.Errorf("restored = %+v (the snapshot must survive a nil snapshot)", got)
	}
	if err := s.Assets().
		SetProjectAssetState(ctx, "prj_a", "ast_none", service.ProjectAssetActive, "x", 1, nil); !errors.Is(
		err,
		service.ErrNotFound,
	) {
		t.Errorf("SetProjectAssetState missing = %v", err)
	}

	all, _ := s.Assets().ListProjectAssets(ctx, "prj_a")
	none, _ := s.Assets().ListProjectAssets(ctx, "prj_a", service.ProjectAssetTrashed)
	if len(all) != 1 || len(none) != 0 {
		t.Errorf("ListProjectAssets = %d, trashed filter = %d", len(all), len(none))
	}

	// TrashEmpty pieces: h2 is unreferenced, h1 is referenced and RESTRICTed.
	un, _ := s.Assets().Unreferenced(ctx)
	if len(un) != 1 || un[0].Hash != h2 {
		t.Errorf("Unreferenced = %+v", un)
	}
	if err := s.Assets().Delete(ctx, h1); err == nil {
		t.Error("deleting a referenced asset must fail (ON DELETE RESTRICT)")
	}
	if err := s.Assets().Delete(ctx, h2); err != nil {
		t.Fatal(err)
	}
	if err := s.Assets().
		SetProjectAssetState(ctx, "prj_a", "ast_1", service.ProjectAssetTrashed, "x", 1, nil); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Assets().DeleteTrashedProjectAssets(ctx, "prj_a"); err != nil || n != 1 {
		t.Errorf("DeleteTrashedProjectAssets = %d, %v", n, err)
	}
}

func TestProjectDeleteCascades(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	_ = s.Operations().Append(ctx, op("prj_a", 1, service.OpEdit, "local:ui"))
	_ = s.Backups().
		Put(ctx, service.BackupRow{OwnerKind: "project", OwnerID: "prj_a", Revision: 1, SchemaVersion: 1, Document: []byte("{}"), Reason: "r", CreatedMs: 1})
	_ = s.Audit().
		Append(ctx, service.AuditRow{AtMs: 1, Actor: "local:ui", Action: "project.create", ProjectID: new("prj_a")})

	if err := s.Projects().Delete(ctx, "prj_a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Projects().Delete(ctx, "prj_a"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("second Delete = %v", err)
	}
	for table, want := range map[string]int{"operations": 0, "document_backups": 0, "audit": 1} {
		if got := scalar[int](t, s, "SELECT count(*) FROM "+table); got != want {
			t.Errorf("%s rows after Delete = %d, want %d", table, got, want)
		}
	}
}

func newJob(id, kind, actor string, created int64) service.JobRow {
	j := service.JobRow{
		ID:        id,
		Kind:      kind,
		Actor:     actor,
		Input:     json.RawMessage(`{}`),
		CreatedMs: created,
		UpdatedMs: created,
	}
	if kind == "ingest" {
		j.AssetHash = new("sha256:01")
	} else {
		j.ProjectID = new("prj_a")
	}
	return j
}

func jobsFixture(t *testing.T) *Store {
	t.Helper()
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	if _, _, err := s.Assets().
		Upsert(ctx, service.AssetRow{Hash: "sha256:01", SizeBytes: 1, OriginalName: "n", CreatedMs: 1, UpdatedMs: 1}); err != nil {
		t.Fatal(err)
	}
	return s
}

// SDD-005 section 18 check 29: 20 concurrent claims, exactly one wins.
func TestJobClaimExactlyOnce(t *testing.T) {
	s := jobsFixture(t)
	if err := s.Jobs().CreateLimited(ctx, newJob("job_1", "export", "local:ui", 1), nil); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			ok, err := s.Jobs().Claim(ctx, "job_1", 10)
			if err != nil {
				t.Error(err)
			}
			if ok {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Errorf("%d claims won, want 1", wins.Load())
	}
	j, _ := s.Jobs().Get(ctx, "job_1")
	if j.State != "running" || j.Attempts != 1 || j.StartedMs == nil || *j.StartedMs != 10 {
		t.Errorf("claimed job = %+v", j)
	}
}

func TestJobLifecycle(t *testing.T) {
	s := jobsFixture(t)
	jobs := s.Jobs()
	if err := jobs.CreateLimited(ctx, newJob("job_1", "export", "local:ui", 1), nil); err != nil {
		t.Fatal(err)
	}
	if ok, _ := jobs.Progress(ctx, "job_1", 0.5, "x", 2); ok {
		t.Error("Progress of a queued job must not update")
	}
	jobs.Claim(ctx, "job_1", 3)
	if ok, _ := jobs.Progress(ctx, "job_1", 0.5, "encoding", 4); !ok {
		t.Error("Progress of a running job must update")
	}
	if ok, _ := jobs.Requeue(ctx, "job_1", "interrupted by shutdown", 5); !ok {
		t.Fatal("Requeue")
	}
	j, _ := jobs.Get(ctx, "job_1")
	if j.State != "queued" || j.Message != "interrupted by shutdown" || j.Attempts != 1 || j.StartedMs != nil {
		t.Errorf("requeued = %+v", j)
	}
	jobs.Claim(ctx, "job_1", 6)
	if running, _ := jobs.Running(ctx); len(running) != 1 || running[0].Attempts != 2 {
		t.Errorf("Running = %+v", running)
	}
	ok, err := jobs.Finish(ctx, "job_1", "succeeded", json.RawMessage(`{"export":{}}`), nil, 7)
	if !ok || err != nil {
		t.Fatalf("Finish = %v, %v", ok, err)
	}
	if ok, _ := jobs.Finish(ctx, "job_1", "failed", nil, json.RawMessage(`{}`), 8); ok {
		t.Error("a finished job must not finish again")
	}
	j, _ = jobs.Get(ctx, "job_1")
	if j.State != "succeeded" || j.Progress != 1 || string(j.Result) != `{"export":{}}` || j.FinishedMs == nil ||
		j.Error != nil {
		t.Errorf("finished = %+v", j)
	}
	if ok, _ := jobs.Claim(ctx, "job_1", 9); ok {
		t.Error("a redelivered message for a finished job must do nothing")
	}
	if _, err := jobs.Get(ctx, "job_none"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("Get missing = %v", err)
	}

	list, _ := jobs.List(ctx, "prj_a", []string{"succeeded"}, 10)
	empty, _ := jobs.List(ctx, "prj_a", []string{"queued"}, 10)
	if len(list) != 1 || len(empty) != 0 {
		t.Errorf("List = %d, filtered = %d", len(list), len(empty))
	}

	if err := jobs.CreateLimited(ctx, newJob("job_2", "export", "local:ui", 20), nil); err != nil {
		t.Fatal(err)
	}
	if q, _ := jobs.QueuedBefore(ctx, 21); len(q) != 1 || q[0].ID != "job_2" {
		t.Errorf("QueuedBefore(21) = %+v", q)
	}
	if q, _ := jobs.QueuedBefore(ctx, 20); len(q) != 0 {
		t.Errorf("QueuedBefore(20) = %+v, want none: the bound is exclusive", q)
	}
}

// Check 30, the store half: the limit is checked and the row inserted together.
func TestJobLimits(t *testing.T) {
	s := jobsFixture(t)
	lim := &service.JobLimit{Kind: "export", Max: 2}
	for i := 1; i <= 2; i++ {
		if err := s.Write(ctx, func(ctx context.Context, tx service.Tx) error {
			return tx.Jobs().CreateLimited(ctx, newJob(fmt.Sprintf("job_%d", i), "export", "local:ui", int64(i)), lim)
		}); err != nil {
			t.Fatal(err)
		}
	}
	err := s.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		return tx.Jobs().CreateLimited(ctx, newJob("job_3", "export", "local:ui", 3), lim)
	})
	var full *service.QueueFullError
	if !errors.As(err, &full) || strings.Join(full.Running, ",") != "job_1,job_2" {
		t.Fatalf("err = %v, want a full queue naming job_1 and job_2", err)
	}
	if _, err := s.Jobs().Get(ctx, "job_3"); !errors.Is(err, service.ErrNotFound) {
		t.Error("no row may be created over the limit")
	}

	// 20 concurrent creates against a limit of 2 (one already exists below).
	s2 := jobsFixture(t)
	var created atomic.Int32
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			err := s2.Write(ctx, func(ctx context.Context, tx service.Tx) error {
				return tx.Jobs().CreateLimited(ctx, newJob(fmt.Sprintf("c_%d", i), "export", "local:ui", int64(i)), lim)
			})
			if err == nil {
				created.Add(1)
			} else {
				if _, ok := errors.AsType[*service.QueueFullError](err); !ok {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if created.Load() != 2 {
		t.Errorf("%d concurrent creates succeeded, want exactly 2", created.Load())
	}

	// A cap per actor.
	s3 := jobsFixture(t)
	per := &service.JobLimit{Kind: "export", Max: 10, Actor: "local:ui", MaxPerActor: 1}
	if err := s3.Jobs().CreateLimited(ctx, newJob("a1", "export", "local:ui", 1), per); err != nil {
		t.Fatal(err)
	}
	if err := s3.Jobs().CreateLimited(ctx, newJob("a2", "export", "local:ui", 2), per); !errors.As(err, &full) {
		t.Errorf("second job of the actor = %v, want QueueFullError", err)
	}
	other := &service.JobLimit{Kind: "export", Max: 10, Actor: "local:claude-code", MaxPerActor: 1}
	if err := s3.Jobs().CreateLimited(ctx, newJob("a3", "export", "local:claude-code", 3), other); err != nil {
		t.Errorf("another actor: %v", err)
	}
}

func TestJobActiveIngestIsUnique(t *testing.T) {
	s := jobsFixture(t)
	if err := s.Jobs().CreateLimited(ctx, newJob("job_1", "ingest", "local:ui", 1), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Jobs().
		CreateLimited(ctx, newJob("job_2", "ingest", "local:ui", 2), nil); !errors.Is(
		err,
		service.ErrExists,
	) {
		t.Errorf("second active ingest of one hash = %v, want ErrExists", err)
	}
	j, err := s.Jobs().ActiveIngestForHash(ctx, "sha256:01")
	if err != nil || j.ID != "job_1" || j.ProjectID != nil {
		t.Errorf("ActiveIngestForHash = %+v, %v", j, err)
	}
	s.Jobs().Claim(ctx, "job_1", 3)
	s.Jobs().Finish(ctx, "job_1", "failed", nil, json.RawMessage(`{"code":"E_INGEST_FAILED"}`), 4)
	if _, err := s.Jobs().ActiveIngestForHash(ctx, "sha256:01"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after failing, no active ingest: %v", err)
	}
	if err := s.Jobs().CreateLimited(ctx, newJob("job_3", "ingest", "local:ui", 5), nil); err != nil {
		t.Errorf("a new ingest after a failed one: %v", err)
	}
}

func TestJobRequestCancel(t *testing.T) {
	s := jobsFixture(t)
	jobs := s.Jobs()
	_ = jobs.CreateLimited(ctx, newJob("q", "export", "local:ui", 1), nil)
	_ = jobs.CreateLimited(ctx, newJob("r", "frame", "local:ui", 2), nil)
	jobs.Claim(ctx, "r", 3)

	q, err := jobs.RequestCancel(ctx, "q", 10)
	if err != nil || q.State != "cancelled" || !q.CancelRequested ||
		!strings.Contains(string(q.Error), "E_CANCELLED") ||
		q.FinishedMs == nil {
		t.Errorf("queued cancel = %+v, %v", q, err)
	}
	if ok, _ := jobs.Claim(ctx, "q", 11); ok {
		t.Error("a cancelled job must not be claimed")
	}
	r, _ := jobs.RequestCancel(ctx, "r", 12)
	if r.State != "running" || !r.CancelRequested {
		t.Errorf("running cancel = %+v", r)
	}
	again, _ := jobs.RequestCancel(ctx, "q", 13)
	if again.State != "cancelled" || again.UpdatedMs != 10 {
		t.Errorf("cancelling twice must be idempotent: %+v", again)
	}
	jobs.Finish(ctx, "r", "succeeded", nil, nil, 14)
	done, _ := jobs.RequestCancel(ctx, "r", 15)
	if done.State != "succeeded" {
		t.Errorf("cancelling after completion = %+v, want the final row", done)
	}
	if _, err := jobs.RequestCancel(ctx, "none", 1); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("cancel missing = %v", err)
	}
}

func TestTemplatesBackupsAudit(t *testing.T) {
	s := testStore(t)
	for i := range 3 {
		err := s.Templates().Create(ctx, service.TemplateRow{
			ID: fmt.Sprintf("tpl_%d", i), Name: "t", SchemaVersion: 1, Document: []byte(`{"a":1}`), DocSHA256: "s",
			Slots: json.RawMessage(`[]`), Summary: service.DocSummary{Width: 1, Height: 2, FPSNum: 30, FPSDen: 1},
			CreatedMs: int64(i), CreatedBy: "local:ui",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Templates().
		Create(ctx, service.TemplateRow{ID: "tpl_0", Name: "t", Document: []byte("{}"), Slots: json.RawMessage(`[]`)}); !errors.Is(
		err,
		service.ErrExists,
	) {
		t.Errorf("duplicate template = %v", err)
	}
	page, next, _ := s.Templates().List(ctx, 2, "", "")
	rest, next2, _ := s.Templates().List(ctx, 2, next, "")
	if len(page) != 2 || page[0].ID != "tpl_2" || len(page[0].Document) != 0 || len(rest) != 1 ||
		rest[0].ID != "tpl_0" ||
		next2 != "" {
		t.Errorf("template paging: %v %v %q", page, rest, next2)
	}
	if err := s.Templates().
		UpdateDocument(ctx, "tpl_1", 2, []byte(`{"b":2}`), "s2", json.RawMessage(`[{"name":"x"}]`), service.DocSummary{Width: 9}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Templates().Get(ctx, "tpl_1")
	if err != nil || got.SchemaVersion != 2 || string(got.Document) != `{"b":2}` ||
		string(got.Slots) != `[{"name":"x"}]` ||
		got.Summary.Width != 9 {
		t.Errorf("Get = %+v, %v", got, err)
	}
	if err := s.Templates().
		UpdateDocument(ctx, "tpl_none", 1, nil, "", json.RawMessage(`[]`), service.DocSummary{}); !errors.Is(
		err,
		service.ErrNotFound,
	) {
		t.Errorf("UpdateDocument missing = %v", err)
	}

	for _, rev := range []int64{3, 5} {
		if err := s.Backups().Put(ctx, service.BackupRow{
			OwnerKind: "project", OwnerID: "prj_a", Revision: rev, SchemaVersion: 1,
			Document: []byte(fmt.Sprintf(`{"rev":%d}`, rev)), Reason: "migrate v1 to v2", CreatedMs: rev,
		}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.Backups().List(ctx, "project", "prj_a")
	if len(list) != 2 || len(list[0].Document) != 0 {
		t.Fatalf("Backups.List = %+v", list)
	}
	b, err := s.Backups().Get(ctx, list[1].ID)
	if err != nil || string(b.Document) != `{"rev":5}` {
		t.Errorf("Backups.Get = %+v, %v", b, err)
	}
	if _, err := s.Backups().Get(ctx, 999); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("Backups.Get missing = %v", err)
	}

	for i := range 5 {
		a := service.AuditRow{
			AtMs:    int64(i),
			Actor:   "local:ui",
			Action:  "media.import",
			Details: json.RawMessage(`{"n":1}`),
		}
		if i%2 == 0 {
			a.ProjectID = new("prj_a")
		}
		if err := s.Audit().Append(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.Audit().List(ctx, "", 0, 10)
	mine, _ := s.Audit().List(ctx, "prj_a", 0, 10)
	older, _ := s.Audit().List(ctx, "", all[1].ID, 10)
	if len(all) != 5 || all[0].ID < all[1].ID || len(mine) != 3 || len(older) != 3 ||
		string(all[0].Details) != `{"n":1}` {
		t.Errorf("audit: all %d, mine %d, older %d", len(all), len(mine), len(older))
	}
}

// T-28 (SDD-007) and SDD-005 check 38, first half.
func TestSecurity_T28_OperationLogAppendOnly(t *testing.T) {
	// The implementation has no UPDATE operations other than the three closed_by statements.
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(src), "\n") {
			if i := strings.Index(line, "UPDATE operations"); i >= 0 {
				count++
				if !strings.Contains(line[i:], "SET closed_by = ?") {
					t.Errorf("%s: an UPDATE of operations that is not a closed_by statement: %s", f, line)
				}
			}
		}
		if strings.Contains(string(src), "DELETE FROM operations") {
			t.Errorf("%s deletes from operations", f)
		}
	}
	if count != 3 {
		t.Errorf("found %d UPDATE operations statements, want 3", count)
	}

	// The database refuses what the code does not do.
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	if err := s.Operations().Append(ctx, op("prj_a", 1, service.OpEdit, "local:ui")); err != nil {
		t.Fatal(err)
	}
	for name, q := range map[string]string{
		"rewrite the diff":  "UPDATE operations SET diff = '[1]' WHERE project_id = 'prj_a'",
		"rewrite the actor": "UPDATE operations SET actor = 'local:mallory', closed_by = 2 WHERE project_id = 'prj_a'",
		"reopen a row":      "UPDATE operations SET closed_by = NULL WHERE project_id = 'prj_a'",
	} {
		if _, err := s.db.ExecContext(ctx, q); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: err = %v, want the append-only abort", name, err)
		}
	}
	if err := s.Operations().Close(ctx, "prj_a", 1, 2); err != nil {
		t.Errorf("closing an open row must work: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE operations SET closed_by = 3 WHERE project_id = 'prj_a'"); err == nil {
		t.Error("a closed row must not change again")
	}
}

func queryPlan(t *testing.T, s *Store, query string) []string {
	t.Helper()
	rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, "a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan
}

// Check 10: the queries use the indexes the SDD names.
func TestQueryPlans(t *testing.T) {
	s := testStore(t)
	// SQLite names the index behind PRIMARY KEY (project_id, revision) of a
	// table that has a rowid; it is the primary key search of SDD-005 section 11.2.
	const pkIndex = "sqlite_autoindex_operations_1 (project_id=? AND revision>?)"
	cases := []struct{ name, query, want string }{
		{
			"project listing",
			`SELECT id FROM timelines WHERE state = 'active' AND (updated_ms, id) < (?, ?) ORDER BY updated_ms DESC, id DESC LIMIT 50`,
			"timelines_state_updated_ms_index",
		},
		{
			"Since",
			`SELECT revision FROM operations WHERE project_id = ? AND revision > ? ORDER BY revision ASC LIMIT 1000`,
			pkIndex,
		},
		{
			"Since excluding an actor",
			`SELECT revision FROM operations WHERE project_id = ? AND revision > ? AND actor <> ? ORDER BY revision ASC`,
			pkIndex,
		},
		{
			"undo candidate",
			`SELECT revision FROM operations WHERE project_id = ? AND actor = ? AND closed_by IS NULL AND kind IN ('edit','redo') ORDER BY revision DESC LIMIT 1`,
			"operations_undo_idx",
		},
		{
			"redo candidate",
			`SELECT revision FROM operations WHERE project_id = ? AND actor = ? AND closed_by IS NULL AND kind = 'undo' ORDER BY revision DESC LIMIT 1`,
			"operations_redo_idx",
		},
	}
	for _, c := range cases {
		plan := queryPlan(t, s, c.query)
		if !strings.Contains(strings.Join(plan, "\n"), c.want) {
			t.Errorf("%s: plan %q lacks %q", c.name, plan, c.want)
		}
	}

	// The SQL the repositories really run is the SQL tested above: the
	// partial-index queries must stay literal (SDD-005 section 12.1).
	src, err := os.ReadFile("operations.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, lit := range []string{"closed_by IS NULL AND kind IN ('edit','redo')", "closed_by IS NULL AND kind = 'undo'"} {
		if !strings.Contains(string(src), lit) {
			t.Errorf("operations.go no longer holds the literal %q", lit)
		}
	}
}

func TestIdempotencyCountLiveCountsOneActorAndProjectAndEvictsNothing(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	mustCreate(t, s, newProject("prj_b", 1))

	repo := s.Idempotency()

	put := func(project, actor, key string, at int64) {
		t.Helper()

		row := service.IdempotencyRow{
			ProjectID: project, Actor: actor, Key: key, RequestHash: "h", Revision: 1,
			Result: json.RawMessage(`{}`), CreatedMs: at, ExpiresMs: at + 1000,
		}
		if err := repo.Put(ctx, row); err != nil {
			t.Fatal(err)
		}
	}

	for i := range 5 {
		put("prj_a", "key:one", "k"+strconv.Itoa(i), int64(100+i))
	}

	put("prj_a", "key:two", "k0", 100)
	put("prj_b", "key:one", "k0", 100)

	if n, first, err := repo.CountLive(ctx, "prj_a", "key:one", 200); err != nil || n != 5 || first != 1100 {
		t.Errorf(
			"CountLive = %d, first %d, %v; want 5 keys of one actor and project, the first ending at 1100",
			n,
			first,
			err,
		)
	}

	if n, first, err := repo.CountLive(ctx, "prj_a", "key:one", 1102); err != nil || n != 2 || first != 1103 {
		t.Errorf("CountLive after three expired = %d, first %d, %v; want 2 left", n, first, err)
	}

	if n, first, err := repo.CountLive(ctx, "prj_a", "key:none", 200); err != nil || n != 0 || first != 0 {
		t.Errorf("CountLive of nobody = %d, first %d, %v", n, first, err)
	}

	for i := range 5 {
		if _, err := repo.Get(ctx, "prj_a", "key:one", "k"+strconv.Itoa(i), 200); err != nil {
			t.Errorf("k%d is gone: %v", i, err)
		}
	}
}

func TestFontOwnersAndBrandKit(t *testing.T) {
	s := testStore(t)
	hash := "sha256:" + strings.Repeat("ab", 32)

	if owns, err := s.Fonts().Owns(ctx, "usr_1", hash); err != nil || owns {
		t.Fatalf("Owns before Record = %v, %v", owns, err)
	}

	for range 2 {
		if err := s.Fonts().Record(ctx, "usr_1", hash, 10); err != nil {
			t.Fatal(err)
		}
	}

	if owns, err := s.Fonts().Owns(ctx, "usr_1", hash); err != nil || !owns {
		t.Fatalf("Owns after Record = %v, %v", owns, err)
	}

	if owns, _ := s.Fonts().Owns(ctx, "usr_2", hash); owns {
		t.Error("another user owns the font")
	}

	if owned, err := s.Fonts().Owned(ctx, "usr_1"); err != nil || len(owned) != 1 || owned[0] != hash {
		t.Errorf("Owned(usr_1) = %v, %v", owned, err)
	}

	if owned, _ := s.Fonts().Owned(ctx, "usr_2"); len(owned) != 0 {
		t.Errorf("Owned(usr_2) = %v", owned)
	}

	if _, err := s.BrandKit().Get(ctx, ""); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("Get of no kit = %v, want ErrNotFound", err)
	}

	row := service.BrandKitRow{
		OwnerUserID: "",
		Document:    []byte(`{"colors":{"a":"#FFFFFF"}}`),
		UpdatedMs:   5,
		UpdatedBy:   "local:ui",
	}
	if err := s.BrandKit().Put(ctx, row); err != nil {
		t.Fatal(err)
	}

	row.Document, row.UpdatedMs = []byte(`{"colors":{"a":"#000000"}}`), 6
	if err := s.BrandKit().Put(ctx, row); err != nil {
		t.Fatal(err)
	}

	got, err := s.BrandKit().Get(ctx, "")
	if err != nil || string(got.Document) != string(row.Document) || got.UpdatedMs != 6 {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}
