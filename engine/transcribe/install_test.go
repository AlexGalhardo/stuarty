package transcribe

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

func sumHex(b []byte) string {
	s := sha256.Sum256(b)

	return hex.EncodeToString(s[:])
}

// tlsServer serves the handler over TLS on loopback and returns the options that trust it.
func tlsServer(t *testing.T, h http.Handler) (*httptest.Server, *outbound.TestOptions) {
	t.Helper()

	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	return srv, &outbound.TestOptions{RootCAs: pool, AllowLoopback: true}
}

func hostOfURL(t *testing.T, raw string) string {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	return u.Hostname()
}

func modelEntry(raw, urlStr string, body []byte) *Entry {
	return &Entry{
		Kind: "model", Name: "tiny", Version: "test", URL: urlStr, Size: int64(len(body)), SHA256: sumHex(body),
		Published: true, Licence: "MIT", Hosts: []string{raw}, File: "ggml-tiny.bin",
	}
}

// noFiles fails when the tools folder holds any regular file: a refused install leaves nothing behind.
func noFiles(t *testing.T, dir string) {
	t.Helper()

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			t.Errorf("%s was left behind", path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func reasonOf(t *testing.T, err error) string {
	t.Helper()

	fe, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok || fe.Code != CodeUnavailable {
		t.Fatalf("err = %v, want E_TRANSCRIBE_UNAVAILABLE", err)
	}

	return fe.Reason
}

// SDD-007 section 15 check 26: a wrong digest, a wrong size, a redirect outside the allowlist and an
// http:// URL are each refused and leave no file.
func TestInstallRefusals(t *testing.T) {
	body := bytes.Repeat([]byte("model"), 400)

	other, _ := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))

	srv, test := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good":
			_, _ = w.Write(body)
		case "/longer":
			_, _ = w.Write(append(bytes.Clone(body), 'x'))
		case "/shorter":
			_, _ = w.Write(body[:len(body)-1])
		case "/chunked-longer":
			w.(http.Flusher).Flush()
			_, _ = w.Write(append(bytes.Clone(body), "tail"...))
		case "/redirect":
			// localhost is not in the allowlist, which holds the address 127.0.0.1 only.
			http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/model", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	host := hostOfURL(t, srv.URL)

	tests := []struct {
		name   string
		mutate func(*Entry)
		path   string
		reason string
	}{
		{"wrong digest", func(e *Entry) { e.SHA256 = sumHex([]byte("other")) }, "/good", ReasonChecksumMismatch},
		{"body longer than the manifest size", nil, "/longer", ReasonSizeMismatch},
		{"body shorter than the manifest size", nil, "/shorter", ReasonSizeMismatch},
		{"body longer with no length", nil, "/chunked-longer", ReasonSizeMismatch},
		{"redirect outside the allowlist", nil, "/redirect", ReasonDownloadRefused},
		{"endless redirects", nil, "/loop", ReasonDownloadRefused},
		{"not found", nil, "/missing", ReasonDownloadRefused},
		{
			"http URL",
			func(e *Entry) { e.URL = strings.Replace(e.URL, "https://", "http://", 1) },
			"/good",
			ReasonDownloadRefused,
		},
		{"an unpublished entry", func(e *Entry) { e.Published = false }, "/good", ReasonNotPublished},
		{
			"a host outside its own allowlist",
			func(e *Entry) { e.Hosts = []string{"example.com"} },
			"/good",
			ReasonDownloadRefused,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			e := modelEntry(host, srv.URL+tt.path, body)

			if tt.mutate != nil {
				tt.mutate(e)
			}

			in := &Installer{ToolsDir: dir, Test: test}
			if got := reasonOf(t, in.Install(context.Background(), e)); got != tt.reason {
				t.Errorf("reason %q, want %q", got, tt.reason)
			}

			noFiles(t, dir)
		})
	}
}

func TestInstallModelAndRecord(t *testing.T) {
	body := bytes.Repeat([]byte("weights"), 1000)
	srv, test := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))

	var (
		mu    sync.Mutex
		dials []string
	)

	restore := outbound.SetDialRecorder(func(p outbound.Purpose, address string) error {
		mu.Lock()
		defer mu.Unlock()
		dials = append(dials, string(p)+" "+address)

		return nil
	})
	defer restore()

	dir := t.TempDir()

	var last int64

	in := &Installer{ToolsDir: dir, Test: test, Progress: func(done, total int64) { last = done + 0*total }}
	if err := in.Install(context.Background(), modelEntry(hostOfURL(t, srv.URL), srv.URL+"/m", body)); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "models", "ggml-tiny.bin"))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("model not in place: %v", err)
	}

	if last != int64(len(body)) {
		t.Errorf("progress ended at %d of %d", last, len(body))
	}

	items, err := ReadInstalled(dir)
	if err != nil || len(items) != 1 || items[0].SHA256 != sumHex(body) || items[0].Path != "models/ggml-tiny.bin" {
		t.Errorf("installed.json: %+v, %v", items, err)
	}

	if m, _ := filepath.Glob(filepath.Join(dir, ".download", "*")); len(m) != 0 {
		t.Errorf("download folder holds %v", m)
	}

	if len(dials) == 0 || !strings.HasPrefix(dials[0], "helper_download 127.0.0.1:") {
		t.Errorf("dials %v: the recorder must see the install connect to the server", dials)
	}
}

func zipOf(t *testing.T, files map[string][]byte, order []string) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		_, _ = w.Write(files[name])
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func helperEntry(urlStr, host, format string, archive []byte, members []Member, links []Link) *Entry {
	exe := "whisper-cli"

	return &Entry{
		Kind: "helper", Name: HelperName, Version: "vtest", OS: runtime.GOOS, Arch: runtime.GOARCH, URL: urlStr,
		Size: int64(len(archive)), SHA256: sumHex(archive), Published: true, Licence: "MIT", Hosts: []string{host},
		Format: format, Members: members, Links: links, Executable: exe,
	}
}

// Archives are extracted by the names of the manifest: an entry the manifest does not list is never
// written, whatever its name says.
func TestInstallHelperExtractsOnlyTheMembersOfTheManifest(t *testing.T) {
	files := map[string][]byte{
		"Release/whisper-cli": []byte("program"),
		"Release/lib.dll":     []byte("library"),
		"Release/extra.exe":   []byte("not listed"),
		"../evil":             []byte("traversal"),
		"/abs/evil":           []byte("absolute"),
	}
	archive := zipOf(
		t,
		files,
		[]string{"../evil", "/abs/evil", "Release/extra.exe", "Release/lib.dll", "Release/whisper-cli"},
	)

	srv, test := tlsServer(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }),
	)
	e := helperEntry(srv.URL+"/h.zip", hostOfURL(t, srv.URL), "zip", archive, []Member{
		{From: "Release/whisper-cli", To: "whisper-cli", Executable: true},
		{From: "Release/lib.dll", To: "lib.dll"},
	}, nil)

	dir := t.TempDir()
	in := &Installer{ToolsDir: dir, Test: test}

	if err := in.Install(context.Background(), e); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "whisper", "vtest", runtime.GOOS+"-"+runtime.GOARCH)

	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
	}

	if !slices.Equal(names, []string{"lib.dll", "whisper-cli"}) {
		t.Errorf("installed %v, want only the listed members", names)
	}

	for _, p := range []string{filepath.Join(dir, "evil"), filepath.Join(dir, "..", "evil"), filepath.Join(dir, "whisper", "evil")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s exists", p)
		}
	}

	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(filepath.Join(dst, "whisper-cli")); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("the program mode is %v, want 0700 (%v)", fi.Mode(), err)
		}
	}
}

func TestInstallHelperRefusesAnArchiveWithAMissingMember(t *testing.T) {
	archive := zipOf(t, map[string][]byte{"a": []byte("x")}, []string{"a"})
	srv, test := tlsServer(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }),
	)
	e := helperEntry(srv.URL+"/h.zip", hostOfURL(t, srv.URL), "zip", archive,
		[]Member{{From: "whisper-cli", To: "whisper-cli", Executable: true}}, nil)

	dir := t.TempDir()
	if got := reasonOf(
		t,
		(&Installer{ToolsDir: dir, Test: test}).Install(context.Background(), e),
	); got != ReasonBadArchive {
		t.Errorf("reason %q", got)
	}

	noFiles(t, dir)
}

func TestInstallHelperFromATarWithLinks(t *testing.T) {
	var raw bytes.Buffer

	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	add := func(name string, body []byte, typ byte, link string) {
		if err := tw.WriteHeader(
			&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: typ, Linkname: link},
		); err != nil {
			t.Fatal(err)
		}

		_, _ = tw.Write(body)
	}
	add("pkg/whisper-cli", []byte("program"), tar.TypeReg, "")
	add("pkg/libx.so.1.0", []byte("lib"), tar.TypeReg, "")
	add("pkg/libx.so", nil, tar.TypeSymlink, "/etc/passwd")
	add("pkg/skipped", []byte("no"), tar.TypeReg, "")

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	archive := raw.Bytes()
	srv, test := tlsServer(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }),
	)
	e := helperEntry(srv.URL+"/h.tgz", hostOfURL(t, srv.URL), "tar.gz", archive, []Member{
		{From: "pkg/whisper-cli", To: "whisper-cli", Executable: true},
		{From: "pkg/libx.so.1.0", To: "libx.so.1.0"},
	}, []Link{{Name: "libx.so", Target: "libx.so.1.0"}})

	dir := t.TempDir()
	if err := (&Installer{ToolsDir: dir, Test: test}).Install(context.Background(), e); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "whisper", "vtest", runtime.GOOS+"-"+runtime.GOARCH)
	if _, err := os.Stat(filepath.Join(dst, "skipped")); err == nil {
		t.Error("an unlisted member was written")
	}

	if runtime.GOOS != "windows" {
		// The link comes from the manifest, never from the archive: its target is the manifest's.
		if target, err := os.Readlink(filepath.Join(dst, "libx.so")); err != nil || target != "libx.so.1.0" {
			t.Errorf("link target %q, %v", target, err)
		}
	}
}

// The digest of the helper is checked against installed.json at each start: a changed program makes the
// helper unavailable and the status says why.
func TestResolveWhisperVerifiesTheDigest(t *testing.T) {
	archive := zipOf(t, map[string][]byte{"whisper-cli": []byte("program")}, []string{"whisper-cli"})
	srv, _ := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	h := helperEntry(srv.URL+"/h.zip", hostOfURL(t, srv.URL), "zip", archive,
		[]Member{{From: "whisper-cli", To: "whisper-cli", Executable: true}}, nil)
	model := []byte("weights")
	m := modelEntry(hostOfURL(t, srv.URL), srv.URL+"/model", model)
	m.Name = DefaultModel

	manifest := &Manifest{Version: 1, Entries: []Entry{*h, *m}}
	dir := t.TempDir()

	w, st, err := ResolveWhisper(dir, manifest, "")
	if err != nil || w.Check(context.Background()) == nil || st.Problem != "" {
		t.Fatalf("nothing installed: %v %v %+v", err, w.Check(context.Background()), st)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/h.zip", func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write(archive) })
	mux.HandleFunc("/model", func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write(model) })

	srv2, test2 := tlsServer(t, mux)
	h.URL, m.URL = srv2.URL+"/h.zip", srv2.URL+"/model"
	h.Hosts, m.Hosts = []string{hostOfURL(t, srv2.URL)}, []string{hostOfURL(t, srv2.URL)}
	m.File = "ggml-small.bin"

	in := &Installer{ToolsDir: dir, Manifest: &Manifest{Version: 1, Entries: []Entry{*h, *m}}, Test: test2}
	if err := in.InstallWhisper(context.Background(), DefaultModel); err != nil {
		t.Fatal(err)
	}

	manifest = in.Manifest

	w, st, err = ResolveWhisper(dir, manifest, "")
	if err != nil || w.Helper == "" || w.ModelPath == "" || st.Problem != "" {
		t.Fatalf("installed: %v %+v %+v", err, w, st)
	}

	if err := w.Check(context.Background()); err != nil {
		t.Errorf("Check after the install: %v", err)
	}

	if err := os.WriteFile(w.Helper, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}

	w, st, err = ResolveWhisper(dir, manifest, "")
	if err != nil || w.Helper != "" || !strings.Contains(st.Problem, ReasonDigestMismatch) {
		t.Errorf("a changed program: %v, helper %q, problem %q", err, w.Helper, st.Problem)
	}

	if _, ok := errors.AsType[*MissingError](w.Check(context.Background())); !ok {
		t.Error("a helper that fails its digest must be reported as not available")
	}
}

func TestManifestHasEntryForEverySupportedPlatform(t *testing.T) {
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range SupportedPlatforms {
		if _, err := m.Helper(p[0], p[1]); err != nil {
			t.Errorf("%s/%s: %v", p[0], p[1], err)
		}
	}

	for _, name := range Models {
		if _, err := m.Model(name); err != nil {
			t.Error(err)
		}
	}

	if _, err := m.Model("custom"); err == nil {
		t.Error("a model outside the manifest was accepted")
	}

	// The supported list of the manifest is the list of this test: an entry for a platform nobody
	// supports would be dead weight.
	for _, e := range m.Entries {
		if e.Kind == "helper" && !slices.Contains(SupportedPlatforms, [2]string{e.OS, e.Arch}) {
			t.Errorf("entry %s/%s is not a supported platform", e.OS, e.Arch)
		}
	}
}

// A-18: every URL is https on a listed host, every published hash is 64 hex characters, and the notice
// text and licence are there for the notices step.
func TestManifestPinsAreWellFormed(t *testing.T) {
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}

	published := 0

	for _, e := range m.Entries {
		if e.Licence == "" || len(e.Notice) < 100 || len(e.Hosts) == 0 {
			t.Errorf("%s: licence, notice or hosts missing", e.Name)
		}

		if !e.Published {
			if e.Kind != "helper" || !strings.Contains(e.URL, "/AlexGalhardo/vukem-video/releases/download/helpers-") {
				t.Errorf("%s: only a helper built by this repository may be unpublished", e.Name)
			}

			continue
		}

		published++

		if !isHex64(e.SHA256) || e.Size <= 0 {
			t.Errorf("%s: size %d, sha %q", e.Name, e.Size, e.SHA256)
		}
	}

	if published < 7 {
		t.Errorf("%d published entries, want at least 3 upstream helpers and 4 models", published)
	}

	if _, err := ParseManifest(
		[]byte(`{"version":1,"entries":[{"kind":"model","name":"tiny","url":"http://x/y","hosts":["x"],"file":"f"}]}`),
	); err == nil {
		t.Error("an http URL in the manifest was accepted")
	}

	if _, err := ParseManifest(
		[]byte(
			`{"version":1,"entries":[{"kind":"helper","name":"w","os":"linux","arch":"amd64","url":"https://x/y","hosts":["x"],"executable":"../w","published":true,"size":1,"sha256":"` + strings.Repeat(
				"a",
				64,
			) + `"}]}`,
		),
	); err == nil {
		t.Error("a traversal in a member name was accepted")
	}
}

// Starting a transcription with no helper installed makes no connection and says how to install it.
func TestJobNeverDownloads(t *testing.T) {
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}

	var dials int

	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error {
		dials++

		return errors.New("no connection is allowed")
	})
	defer restore()

	w, _, err := ResolveWhisper(t.TempDir(), m, "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = w.Transcribe(context.Background(), Input{Audio: "x.wav", Duration: 1}, nil)

	missing, ok := errors.AsType[*MissingError](err)
	if !ok {
		t.Fatalf("err = %v, want *MissingError", err)
	}

	fe := UnavailableError(missing, w.ModelName)
	if fe.Code != CodeUnavailable ||
		fe.Details["install_command"] != "vukem-video tools:install whisper --model small" {
		t.Errorf("error %+v", fe)
	}

	if fe.Details["download_bytes"].(int64) <= 0 {
		t.Errorf("download_bytes %v", fe.Details["download_bytes"])
	}

	if dials != 0 {
		t.Errorf("%d dials, want 0", dials)
	}
}

func TestAnnounceStatesSizeAndSource(t *testing.T) {
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}

	h, _ := m.Helper("windows", "amd64")
	md, _ := m.Model("small")

	got := Announce(h, md)
	for _, want := range []string{"whisper", "github.com", "small", "huggingface.co", "495.8 MB"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
}
