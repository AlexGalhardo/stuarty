package shell

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

var fakeBinary string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run builds the fake server once; a function of its own so the deferred
// cleanup runs before os.Exit.
func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "shell-fake")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	name := "fakeserver"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	fakeBinary = filepath.Join(dir, name)
	if out, err := exec.Command("go", "build", "-o", fakeBinary, "./testdata/fakeserver").CombinedOutput(); err != nil {
		_, _ = os.Stderr.Write(out)
		panic(err)
	}
	return m.Run()
}

func newConfig(t *testing.T, mode string) Config {
	t.Helper()
	t.Setenv("FAKE_MODE", mode)
	ws, err := workspace.Resolve(func(string) string { return t.TempDir() })
	if err != nil {
		t.Fatal(err)
	}
	return Config{Binary: fakeBinary, Workspace: ws, ReadyTimeout: 10 * time.Second, StopGrace: 2 * time.Second}
}

func TestStartAndStop(t *testing.T) {
	s, err := Start(context.Background(), newConfig(t, "ok"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)

	if s.Port == 0 || s.Addr != net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port)) {
		t.Errorf("addr %q port %d", s.Addr, s.Port)
	}
	if len(s.Token) != workspace.TokenLength {
		t.Errorf("token length %d", len(s.Token))
	}
	if !Allowed(s.URL()+"/projects", s.Port) {
		t.Errorf("the shell would refuse its own server %s", s.URL())
	}
	resp, err := http.Get(s.URL() + "/up") //nolint:forbidigo // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	s.Stop()
	select {
	case <-s.Done():
	default:
		t.Fatal("the child is still running after Stop")
	}
	//nolint:forbidigo // loopback test server
	if c, err := net.DialTimeout("tcp", s.Addr, 300*time.Millisecond); err == nil {
		_ = c.Close()
		t.Error("the port still answers after Stop")
	}
	s.Stop() // a second Stop is harmless
}

func TestStopKillsAChildThatIgnoresTheSignal(t *testing.T) {
	cfg := newConfig(t, "ignore-term")
	cfg.StopGrace = 300 * time.Millisecond
	s, err := Start(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	s.Stop()
	select {
	case <-s.Done():
	default:
		t.Fatal("the child survived Stop")
	}
}

func TestReadinessTimeout(t *testing.T) {
	cfg := newConfig(t, "never-ready")
	cfg.ReadyTimeout = 600 * time.Millisecond
	s, err := Start(context.Background(), cfg)
	assertReason(t, err, ReasonTimeout)
	if s != nil {
		t.Error("a failed start returned a server")
	}
}

func TestChildExitsWhileStarting(t *testing.T) {
	_, err := Start(context.Background(), newConfig(t, "exit"))
	assertReason(t, err, ReasonExited)
	var se *StartupError
	if errors.As(err, &se) && !strings.Contains(se.Message, "cannot open the workspace") {
		t.Errorf("the message of the child is not in %q", se.Message)
	}
}

func TestMissingBinary(t *testing.T) {
	cfg := newConfig(t, "ok")
	cfg.Binary = filepath.Join(t.TempDir(), "nope")
	_, err := Start(context.Background(), cfg)
	assertReason(t, err, ReasonSpawn)
}

func TestRelativeBinaryIsRefused(t *testing.T) {
	cfg := newConfig(t, "ok")
	cfg.Binary = "stuarty"
	_, err := Start(context.Background(), cfg)
	assertReason(t, err, ReasonSpawn)
}

func TestPortInUseIsReported(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	cfg := newConfig(t, "ok")
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("not a TCP address")
	}
	cfg.Port = tcp.Port
	_, err = Start(context.Background(), cfg)
	assertReason(t, err, ReasonPortInUse)
}

func TestStaleRuntimeIsNotTakenForTheChild(t *testing.T) {
	cfg := newConfig(t, "never-ready")
	cfg.ReadyTimeout = 600 * time.Millisecond
	stale := workspace.Runtime{PID: 1, Addr: "127.0.0.1:9", Mode: workspace.ModeLocal}
	if err := cfg.Workspace.WriteRuntime(stale); err != nil {
		t.Fatal(err)
	}
	_, err := Start(context.Background(), cfg)
	assertReason(t, err, ReasonTimeout)
}

func assertReason(t *testing.T, err error, want Reason) {
	t.Helper()
	var se *StartupError
	if !errors.As(err, &se) {
		t.Fatalf("want a StartupError, got %v", err)
	}
	if se.Reason != want {
		t.Errorf("reason %q, want %q (%v)", se.Reason, want, se)
	}
	if se.Message == "" {
		t.Error("the message for the dialog is empty")
	}
}

// The child must not outlive the shell: closing the job handle (what a crash does) ends it (VV-006).
func TestBoundChildDiesWithTheShell(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the job object is Windows only")
	}

	cmd := exec.Command("ping", "-n", "60", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	closeJob, err := bindToShell(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}

	closeJob()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("the child survived the close of the job")
	}
}

func TestWorkspaceOfARunningServerIsReportedWithWhatToDo(t *testing.T) {
	cfg := newConfig(t, "ok")
	rt := workspace.Runtime{PID: os.Getpid(), Addr: "127.0.0.1:3490", Mode: workspace.ModeLocal, StartedAt: time.Now()}
	if err := cfg.Workspace.WriteRuntime(rt); err != nil {
		t.Fatal(err)
	}

	_, err := Start(context.Background(), cfg)
	assertReason(t, err, ReasonInUse)

	if se, ok := errors.AsType[*StartupError](err); ok {
		for _, want := range []string{strconv.Itoa(os.Getpid()), "127.0.0.1:3490", "stuarty open", "STUARTY_HOME"} {
			if !strings.Contains(se.Message, want) {
				t.Errorf("message %q lacks %q", se.Message, want)
			}
		}
	}

	if _, err := cfg.Workspace.ReadRuntime(); err != nil {
		t.Errorf("the runtime.json of the running server must stay: %v", err)
	}
}
