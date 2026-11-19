package servicetest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.String()
}

// SDD-007 T-42, SDD-014 section 9: a credential of a connection and the master key are in no log
// line, no answer, no error and no file of the diagnostic bundle, even when a destination echoes the
// credential back, a destination cannot be reached, a careless line was written to the log file, and
// the master key is also in the environment.
func TestSecurity_T42_SecretNeverInLogsOrBundle(t *testing.T) {
	const secret = "sk-live-CANARY-T42-9e8d7c6b5a49"

	master := bytes.Repeat([]byte{0x7E}, vault.KeySize)

	forbidden := []string{
		secret,
		base64.StdEncoding.EncodeToString([]byte(secret)),
		hex.EncodeToString([]byte(secret)),
		base64.StdEncoding.EncodeToString(master),
		base64.RawURLEncoding.EncodeToString(master),
		hex.EncodeToString(master),
	}

	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "refused "+r.Header.Get("Authorization"), http.StatusUnauthorized)
	}))

	var logs syncBuffer

	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Version = "0.0.0-test"
		o.Log = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		o.Connections = connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Now: e.clock.now,
			Vault: vault.Open(filepath.Join(string(e.dir), connections.VaultFile), vault.NewMemoryProvider(master)),
		})
		o.Doctor = service.DoctorOptions{
			Mode: service.ModeLocal,
			Environ: func() []string {
				return []string{
					vault.EnvMasterKey + "=" + base64.StdEncoding.EncodeToString(master),
					"STUARTY_HOME=" + string(e.dir),
				}
			},
		}
	})

	ctx := service.WithCaller(context.Background(), operator())

	var seen strings.Builder

	record := func(v any, err error) {
		if raw, merr := json.Marshal(v); merr == nil {
			seen.Write(raw)
		}

		if err != nil {
			seen.WriteString(err.Error())
		}
	}

	good, err := e.svc.ConnectionCreate(ctx, connectionRequest(echo.URL, secret))
	record(good, err)

	if err != nil {
		t.Fatal(err)
	}

	// The destination echoes the credential; then it goes away and the next test fails to connect.
	tested, err := e.svc.ConnectionTest(ctx, service.ConnectionRef{ID: good.ID})
	record(tested, err)

	if err != nil || tested.Status != http.StatusUnauthorized || !strings.Contains(tested.Message, "[redacted]") {
		t.Fatalf("the test of an echoing destination: %+v, %v", tested, err)
	}

	echo.Close()

	tested, err = e.svc.ConnectionTest(ctx, service.ConnectionRef{ID: good.ID})
	record(tested, err)

	if err != nil || tested.OK {
		t.Fatalf("the test of a closed destination: %+v, %v", tested, err)
	}

	list, err := e.svc.ConnectionList(ctx, service.ConnectionListRequest{})
	record(list, err)

	// A careless line of an older build, or of a connector of a later epic, is in the log file.
	dir := filepath.Join(string(e.dir), "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	line := fmt.Sprintf(
		"level=ERROR msg=\"call failed\" authorization=%q key=%s\n",
		"Bearer "+secret,
		base64.StdEncoding.EncodeToString(master),
	)
	if err := os.WriteFile(filepath.Join(dir, "stuarty.log"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.Doctor(ctx, service.DoctorRequest{Full: true, Bundle: true})
	record(res, err)

	if err != nil {
		t.Fatal(err)
	}

	files := unzipAll(t, filepath.Join(string(e.dir), filepath.FromSlash(res.Bundle)))

	if !strings.Contains(files["connections.txt"], good.ID) ||
		!strings.Contains(files["logs/stuarty.log"], "[redacted]") {
		t.Errorf("the bundle does not list the connection or did not redact the log: %v", keys(files))
	}

	for name, body := range files {
		seen.WriteString(body)

		for _, f := range forbidden {
			if strings.Contains(body, f) {
				t.Errorf("%s of the bundle holds a credential", name)
			}
		}
	}

	if _, err := e.svc.ConnectionRemove(ctx, service.ConnectionRef{ID: good.ID}); err != nil {
		t.Fatal(err)
	}

	seen.WriteString(logs.String())

	for _, f := range forbidden {
		if strings.Contains(seen.String(), f) {
			t.Errorf("a log line, an answer or an error holds %q", f)
		}
	}

	if logs.String() == "" {
		t.Log("the service logged nothing at all for these operations")
	}
}
