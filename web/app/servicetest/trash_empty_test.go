package servicetest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

func (m *mediaEnv) browser() context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})
}

func (m *mediaEnv) trash(project string) {
	m.t.Helper()

	if _, err := m.svc.ProjectTrash(m.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		m.t.Fatal(err)
	}
}

func (m *mediaEnv) projectCount() int {
	m.t.Helper()

	return len(m.projectIDs())
}

func fileCount(t *testing.T, dir string) int {
	t.Helper()

	n := 0

	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}

		return nil
	})

	return n
}

// SDD-005 section 18 check 20: only a browser session or an admin key empties
// the trash; the local token and a key without admin are refused.
func TestTrashEmptyNeedsAHumanSessionOrAnAdminKey(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	m.trash(project)

	local := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})
	if _, err := m.svc.TrashEmpty(local, service.TrashEmptyRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("the local token = %v, want E_FORBIDDEN", err)
	}

	plain := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:k", Scopes: []string{service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender},
	})
	if _, err := m.svc.TrashEmpty(plain, service.TrashEmptyRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("a key without admin = %v, want E_FORBIDDEN", err)
	}

	if m.projectCount() != 1 {
		t.Fatal("a refused call deleted a project")
	}

	admin := service.WithCaller(context.Background(), service.Caller{Actor: "key:a", Scopes: service.AllScopes})
	if res, err := m.svc.TrashEmpty(admin, service.TrashEmptyRequest{}); err != nil || res.Projects != 1 {
		t.Errorf("an admin key = %+v, %v", res, err)
	}
}

// A hash shared with a live project survives the emptying of another project;
// an asset nothing references any more goes with its files.
func TestTrashEmptyKeepsWhatIsStillReferenced(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	keep, gone := m.newProject("keep"), m.newProject("gone")

	shared := m.write("shared.mp4", clipBytes)
	m.importOne(keep, shared)
	m.importOne(gone, shared)
	m.importOne(gone, m.write("alone.mp4", clipBytes+"x"))

	if err := os.MkdirAll(filepath.Join(m.ws, "exports", gone), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(m.ws, "exports", gone, "job.mp4"), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	before := fileCount(t, filepath.Join(m.ws, "store"))
	m.trash(gone)

	res, err := m.svc.TrashEmpty(m.browser(), service.TrashEmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if res.Projects != 1 || res.Assets != 1 || res.BytesFreed == 0 {
		t.Errorf("result = %+v", res)
	}

	if after := fileCount(t, filepath.Join(m.ws, "store")); after != before-1 {
		t.Errorf("store files %d -> %d; only the file nothing references may go", before, after)
	}

	if _, err := os.Stat(filepath.Join(m.ws, "exports", gone)); err == nil {
		t.Error("the exports of the emptied project survived")
	}

	if got := m.projectIDs(); len(got) != 1 || got[0] != keep {
		t.Errorf("projects = %v", got)
	}

	if rows, _ := m.store.Assets().ListProjectAssets(context.Background(), keep); len(rows) != 1 {
		t.Errorf("the live project lost its asset rows: %v", rows)
	}

	audit, err := m.store.Audit().List(context.Background(), "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, a := range audit {
		found = found || a.Action == "trash.empty" && a.Actor == "local:ui"
	}

	if !found {
		t.Error("no trash.empty audit row")
	}
}

func TestTrashEmptyFilters(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	a, b, live := m.newProject("a"), m.newProject("b"), m.newProject("live")
	m.trash(a)
	m.trash(b)

	if _, err := m.svc.TrashEmpty(
		m.browser(),
		service.TrashEmptyRequest{ProjectID: live},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("a live project = %v, want E_NOT_FOUND", err)
	}

	if _, err := m.svc.TrashEmpty(
		m.browser(),
		service.TrashEmptyRequest{OlderThanDays: -1},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("negative age = %v", err)
	}

	// Nothing has been in the trash for a week yet.
	if res, err := m.svc.TrashEmpty(
		m.browser(),
		service.TrashEmptyRequest{OlderThanDays: 7},
	); err != nil ||
		res.Projects != 0 {
		t.Errorf("older than 7 days = %+v, %v", res, err)
	}

	if res, err := m.svc.TrashEmpty(
		m.browser(),
		service.TrashEmptyRequest{ProjectID: a},
	); err != nil ||
		res.Projects != 1 {
		t.Errorf("one project = %+v, %v", res, err)
	}

	if m.projectCount() != 2 {
		t.Errorf("projects = %v", m.projectIDs())
	}
}

// SDD-005 section 18 check 20: a workload of every other operation with the
// scopes of an agent key never reduces the projects or the files of the store.
func TestNoAgentOperationRemovesData(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	agent := service.WithCaller(context.Background(), service.Caller{
		Actor:  "key:agent",
		Scopes: []string{service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender},
	})

	project := m.templateProject()
	asset := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0].AssetID
	files := fileCount(t, filepath.Join(m.ws, "store"))

	mustOK := func(err error) {
		t.Helper()

		if err != nil {
			t.Fatal(err)
		}
	}

	_, err := m.svc.MediaTrash(agent, service.MediaTrashRequest{ProjectID: project, AssetID: asset})
	mustOK(err)
	_, err = m.svc.MediaRestore(agent, service.MediaTrashRequest{ProjectID: project, AssetID: asset})
	mustOK(err)
	_, err = m.svc.TemplateSave(agent, service.TemplateSaveRequest{ProjectID: project, Name: "t"})
	mustOK(err)
	_, err = m.svc.ProjectTrash(agent, service.ProjectTrashRequest{ProjectID: project})
	mustOK(err)
	_, err = m.svc.ProjectRestore(agent, service.ProjectTrashRequest{ProjectID: project})
	mustOK(err)
	_, err = m.svc.ProjectTrash(agent, service.ProjectTrashRequest{ProjectID: project})
	mustOK(err)

	if _, err := m.svc.TrashEmpty(agent, service.TrashEmptyRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("TrashEmpty by an agent key = %v", err)
	}

	if m.projectCount() != 1 || fileCount(t, filepath.Join(m.ws, "store")) != files {
		t.Error("an operation of an agent key removed a project or a file")
	}
}

// SDD-005 section 18 check 21: the sweep removes what a crashed emptying left,
// only when it is old enough, and never touches templates/.
func TestSweepRemovesOrphansOlderThan24Hours(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	m.importOne(project, m.write("a.mp4", clipBytes))

	hexOld := strings.Repeat("ab", 32)
	hexNew := strings.Repeat("cd", 32)

	old := filepath.Join(m.ws, "store", hexOld[:2], hexOld)
	fresh := filepath.Join(m.ws, "store", hexNew[:2], hexNew)

	for _, p := range []string{old, fresh} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(p, []byte("orphan"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Aged by the clock of the service, not the wall clock: the env clock is
	// fixed, so a wall-clock age shrinks below 24 hours as real days pass.
	longAgo := m.clock.now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, longAgo, longAgo); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{
		filepath.Join(m.ws, "artifacts", hexOld), filepath.Join(m.ws, "exports", "prj_gone000001"),
		filepath.Join(m.ws, "frames", "prj_gone000001"), filepath.Join(m.ws, "templates"),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(
		filepath.Join(m.ws, "templates", "tpl_x.stuarty-template.zip"),
		[]byte("z"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	sweeper, ok := service.SweeperOf(m.svc)
	if !ok {
		t.Fatal("the service has no sweeper")
	}

	res, err := sweeper.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.StoreFiles != 1 || res.ArtifactDirs != 1 || res.ExportDirs != 1 || res.FrameDirs != 1 {
		t.Errorf("sweep = %+v", res)
	}

	if _, err := os.Stat(old); err == nil {
		t.Error("the old orphan survived")
	}

	if _, err := os.Stat(fresh); err != nil {
		t.Error("an orphan younger than 24 hours was removed")
	}

	if _, err := os.Stat(filepath.Join(m.ws, "templates", "tpl_x.stuarty-template.zip")); err != nil {
		t.Error("the sweep removed a template file")
	}

	if m.projectCount() != 1 {
		t.Error("the sweep touched the database")
	}
}

// A key limited to one project empties that project and nothing else.
func TestTrashEmptyOfAProjectLimitedKeyStaysInItsProject(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	mine, other := m.newProject("mine"), m.newProject("other")
	m.importOne(other, m.write("a.mp4", clipBytes))
	m.trash(mine)
	m.trash(other)

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: mine,
	})

	if _, err := m.svc.TrashEmpty(
		limited,
		service.TrashEmptyRequest{ProjectID: other},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("another project = %v, want E_FORBIDDEN", err)
	}

	res, err := m.svc.TrashEmpty(limited, service.TrashEmptyRequest{})
	if err != nil || res.Projects != 1 || res.ProjectAssets != 0 || res.Assets != 0 {
		t.Fatalf("empty = %+v, %v", res, err)
	}

	ids := m.projectIDs()
	if len(ids) != 1 || ids[0] != other {
		t.Errorf("projects = %v; only %s may go", ids, mine)
	}
}

// SDD-005 section 13.4: a kill during a bundle export leaves exports/<project>/bundle-r*.vvbundle.tmp.
// The sweep removes that exact pattern when the file is older than the running process, and nothing
// else in exports/: a temp file of an export that runs now, a final bundle and other names stay.
func TestSweepRemovesStaleBundleTempFiles(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	dir := filepath.Join(m.ws, "exports", project)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	longAgo := time.Now().Add(-time.Hour)
	stale := filepath.Join(dir, "bundle-r3-job_x.vvbundle.tmp")
	running := filepath.Join(dir, "bundle-r4-job_y.vvbundle.tmp")
	final := filepath.Join(dir, "bundle-r3-job_x.vvbundle")
	other := filepath.Join(dir, "notes.tmp")
	prefixOnly := filepath.Join(dir, "other-r3-job_x.vvbundle.tmp")

	for _, p := range []string{stale, running, final, other, prefixOnly} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, p := range []string{stale, final, other, prefixOnly} {
		if err := os.Chtimes(p, longAgo, longAgo); err != nil {
			t.Fatal(err)
		}
	}

	// Linux stamps a new file with a coarse clock that can read a few milliseconds before the
	// service start, so the running export's time is set explicitly instead of left to the write.
	soon := time.Now().Add(time.Minute)
	if err := os.Chtimes(running, soon, soon); err != nil {
		t.Fatal(err)
	}

	sweeper, _ := service.SweeperOf(m.svc)

	res, err := sweeper.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.IncomingFiles != 1 {
		t.Errorf("sweep = %+v, want one partial file removed", res)
	}

	if _, err := os.Stat(stale); err == nil {
		t.Error("the stale temp file survived")
	}

	for _, p := range []string{running, final, other, prefixOnly} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed: %v", filepath.Base(p), err)
		}
	}
}

// A file whose ingest is still queued or running is read and written by that
// ingest: emptying the trash leaves its asset, store file and job alone.
func TestTrashEmptyKeepsAnAssetWhoseIngestIsActive(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	gone := m.newProject("gone")
	m.importOne(gone, m.write("clip.mp4", clipBytes))

	rows, err := m.store.Assets().ListProjectAssets(context.Background(), gone)
	if err != nil || len(rows) != 1 {
		t.Fatalf("project assets = %v, %v", rows, err)
	}

	hash := rows[0].AssetHash
	row := service.JobRow{
		ID: "job_ingest_active", Kind: service.JobIngest, AssetHash: &hash, Actor: ui,
		State: service.JobQueued, CreatedMs: 1, UpdatedMs: 1,
	}

	if err := m.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		return tx.Jobs().CreateLimited(ctx, row, nil)
	}); err != nil {
		t.Fatal(err)
	}

	before := fileCount(t, filepath.Join(m.ws, "store"))
	m.trash(gone)

	res, err := m.svc.TrashEmpty(m.browser(), service.TrashEmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if res.Assets != 0 {
		t.Errorf("result = %+v; the asset of an active ingest was counted", res)
	}

	if _, err := m.store.Assets().Get(context.Background(), hash); err != nil {
		t.Errorf("the asset row is gone: %v", err)
	}

	if after := fileCount(t, filepath.Join(m.ws, "store")); after != before {
		t.Errorf("store files %d -> %d", before, after)
	}

	if _, err := m.store.Jobs().Get(context.Background(), row.ID); err != nil {
		t.Errorf("the ingest job row is gone: %v", err)
	}
}

// A job still running after the wait keeps its project: deleting it would
// cascade the job row away without a final state.
func TestTrashEmptyKeepsAProjectWithARunningJob(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{jobWait: 50 * time.Millisecond})
	project := m.newProject("busy")
	ctx := context.Background()
	row := service.JobRow{
		ID: "job_running_export", Kind: service.JobExport, ProjectID: &project, Actor: ui,
		State: service.JobQueued, CreatedMs: 1, UpdatedMs: 1,
	}

	if err := m.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		return tx.Jobs().CreateLimited(ctx, row, nil)
	}); err != nil {
		t.Fatal(err)
	}

	if ok, err := m.store.Jobs().Claim(ctx, row.ID, 2); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}

	m.trash(project)

	res, err := m.svc.TrashEmpty(m.browser(), service.TrashEmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if res.Projects != 0 {
		t.Errorf("result = %+v; the project of a running job was deleted", res)
	}

	if m.projectCount() != 1 {
		t.Error("the project is gone")
	}

	if _, err := m.store.Jobs().Get(context.Background(), "job_running_export"); err != nil {
		t.Errorf("the job row is gone: %v", err)
	}
}
