package main

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

func upHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/up", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

type fixture struct {
	o       *opener
	out     *bytes.Buffer
	errOut  *bytes.Buffer
	browsed []string
	started int
	env     []string
	home    string
}

func newFixture(t *testing.T, port string, extra map[string]string) *fixture {
	t.Helper()
	f := &fixture{out: &bytes.Buffer{}, errOut: &bytes.Buffer{}, home: t.TempDir()}
	vars := map[string]string{"STUARTY_HOME": f.home, "PORT": port, "HOST": "127.0.0.1"}
	maps.Copy(vars, extra)
	f.o = &opener{
		getenv: func(k string) string { return vars[k] }, stdout: f.out, stderr: f.errOut,
		client:  &http.Client{Timeout: time.Second}, //nolint:forbidigo // loopback test server
		self:    func() (string, error) { return "/path/to/stuarty", nil },
		browse:  func(u string) error { f.browsed = append(f.browsed, u); return nil },
		timeout: 3 * time.Second,
	}
	return f
}

func TestOpenReusesARunningServer(t *testing.T) {
	srv := httptest.NewServer(upHandler())
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	f := newFixture(t, port, nil)
	f.o.startSrv = func(string, string, string, []string, string) (<-chan error, error) {
		f.started++
		return nil, errors.New("must not start a second server")
	}
	if err := f.o.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.started != 0 || len(f.browsed) != 1 || f.browsed[0] != "http://127.0.0.1:"+port+"/" {
		t.Errorf("started %d, browsed %v", f.started, f.browsed)
	}
}

// VV-079: open finds the server through runtime.json, whatever address it was started on, and opens the
// launch address, not the plain one that answers 401.
func TestOpenUsesTheRunningServerOfTheWorkspaceAndItsLaunchCode(t *testing.T) {
	code := strings.Repeat("ab", 32)
	token := ""
	mux := upHandler().(*http.ServeMux)
	mux.HandleFunc("/api/v1/handshake", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"proof":"` + workspace.HandshakeProof(token, r.URL.Query().Get("nonce")) + `"}`))
	})
	mux.HandleFunc("/api/v1/system/launch", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"code":"` + code + `","url":"http://127.0.0.1:1/auth/launch?code=` + code + `"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	f := newFixture(t, freePort(t), map[string]string{envNoBrowser: "1"})
	ws := workspace.Dir(f.home)
	var err error
	if token, err = ws.EnsureToken(); err != nil {
		t.Fatal(err)
	}
	lock, err := ws.TryLock() // a real owner holds the lock while it publishes runtime.json
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Release() })
	if err := ws.WriteRuntime(workspace.Runtime{PID: os.Getpid(), Addr: addr, Mode: workspace.ModeLocal}); err != nil {
		t.Fatal(err)
	}
	f.o.startSrv = func(string, string, string, []string, string) (<-chan error, error) {
		f.started++
		return nil, errors.New("must not start a second server")
	}

	if err := f.o.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := srv.URL + "/auth/launch?code=" + code
	if f.started != 0 || strings.TrimSpace(f.out.String()) != want {
		t.Errorf("started %d, printed %q, want %q", f.started, f.out.String(), want)
	}
}

func TestOpenStartsTheServerWhenNoneRuns(t *testing.T) {
	port := freePort(t)
	f := newFixture(t, port, nil)
	var srv *httptest.Server
	t.Cleanup(func() {
		if srv != nil {
			srv.Close()
		}
	})
	f.o.startSrv = func(self, host, p string, env []string, log string) (<-chan error, error) {
		f.started++
		f.env = env
		if self != "/path/to/stuarty" || host != "127.0.0.1" || p != port {
			t.Errorf("start(%q, %q, %q)", self, host, p)
		}
		if filepath.Base(log) != "server.log" || filepath.Base(filepath.Dir(log)) != "logs" {
			t.Errorf("log %s", log)
		}
		l, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err != nil {
			return nil, err
		}
		srv = httptest.NewUnstartedServer(upHandler())
		srv.Listener = l
		go func() { time.Sleep(400 * time.Millisecond); srv.Start() }()
		return make(chan error), nil
	}
	if err := f.o.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.started != 1 || len(f.browsed) != 1 {
		t.Fatalf("started %d browsed %v", f.started, f.browsed)
	}

	// The child gets the address and a key from the workspace, and the key file is private.
	joined := "\n" + strings.Join(f.env, "\n") + "\n"
	if !strings.Contains(joined, "\nPORT="+port+"\n") || !strings.Contains(joined, "\nHOST=127.0.0.1\n") ||
		!strings.Contains(joined, "\nAPP_KEY=base64:") {
		t.Errorf("child environment lacks the address or the key")
	}
	keyFile := filepath.Join(f.home, "app.key")
	b, err := os.ReadFile(keyFile)
	if err != nil || !strings.HasPrefix(string(b), "base64:") {
		t.Fatalf("key file: %v %q", err, b)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(keyFile); st.Mode().Perm() != 0o600 {
			t.Errorf("key file mode %v", st.Mode().Perm())
		}
	}
	for _, d := range []string{"logs", "store", "artifacts"} {
		if _, err := os.Stat(filepath.Join(f.home, d)); err != nil {
			t.Errorf("workspace folder %s: %v", d, err)
		}
	}
	// The same key is used the next time.
	k2, err := ensureAppKey(workspace.Dir(f.home))
	if err != nil || k2+"\n" != string(b) {
		t.Errorf("the key changed: %q vs %q (%v)", k2, b, err)
	}
}

func TestOpenKeepsAnExistingKeyOrEnvironment(t *testing.T) {
	port := freePort(t)
	f := newFixture(t, port, map[string]string{"APP_KEY": "base64:given"})
	f.o.startSrv = func(_, _, _ string, env []string, _ string) (<-chan error, error) {
		f.env = env
		return nil, errors.New("stop here")
	}
	_ = f.o.run(context.Background())
	if strings.Contains(strings.Join(f.env, "\n"), "\nAPP_KEY=base64:") &&
		!strings.Contains(strings.Join(f.env, "\n"), "given") {
		t.Error("a key was made although APP_KEY is set")
	}
	if _, err := os.Stat(filepath.Join(f.home, "app.key")); err == nil {
		t.Error("a key file was written although APP_KEY is set")
	}
}

func TestOpenReportsAServerThatDiesOrNeverAnswers(t *testing.T) {
	port := freePort(t)
	f := newFixture(t, port, nil)
	f.o.startSrv = func(string, string, string, []string, string) (<-chan error, error) {
		ch := make(chan error, 1)
		ch <- errors.New("exit status 1")
		return ch, nil
	}
	err := f.o.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "stopped right after it started") ||
		!strings.Contains(err.Error(), "server.log") ||
		len(f.browsed) != 0 {
		t.Errorf("early exit: %v browsed %v", err, f.browsed)
	}

	f = newFixture(t, port, nil)
	f.o.timeout = 400 * time.Millisecond
	f.o.startSrv = func(string, string, string, []string, string) (<-chan error, error) { return make(chan error), nil }
	if err := f.o.run(context.Background()); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Errorf("timeout: %v", err)
	}

	f = newFixture(t, port, nil)
	f.o.startSrv = func(string, string, string, []string, string) (<-chan error, error) {
		return nil, errors.New("no such file")
	}
	if err := f.o.run(context.Background()); err == nil || !strings.Contains(err.Error(), "could not be started") {
		t.Errorf("spawn failure: %v", err)
	}
}

func TestOpenNoBrowserPrintsTheAddress(t *testing.T) {
	srv := httptest.NewServer(upHandler())
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	f := newFixture(t, port, map[string]string{envNoBrowser: "1"})
	if err := f.o.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.browsed) != 0 || strings.TrimSpace(f.out.String()) != "http://127.0.0.1:"+port+"/" {
		t.Errorf("browsed %v out %q", f.browsed, f.out.String())
	}

	// A browser that cannot start is an error, with the address printed so the user can open it.
	f = newFixture(t, port, nil)
	f.o.browse = func(string) error { return errors.New("xdg-open was not found") }
	err := f.o.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "browser could not be started") ||
		!strings.Contains(f.out.String(), "http://127.0.0.1:"+port+"/") {
		t.Errorf("%v / %q", err, f.out.String())
	}
}

func TestResolveAddr(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for _, c := range []struct {
		env        map[string]string
		host, port string
		bad        bool
	}{
		{map[string]string{}, "127.0.0.1", "7878", false},
		{map[string]string{"PORT": "9000"}, "127.0.0.1", "9000", false},
		{map[string]string{"STUARTY_ADDR": "localhost:8080"}, "localhost", "8080", false},
		{map[string]string{"STUARTY_ADDR": "[::1]:8080"}, "::1", "8080", false},
		{map[string]string{"STUARTY_ADDR": "0.0.0.0:7878"}, "", "", true},
		{map[string]string{"HOST": "192.168.1.5"}, "", "", true},
		{map[string]string{"STUARTY_ADDR": "no-port"}, "", "", true},
		{map[string]string{"PORT": "80/../x"}, "", "", true},
	} {
		h, p, err := resolveAddr(env(c.env))
		if c.bad != (err != nil) || h != c.host || p != c.port {
			t.Errorf("%v: %q %q %v", c.env, h, p, err)
		}
	}
	if baseURL("::1", "7878") != "http://[::1]:7878" {
		t.Errorf("baseURL = %s", baseURL("::1", "7878"))
	}
}

func TestRunOpenRefusesArguments(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runOpen([]string{"--x"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "usage") {
		t.Errorf("code %d %q", code, errOut.String())
	}
}

func TestOpenBrowserRefusesWhatIsNotAnHTTPAddress(t *testing.T) {
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "http://", "", "ftp://x"} {
		if err := openBrowser(u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

// holdOwner makes this test the owner of a workspace: it holds the lock, publishes runtime.json for
// an /up whose answers wait until gate is closed. When openAfter is above zero the owner closes the
// gate itself on that many probes: it is "slow" by a count, not by a clock.
func holdOwner(t *testing.T, f *fixture, gate chan struct{}, openAfter int32) {
	t.Helper()
	hits := &atomic.Int32{}
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/up", func(w http.ResponseWriter, r *http.Request) {
		if openAfter > 0 && hits.Add(1) >= openAfter {
			once.Do(func() { close(gate) })
		}
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ws := workspace.Dir(f.home)
	lock, err := ws.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Release() })
	rt := workspace.Runtime{PID: os.Getpid(), Addr: strings.TrimPrefix(srv.URL, "http://"), Mode: workspace.ModeLocal}
	if err := ws.WriteRuntime(rt); err != nil {
		t.Fatal(err)
	}
	f.o.startSrv = func(string, string, string, []string, string) (<-chan error, error) {
		f.started++
		return nil, errors.New("must not start a second server")
	}
	f.o.client = &http.Client{Timeout: 50 * time.Millisecond} //nolint:forbidigo // loopback test server
	f.o.ownerWait = 5 * time.Second
	f.o.poll = 10 * time.Millisecond
}

// VV-258: an owner that holds the lock and answers /up late is waited for, never replaced.
func TestOpenWaitsForASlowOwnerThatHoldsTheLock(t *testing.T) {
	f := newFixture(t, freePort(t), map[string]string{envNoBrowser: "1"})
	gate := make(chan struct{})
	holdOwner(t, f, gate, 3)
	if err := f.o.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.started != 0 || !strings.HasPrefix(strings.TrimSpace(f.out.String()), "http://127.0.0.1:") {
		t.Errorf("started %d, printed %q", f.started, f.out.String())
	}
}

// VV-258: an owner that never answers is reported with E_WORKSPACE_LOCKED, never as a server that
// stopped right after it started.
func TestOpenReportsAnOwnerThatNeverAnswers(t *testing.T) {
	f := newFixture(t, freePort(t), map[string]string{envNoBrowser: "1"})
	gate := make(chan struct{})
	t.Cleanup(func() { close(gate) })
	holdOwner(t, f, gate, 0)
	f.o.ownerWait = 300 * time.Millisecond
	err := f.o.run(context.Background())
	var werr *workspace.Error
	if !errors.As(err, &werr) || werr.Code != workspace.CodeWorkspaceLocked || !werr.Retryable {
		t.Fatalf("error %v, want a retryable E_WORKSPACE_LOCKED", err)
	}
	if !strings.Contains(err.Error(), "another Stuarty is running and not answering") ||
		strings.Contains(err.Error(), "stopped right after") {
		t.Errorf("message %q", err)
	}
	if f.started != 0 {
		t.Errorf("started %d servers beside the owner", f.started)
	}
	if got := runExitCode(err); got != 2 {
		t.Errorf("exit code %d, want 2 (retryable)", got)
	}
}
