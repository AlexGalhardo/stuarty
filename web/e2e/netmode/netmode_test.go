// Package netmode runs the real binary in network mode and walks the scenario
// of the audit of group G2.13 (ROADMAP P2-SEC-002, SDD-007 section 15 checks 8,
// 11 and 12): the first administrator comes from the environment, logs in,
// creates a key, and the key is limited and revocable.
package netmode_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// lockedBuffer is the output of the running server: exec copies into it from
// its own goroutine while the test reads it.
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

func randomHex(t *testing.T, n int) string {
	t.Helper()

	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}

	return hex.EncodeToString(b)
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

func buildBinary(t *testing.T) string {
	t.Helper()

	name := "vukem-video"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	bin := filepath.Join(t.TempDir(), name)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "github.com/AlexGalhardo/vukem-video/web/cmd/app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	return bin
}

type server struct {
	base string
	host string
	out  *lockedBuffer
}

// startServer runs `serve` in network mode as a real process, with the first
// administrator in its environment, and waits until runtime.json names it.
func startServer(t *testing.T, bin, email, password, appKey string) *server {
	t.Helper()

	home := filepath.Join(t.TempDir(), "ws")
	port := freePort(t)

	cmd := exec.CommandContext(t.Context(), bin, "serve")
	cmd.Env = append(os.Environ(),
		"VUKEM_VIDEO_HOME="+home, "VUKEM_VIDEO_URL=", "APP_ENV=", "VUKEM_VIDEO_MIN_FREE_BYTES=1",
		"VUKEM_VIDEO_MODE=network", "VUKEM_VIDEO_ADDR=127.0.0.1:"+port, "APP_URL=http://localhost:"+port,
		"APP_KEY=base64:"+base64.StdEncoding.EncodeToString([]byte(appKey)),
		"VUKEM_VIDEO_INSECURE_ALLOW_ROOT=true",
		"VUKEM_VIDEO_ADMIN_EMAIL="+email, "VUKEM_VIDEO_ADMIN_PASSWORD="+password,
	)

	out := &lockedBuffer{}
	cmd.Stdout, cmd.Stderr = out, out

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	go func() { _ = cmd.Wait(); close(done) }()

	t.Cleanup(func() {
		_ = cmd.Process.Kill() // a no-op once it exited
		<-done
	})

	dir := workspace.Dir(home)
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		if rt, err := dir.ReadRuntime(); err == nil && rt.PID == cmd.Process.Pid {
			return &server{base: "http://127.0.0.1:" + port, host: "localhost:" + port, out: out}
		}

		select {
		case <-done:
			t.Fatalf("serve exited\n%s", out)
		case <-time.After(100 * time.Millisecond):
		}
	}

	t.Fatalf("serve did not announce itself\n%s", out)

	return nil
}

type reply struct {
	status  int
	body    []byte
	cookies []*http.Cookie
}

func (r reply) field(name string) string {
	var m map[string]any
	_ = json.Unmarshal(r.body, &m)
	v, _ := m[name].(string)

	return v
}

func (s *server) send(t *testing.T, method, path, body string, cookie *http.Cookie, bearer string) reply {
	t.Helper()

	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, s.base+path, rd)
	if err != nil {
		t.Fatal(err)
	}

	req.Host = s.host
	req.Header.Set("Origin", "http://"+s.host)

	if body != "" || method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}

	if cookie != nil {
		req.AddCookie(cookie)
	}

	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	client := outbound.LocalServer(30 * time.Second)

	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	raw, _ := io.ReadAll(res.Body)

	return reply{status: res.StatusCode, body: raw, cookies: res.Cookies()}
}

func TestNetworkModeScenario(t *testing.T) {
	bin := buildBinary(t)
	email := "admin-" + randomHex(t, 4) + "@example.com"
	password := "pw-" + randomHex(t, 12)
	s := startServer(t, bin, email, password, strings.Repeat("k", 32))

	creds := func(pw string) string {
		b, err := json.Marshal(map[string]string{"email": email, "password": pw})
		if err != nil {
			t.Fatal(err)
		}

		return string(b)
	}

	if res := s.send(
		t,
		http.MethodPost,
		"/auth/login",
		creds(password+"x"),
		nil,
		"",
	); res.status != http.StatusUnauthorized {
		t.Fatalf("login with a wrong password = %d %s, want 401", res.status, res.body)
	}

	login := s.send(t, http.MethodPost, "/auth/login", creds(password), nil, "")
	if login.status != http.StatusOK || len(login.cookies) == 0 {
		t.Fatalf("login = %d %s", login.status, login.body)
	}

	session := login.cookies[0]

	created := s.send(t, http.MethodPost, "/api/v1/keys", `{"name":"reader","scopes":["read"]}`, session, "")
	if created.status != http.StatusCreated {
		t.Fatalf("key create = %d %s", created.status, created.body)
	}

	var body struct {
		Key struct {
			KeyID string `json:"key_id"`
		} `json:"key"`
		Secret string `json:"secret"`
	}

	if err := json.Unmarshal(created.body, &body); err != nil || body.Secret == "" || body.Key.KeyID == "" {
		t.Fatalf("key body = %s (%v)", created.body, err)
	}

	if res := s.send(t, http.MethodGet, "/api/v1/projects", "", nil, body.Secret); res.status != http.StatusOK {
		t.Errorf("GET /projects with the read key = %d %s, want 200", res.status, res.body)
	}

	if res := s.send(
		t,
		http.MethodPost,
		"/api/v1/projects",
		`{"name":"x"}`,
		nil,
		body.Secret,
	); res.status != http.StatusForbidden ||
		res.field("code") != "E_FORBIDDEN" {
		t.Errorf("POST /projects with the read key = %d %s, want 403 E_FORBIDDEN", res.status, res.body)
	}

	if res := s.send(t, http.MethodDelete, "/api/v1/keys/"+body.Key.KeyID, "", session, ""); res.status/100 != 2 {
		t.Fatalf("revoke = %d %s", res.status, res.body)
	}

	if res := s.send(
		t,
		http.MethodGet,
		"/api/v1/projects",
		"",
		nil,
		body.Secret,
	); res.status != http.StatusUnauthorized ||
		res.field("code") != "E_UNAUTHORIZED" {
		t.Errorf("GET /projects with the revoked key = %d %s, want 401 E_UNAUTHORIZED", res.status, res.body)
	}

	// T-04 in the running process: no credential reached its output.
	logs := s.out.String()
	for name, secret := range map[string]string{"password": password, "key secret": body.Secret, "session id": session.Value} {
		if strings.Contains(logs, secret) {
			t.Errorf("the %s is in the output of the server", name)
		}
	}
}
