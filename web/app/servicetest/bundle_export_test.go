package servicetest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
)

type bundleExporter interface {
	ProjectBundleExport(ctx context.Context, req service.ProjectBundleExportRequest) (*service.Job, error)
}

// exportBundle starts the bundle export of the project and runs its job to the end.
func (m *mediaEnv) exportBundle(project string, includeLog *bool) *service.Job {
	m.t.Helper()

	job, err := m.svc.(bundleExporter).ProjectBundleExport(
		m.as(ui), service.ProjectBundleExportRequest{ProjectID: project, IncludeLog: includeLog},
	)
	if err != nil {
		m.t.Fatal(err)
	}

	if job.State == service.JobQueued {
		if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
			m.t.Fatal(err)
		}
	}

	return m.job(job.JobID)
}

// bundleProject is a project with two imported clips, a text item and a few revisions of history.
func (m *mediaEnv) bundleProject() (project string, hashes []string) {
	m.t.Helper()

	project, track := m.withTrack()
	if _, err := m.addText(ui, project, track, 1, "0s", "hello"); err != nil {
		m.t.Fatal(err)
	}

	for _, name := range []string{"a.mp4", "b.mp4"} {
		m.importOne(project, m.write(name, clipBytes+name))
	}

	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		m.t.Fatal(err)
	}

	for id := range doc.Assets {
		hashes = append(hashes, doc.Assets[id].Hash)
	}

	return project, hashes
}

func readZip(t *testing.T, path string) (*zip.Reader, map[string][]byte) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}

		b, err := io.ReadAll(rc)
		_ = rc.Close()

		if err != nil {
			t.Fatal(err)
		}

		files[f.Name] = b
	}

	return zr, files
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)

	return hex.EncodeToString(s[:])
}

func TestBundleExport(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, hashes := m.bundleProject()
	head := m.head(project)

	job := m.exportBundle(project, nil)
	if job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
		t.Fatalf("job = %+v", job)
	}

	res := job.Result.Export

	want := filepath.Join(
		m.ws,
		"exports",
		project,
		"bundle-r"+strconv.FormatInt(head.Revision, 10)+"-"+job.JobID+".vvbundle",
	)
	if got, _ := filepath.Abs(want); res.Path != got || res.Revision != head.Revision {
		t.Fatalf("result = %+v, want path %s", res, want)
	}

	zr, files := readZip(t, res.Path)

	if zr.File[0].Name != "manifest.json" {
		t.Errorf("the first entry is %s, want manifest.json", zr.File[0].Name)
	}

	for _, f := range zr.File {
		if strings.ContainsAny(f.Name, `/\`) {
			t.Errorf("entry %q is not flat", f.Name)
		}
	}

	var man service.BundleManifest
	if err := json.Unmarshal(files["manifest.json"], &man); err != nil {
		t.Fatal(err)
	}

	if man.Format != "stuarty-bundle" || man.BundleVersion != 1 || man.Project.ID != project ||
		man.Project.Revision != head.Revision || man.Project.SchemaVersion != timeline.CurrentSchemaVersion {
		t.Errorf("manifest = %+v", man)
	}

	if man.Document.SHA256 != hexSum(files["document.json"]) ||
		man.Document.Bytes != int64(len(files["document.json"])) {
		t.Errorf("the document entry does not match the manifest: %+v", man.Document)
	}

	if !bytes.Equal(files["document.json"], m.document(project)) {
		t.Error("document.json is not the stored canonical document")
	}

	if man.Operations == nil || man.Operations.SHA256 != hexSum(files["operations.ndjson"]) ||
		man.Operations.Count != head.Revision {
		t.Errorf("operations = %+v", man.Operations)
	}

	lines := strings.Split(strings.TrimSuffix(string(files["operations.ndjson"]), "\n"), "\n")
	if int64(len(lines)) != head.Revision {
		t.Fatalf("%d log lines, want %d", len(lines), head.Revision)
	}

	for i, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil || row["revision"] != float64(i+1) ||
			row["project_id"] != nil {
			t.Errorf("line %d = %s (%v)", i+1, line, err)
		}
	}

	if len(man.Media) != len(hashes) {
		t.Fatalf("%d media in the manifest, want %d", len(man.Media), len(hashes))
	}

	for _, h := range hashes {
		body, ok := files["media."+strings.TrimPrefix(h, "sha256:")]
		if !ok || "sha256:"+hexSum(body) != h {
			t.Errorf("media entry of %s missing or altered", h)
		}
	}

	if len(zr.File) != 3+len(hashes) {
		t.Errorf("%d entries, want manifest, document, log and %d media", len(zr.File), len(hashes))
	}

	// The download route serves the bundle, not an mp4.
	f, err := m.svc.ExportFile(m.as(ui), service.ExportFileRequest{JobID: job.JobID})
	if err != nil || f.ContentType != "application/zip" || !strings.HasSuffix(f.Path, ".vvbundle") {
		t.Errorf("ExportFile = %+v, %v", f, err)
	}

	// No log, no operations entry.
	no := false

	shared := m.exportBundle(project, &no)
	if shared.State != service.JobSucceeded {
		t.Fatalf("job = %+v", shared)
	}

	_, bare := readZip(t, shared.Result.Export.Path)
	if _, has := bare["operations.ndjson"]; has {
		t.Error("include_log false still wrote the log")
	}

	var bareMan service.BundleManifest
	if err := json.Unmarshal(bare["manifest.json"], &bareMan); err != nil || bareMan.Operations != nil {
		t.Errorf("manifest without a log = %+v, %v", bareMan.Operations, err)
	}
}

func TestBundleExportRefusesWhatTheStoreCannotBack(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, hashes := m.bundleProject()
	store := enginestore.New(m.ws)

	path, err := store.Path(strings.TrimPrefix(hashes[0], "sha256:"))
	if err != nil {
		t.Fatal(err)
	}

	// Altered bytes: the hash recomputed while streaming differs.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}

	job := m.exportBundle(project, nil)
	if job.State != service.JobFailed || job.Error == nil || job.Error.Code != service.CodeExportFailed ||
		job.Error.Details["reason"] != "store_corrupt" || job.Error.Details["hash"] != hashes[0] {
		t.Errorf("corrupt store file: %+v", job.Error)
	}

	assertNoPartial(t, m, project)

	// A file that is gone.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	job = m.exportBundle(project, nil)
	if job.State != service.JobFailed || job.Error == nil || job.Error.Details["reason"] != "missing_media" ||
		job.Error.Details["hash"] != hashes[0] {
		t.Errorf("missing store file: %+v", job.Error)
	}

	assertNoPartial(t, m, project)
}

func assertNoPartial(t *testing.T, m *mediaEnv, project string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(m.ws, "exports", project))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") || strings.HasPrefix(e.Name(), ".operations-") ||
			strings.HasSuffix(e.Name(), ".vvbundle") {
			t.Errorf("%s was left behind", e.Name())
		}
	}
}

// Cancelling a running bundle export stops the copy and leaves no .tmp file (SDD-005 section 16.2).
func TestBundleExportCancel(t *testing.T) {
	var (
		m         *mediaEnv
		cancelled atomic.Bool
	)

	m = newMediaEnv(t, mediaCfg{onEvent: func(ev service.Event) {
		if ev.Type == "job.updated" && ev.State == service.JobRunning && ev.Progress > 0 &&
			cancelled.CompareAndSwap(false, true) {
			if err := m.runner.Cancel(context.Background(), ev.JobID); err != nil {
				t.Error(err)
			}
		}
	}})

	project, _ := m.bundleProject()

	big := filepath.Join(m.files, "big.mp4")
	if err := os.WriteFile(big, bytes.Repeat([]byte("x"), 8<<20), 0o600); err != nil {
		t.Fatal(err)
	}

	m.importOne(project, big)

	job := m.exportBundle(project, nil)
	if !cancelled.Load() || job.State != service.JobCancelled {
		t.Fatalf("job = %+v (cancelled requested: %v)", job, cancelled.Load())
	}

	assertNoPartial(t, m, project)
}

// TestBundleExportMemory exports a 3 GiB project and keeps the Go process under 500 MB: the media is
// streamed, never held. It writes about 6 GiB, so it runs only when STUARTY_LARGE_TESTS is set.
func TestBundleExportMemory(t *testing.T) {
	if os.Getenv("STUARTY_LARGE_TESTS") == "" {
		t.Skip("set STUARTY_LARGE_TESTS=1 to export 3 GiB") // verify:env-skip a 3 GiB export that runs on request
	}

	m := newMediaEnv(t, mediaCfg{})
	project, _ := m.bundleProject()

	big := filepath.Join(m.files, "huge.mp4")

	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}

	chunk := bytes.Repeat([]byte("v"), 1<<20)
	for range 3 << 10 {
		if _, err := f.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	m.importOne(project, big)

	var peak atomic.Uint64

	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)

		var ms runtime.MemStats

		for {
			runtime.ReadMemStats(&ms)

			if ms.Sys > peak.Load() {
				peak.Store(ms.Sys)
			}

			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()

	job := m.exportBundle(project, nil)

	close(stop)
	<-done

	if job.State != service.JobSucceeded {
		t.Fatalf("job = %+v", job)
	}

	t.Logf("peak memory obtained from the system: %d MB", peak.Load()>>20)

	if peak.Load() > 500<<20 {
		t.Errorf("the process held %d MB while exporting 3 GiB, want under 500 MB", peak.Load()>>20)
	}
}
