package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
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
	os.Exit(m.Run())
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
