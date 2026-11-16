package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/agentdocs"
	"github.com/AlexGalhardo/vukem-video/web/app/cli"
)

type serveProc struct {
	cmd  *exec.Cmd
	out  *lockedBuffer
	done chan struct{} // closed when the process has exited
	err  error
}

// lockedBuffer is the output of a running process: exec copies into it from its
// own goroutine while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// startServe runs `serve` as a real process and waits until it announces itself.
func startServe(t *testing.T, bin, home string) *serveProc {
	t.Helper()

	return startServeEnv(t, bin, home, "HOST=127.0.0.1", "PORT="+freePort(t))
}

// startServeEnv is startServe with its own environment on top of the defaults.
func startServeEnv(t *testing.T, bin, home string, env ...string) *serveProc {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bin, "serve")
	cmd.Env = append(append(os.Environ(), "VUKEM_VIDEO_HOME="+home, "VUKEM_VIDEO_URL=", "APP_KEY=", "APP_ENV=",
		"VUKEM_VIDEO_MIN_FREE_BYTES=1"), env...)

	p := &serveProc{cmd: cmd, out: &lockedBuffer{}, done: make(chan struct{})}
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
	if err != nil || rt.Mode != workspace.ModeLocal || rt.Version != agentdocs.Version {
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

// SDD-007 section 4.5: an unsafe configuration exits 1 with a message that names
// the variable, before anything is locked or written (feeds T-34).
func TestServeRefusesAnUnsafeConfiguration(t *testing.T) {
	bin := buildBinary(t)

	for _, c := range []struct {
		name string
		env  []string
		want string
	}{
		{"unknown mode", []string{"VUKEM_VIDEO_MODE=lan"}, "VUKEM_VIDEO_MODE"},
		{"non-loopback address in local mode", []string{"VUKEM_VIDEO_ADDR=0.0.0.0:0"}, "VUKEM_VIDEO_ADDR"},
		{"network mode without APP_URL", []string{"VUKEM_VIDEO_MODE=network", "APP_URL=", "PORT=0"}, "APP_URL"},
		{"network mode with a short key", []string{
			"VUKEM_VIDEO_MODE=network", "APP_URL=http://localhost:7878", "APP_KEY=short",
		}, "APP_KEY"},
		{"network mode with a zero limit", []string{
			"VUKEM_VIDEO_MODE=network", "APP_URL=http://localhost:7878", "VUKEM_VIDEO_MAX_EXPORTS=0",
		}, "invalid limit"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "ws")

			cmd := exec.CommandContext(t.Context(), bin, "serve")
			cmd.Env = append(os.Environ(), "VUKEM_VIDEO_HOME="+home, "VUKEM_VIDEO_URL=", "APP_ENV=", "APP_KEY=")
			cmd.Env = append(cmd.Env, c.env...)

			var out bytes.Buffer

			cmd.Stderr = &out

			err := cmd.Run()

			ee, ok := err.(*exec.ExitError) //nolint:errorlint // exact type from Run
			if !ok || ee.ExitCode() != 1 || !strings.Contains(out.String(), c.want) {
				t.Fatalf("err = %v, stderr = %q", err, out.String())
			}

			if _, err := workspace.Dir(home).ReadRuntime(); err == nil {
				t.Error("a refused configuration announced itself")
			}
		})
	}
}

// Network mode in the container-on-own-machine form: it announces itself as
// network, creates no token file, and answers every API request 401 while there
// is no user; a request for another Host is refused (SDD-007 sections 4.3, 4.5).
func TestServeNetworkModeWithNoUsers(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")
	port := freePort(t)

	p := startServeEnv(
		t,
		bin,
		home,
		"VUKEM_VIDEO_MODE=network",
		"VUKEM_VIDEO_ADDR=127.0.0.1:"+port,
		"APP_URL=http://localhost:"+port,
		"APP_KEY=base64:"+base64.StdEncoding.EncodeToString(
			[]byte(strings.Repeat("k", 32)),
		),
		"VUKEM_VIDEO_INSECURE_ALLOW_ROOT=true",
	)

	rt, err := workspace.Dir(home).ReadRuntime()
	if err != nil || rt.Mode != workspace.ModeNetwork || rt.Addr != "127.0.0.1:"+port {
		t.Fatalf("runtime.json = %+v, %v", rt, err)
	}

	if _, err := os.Stat(filepath.Join(home, "token")); err == nil {
		t.Error("network mode created the local token file")
	}

	get := func(path, host string) int {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:"+port+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		req.Host = host

		resp, err := outbound.LocalServer(5 * time.Second).Do(req)
		if err != nil {
			t.Fatal(err)
		}

		_ = resp.Body.Close()

		return resp.StatusCode
	}

	if got := get("/api/v1/projects", "localhost:"+port); got != http.StatusUnauthorized {
		t.Errorf("no users: status = %d, want 401", got)
	}

	if got := get("/api/v1/projects", "192.168.1.5:"+port); got != http.StatusForbidden {
		t.Errorf("another host: status = %d, want 403", got)
	}

	if got := get("/up", "127.0.0.1:"+port); got != http.StatusOK {
		t.Errorf("/up: status = %d, want 200", got)
	}

	if !strings.Contains(p.out.String(), "user:create") {
		t.Errorf("the log does not say how to create a user: %s", p.out)
	}
}
