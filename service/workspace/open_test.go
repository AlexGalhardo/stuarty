package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func openEnv(home string, extra map[string]string) func(string) string {
	return func(k string) string {
		if v, ok := extra[k]; ok {
			return v
		}
		if k == EnvHome {
			return home
		}
		return ""
	}
}

// fakeOwner is an HTTP server that behaves like an owner process: /up and the
// handshake keyed by token.
func fakeOwner(t *testing.T, token string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/up", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc(handshakePath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"proof": HandshakeProof(token, r.URL.Query().Get("nonce"))})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func advertise(t *testing.T, d Dir, srv *httptest.Server, mode string) {
	t.Helper()
	err := d.WriteRuntime(Runtime{PID: os.Getpid(), Addr: srv.Listener.Addr().String(), Mode: mode, StartedAt: time.Now(), Version: "0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
}

func fast(home string) Options {
	return Options{Getenv: openEnv(home, nil), Wait: 2 * time.Second, Poll: 10 * time.Millisecond}
}

func lockedCode(t *testing.T, err error) {
	t.Helper()
	var we *Error
	if !errors.As(err, &we) || we.Code != CodeWorkspaceLocked || !we.Retryable {
		t.Fatalf("err = %v, want retryable %s", err, CodeWorkspaceLocked)
	}
}

func TestOpenNoOwnerRunsInProcessAndRemovesStaleRuntime(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	d := Dir(home)

	// A runtime.json of an owner that is gone.
	if err := d.WriteRuntime(Runtime{PID: deadPID(t), Addr: "127.0.0.1:1", Mode: ModeLocal, StartedAt: time.Now(), Version: "0"}); err != nil {
		t.Fatal(err)
	}

	s, err := Open(context.Background(), fast(home))
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != ModeInProcess || s.Lock == nil {
		t.Fatalf("session = %+v, want in-process with the lock", s)
	}
	if _, err := os.Stat(d.Path("runtime.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale runtime.json was not removed: %v", err)
	}
	if _, err := d.TryLock(); !errors.Is(err, ErrLocked) {
		t.Errorf("the session must hold the lock: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if l, err := d.TryLock(); err != nil {
		t.Errorf("lock not released by Close: %v", err)
	} else {
		_ = l.Release()
	}
}

func TestOpenUsesRunningOwner(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	d := Dir(home)
	tok, err := d.EnsureToken()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := d.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	srv := fakeOwner(t, tok)
	advertise(t, d, srv, ModeLocal)

	s, err := Open(context.Background(), fast(home))
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != ModeHTTP || s.BaseURL != "http://"+srv.Listener.Addr().String() || s.Token != tok {
		t.Errorf("session = %+v", s)
	}
}

func TestOpenRefusesImpostorWithoutSendingToken(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	d := Dir(home)
	if _, err := d.EnsureToken(); err != nil {
		t.Fatal(err)
	}
	owner, _ := d.TryLock()
	defer owner.Release()
	// An owner that does not know the token.
	srv := fakeOwner(t, "vvl_"+"0000000000000000000000000000000000000000000000000000000000000000")
	advertise(t, d, srv, ModeLocal)

	_, err := Open(context.Background(), fast(home))
	var we *Error
	if !errors.As(err, &we) || we.Code != CodeUnauthorized {
		t.Fatalf("err = %v, want %s", err, CodeUnauthorized)
	}
}

func TestOpenNetworkModeNeedsKey(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	d := Dir(home)
	owner, _ := d.TryLock()
	defer owner.Release()
	srv := fakeOwner(t, "")
	advertise(t, d, srv, ModeNetwork)

	_, err := Open(context.Background(), fast(home))
	var we *Error
	if !errors.As(err, &we) || we.Code != CodeUnauthorized {
		t.Fatalf("err = %v, want %s", err, CodeUnauthorized)
	}

	o := fast(home)
	o.Getenv = openEnv(home, map[string]string{EnvKey: "key-1"})
	s, err := Open(context.Background(), o)
	if err != nil || s.Mode != ModeHTTP || s.Token != "key-1" {
		t.Fatalf("session = %+v, %v", s, err)
	}
	if _, err := os.Stat(d.Path("token")); err == nil {
		t.Error("network mode must not create the local token")
	}
}

func TestOpenRemoteURLReadsNoLocalFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir() + "/never-created"
	o := Options{Getenv: openEnv(home, map[string]string{EnvURL: "https://videos.example.org/", EnvKey: "k"})}
	s, err := Open(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != ModeHTTP || s.BaseURL != "https://videos.example.org" || s.Token != "k" || s.Dir != "" {
		t.Errorf("session = %+v", s)
	}
	if _, err := os.Stat(home); err == nil {
		t.Error("the workspace must not be touched when VUKEM_VIDEO_URL is set")
	}

	o.Getenv = openEnv(home, map[string]string{EnvURL: "https://videos.example.org"})
	var we *Error
	if _, err := Open(context.Background(), o); !errors.As(err, &we) || we.Code != CodeUnauthorized {
		t.Errorf("missing key: err = %v, want %s", err, CodeUnauthorized)
	}
}

// SDD-005 section 18 check 23: a CLI started while the owner is still
// starting waits and then connects.
func TestOpenWaitsForOwnerStarting(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	d := Dir(home)
	tok, err := d.EnsureToken()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := d.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	srv := fakeOwner(t, tok)

	go func() {
		time.Sleep(300 * time.Millisecond) // migrating
		advertise(t, d, srv, ModeLocal)
	}()

	start := time.Now()
	s, err := Open(context.Background(), fast(home))
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != ModeHTTP || time.Since(start) < 250*time.Millisecond {
		t.Errorf("session = %+v after %v", s, time.Since(start))
	}
}

// Check 23: two commands started together with no owner: one runs in
// process, the other waits and then runs in its turn.
func TestOpenSecondCommandWaitsAndRunsInItsTurn(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	first, err := Open(context.Background(), fast(home))
	if err != nil || first.Mode != ModeInProcess {
		t.Fatalf("first = %+v, %v", first, err)
	}

	type result struct {
		s   *Session
		err error
	}
	done := make(chan result, 1)
	go func() {
		s, err := Open(context.Background(), fast(home))
		done <- result{s, err}
	}()

	select {
	case <-done:
		t.Fatal("the second command must wait while the first holds the lock")
	case <-time.After(200 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil || r.s.Mode != ModeInProcess {
		t.Fatalf("second = %+v, %v", r.s, r.err)
	}
	_ = r.s.Close()
}

// Check 23: a lock held by a process that never writes runtime.json gives
// E_WORKSPACE_LOCKED after the wait limit.
func TestOpenLockedWithoutRuntimeTimesOut(t *testing.T) {
	t.Parallel()
	if WaitLimit != 10*time.Second {
		t.Fatalf("WaitLimit = %v, want 10 s", WaitLimit)
	}
	home := t.TempDir()
	holder, err := Dir(home).TryLock()
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	o := fast(home)
	o.Wait = 400 * time.Millisecond
	start := time.Now()
	_, err = Open(context.Background(), o)
	lockedCode(t, err)
	if el := time.Since(start); el < 350*time.Millisecond || el > 3*time.Second {
		t.Errorf("gave up after %v, want about the configured wait", el)
	}
}

// The real 10 s limit, with the default options.
func TestOpenLockedWithoutRuntimeTenSeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("takes 10 s")
	}
	t.Parallel()
	home := t.TempDir()
	holder, err := Dir(home).TryLock()
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	start := time.Now()
	_, err = Open(context.Background(), Options{Getenv: openEnv(home, nil)})
	lockedCode(t, err)
	if el := time.Since(start); el < 9500*time.Millisecond || el > 12*time.Second {
		t.Errorf("gave up after %v, want 10 s", el)
	}
}

func TestOpenForce(t *testing.T) {
	t.Parallel()

	t.Run("InProcess fails at once when an owner exists", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		holder, _ := Dir(home).TryLock()
		defer holder.Release()
		o := fast(home)
		o.Force = InProcess
		start := time.Now()
		_, err := Open(context.Background(), o)
		lockedCode(t, err)
		if time.Since(start) > time.Second {
			t.Error("InProcess must not wait")
		}
	})

	t.Run("Server fails at once when nobody owns the workspace", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		o := fast(home)
		o.Force = Server
		start := time.Now()
		_, err := Open(context.Background(), o)
		lockedCode(t, err)
		if time.Since(start) > time.Second {
			t.Error("Server must not wait when no process holds the lock")
		}
		if l, err := Dir(home).TryLock(); err != nil {
			t.Errorf("Server must not keep the lock: %v", err)
		} else {
			_ = l.Release()
		}
	})

	t.Run("Server uses a running owner", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		d := Dir(home)
		tok, _ := d.EnsureToken()
		holder, _ := d.TryLock()
		defer holder.Release()
		advertise(t, d, fakeOwner(t, tok), ModeLocal)
		o := fast(home)
		o.Force = Server
		s, err := Open(context.Background(), o)
		if err != nil || s.Mode != ModeHTTP {
			t.Fatalf("session = %+v, %v", s, err)
		}
	})
}

func TestBaseURLReplacesUnspecifiedHost(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"0.0.0.0:7878":   "http://127.0.0.1:7878",
		"[::]:7878":      "http://[::1]:7878",
		"127.0.0.1:7878": "http://127.0.0.1:7878",
		"[::1]:9":        "http://[::1]:9",
	} {
		if got, err := baseURL(in); err != nil || got != want {
			t.Errorf("baseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := baseURL("nonsense"); err == nil {
		t.Error("an address without a port must fail")
	}
}

func TestIsLoopback(t *testing.T) {
	t.Parallel()
	for addr, want := range map[string]bool{
		"127.0.0.1:1": true, "[::1]:1": true, "[::ffff:127.0.0.1]:1": true,
		"192.168.0.2:1": false, "0.0.0.0:1": false, "localhost:1": false, "bad": false,
	} {
		if got := isLoopback(addr); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestHandshakeProofIsKeyed(t *testing.T) {
	t.Parallel()
	a := HandshakeProof("t1", "n")
	if a == HandshakeProof("t2", "n") || a == HandshakeProof("t1", "m") || len(a) != 64 {
		t.Errorf("proof %q does not depend on key and nonce", a)
	}
}
