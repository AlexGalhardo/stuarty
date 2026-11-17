package servicetest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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
	archive  []byte
	// serve replaces the model body, for the refusal tests.
	serveModel atomic.Pointer[[]byte]
}

func newToolsServer(t *testing.T) *toolsServer {
	t.Helper()

	ts := &toolsServer{helper: []byte("#!helper\n"), model: []byte("model-bytes-0123456789")}
	ts.archive = zipOf(t, map[string][]byte{"bin/whisper-cli": ts.helper, "bin/unlisted": []byte("never written")})

	ts.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.requests.Add(1)

		switch r.URL.Path {
		case "/whisper-cli":
			_, _ = w.Write(ts.helper)
		case "/whisper.zip":
			_, _ = w.Write(ts.archive)
		case "/redirect":
			http.Redirect(w, r, "https://elsewhere.invalid/model.bin", http.StatusFound)
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

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
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

// VV-080: listing what is installed is a read; an agent reads it before it asks for a transcript.
func TestToolsStatusIsReadableByTheLocalTokenAndAReadKey(t *testing.T) {
	ts := newToolsServer(t)
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(t.TempDir())})

	for name, c := range map[string]service.Caller{
		"the local token": {Actor: "local:cli", Scopes: service.AllScopes},
		"a read key":      {Actor: "key:r", Scopes: []string{service.ScopeRead}},
	} {
		st, err := m.svc.ToolsStatus(service.WithCaller(context.Background(), c), service.ToolsStatusRequest{})
		if err != nil || st.State != "idle" {
			t.Errorf("%s: %+v, %v", name, st, err)
		}
	}

	_, err := m.svc.ToolsStatus(context.Background(), service.ToolsStatusRequest{})
	if code(err) != service.CodeUnauthorized {
		t.Errorf("no caller = %v, want E_UNAUTHORIZED", err)
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

// VV-077: the person at the terminal installs while a server owns the workspace, from a process with no
// store, into the same tools directory; the same installer still refuses the local token.
func TestToolsInstallerOfTheTerminalInstallsWithoutAStore(t *testing.T) {
	ts := newToolsServer(t)
	dir := filepath.Join(t.TempDir(), "tools")
	inst := service.NewToolsInstaller(ts.tools(dir))
	req := service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}

	if _, err := inst.ToolsInstall(service.WithCaller(context.Background(), service.Caller{
		Actor: "local:cli", Scopes: service.AllScopes,
	}), req); code(err) != service.CodeForbidden {
		t.Fatalf("the local token = %v, want E_FORBIDDEN", err)
	}

	operator := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:cli", Scopes: service.AllScopes, Operator: true,
	})

	if _, err := inst.ToolsInstall(operator, req); err != nil {
		t.Fatal(err)
	}

	eventually(t, "the install to finish", func() bool {
		st, err := inst.ToolsStatus(operator, service.ToolsStatusRequest{})

		return err == nil && st.State == "idle" && len(st.Installed) == 2
	})

	if _, ok := service.VerifiedTool(dir, "whisper", transcribe.KindHelper, ""); !ok {
		t.Fatal("the installed helper does not verify")
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

// The install step is system-level: an admin key limited to one project is
// refused like any other agent caller. The status is a read (VV-080).
func TestToolsInstallRefusesAProjectLimitedAdminKey(t *testing.T) {
	ts := newToolsServer(t)
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(t.TempDir())})
	project := m.newProject("p")

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: project,
	})

	if _, err := m.svc.ToolsInstall(
		limited,
		service.ToolsInstallRequest{Name: "whisper", Model: "tiny"},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("install by a limited admin key = %v, want E_FORBIDDEN", err)
	}

	if _, err := m.svc.ToolsStatus(limited, service.ToolsStatusRequest{}); err != nil {
		t.Errorf("status by a limited key = %v, want it readable", err)
	}

	if ts.requests.Load() != 0 {
		t.Error("a refused install made a request")
	}
}

// A helper that comes in an archive is unpacked only after the archive digest
// matched, only the members the manifest lists are written, and the digest
// that installed.json keeps is the program's, not the archive's.
func TestToolsInstallUnpacksAnArchiveAndRecordsTheProgramDigest(t *testing.T) {
	ts := newToolsServer(t)
	dir := filepath.Join(t.TempDir(), "tools")
	tl := ts.tools(dir)
	helper := &tl.Manifest.Entries[0]
	helper.URL = ts.srv.URL + "/whisper.zip"
	helper.Bytes = int64(len(ts.archive))
	helper.SHA256 = sum(ts.archive)
	helper.Format = "zip"
	helper.Members = []transcribe.Member{{From: "bin/whisper-cli", To: "whisper-cli", Executable: true}}

	m := newMediaEnv(t, mediaCfg{tools: tl})
	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	if _, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}); err != nil {
		t.Fatal(err)
	}

	if st := m.waitInstall(person); st.State != "idle" || len(st.Installed) != 2 {
		t.Fatalf("status = %+v", st)
	}

	program, ok := service.VerifiedTool(dir, "whisper", transcribe.KindHelper, "")
	if !ok {
		t.Fatal("the unpacked helper does not verify")
	}

	if got, _ := os.ReadFile(program); !bytes.Equal(got, ts.helper) {
		t.Errorf("program = %q", got)
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(program), "unlisted")); err == nil {
		t.Error("a member the manifest does not list was written")
	}

	if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
		t.Errorf("leftovers in .download: %v", entries)
	}
}

// hopGuard does what outbound.HelperDownload does at every redirect hop (the
// manifest hosts only), without its public-address dial rule, which a loopback
// test server cannot meet. It does not follow a redirect that leaves the list.
type hopGuard struct {
	rt    http.RoundTripper
	hosts []string
	hops  []string
}

func (g *hopGuard) Do(req *http.Request) (*http.Response, error) {
	resp, err := g.rt.RoundTrip(req)
	if err != nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return resp, err
	}

	defer func() { _ = resp.Body.Close() }()

	next, err := resp.Location()
	if err != nil {
		return nil, err
	}

	g.hops = append(g.hops, next.Hostname())

	if !slices.Contains(g.hosts, next.Hostname()) {
		return nil, errors.New("host not allowed")
	}

	return nil, errors.New("the test does not follow allowed hops")
}

// T-26 (SDD-007 section 6.8): nothing is kept and nothing is run when the
// bytes are not the pinned ones (digest or size), the URL is not https, or a
// redirect leaves the allowlist. The install lives in the service, so the test
// is here and not beside the manifest.
func TestSecurity_T26_HelperDownloadVerified(t *testing.T) {
	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})
	req := service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}

	t.Run("a wrong digest at the pinned size", func(t *testing.T) {
		ts := newToolsServer(t)
		dir := filepath.Join(t.TempDir(), "tools")
		m := newMediaEnv(t, mediaCfg{tools: ts.tools(dir)})
		other := bytes.Repeat([]byte{'x'}, len(ts.model))
		ts.serveModel.Store(&other)

		if _, err := m.svc.ToolsInstall(person, req); err != nil {
			t.Fatal(err)
		}

		st := m.waitInstall(person)
		if st.State != "failed" || st.Error == nil || st.Error.Details["reason"] != "checksum_mismatch" {
			t.Errorf("status = %+v", st)
		}

		if _, ok := service.VerifiedTool(dir, "whisper", transcribe.KindModel, "tiny"); ok {
			t.Error("the model was installed")
		}

		if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
			t.Errorf("leftovers in .download: %v", entries)
		}
	})

	t.Run("a body shorter than the pinned size", func(t *testing.T) {
		ts := newToolsServer(t)
		dir := filepath.Join(t.TempDir(), "tools")
		m := newMediaEnv(t, mediaCfg{tools: ts.tools(dir)})
		short := ts.model[:len(ts.model)-3]
		ts.serveModel.Store(&short)

		if _, err := m.svc.ToolsInstall(person, req); err != nil {
			t.Fatal(err)
		}

		st := m.waitInstall(person)
		if st.State != "failed" || st.Error == nil || st.Error.Details["reason"] != "checksum_mismatch" {
			t.Errorf("status = %+v", st)
		}

		if _, ok := service.VerifiedTool(dir, "whisper", transcribe.KindModel, "tiny"); ok {
			t.Error("the model was installed")
		}

		if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
			t.Errorf("leftovers in .download: %v", entries)
		}
	})

	t.Run("a redirect outside the allowlist", func(t *testing.T) {
		ts := newToolsServer(t)
		dir := filepath.Join(t.TempDir(), "tools")
		tl := ts.tools(dir)
		tl.Manifest.Entries[1].URL = ts.srv.URL + "/redirect"

		guard := &hopGuard{rt: ts.srv.Client().Transport, hosts: tl.Manifest.Hosts}
		tl.Client = guard

		m := newMediaEnv(t, mediaCfg{tools: tl})

		if _, err := m.svc.ToolsInstall(person, req); err != nil {
			t.Fatal(err)
		}

		st := m.waitInstall(person)
		if st.State != "failed" || st.Error == nil || st.Error.Details["reason"] != "download_failed" {
			t.Errorf("status = %+v", st)
		}

		if len(guard.hops) != 1 || guard.hops[0] != "elsewhere.invalid" {
			t.Errorf("hops = %v", guard.hops)
		}

		if _, ok := service.VerifiedTool(dir, "whisper", transcribe.KindModel, "tiny"); ok {
			t.Error("the model was installed")
		}
	})

	t.Run("a plain http url", func(t *testing.T) {
		ts := newToolsServer(t)
		tl := ts.tools(t.TempDir())

		u, _ := url.Parse(ts.srv.URL + "/model.bin")
		u.Scheme = "http"
		tl.Manifest.Entries[1].URL = u.String()

		m := newMediaEnv(t, mediaCfg{tools: tl})

		if _, err := m.svc.ToolsInstall(person, req); err == nil {
			t.Error("an http url was accepted")
		}

		if ts.requests.Load() != 0 {
			t.Errorf("an http url made %d requests", ts.requests.Load())
		}
	})
}

// The model an install brings is recorded as the workspace default, so a transcribe job without
// a model uses it (VV-007).
func TestToolsInstallRecordsTheModelAsTheWorkspaceDefault(t *testing.T) {
	ts := newToolsServer(t)
	dir := filepath.Join(t.TempDir(), "tools")
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(dir)})
	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	if _, ok := service.DefaultModel(dir, "whisper"); ok {
		t.Fatal("a default is recorded before any install")
	}

	if _, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}); err != nil {
		t.Fatal(err)
	}

	m.waitInstall(person)

	if got, ok := service.DefaultModel(dir, "whisper"); !ok || got != "tiny" {
		t.Fatalf("default = %q, %v; want tiny", got, ok)
	}
}

// VV-167: an install of what is already installed and verified downloads nothing; force installs again.
func TestToolsInstallOfWhatIsInstalledDownloadsNothing(t *testing.T) {
	ts := newToolsServer(t)
	dir := filepath.Join(t.TempDir(), "tools")
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(dir)})
	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	if _, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}); err != nil {
		t.Fatal(err)
	}

	m.waitInstall(person)

	first := ts.requests.Load()

	res, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny"})
	if err != nil || res.State != "installed" || res.DownloadBytes != 0 ||
		!strings.Contains(res.Text, "already installed") {
		t.Fatalf("second install = %+v, %v", res, err)
	}

	if ts.requests.Load() != first {
		t.Errorf("an installed tool was downloaded again: %d requests, %d before", ts.requests.Load(), first)
	}

	res, err = m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny", Force: true})
	if err != nil || res.State != "running" {
		t.Fatalf("forced install = %+v, %v", res, err)
	}

	m.waitInstall(person)

	if ts.requests.Load() == first {
		t.Error("a forced install downloaded nothing")
	}

	// A file that changed is not installed: the next install repairs it.
	helper, _ := service.VerifiedTool(dir, "whisper", transcribe.KindHelper, "")
	if err := os.WriteFile(helper, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}

	if res, err = m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}); err != nil ||
		res.State != "running" {
		t.Fatalf("install over a tampered helper = %+v, %v", res, err)
	}

	m.waitInstall(person)
}
