package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/cli"
)

type serveProc struct {
	cmd  *exec.Cmd
	out  *bytes.Buffer
	done chan struct{} // closed when the process has exited
	err  error
}

// startServe runs `serve` as a real process and waits until it announces itself.
func startServe(t *testing.T, bin, home string) *serveProc {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bin, "serve")
	cmd.Env = append(os.Environ(), "VUKEM_VIDEO_HOME="+home, "VUKEM_VIDEO_URL=", "APP_KEY=", "APP_ENV=",
		"HOST=127.0.0.1", "PORT="+freePort(t), "VUKEM_VIDEO_MIN_FREE_BYTES=1")

	p := &serveProc{cmd: cmd, out: &bytes.Buffer{}, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = p.out, p.out

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	go func() { p.err = cmd.Wait(); close(p.done) }()

	t.Cleanup(func() {
		_ = cmd.Process.Kill() // a no-op once it exited
		<-p.done
	})

	dir := workspace.Dir(home)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if rt, err := dir.ReadRuntime(); err == nil && rt.PID == cmd.Process.Pid {
			return p
		}

		select {
		case <-p.done:
			t.Fatalf("serve exited: %v\n%s", p.err, p.out)
		case <-time.After(100 * time.Millisecond):
		}
	}

	t.Fatalf("serve did not announce itself\n%s", p.out)

	return nil
}

func resolve(t *testing.T, home string) *cli.Connection {
	t.Helper()

	conn, err := cli.LocalResolver{Getenv: func(k string) string {
		if k == workspace.EnvHome {
			return home
		}

		return ""
	}}.Resolve(context.Background(), cli.Target{Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}

	return conn
}

// The serve process owns the workspace: runtime.json names it, a second serve
// is refused with E_WORKSPACE_LOCKED, and the CLI finds the owner and uses HTTP
// mode (SDD-005 section 6.4); after a stop the CLI becomes the owner itself and
// finds what the server committed.
func TestServeIsTheOwnerAndTheCLIUsesIt(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")
	p := startServe(t, bin, home)

	rt, err := workspace.Dir(home).ReadRuntime()
	if err != nil || rt.Mode != workspace.ModeLocal || rt.Version != "dev" {
		t.Fatalf("runtime.json = %+v, %v", rt, err)
	}

	if resp, err := outbound.LocalServer(5 * time.Second).
		Get("http://" + rt.Addr + "/up"); err != nil ||
		resp.StatusCode != http.StatusOK {
		t.Fatalf("/up: %v", err)
	} else {
		_ = resp.Body.Close()
	}

	stderr, code := serveOnce(t, bin, home)
	if code != exitLocked || !strings.Contains(stderr, "E_WORKSPACE_LOCKED") {
		t.Errorf("a second serve: exit %d, %q", code, stderr)
	}

	conn := resolve(t, home)
	if conn.InProcess {
		t.Fatal("the CLI took the workspace from a running server instead of using HTTP")
	}

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "cli", Scopes: service.AllScopes})

	created, err := conn.Service.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "over http"})
	if err != nil {
		t.Fatal(err)
	}

	if conn.Close != nil {
		_ = conn.Close()
	}

	if runtime.GOOS == "windows" {
		_ = p.cmd.Process.Kill() // Windows has no SIGTERM for another process
	} else {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
	}

	select {
	case <-p.done:
		if runtime.GOOS != "windows" && p.err != nil {
			t.Errorf("a graceful stop must exit 0: %v\n%s", err, p.out)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("serve did not stop")
	}

	if runtime.GOOS != "windows" {
		if _, err := workspace.Dir(home).ReadRuntime(); err == nil {
			t.Error("runtime.json survived a graceful stop")
		}
	}

	again := resolve(t, home)
	if !again.InProcess {
		t.Fatal("with no server the CLI must own the workspace")
	}

	defer func() { _ = again.Close() }()

	got, err := again.Service.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: created.ProjectID})
	if err != nil || got.Name != "over http" {
		t.Errorf("the project made through the server is gone: %+v, %v", got, err)
	}
}

func serveOnce(t *testing.T, bin, home string) (stderr string, code int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Env = append(os.Environ(), "VUKEM_VIDEO_HOME="+home, "VUKEM_VIDEO_URL=", "PORT="+freePort(t))

	var errOut bytes.Buffer

	cmd.Stderr = &errOut

	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok { //nolint:errorlint // exact type from Run
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	return errOut.String(), code
}
