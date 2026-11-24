//go:build crashtest

package servicetest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/cache"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/frame"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/exporter"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// videoSeconds is how long the text item of the export cases lasts: long enough that the export is seen to
// grow its file before it ends.
const videoSeconds = 60

// openExportService opens a workspace as the process of the product does, with the real exporter on the pinned
// FFmpeg and no queue: a job runs when the process asks for it.
func openExportService(tb testing.TB, ws string) (*store.Store, service.Service, service.JobRunner) {
	tb.Helper()

	st, err := store.Open(context.Background(), workspace.Dir(ws), store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = st.Close() })

	guard, err := pathguard.New(pathguard.Config{Workspace: ws})
	if err != nil {
		tb.Fatal(err)
	}

	runner, err := ffmpeg.NewRunner(pinnedFFmpeg(tb), ws)
	if err != nil {
		tb.Fatal(err)
	}

	frames, err := cache.Open(ws, 0)
	if err != nil {
		tb.Fatal(err)
	}

	svc, err := service.New(service.Options{
		Store: st, Registry: command.Default(), Limits: service.Limits{MinFreeBytes: -1}, Workspace: ws,
		Media: service.Media{
			Content: enginestore.New(ws), Guard: guard, Ingest: ingest.Run, Runner: runner,
			FreeBytes: func(string) (uint64, error) { return 1 << 40, nil },
			Frames:    &frame.Frames{Runner: runner, ArtifactsDir: filepath.Join(ws, "artifacts"), Cache: frames},
		},
		Exporter: exporter.New(exporter.Config{Runner: runner, Workspace: ws}),
	})
	if err != nil {
		tb.Fatal(err)
	}

	jr, ok := service.RunnerOf(svc)
	if !ok {
		tb.Fatal("the service has no job runner")
	}

	return st, svc, jr
}

// killRealExportChild is the child of the export cases: it runs the job (video) or exports the image over and
// over (image), printing a line before and after each step so the parent sees where it is.
func killRealExportChild(t *testing.T, kind string) {
	ws := os.Getenv(killRealDirEnv)
	arg := os.Getenv(killRealArgsEnv)

	switch kind {
	case "video":
		_, _, jr := openExportService(t, ws)

		fmt.Println("running", arg)

		if err := jr.RunJob(context.Background(), arg); err != nil {
			t.Fatal(err)
		}

		fmt.Println("finished", arg)
	case "image":
		st, err := store.Open(context.Background(), workspace.Dir(ws), store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
		if err != nil {
			t.Fatal(err)
		}

		svc, err := service.New(service.Options{
			Store: st, Registry: command.Default(), Limits: service.Limits{MinFreeBytes: -1}, Workspace: ws,
		})
		if err != nil {
			t.Fatal(err)
		}

		ctx := service.WithCaller(context.Background(), service.Caller{Actor: agent, Scopes: service.AllScopes})

		for i := range imageExports {
			fmt.Println("begin", i)

			// Each export asks for another width, so each file has a name of its own.
			if _, err := svc.ImageExport(ctx, service.ImageExportRequest{
				ImageID: arg, Format: "png", Width: imageBaseWidth + 16*i,
			}); err != nil {
				t.Fatal(err)
			}

			fmt.Println("done", i)
		}

		fmt.Println("finished")
	default:
		t.Fatalf("unknown export kind %q", kind)
	}
}

const (
	imageExports   = 12
	imageBaseWidth = 1200
)

// orphansOf lists the processes that were started by the process pid, which is gone, and are still alive. On
// Windows a process keeps the id of the parent that started it, so they are found by it; elsewhere an orphan is
// adopted by init and is found by the marker (the id of the job) in its command line.
func orphansOf(pid int, marker string) []int {
	var out []byte

	var err error

	if onWindows() {
		out, err = exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf("Get-CimInstance Win32_Process -Filter 'ParentProcessId=%d' | ForEach-Object { $_.ProcessId }", pid)).Output()
	} else {
		out, err = exec.Command("pgrep", "-f", marker).Output()
	}

	if err != nil && len(out) == 0 {
		return nil // no process has that parent
	}

	var ids []int

	for _, f := range strings.Fields(string(out)) {
		if n, aerr := strconv.Atoi(f); aerr == nil {
			ids = append(ids, n)
		}
	}

	return ids
}

// processAlive says whether a process with the pid exists.
func processAlive(pid int) bool {
	if onWindows() {
		out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH").Output()

		return err == nil && strings.Contains(string(out), strconv.Itoa(pid))
	}

	return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil
}

// endProcessTree ends a process and every process it started, by pid.
func endProcessTree(pid int) {
	if onWindows() {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()

		return
	}

	_ = exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
}

// killReport is what a kill of a child that has started other processes found.
type killReport struct {
	// orphans are the processes the child had started that were still alive once the child was gone.
	orphans []int
}

// killWithTree ends the child with Process.Kill, as the kill cases do, and then reports whether the processes it
// had started (FFmpeg) outlived it; the orphans are ended afterwards, so that none keeps writing or holding a
// file of the workspace. check runs while the orphans, if any, are still alive.
func killWithTree(t *testing.T, c *realChild, marker string, check func()) killReport {
	t.Helper()

	c.kill(t)

	var rep killReport

	for _, pid := range orphansOf(c.cmd.Process.Pid, marker) {
		if processAlive(pid) {
			rep.orphans = append(rep.orphans, pid)
		}
	}

	if check != nil {
		check()
	}

	for _, pid := range rep.orphans {
		endProcessTree(pid)
	}

	// An orphan that was ended is gone before the workspace is opened again.
	for _, pid := range rep.orphans {
		pid := pid

		waitFor(t, "the orphan of the child to end", func() bool { return !processAlive(pid) }, func() string {
			return "process " + strconv.Itoa(pid) + " is still alive"
		})
	}

	t.Logf("killed child %d; %d process(es) it had started outlived it on %s", c.cmd.Process.Pid,
		len(rep.orphans), runtime.GOOS)

	return rep
}

// A video export is killed after FFmpeg started and the partial file has grown; after the kill no file has the
// final name, the job is not left running, and the export runs again to a whole file with the recorded digest.
func TestKillRealExportVideo(t *testing.T) {
	pinnedFFmpeg(t) // skips with its reason when the pinned FFmpeg is not installed

	for _, moment := range []string{"started", "grown"} {
		t.Run(moment, func(t *testing.T) {
			w := newVideoExportWorld(t)
			child := w.startChild(t)

			switch moment {
			case "started":
				w.waitPartial(t, child, 1)
			default:
				// The encoder grows the file in a few steps (its header, then blocks); the third size is well inside
				// the encode of a video this long.
				w.waitPartial(t, child, 3)
			}

			rep := killWithTree(t, child, w.job, func() {
				// An orphan that is left alone must not write the final name: FFmpeg writes the partial file in the
				// run directory of the job, and only the verified file is renamed into place.
				w.assertNoFinalFile(t)
			})

			w.recoverAndRunAgain(t, 1)

			t.Logf("moment %q: orphan FFmpeg processes after the kill: %d", moment, len(rep.orphans))
		})
	}
}

// A video export that keeps ending its process is given up: after the attempts the job is failed with the reason
// interrupted, no file has the final name and nothing is left running.
func TestKillRealExportVideoGivenUpAfterTheAttempts(t *testing.T) {
	pinnedFFmpeg(t)

	w := newVideoExportWorld(t)

	for attempt := range 3 {
		child := w.startChild(t)
		w.waitPartial(t, child, 1)

		killWithTree(t, child, w.job, nil)

		// The start-up of the next process: the job that was running is requeued, or given up at the third.
		st, svc, jr := openExportService(t, w.ws)
		integrityOK(t, st)

		if err := jr.Recover(context.Background()); err != nil {
			t.Fatal(err)
		}

		var attempts int

		var state string
		if err := st.DB().QueryRowContext(context.Background(), `SELECT attempts, state FROM jobs WHERE id = ?`, w.job).
			Scan(&attempts, &state); err != nil {
			t.Fatal(err)
		}

		t.Logf("after the kill of attempt %d and the recovery: the job is %s with %d attempt(s)", attempt+1, state, attempts)

		_ = svc
		_ = st.Close()
	}

	_, svc, _ := openExportService(t, w.ws)

	job, err := svc.JobGet(service.WithCaller(context.Background(), service.Caller{Actor: ui, Scopes: service.AllScopes}),
		service.JobGetRequest{JobID: w.job})
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobFailed || job.Error == nil || job.Error.Code == "" {
		t.Fatalf("after three interrupted attempts the job is %s with %+v, want failed with a stable code", job.State, job.Error)
	}

	if got := fmt.Sprint(job.Error.Details["reason"]); got != "interrupted" {
		t.Errorf("the reason of the failure = %q, want interrupted", got)
	}

	w.assertNoFinalFile(t)
	w.assertNothingRunning(t, svc)

	// The partial video of a job that was given up is garbage: nothing will run the job again to replace it, so the
	// sweep of the start-up removes its run directory.
	if _, err := os.Stat(filepath.Dir(w.partialPath())); err != nil {
		t.Fatalf("the run directory of the abandoned job is not there to be swept: %v", err)
	}

	sw, ok := service.SweeperOf(svc)
	if !ok {
		t.Fatal("no sweeper")
	}

	if _, err := sw.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Dir(w.partialPath())); !os.IsNotExist(err) {
		t.Errorf("the run directory of the abandoned job survived the sweep: %v", err)
	}
}

// videoWorld is a workspace with a project and one export job that has not run.
type videoWorld struct {
	ws, project, job string
	since            time.Time
}

func newVideoExportWorld(t *testing.T) *videoWorld {
	t.Helper()

	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true, ingest: ingest.Run})

	project, track := m.withTrack()
	m.mustEdit(ui, project, m.head(project).Revision, op("settings.set", `{"width":320,"height":240}`))
	m.mustEdit(ui, project, m.head(project).Revision, op("text.add",
		fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"%ds","text":"kill me"}`, track, videoSeconds)))

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobQueued {
		t.Fatalf("the job is %s, want queued", job.State)
	}

	if err := m.store.Close(); err != nil { // the child owns the database now
		t.Fatal(err)
	}

	return &videoWorld{ws: m.ws, project: project, job: job.JobID}
}

func (w *videoWorld) partialPath() string {
	return filepath.Join(w.ws, "tmp", w.job, w.job+".partial.mp4")
}

// startChild starts a child that runs the job and remembers when, so that the partial file of an earlier,
// killed attempt is not taken for the file of this one.
func (w *videoWorld) startChild(t *testing.T) *realChild {
	t.Helper()

	w.since = time.Now()

	return startRealChild(t, "export-video", killVars(w.ws, "", nil, 0, w.job)...)
}

// waitPartial waits until the partial file of the child exists (written after the child started) and has been
// seen at that many different sizes, each larger than the one before: one is the start of the encode, more is
// the middle of it.
func (w *videoWorld) waitPartial(t *testing.T, child *realChild, grows int) {
	t.Helper()

	var sizes []int64

	waitFor(t, "the partial file of the export to grow", func() bool {
		fi, err := os.Stat(w.partialPath())
		if err != nil || !fi.ModTime().After(w.since) || fi.Size() == 0 {
			return false
		}

		if n := len(sizes); n == 0 || fi.Size() > sizes[n-1] {
			sizes = append(sizes, fi.Size())
		}

		return len(sizes) >= grows
	}, func() string { return fmt.Sprintf("sizes seen %v\n%s", sizes, child.output()) })
}

func (w *videoWorld) exportsDir() string { return filepath.Join(w.ws, "exports", w.project) }

// assertNoFinalFile checks that nothing is at a final name of the exports folder of the project: the job has
// no result, so any .mp4 there would be a file offered as a finished export.
func (w *videoWorld) assertNoFinalFile(t *testing.T) {
	t.Helper()

	entries, err := os.ReadDir(w.exportsDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			t.Errorf("exports/%s holds %s at a final name, and the job has no result", w.project, e.Name())
		}
	}
}

func (w *videoWorld) assertNothingRunning(t *testing.T, svc service.Service) {
	t.Helper()

	list, err := svc.JobList(service.WithCaller(context.Background(), service.Caller{Actor: ui, Scopes: service.AllScopes}),
		service.JobListRequest{ProjectID: w.project, State: service.JobRunning})
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Jobs) != 0 {
		t.Errorf("%d job(s) are still running after the start-up recovery: %+v", len(list.Jobs), list.Jobs[0])
	}
}

// recoverAndRunAgain is the next process: it opens the workspace, recovers, finds no job running, and runs the
// export again to a whole file whose size and digest are the ones the job records.
func (w *videoWorld) recoverAndRunAgain(t *testing.T, attempts int) {
	t.Helper()

	st, svc, jr := openExportService(t, w.ws)
	integrityOK(t, st)

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: ui, Scopes: service.AllScopes})

	if err := jr.Recover(ctx); err != nil {
		t.Fatal(err)
	}

	w.assertNothingRunning(t, svc)
	w.assertNoFinalFile(t)

	job, err := svc.JobGet(ctx, service.JobGetRequest{JobID: w.job})
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobQueued {
		t.Fatalf("after the recovery the job is %s (%s), want queued: resumed", job.State, job.Message)
	}

	if err := jr.RunJob(ctx, w.job); err != nil {
		t.Fatal(err)
	}

	done, err := svc.JobGet(ctx, service.JobGetRequest{JobID: w.job})
	if err != nil {
		t.Fatal(err)
	}

	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Export == nil {
		t.Fatalf("the export run again ended %s: %+v", done.State, done.Error)
	}

	assertExportWhole(t, done.Result.Export.Path, done.Result.Export.Bytes, done.Result.Export.SHA256)

	for _, e := range mustReadDir(t, w.exportsDir()) {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("a temporary file stayed in the exports folder: %s", e.Name())
		}
	}

	w.assertNothingRunning(t, svc)
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	return entries
}

// assertExportWhole checks a file against the size and digest a job records.
func assertExportWhole(t *testing.T, path string, size int64, digest string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the export the job records is not on disk: %v", err)
	}

	sum := sha256.Sum256(data)

	if int64(len(data)) != size || hex.EncodeToString(sum[:]) != digest {
		t.Errorf("%s holds %d bytes with digest %x; the job records %d bytes and %s", path, len(data), sum[:6], size, digest)
	}
}

var finalImageName = regexp.MustCompile(`^(\d+)-([0-9a-f]{12})\.png$`)

// An image export is killed at the start and after several files of a series; after the kill every file at a
// final name is whole (its name carries the digest of its bytes), an export run again writes the file with the
// digest it reports, and the sweep removes what the kill left behind.
func TestKillRealExportImage(t *testing.T) {
	for _, c := range []struct {
		name  string
		after int // the number of files done when the child is killed; -1 is the start of the first
	}{{"start", -1}, {"after two files", 2}, {"after six files", 6}} {
		t.Run(c.name, func(t *testing.T) {
			ws, imageID := newImageExportWorld(t)
			child := startRealChild(t, "export-image", killVars(ws, "", nil, 0, imageID)...)

			if c.after < 0 {
				child.line(t, "begin 0")
			} else {
				child.line(t, "done "+strconv.Itoa(c.after-1))
			}

			child.kill(t)

			dir := filepath.Join(ws, "exports", "images", imageID)
			whole := assertImageFilesWhole(t, dir)

			st, err := store.Open(context.Background(), workspace.Dir(ws), store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
			if err != nil {
				t.Fatalf("reopen after the kill: %v", err)
			}

			defer func() { _ = st.Close() }()

			integrityOK(t, st)

			svc, err := service.New(service.Options{
				Store: st, Registry: command.Default(), Limits: service.Limits{MinFreeBytes: -1}, Workspace: ws,
			})
			if err != nil {
				t.Fatal(err)
			}

			ctx := service.WithCaller(context.Background(), service.Caller{Actor: agent, Scopes: service.AllScopes})

			// Every export job that was recorded names a file that is on disk and whole.
			assertImageJobsMatchFiles(t, st, svc, ctx, ws, imageID)

			// The export run again writes a whole file with the digest it reports.
			out, err := svc.ImageExport(ctx, service.ImageExportRequest{ImageID: imageID, Format: "png", Width: imageBaseWidth})
			if err != nil {
				t.Fatal(err)
			}

			assertExportWhole(t, out.Path, out.Bytes, out.SHA256)

			sw, ok := service.SweeperOf(svc)
			if !ok {
				t.Fatal("no sweeper")
			}

			// The next day: what a kill left is old by then (the sweep leaves a file younger than ten minutes, which
			// another process may be writing).
			before := mustReadDir(t, dir)
			longAgo := time.Now().Add(-time.Hour)

			for _, e := range before {
				if err := os.Chtimes(filepath.Join(dir, e.Name()), longAgo, longAgo); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := sw.Sweep(ctx); err != nil {
				t.Fatal(err)
			}

			left := mustReadDir(t, dir) // the sweep keeps the exported images: the folder is still there

			for _, e := range left {
				if !finalImageName.MatchString(e.Name()) {
					t.Errorf("a temporary file stayed after the sweep: %s", e.Name())
				}
			}

			if got := assertImageFilesWhole(t, dir); got < whole {
				t.Errorf("the sweep removed exported images: %d whole files before it, %d after", whole, got)
			}

			t.Logf("moment %q: %d whole file(s) at the kill, %d entries before the sweep, %d after", c.name, whole,
				len(before), len(left))
		})
	}
}

func newImageExportWorld(t *testing.T) (ws, imageID string) {
	t.Helper()

	e := newEnvWith(t, func(e *env, o *service.Options) { o.Workspace = string(e.dir) })
	ctx := e.as(agent)

	created, err := e.svc.ImageCreate(ctx, service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Kill"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ImageEdit(ctx, service.ImageEditRequest{
		ImageID: created.ImageID, BaseRevision: 1, Operations: []command.Operation{
			op("layer.add_fill", `{"color": "#101820", "id": "lyr_aaaaaaaaaa"}`),
			op("layer.add_text", `{"text": "Kill me", "id": "lyr_bbbbbbbbbb"}`),
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.store.Close(); err != nil {
		t.Fatal(err)
	}

	return string(e.dir), created.ImageID
}

// assertImageFilesWhole checks every file at a final name of the folder against the digest in its name and
// returns how many there are. Files that are not at a final name are temporary: the sweep deals with them.
func assertImageFilesWhole(t *testing.T, dir string) int {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	whole := 0

	for _, e := range entries {
		m := finalImageName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}

		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}

		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:])[:12] != m[2] || len(data) == 0 {
			t.Errorf("%s holds %d bytes whose digest starts %x: it is not what its name says", e.Name(), len(data), sum[:6])
		}

		whole++
	}

	return whole
}

// assertImageJobsMatchFiles reads the recorded image exports back: each names a file on disk with the size and
// digest the job holds.
func assertImageJobsMatchFiles(
	t *testing.T, st *store.Store, svc service.Service, ctx context.Context, ws, imageID string,
) {
	t.Helper()

	rows, err := st.DB().QueryContext(ctx, `SELECT id FROM jobs WHERE kind = ? AND state = ?`,
		service.JobImageExport, service.JobSucceeded)
	if err != nil {
		t.Fatal(err)
	}

	var ids []string

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}

		ids = append(ids, id)
	}

	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	for _, id := range ids {
		j, err := svc.JobGet(ctx, service.JobGetRequest{JobID: id})
		if err != nil {
			t.Fatal(err)
		}

		if j.Result == nil || j.Result.Image == nil {
			t.Errorf("job %s is a succeeded image export with no result", id)

			continue
		}

		img := j.Result.Image
		file := filepath.FromSlash(img.Path)
		if !filepath.IsAbs(file) {
			file = filepath.Join(ws, file)
		}

		assertExportWhole(t, file, img.Bytes, img.SHA256)
	}

	t.Logf("%d recorded image export(s) of %s checked against their files", len(ids), imageID)
}
