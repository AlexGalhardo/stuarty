package servicetest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
)

// toolsServer serves the two files of the test manifest over TLS and counts
// the requests it gets.
type toolsServer struct {
	srv      *httptest.Server
	requests atomic.Int64
	helper   []byte
	model    []byte
	// serve replaces the model body, for the refusal tests.
	serveModel atomic.Pointer[[]byte]
}

func newToolsServer(t *testing.T) *toolsServer {
	t.Helper()

	ts := &toolsServer{helper: []byte("#!helper\n"), model: []byte("model-bytes-0123456789")}

	ts.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.requests.Add(1)

		switch r.URL.Path {
		case "/whisper-cli":
			_, _ = w.Write(ts.helper)
		case "/model.bin":
			body := ts.model
			if alt := ts.serveModel.Load(); alt != nil {
				body = *alt
			}

			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(ts.srv.Close)

	return ts
}

func sum(b []byte) string {
	h := sha256.Sum256(b)

	return hex.EncodeToString(h[:])
}

func (ts *toolsServer) tools(dir string) *service.Tools {
	host := ts.srv.Listener.Addr().String()
	hostname := host[:len(host)-len(host[lastColon(host):])]

	return &service.Tools{
		Dir: dir, Client: ts.srv.Client(), GOOS: "testos", GOARCH: "testarch",
		Manifest: transcribe.Manifest{
			Hosts: []string{hostname},
			Entries: []transcribe.ManifestEntry{
				{
					Name:    "whisper",
					Kind:    transcribe.KindHelper,
					Version: "1.0",
					OS:      "testos",
					Arch:    "testarch",
					URL:     ts.srv.URL + "/whisper-cli",
					Bytes:   int64(len(ts.helper)),
					SHA256:  sum(ts.helper),
					File:    "whisper-cli",
				},
				{
					Name:    "whisper",
					Kind:    transcribe.KindModel,
					Model:   "tiny",
					Version: "1",
					URL:     ts.srv.URL + "/model.bin",
					Bytes:   int64(len(ts.model)),
					SHA256:  sum(ts.model),
					File:    "model.bin",
				},
			},
		},
	}
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}

	return len(s)
}

func (m *mediaEnv) waitInstall(ctx context.Context) *service.ToolsStatusResult {
	m.t.Helper()

	var st *service.ToolsStatusResult

	eventually(m.t, "the install to finish", func() bool {
		var err error
		if st, err = m.svc.ToolsStatus(ctx, service.ToolsStatusRequest{}); err != nil {
			m.t.Fatal(err)
		}

		return st.State != "running"
	})

	return st
}

// SDD-007 section 15 check 26 (T-26): only a person starts the download; the
// local token, a key without admin and every other caller are refused before
// a single request is made.
func TestSecurity_T26_DownloadNeverTriggeredByAgentCall(t *testing.T) {
	ts := newToolsServer(t)
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(t.TempDir())})
	req := service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}

	for name, c := range map[string]service.Caller{
		"the local token": {Actor: "local:cli", Scopes: service.AllScopes},
		"a key without admin": {
			Actor: "key:k", Scopes: []string{service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender},
		},
		"a read key": {Actor: "key:r", Scopes: []string{service.ScopeRead}},
	} {
		if _, err := m.svc.ToolsInstall(
			service.WithCaller(context.Background(), c),
			req,
		); code(
			err,
		) != service.CodeForbidden {
			t.Errorf("%s = %v, want E_FORBIDDEN", name, err)
		}
	}

	// A transcribe job with the helper missing downloads nothing either.
	if ts.requests.Load() != 0 {
		t.Errorf("a refused install made %d requests", ts.requests.Load())
	}
}

func TestToolsInstallVerifiesBeforeItKeepsAnything(t *testing.T) {
	ts := newToolsServer(t)
	dir := filepath.Join(t.TempDir(), "tools")
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(dir)})
	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	res, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny"})
	if err != nil || res.State != "running" || len(res.Files) != 2 ||
		res.DownloadBytes != int64(len(ts.helper)+len(ts.model)) {
		t.Fatalf("install = %+v, %v", res, err)
	}

	st := m.waitInstall(person)
	if st.State != "idle" || len(st.Installed) != 2 || st.Done != st.Total {
		t.Fatalf("status = %+v", st)
	}

	helper, ok := service.VerifiedTool(dir, "whisper", transcribe.KindHelper, "")
	if !ok {
		t.Fatal("the installed helper does not verify")
	}

	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(helper); info.Mode().Perm() != 0o700 {
			t.Errorf("helper mode = %v, want 0700", info.Mode().Perm())
		}
	}

	// A file that changed after the install is not used.
	if err := os.WriteFile(helper, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, ok := service.VerifiedTool(dir, "whisper", transcribe.KindHelper, ""); ok {
		t.Error("a tampered helper still verifies")
	}

	// The part files are gone.
	if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
		t.Errorf("leftovers in .download: %v", entries)
	}
}

func TestToolsInstallDeletesAFileThatIsNotTheOnePinned(t *testing.T) {
	ts := newToolsServer(t)
	dir := filepath.Join(t.TempDir(), "tools")
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(dir)})
	operator := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:cli", Scopes: service.AllScopes, Operator: true,
	})

	// Same length, different bytes; then a longer body than the manifest says.
	for name, body := range map[string][]byte{
		"another file": []byte("xxxxxxxxxxxxxxxxxxxxxx"),
		"a longer one": append(append([]byte{}, ts.model...), []byte("more")...),
	} {
		b := body
		ts.serveModel.Store(&b)

		if _, err := m.svc.ToolsInstall(
			operator,
			service.ToolsInstallRequest{Name: "whisper", Model: "tiny"},
		); err != nil {
			t.Fatal(err)
		}

		st := m.waitInstall(operator)
		if st.State != "failed" || st.Error == nil || st.Error.Details["reason"] != "checksum_mismatch" {
			t.Errorf("%s: status = %+v", name, st)
		}

		if _, ok := service.VerifiedTool(dir, "whisper", transcribe.KindModel, "tiny"); ok {
			t.Errorf("%s: the model was installed", name)
		}

		if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
			t.Errorf("%s: leftovers in .download: %v", name, entries)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestToolsInstallRefusesWhatTheManifestDoesNotPin(t *testing.T) {
	ts := newToolsServer(t)
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(t.TempDir())})
	person := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)

	for name, req := range map[string]service.ToolsInstallRequest{
		"another tool":  {Name: "evil", Model: "tiny"},
		"another model": {Name: "whisper", Model: "huge"},
	} {
		if _, err := m.svc.ToolsInstall(person, req); code(err) != "E_VALIDATION" {
			t.Errorf("%s = %v, want E_VALIDATION", name, err)
		}
	}

	if ts.requests.Load() != 0 {
		t.Error("a refused install made a request")
	}
}
