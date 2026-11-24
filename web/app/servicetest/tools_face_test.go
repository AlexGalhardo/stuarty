package servicetest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/aihost"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service"
)

// faceServer serves the three files of the face set and counts the requests.
type faceServer struct {
	srv      *httptest.Server
	requests atomic.Int64
	helper   []byte
	runtime  []byte
	archive  []byte
	model    []byte
	// body replaces the model, for the refusal tests; cut closes the connection halfway through it.
	body atomic.Pointer[[]byte]
	cut  atomic.Bool
}

func newFaceServer(t *testing.T) *faceServer {
	t.Helper()

	fs := &faceServer{
		helper:  []byte("helper-bytes"),
		runtime: []byte("onnxruntime-library-bytes"),
		model:   bytes.Repeat([]byte("yunet"), 400),
	}
	fs.archive = zipOf(t, map[string][]byte{
		"runtimes/win-x64/native/onnxruntime.dll":                  fs.runtime,
		"runtimes/win-x64/native/onnxruntime_providers_shared.dll": []byte("shared"),
		"runtimes/win-x64/native/unlisted.dll":                     []byte("never written"),
	})

	fs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.requests.Add(1)

		switch r.URL.Path {
		case "/helper.exe":
			_, _ = w.Write(fs.helper)
		case "/runtime.nupkg":
			_, _ = w.Write(fs.archive)
		case "/model.onnx":
			body := fs.model
			if alt := fs.body.Load(); alt != nil {
				body = *alt
			}

			if fs.cut.Load() {
				w.Header().Set("Content-Length", "100000")
				_, _ = w.Write(body[:100])

				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}

				panic(http.ErrAbortHandler)
			}

			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(fs.srv.Close)

	return fs
}

func (fs *faceServer) tools(dir string, published bool) *service.Tools {
	host := fs.srv.Listener.Addr().String()
	hostname := host[:lastColon(host)]

	entry := func(name, kind, model, file, path string, body []byte) aihost.Entry {
		return aihost.Entry{
			Name: name, Kind: kind, Model: model, Version: "1", OS: "testos", Arch: "testarch",
			URL: fs.srv.URL + path, Bytes: int64(len(body)), SHA256: sum(body), File: file, License: "MIT",
			Notice: "MIT", Published: published, Dataset: "a test dataset",
		}
	}

	rt := entry(aihost.ToolRuntime, transcribe.KindHelper, "", "onnxruntime.dll", "/runtime.nupkg", fs.archive)
	rt.Format = "zip"
	rt.Members = []transcribe.Member{
		{From: "runtimes/win-x64/native/onnxruntime.dll", To: "onnxruntime.dll"},
		{From: "runtimes/win-x64/native/onnxruntime_providers_shared.dll", To: "onnxruntime_providers_shared.dll"},
	}

	return &service.Tools{
		Dir: dir, Client: fs.srv.Client(), GOOS: "testos", GOARCH: "testarch",
		Face: aihost.Manifest{
			Hosts: []string{hostname},
			Entries: []aihost.Entry{
				entry(aihost.ToolHelper, transcribe.KindHelper, "", "stuarty-aihelper", "/helper.exe", fs.helper),
				rt,
				entry(
					aihost.ToolModel,
					transcribe.KindModel,
					aihost.ModelVersion,
					"yunet.onnx",
					"/model.onnx",
					fs.model,
				),
			},
		},
	}
}

func faceDir(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "tools")
}

func asPerson() context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})
}

func faceInstall(t *testing.T, m *mediaEnv) *service.ToolsStatusResult {
	t.Helper()

	res, err := m.svc.ToolsInstall(asPerson(), service.ToolsInstallRequest{Name: aihost.SetName})
	if err != nil || res.State != "running" || len(res.Files) != 3 {
		t.Fatalf("install = %+v, %v", res, err)
	}

	return m.waitInstall(asPerson())
}

// Acceptance criterion 1 of epic 70: with nothing installed nothing is downloaded, whoever asks and
// whatever is read, and the status says what an install would cost before anything starts.
func TestSecurity_T26_FaceInstallNeverAutoDownloaded(t *testing.T) {
	fs := newFaceServer(t)
	dir := faceDir(t)
	tl := fs.tools(dir, true)
	m := newMediaEnv(t, mediaCfg{tools: tl})

	st, err := m.svc.ToolsStatus(asPerson(), service.ToolsStatusRequest{})
	if err != nil || len(st.Offers) != 1 || st.Offers[0].Name != aihost.SetName || st.Offers[0].Installed {
		t.Fatalf("status = %+v, %v", st, err)
	}

	offer := st.Offers[0]
	if len(offer.Files) != 3 || offer.DownloadBytes != int64(len(fs.helper)+len(fs.archive)+len(fs.model)) ||
		offer.Files[0].License != "MIT" || offer.Dataset != "a test dataset" || !offer.Published {
		t.Errorf("offer = %+v", offer)
	}

	if _, ok := tl.FaceStack(); ok {
		t.Error("an empty tools folder holds the face set")
	}

	for name, c := range map[string]service.Caller{
		"the local token": {Actor: "local:cli", Scopes: service.AllScopes},
		"a key without admin": {
			Actor: "key:k", Scopes: []string{service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender},
		},
		"a read key": {Actor: "key:r", Scopes: []string{service.ScopeRead}},
	} {
		_, err := m.svc.ToolsInstall(
			service.WithCaller(context.Background(), c),
			service.ToolsInstallRequest{Name: aihost.SetName},
		)
		if code(err) != service.CodeForbidden {
			t.Errorf("%s = %v, want E_FORBIDDEN", name, err)
		}
	}

	if fs.requests.Load() != 0 {
		t.Errorf("%d requests were made without an install", fs.requests.Load())
	}

	if _, err := os.Stat(dir); err == nil {
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("tools/ holds %v before any install", entries)
		}
	}
}

func TestFaceInstallVerifiesAndRecordsEveryFile(t *testing.T) {
	fs := newFaceServer(t)
	dir := faceDir(t)
	tl := fs.tools(dir, true)
	m := newMediaEnv(t, mediaCfg{tools: tl})

	if st := faceInstall(t, m); st.State != "idle" || len(st.Installed) != 3 || st.Done != st.Total {
		t.Fatalf("status = %+v", st)
	}

	paths, ok := tl.FaceStack()
	if !ok {
		t.Fatal("the set does not verify after the install")
	}

	for _, p := range []string{paths.Helper, paths.Runtime, paths.Model} {
		if !filepath.IsAbs(p) {
			t.Errorf("%s is not absolute", p)
		}
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(paths.Runtime), "unlisted.dll")); err == nil {
		t.Error("a file the manifest does not list was unpacked")
	}

	if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
		t.Errorf("leftovers in .download: %v", entries)
	}

	st, _ := m.svc.ToolsStatus(asPerson(), service.ToolsStatusRequest{})
	if !st.Offers[0].Installed {
		t.Error("the offer does not say it is installed")
	}

	before := fs.requests.Load()

	res, err := m.svc.ToolsInstall(asPerson(), service.ToolsInstallRequest{Name: aihost.SetName})
	if err != nil || res.State != "installed" || fs.requests.Load() != before {
		t.Errorf("a second install = %+v, %v, %d new requests", res, err, fs.requests.Load()-before)
	}

	// A file that changed after the install is not used.
	if err := os.WriteFile(paths.Model, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, ok := tl.FaceStack(); ok {
		t.Error("a tampered model still verifies")
	}
}

// Acceptance criterion 2 of epic 70: a model with a wrong hash is refused and removed.
func TestSecurity_T26_FaceModelWithAWrongHashIsRefusedAndRemoved(t *testing.T) {
	for name, body := range map[string][]byte{
		"the same size, other bytes": bytes.Repeat([]byte("x"), 2000),
		"a longer body":              append(bytes.Repeat([]byte("yunet"), 400), 'z'),
		"a shorter body":             bytes.Repeat([]byte("yunet"), 399),
	} {
		t.Run(name, func(t *testing.T) {
			fs := newFaceServer(t)
			dir := faceDir(t)
			tl := fs.tools(dir, true)
			m := newMediaEnv(t, mediaCfg{tools: tl})
			fs.body.Store(&body)

			st := faceInstall(t, m)
			if st.State != "failed" || st.Error == nil || st.Error.Details["reason"] != "checksum_mismatch" {
				t.Fatalf("status = %+v", st)
			}

			if _, ok := service.VerifiedTool(dir, aihost.ToolModel, transcribe.KindModel, aihost.ModelVersion); ok {
				t.Error("the model was installed")
			}

			if _, ok := tl.FaceStack(); ok {
				t.Error("the set verifies with a refused model")
			}

			if _, err := os.Stat(filepath.Join(dir, aihost.ToolModel)); err == nil {
				t.Error("the folder of the refused model was kept")
			}

			if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
				t.Errorf("leftovers in .download: %v", entries)
			}
		})
	}
}

func TestFaceInstallInterruptedDownloadLeavesNothing(t *testing.T) {
	fs := newFaceServer(t)
	dir := faceDir(t)
	tl := fs.tools(dir, true)
	m := newMediaEnv(t, mediaCfg{tools: tl})
	fs.cut.Store(true)

	st := faceInstall(t, m)
	if st.State != "failed" || st.Error == nil {
		t.Fatalf("status = %+v", st)
	}

	if _, ok := service.VerifiedTool(dir, aihost.ToolModel, transcribe.KindModel, aihost.ModelVersion); ok {
		t.Error("an interrupted model was installed")
	}

	if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
		t.Errorf("leftovers in .download: %v", entries)
	}

	// The next attempt works: nothing half-written is in the way.
	fs.cut.Store(false)

	if st := faceInstall(t, m); st.State != "idle" {
		t.Fatalf("the retry = %+v", st)
	}

	if _, ok := tl.FaceStack(); !ok {
		t.Error("the set does not verify after the retry")
	}
}

func TestFaceInstallRefusesAFileThatIsNotPublishedBeforeAnyRequest(t *testing.T) {
	fs := newFaceServer(t)
	m := newMediaEnv(t, mediaCfg{tools: fs.tools(faceDir(t), false)})

	_, err := m.svc.ToolsInstall(asPerson(), service.ToolsInstallRequest{Name: aihost.SetName})
	if code(err) != "E_TRANSCRIBE_UNAVAILABLE" {
		t.Fatalf("error = %v, want E_TRANSCRIBE_UNAVAILABLE", err)
	}

	if fs.requests.Load() != 0 {
		t.Errorf("%d requests for a file that is not published", fs.requests.Load())
	}

	st, _ := m.svc.ToolsStatus(asPerson(), service.ToolsStatusRequest{})
	if len(st.Offers) != 1 || st.Offers[0].Published {
		t.Errorf("the offer says it is published: %+v", st.Offers)
	}
}
