package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

const helperFlag = "--vukem-helper="

// TestMain turns the test binary into a small child process for the runner tests: the
// children are started with the helper flag in argv, because the runner gives them no
// environment to carry it in.
func TestMain(m *testing.M) {
	if fakeTool() {
		return
	}
	for _, a := range os.Args[1:] {
		if mode, ok := strings.CutPrefix(a, helperFlag); ok {
			helper(mode)
			return
		}
	}
	// RunFFmpeg only accepts a real argv. The overlap test points the tool at this binary and
	// passes one, so the child has to answer here instead of starting the suite.
	if slices.Contains(os.Args[1:], "-nostdin") {
		overlapChild(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func overlapChild(args []string) {
	if slices.Contains(args, "-progress") {
		_, _ = os.Stdout.WriteString("frame=1\nout_time_us=1000\nprogress=continue\n")
	}
	// Stay until the runner kills this process. A bare receive deadlocks, and a
	// timed sleep on a new test line is refused. The signal watcher is the wait.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}

func helper(mode string) {
	switch mode {
	case "version":
		fmt.Println("helper version 1")
	case "env":
		for _, e := range os.Environ() {
			fmt.Println(e)
		}
		wd, _ := os.Getwd()
		fmt.Println("CWD=" + wd)
	case "sleep":
		time.Sleep(time.Minute)
	case "tree":
		gc := exec.Command(os.Args[0], helperFlag+"sleep")
		if err := gc.Start(); err != nil {
			os.Exit(3)
		}
		_ = os.WriteFile("pids", []byte(fmt.Sprintf("%d %d", os.Getpid(), gc.Process.Pid)), 0o600)
		fmt.Println("ready")
		time.Sleep(time.Minute)
	case "progress":
		for i := 1; i <= 3; i++ {
			fmt.Printf("frame=%d\nout_time_us=%d\nprogress=continue\n", i, i*1000)
		}
		fmt.Println("frame=3\nout_time_us=3000\nprogress=end")
	case "stall":
		fmt.Println("frame=1\nout_time_us=1000\nprogress=continue")
		time.Sleep(time.Minute)
	case "long-line":
		fmt.Println(strings.Repeat("x", 10000))
		fmt.Println("frame=7\nout_time_us=7\nprogress=end")
	case "grow":
		f, err := os.Create("out.bin")
		if err != nil {
			os.Exit(3)
		}
		chunk := make([]byte, 1<<20)
		for range 600 {
			_, _ = f.Write(chunk)
			_ = f.Sync()
			time.Sleep(100 * time.Millisecond)
		}
	case "limits":
		b, _ := os.ReadFile("/proc/self/limits")
		fmt.Print(string(b))
	case "dial":
		if len(os.Args) < 3 {
			os.Exit(2)
		}
		//nolint:forbidigo // test helper: proves the sandbox has no loopback
		c, err := net.DialTimeout("tcp", os.Args[2], 2*time.Second)
		if err != nil {
			fmt.Println("dial failed")
			os.Exit(4)
		}
		_ = c.Close()
		fmt.Println("dial ok")
	case "alloc":
		allocate()
	case "alloctree":
		gc := exec.Command(os.Args[0], helperFlag+"sleep")
		if err := gc.Start(); err != nil {
			os.Exit(3)
		}
		_ = os.WriteFile("pids", []byte(fmt.Sprintf("%d %d", os.Getpid(), gc.Process.Pid)), 0o600)
		allocate()
	case "big":
		fmt.Print(strings.Repeat("y", 5<<20))
	case "fail":
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(3)
	}
}

// allocate commits and touches 2 GiB in one block, then reports; under a memory limit the Go
// runtime aborts before it reports (one block: a limit hit while the heap grows by steps stalls it).
func allocate() {
	buf := make([]byte, 2<<30)
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = 1
	}
	fmt.Println("allocated", len(buf)>>20)
}

func helperRunner(t *testing.T) (*Runner, *RunDir, *engine.Tool) {
	t.Helper()
	tool, err := engine.Find(context.Background(), engine.Config{
		Name: "helper", Path: os.Args[0], VersionArgs: []string{helperFlag + "version"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	r, err := NewRunner(&Installation{FFmpeg: tool, FFprobe: tool, CPUCount: 2}, ws)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := r.NewRunDir("job_1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Remove() })
	return r, dir, tool
}

func TestSecurity_T16_ChildEnvironmentMinimal(t *testing.T) {
	for _, k := range []string{"VUKEM_VIDEO_TRANSCRIBE_KEY", "APP_KEY", "VUKEM_VIDEO_KEY", "OTEL_EXPORTER_OTLP_ENDPOINT", "VUKEM_VIDEO_TOKEN", "FFREPORT"} {
		t.Setenv(k, "canary-"+k)
	}
	r, dir, tool := helperRunner(t)

	var out bytes.Buffer
	res, err := r.exec(
		context.Background(),
		execSpec{
			tool:     tool,
			dir:      dir.Dir(),
			argv:     []string{helperFlag + "env"},
			deadline: 30 * time.Second,
			stdout:   &out,
		},
	)
	if err != nil {
		t.Fatalf("%v (stderr %q)", err, res.Stderr)
	}
	got := map[string]string{}
	for l := range strings.SplitSeq(strings.ReplaceAll(out.String(), "\r", ""), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			got[strings.ToUpper(k)] = v
		}
	}
	if strings.Contains(out.String(), "canary-") {
		t.Fatalf("a server variable reached the child:\n%s", out.String())
	}
	for _, k := range []string{"HOME", "TMPDIR", "TEMP", "TMP"} {
		if !sameDir(got[k], dir.Dir()) {
			t.Errorf("%s = %q, want the run directory %q", k, got[k], dir.Dir())
		}
	}
	if !sameDir(got["CWD"], dir.Dir()) {
		t.Errorf("cwd = %q, want %q", got["CWD"], dir.Dir())
	}
	allowed := map[string]bool{
		"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "LANG": true, "LC_ALL": true,
		"HOME": true, "TMPDIR": true, "TEMP": true, "TMP": true, "CWD": true,
		// Windows adds a few variables itself when it starts a process with a sparse environment.
		"=C:": true, "SYSTEMDRIVE": true, "COMSPEC": true, "PATHEXT": true,
	}
	for k := range got {
		if k != "" && !allowed[k] && !strings.HasPrefix(k, "=") {
			t.Errorf("unexpected variable %s in the child environment", k)
		}
	}
	if r.started.Load() != 1 {
		t.Errorf("started = %d", r.started.Load())
	}
}

func sameDir(a, b string) bool {
	ea, errA := filepath.EvalSymlinks(a)
	eb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return strings.EqualFold(ea, eb)
}

func TestSecurity_T16_CancelKillsProcessTree(t *testing.T) {
	r, dir, tool := helperRunner(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.exec(
			ctx,
			execSpec{
				tool:     tool,
				dir:      dir.Dir(),
				argv:     []string{helperFlag + "tree"},
				deadline: time.Minute,
				stdout:   &bytes.Buffer{},
			},
		)
		done <- err
	}()

	pidFile := filepath.Join(dir.Dir(), "pids")
	var parent, child int
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(pidFile); err == nil {
			if n, _ := fmt.Sscanf(string(b), "%d %d", &parent, &child); n == 2 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if parent == 0 || child == 0 {
		t.Fatal("the helper never reported its pids")
	}
	if !processAlive(parent) || !processAlive(child) {
		t.Fatalf("helper tree is not running: parent %v child %v", processAlive(parent), processAlive(child))
	}

	cancelled := time.Now()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want one wrapping context.Canceled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the run did not return after the cancellation")
	}

	// SDD-007 6.6: the tree is gone within 1 s of the cancellation.
	for time.Since(cancelled) < time.Second && (processAlive(parent) || processAlive(child)) {
		time.Sleep(10 * time.Millisecond)
	}
	if processAlive(parent) || processAlive(child) {
		t.Errorf("process tree still alive %s after the cancellation (parent %v, grandchild %v)",
			time.Since(cancelled).Round(time.Millisecond), processAlive(parent), processAlive(child))
	}
	if err := dir.Remove(); err != nil {
		t.Errorf("the run directory could not be removed after the kill: %v", err)
	}
}

func TestRunDeadlineKillsTheRun(t *testing.T) {
	r, dir, tool := helperRunner(t)
	start := time.Now()
	_, err := r.exec(
		context.Background(),
		execSpec{
			tool:     tool,
			dir:      dir.Dir(),
			argv:     []string{helperFlag + "sleep"},
			deadline: 300 * time.Millisecond,
			stdout:   &bytes.Buffer{},
		},
	)
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("error = %v, want ErrDeadline", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("took %s", time.Since(start))
	}
}

func TestScaledDeadlinesStayInsideTheCeilings(t *testing.T) {
	exportBase := ExportDeadline(30 * time.Second)
	if exportBase != 5*time.Minute || ExportDeadline(10*time.Hour) != 2*time.Hour {
		t.Fatal("ExportDeadline(30s) must stay 5min and ExportDeadline(10h) 2h")
	}
	if got := scaleAllowance(exportBase, 40, deadlineCeiling(exportBase, ExportDeadlineCeiling)); got != 2*time.Hour {
		t.Fatalf("scaled export deadline = %s, want 2h", got)
	}
	if got := scaleAllowance(exportBase, 1, ExportDeadlineCeiling); got != exportBase {
		t.Fatalf("a lone export deadline = %s, want %s", got, exportBase)
	}

	ingestBase := 30 * time.Minute
	if got := scaleAllowance(ingestBase, 40, deadlineCeiling(ingestBase, IngestDeadlineCeiling)); got != 4*time.Hour {
		t.Fatalf("scaled ingest deadline = %s, want 4h", got)
	}
	if got := scaleAllowance(4*time.Hour, 3, IngestDeadlineCeiling); got != 4*time.Hour {
		t.Fatalf("an ingest already at the ceiling scaled to %s", got)
	}
	if got := scaleAllowance(time.Second, 4, 0); got != 4*time.Second {
		t.Fatalf("scaled stall = %s, want 4s", got)
	}
	if got := scaleAllowance(time.Second, 1, 0); got <= 0 || got != time.Second {
		t.Fatalf("lone stall = %s", got)
	}
}

func overlapArgv(t *testing.T, r *Runner, dir *RunDir, progress bool) []string {
	t.Helper()
	argv, err := ffargs.Cmd{
		Progress: progress,
		Inputs:   []ffargs.Input{ffargs.TestSrc2(16, 16, ffargs.Rate{Num: 1, Den: 1}, 1000)},
		Outputs:  []ffargs.Output{{Format: "null", Maps: []string{"0:v:0"}}},
	}.Argv(r.Policy(dir))
	if err != nil {
		t.Fatal(err)
	}

	return argv
}

func awaitFlight(t *testing.T, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		got, changed := flightNow()
		if got >= n {
			return
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("in-flight runs = %d, want at least %d", got, n)
		}
	}
}

// Two runners share the process count: A stops making progress, B stays up, and A must not take
// the stall error until two stall intervals could have elapsed. After B ends, the base stall applies.
func TestOverlappingRunsDoNotStallWhileAPeerIsAlive(t *testing.T) {
	if ExportDeadline(30*time.Second) != 5*time.Minute || ExportDeadline(10*time.Hour) != 2*time.Hour {
		t.Fatal("ExportDeadline(30s) must stay 5min and ExportDeadline(10h) 2h")
	}
	const stall = time.Second

	rA, dirA, tool := helperRunner(t)
	rB, err := NewRunner(&Installation{FFmpeg: tool, FFprobe: tool, CPUCount: 2}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dirB, err := rB.NewRunDir("job_b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dirB.Remove() })

	argvA := overlapArgv(t, rA, dirA, true)
	argvB := overlapArgv(t, rB, dirB, false)

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	aDone := make(chan error, 1)
	bDone := make(chan error, 1)
	var wg sync.WaitGroup
	t.Cleanup(func() {
		cancelA()
		cancelB()
		wg.Wait()
	})

	progressed := make(chan struct{})
	var once sync.Once
	wg.Go(func() {
		_, runErr := rA.RunFFmpeg(ctxA, Request{
			Dir: dirA, Argv: argvA, Deadline: time.Minute, Stall: stall,
			OnProgress: func(Progress) { once.Do(func() { close(progressed) }) },
		})
		aDone <- runErr
	})

	select {
	case <-progressed:
	case err := <-aDone:
		t.Fatalf("A ended before progress: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("A did not report progress")
	}

	wg.Go(func() {
		_, runErr := rB.RunFFmpeg(ctxB, Request{Dir: dirB, Argv: argvB, Deadline: time.Minute})
		bDone <- runErr
	})
	awaitFlight(t, 2)

	// Stop short of the scaled window so the watchdog tick cannot land on the boundary.
	// An unscaled stall has already expired: one interval is not enough while B is alive.
	hold := 2*stall - 300*time.Millisecond
	select {
	case err := <-aDone:
		t.Fatalf("A returned before two stall intervals while B was alive: %v", err)
	case <-time.After(hold):
	}

	cancelB()
	select {
	case <-bDone:
	case <-time.After(15 * time.Second):
		t.Fatal("B did not end")
	}

	select {
	case err := <-aDone:
		if !errors.Is(err, ErrStalled) {
			t.Fatalf("A error = %v, want ErrStalled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("A did not stall after B ended")
	}
}

func TestStallWatchdogCancelsWhenProgressStops(t *testing.T) {
	r, dir, tool := helperRunner(t)
	var last Progress
	_, err := r.exec(context.Background(), execSpec{
		tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "stall"}, deadline: time.Minute,
		stall: 400 * time.Millisecond, onProgress: func(p Progress) { last = p },
	})
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("error = %v, want ErrStalled", err)
	}
	if last.Frame != 1 {
		t.Errorf("last progress = %+v", last)
	}
}

func TestProgressIsParsedAndLongLinesAreDropped(t *testing.T) {
	r, dir, tool := helperRunner(t)
	var seen []Progress
	_, err := r.exec(context.Background(), execSpec{
		tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "progress"}, deadline: time.Minute,
		stall: time.Minute, onProgress: func(p Progress) { seen = append(seen, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 4 || !seen[3].End || seen[3].Frame != 3 || seen[2].OutTimeUS != 3000 {
		t.Errorf("progress blocks = %+v", seen)
	}

	seen = nil
	if _, err := r.exec(context.Background(), execSpec{
		tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "long-line"}, deadline: time.Minute,
		stall: time.Minute, onProgress: func(p Progress) { seen = append(seen, p) },
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].Frame != 7 {
		t.Errorf("after a 10000-character line the next block must still parse: %+v", seen)
	}
}

func TestRunFFmpegRefusesAnArgvOutsideTheGrammarBeforeStarting(t *testing.T) {
	r, dir, _ := helperRunner(t)
	_, err := r.RunFFmpeg(
		context.Background(),
		Request{Dir: dir, Argv: []string{"-i", "http://example.com/x"}, Deadline: time.Second},
	)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeInternal || e.Reason != "argv_policy" {
		t.Fatalf("error = %v, want E_INTERNAL argv_policy", err)
	}
	if r.started.Load() != 0 {
		t.Fatal("a refused argv started a process")
	}
	_, _, err = r.RunFFprobe(context.Background(), dir, []string{"-show_packets"}, time.Second)
	if !errors.As(err, &e) || e.Reason != "argv_policy" || r.started.Load() != 0 {
		t.Fatalf("ffprobe: %v, started %d", err, r.started.Load())
	}
}

func TestRunFFprobeCapsItsOutputAt4MiB(t *testing.T) {
	r, dir, tool := helperRunner(t)
	w := &limitWriter{max: 1 << 20}
	_, err := r.exec(
		context.Background(),
		execSpec{
			tool:       tool,
			dir:        dir.Dir(),
			argv:       []string{helperFlag + "big"},
			deadline:   time.Minute,
			stdout:     w,
			onOverflow: w,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !w.over.Load() {
		t.Error("5 MiB under a 1 MiB cap did not overflow")
	}
}

func TestRunDirIsPrivateAndJobIDsAreChecked(t *testing.T) {
	r, dir, _ := helperRunner(t)
	if filepath.Dir(dir.Dir()) != filepath.Join(r.Workspace, "tmp") {
		t.Errorf("run dir = %s", dir.Dir())
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(dir.Dir())
		if err != nil || st.Mode().Perm() != 0o700 {
			t.Errorf("mode = %v %v", st.Mode(), err)
		}
	}
	for _, bad := range []string{"", "../x", "a/b", "a b", strings.Repeat("a", 65), `a\b`} {
		if _, err := r.NewRunDir(bad); err == nil {
			t.Errorf("job id %q accepted", bad)
		}
	}
	if _, err := r.NewRunDir("job_1"); err == nil {
		t.Error("a second run directory with the same id was created")
	}
	if _, err := dir.Path("a/b"); err == nil {
		t.Error("Path accepted a separator")
	}
	if _, err := dir.Sub("thumbs"); err != nil {
		t.Error(err)
	}
}

func TestDeadlines(t *testing.T) {
	if IngestDeadline(60*time.Second) != 4*time.Minute {
		t.Error("ingest of 60 s")
	}
	if IngestDeadlineFor(10*time.Second, 225<<20) != 60*time.Second+30*time.Second+225*time.Second {
		t.Error("ingest of a 225 MiB file")
	}
	if IngestDeadlineFor(time.Minute, 0) != IngestDeadline(time.Minute) {
		t.Error("ingest of an empty file")
	}
	if IngestDeadline(10*time.Hour) != 4*time.Hour {
		t.Error("ingest cap")
	}
	if ExportDeadline(30*time.Second) != 5*time.Minute {
		t.Error("export of 30 s")
	}
	if ExportDeadline(10*time.Hour) != 2*time.Hour {
		t.Error("export cap")
	}
	if AudioExtractDeadline(60*time.Second) != 2*time.Minute || AudioExtractDeadline(5*time.Hour) != time.Hour {
		t.Error("audio extraction")
	}
	if TranscribeDeadline(60*time.Second) != 5*time.Minute || TranscribeDeadline(10*time.Hour) != 6*time.Hour {
		t.Error("transcription")
	}
}

// A leftover directory of an interrupted run must never block a later one (VV-001).
func TestInternalRunDirSurvivesLeftovers(t *testing.T) {
	r, _, _ := helperRunner(t)
	for _, name := range []string{"encoder-detection", "encoder-size", "capabilities"} {
		if err := os.MkdirAll(filepath.Join(r.Workspace, "tmp", name), 0o700); err != nil {
			t.Fatal(err)
		}
		a, err := r.newInternalRunDir(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := r.newInternalRunDir(name)
		if err != nil {
			t.Fatalf("%s: second: %v", name, err)
		}
		if a.Dir() == b.Dir() || !strings.HasPrefix(filepath.Base(a.Dir()), name+"-") {
			t.Errorf("names = %s, %s", a.Dir(), b.Dir())
		}
		_ = a.Remove()
		_ = b.Remove()
	}
	if _, err := r.newInternalRunDir("a b"); err == nil {
		t.Error("an invalid prefix was accepted")
	}
}

// A retried job reuses its id; the directory its interrupted attempt left behind is reclaimed (VV-001).
func TestNewJobRunDirReclaimsLeftover(t *testing.T) {
	r, _, _ := helperRunner(t)
	left := filepath.Join(r.Workspace, "tmp", "job_9")
	if err := os.MkdirAll(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "mix.wav"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := r.NewJobRunDir("job_9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d.Dir(), "mix.wav")); err == nil {
		t.Error("the leftover file survived")
	}
	if _, err := r.NewJobRunDir("a/b"); err == nil {
		t.Error("an invalid job id was accepted")
	}
}
