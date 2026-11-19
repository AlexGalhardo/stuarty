package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
	err := d.WriteRuntime(
		Runtime{
			PID:       os.Getpid(),
			Addr:      srv.Listener.Addr().String(),
			Mode:      mode,
			StartedAt: time.Now(),
			Version:   "0.0.0",
		},
	)
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
	if err := d.WriteRuntime(
		Runtime{PID: deadPID(t), Addr: "127.0.0.1:1", Mode: ModeLocal, StartedAt: time.Now(), Version: "0"},
	); err != nil {
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
	secret, err := d.NewRuntimeSecret()
	if err != nil {
		t.Fatal(err)
	}
	srv := fakeOwner(t, secret)
	advertise(t, d, srv, ModeNetwork)

	_, err = Open(context.Background(), fast(home))
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
		t.Error("the workspace must not be touched when STUARTY_URL is set")
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

	// The owner finishes starting on the third poll: the command has waited
	// twice by then, whatever the speed of the machine.
	polls := 0
	o := fast(home)
	o.OnPoll = func() {
		polls++
		if polls == 3 {
			advertise(t, d, srv, ModeLocal)
		}
	}

	s, err := Open(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != ModeHTTP || polls < 3 {
		t.Errorf("session = %+v after %d polls", s, polls)
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

func TestOpenRemoteURLRefusesWhatIsNotHTTP(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"ftp://videos.example.org", "videos.example.org", "http://"} {
		o := Options{Getenv: openEnv(t.TempDir(), map[string]string{EnvURL: raw, EnvKey: "k"})}
		if _, err := Open(context.Background(), o); err == nil {
			t.Errorf("Open(%q) = nil, want a refusal", raw)
		}
	}
}

// SDD-007 section 4.3: the key never crosses the network in clear.
func TestOpenRemoteURLRefusesPlainHTTPBeyondLoopback(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"http://videos.example.org", "http://192.168.1.5:7878"} {
		o := Options{Getenv: openEnv(t.TempDir(), map[string]string{EnvURL: raw, EnvKey: "k"})}
		if _, err := Open(context.Background(), o); err == nil || !strings.Contains(err.Error(), EnvURL) {
			t.Errorf("Open(%q) = %v, want a refusal naming %s", raw, err, EnvURL)
		}
	}

	for _, raw := range []string{"http://localhost:7878", "http://127.0.0.1:7878", "https://videos.example.org"} {
		o := Options{Getenv: openEnv(t.TempDir(), map[string]string{EnvURL: raw, EnvKey: "k"})}
		if s, err := Open(context.Background(), o); err != nil || s.Token != "k" {
			t.Errorf("Open(%q) = %+v, %v", raw, s, err)
		}
	}
}

func TestOpenRemoteURLReadsTheKeyFile(t *testing.T) {
	t.Parallel()

	keyFile := t.TempDir() + "/key"
	if err := os.WriteFile(keyFile, []byte("  key-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	o := Options{
		Getenv:  openEnv(t.TempDir(), map[string]string{EnvURL: "https://videos.example.org"}),
		KeyFile: keyFile,
	}

	s, err := Open(context.Background(), o)
	if err != nil || s.Token != "key-from-file" {
		t.Fatalf("session = %+v, %v", s, err)
	}

	o.KeyFile = keyFile + ".missing"
	if _, err := Open(context.Background(), o); err == nil {
		t.Error("a missing key file was accepted")
	}
}

func TestOpenNetworkModeReadsTheKeyFile(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	d := Dir(home)

	owner, _ := d.TryLock()
	defer owner.Release()

	secret, err := d.NewRuntimeSecret()
	if err != nil {
		t.Fatal(err)
	}

	advertise(t, d, fakeOwner(t, secret), ModeNetwork)

	keyFile := t.TempDir() + "/key"
	if err := os.WriteFile(keyFile, []byte("key-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	o := fast(home)
	o.KeyFile = keyFile

	s, err := Open(context.Background(), o)
	if err != nil || s.Token != "key-2" {
		t.Fatalf("session = %+v, %v", s, err)
	}

	o.KeyFile = keyFile + ".missing"
	if _, err := Open(context.Background(), o); err == nil {
		t.Error("a missing key file was accepted")
	}
}

// The token never goes to a local owner that advertises a non-loopback address
// (T-30, SDD-007 section 3.8): the client refuses before the handshake.
func TestSecurity_T30_RuntimeJSONNonLoopbackRefused(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{
		"192.0.2.10:7878", "[2001:db8::1]:7878", "[::ffff:192.0.2.10]:7878", "owner.example:7878", "0.0.0.0:7878",
	} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			d := Dir(home)

			if _, err := d.EnsureToken(); err != nil {
				t.Fatal(err)
			}

			owner, _ := d.TryLock()
			defer owner.Release()

			var (
				mu   sync.Mutex
				seen []string
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery+"|"+r.Header.Get("Authorization"))
				mu.Unlock()

				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
			t.Cleanup(srv.Close)

			err := d.WriteRuntime(Runtime{
				PID: os.Getpid(), Addr: addr, Mode: ModeLocal, StartedAt: time.Now(), Version: "0.0.0",
			})
			if err != nil {
				t.Fatal(err)
			}

			// Whatever address is dialled, the fake owner answers: only the refusal can stop the token.
			o := fast(home)
			o.Client = &http.Client{Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
				},
			}}

			var we *Error
			if _, err := Open(context.Background(), o); !errors.As(err, &we) || we.Code != CodeUnauthorized {
				t.Fatalf("err = %v, want %s", err, CodeUnauthorized)
			}

			mu.Lock()
			defer mu.Unlock()

			for _, s := range seen {
				if strings.Contains(s, "Bearer") || strings.Contains(s, handshakePath) {
					t.Errorf("a request reached the non-loopback owner: %q", s)
				}
			}
		})
	}
}

func TestOpenStopsWhenTheContextIsCancelledWhileWaiting(t *testing.T) {
	t.Parallel()

	home := t.TempDir()

	holder, _ := Dir(home).TryLock()
	defer holder.Release()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	o := fast(home)
	o.Wait = 10 * time.Second

	if _, err := Open(ctx, o); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// A process that took the advertised address of a network-mode owner cannot read the per-run
// secret, so it never receives the API key (SDD-007 section 3.8).
func TestSecurity_T30_NetworkOwnerMustProveItselfBeforeTheKey(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	d := Dir(home)

	owner, _ := d.TryLock()
	defer owner.Release()

	if _, err := d.NewRuntimeSecret(); err != nil {
		t.Fatal(err)
	}

	var (
		mu   sync.Mutex
		seen []string
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/up", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.HandleFunc(handshakePath, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()

		_ = json.NewEncoder(w).
			Encode(map[string]string{"proof": HandshakeProof("not-the-secret", r.URL.Query().Get("nonce"))})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	advertise(t, d, srv, ModeNetwork)

	o := fast(home)
	o.Getenv = openEnv(home, map[string]string{EnvKey: "key-1"})

	_, err := Open(context.Background(), o)

	var we *Error
	if !errors.As(err, &we) || we.Code != CodeUnauthorized {
		t.Fatalf("err = %v, want %s", err, CodeUnauthorized)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, h := range seen {
		if h != "" {
			t.Errorf("the key was sent to an owner that failed the proof: %q", h)
		}
	}
}

func TestRuntimeSecretIsPrivatePerRunAndRemovedWithRuntime(t *testing.T) {
	t.Parallel()

	d := Dir(t.TempDir())

	first, err := d.NewRuntimeSecret()
	if err != nil {
		t.Fatal(err)
	}

	second, err := d.NewRuntimeSecret()
	if err != nil || first == second || len(first) != 64 {
		t.Fatalf("secrets %q and %q (%v), want two different 64 character values", first, second, err)
	}

	if got, err := d.LoadRuntimeSecret(); err != nil || got != second {
		t.Fatalf("LoadRuntimeSecret = %q, %v", got, err)
	}

	if err := d.RemoveRuntime(); err != nil {
		t.Fatal(err)
	}

	if _, err := d.LoadRuntimeSecret(); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after RemoveRuntime: %v, want not exist", err)
	}
}

// VV-252: an owner that holds the lock but does not answer in time (slow, not
// absent) is never replaced by in-process access. Every probe is held until
// the client gives up on it, so each one misses by construction; Open waits
// out its limit and reports E_WORKSPACE_LOCKED.
func TestOpenSlowOwnerHoldingTheLockIsNeverReplacedByInProcess(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	d := Dir(home)
	holder, err := d.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	var probes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	advertise(t, d, srv, ModeLocal)

	o := fast(home)
	o.Wait = 300 * time.Millisecond
	o.Client = &http.Client{Timeout: 20 * time.Millisecond}
	s, err := Open(context.Background(), o)
	if s != nil {
		t.Fatalf("session = %+v, want none: a slow owner must not be replaced", s)
	}
	lockedCode(t, err)
	if probes.Load() == 0 {
		t.Error("the owner was never probed")
	}
}
