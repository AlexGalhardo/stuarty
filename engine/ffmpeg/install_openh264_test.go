package ffmpeg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

type countingClient struct {
	calls atomic.Int32
	body  []byte
	code  int
	err   error
}

func (c *countingClient) Do(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	if c.err != nil {
		return nil, c.err
	}
	code := c.code
	if code == 0 {
		code = http.StatusOK
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(c.body))}, nil
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fakeLibrary is a manifest for the test platform that pins the stand-in library of testdata/openh264.
func fakeLibrary(t *testing.T) (m OpenH264Manifest, archive, library []byte) {
	t.Helper()
	archive, err := os.ReadFile(filepath.Join("testdata", "openh264", "library.bin.bz2"))
	if err != nil {
		t.Fatal(err)
	}
	library, err = os.ReadFile(filepath.Join("testdata", "openh264", "library.bin"))
	if err != nil {
		t.Fatal(err)
	}
	library = bytes.ReplaceAll(library, []byte("\r\n"), []byte("\n"))
	m = OpenH264Manifest{
		Hosts: []string{"ciscobinary.openh264.org"},
		Entries: []OpenH264Entry{{
			Name: OpenH264Tool, Version: "2.6.0", OS: runtime.GOOS, Arch: runtime.GOARCH,
			URL:   "https://ciscobinary.openh264.org/fake.bz2",
			Bytes: int64(len(archive)), SHA256: hexSum(archive),
			File: "libopenh264.so.8", LibraryBytes: int64(len(library)), LibrarySHA256: hexSum(library),
			License: "BSD-2-Clause", Published: true,
		}},
	}
	return m, archive, library
}

func noLibraryLeft(t *testing.T, toolsDir string) {
	t.Helper()
	var left []string
	_ = filepath.WalkDir(toolsDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			left = append(left, p)
		}
		return nil
	})
	if len(left) != 0 {
		t.Errorf("files left behind: %v", left)
	}
}

// TestInstallOpenH264Verified is T-26 for the Cisco library: a wrong digest or size is refused and leaves
// no library; the right file installs and is verified again at each check.
func TestInstallOpenH264Verified(t *testing.T) {
	m, archive, library := fakeLibrary(t)
	flip := func(b []byte) []byte {
		c := bytes.Clone(b)
		c[len(c)-1] ^= 0xff
		return c
	}

	cases := []struct {
		name   string
		mutate func(*OpenH264Manifest)
		client *countingClient
		reason string
	}{
		{
			"wrong archive digest", func(m *OpenH264Manifest) { m.Entries[0].SHA256 = hexSum([]byte("x")) },
			&countingClient{body: archive}, ReasonChecksumMismatch,
		},
		{
			"archive longer than the size", func(m *OpenH264Manifest) { m.Entries[0].Bytes-- },
			&countingClient{body: archive}, ReasonSizeMismatch,
		},
		{
			"archive shorter than the size", func(m *OpenH264Manifest) { m.Entries[0].Bytes++ },
			&countingClient{body: archive}, ReasonSizeMismatch,
		},
		{
			"another file with the right name", nil,
			&countingClient{body: flip(archive)}, ReasonChecksumMismatch,
		},
		{
			"wrong library digest", func(m *OpenH264Manifest) { m.Entries[0].LibrarySHA256 = hexSum([]byte("x")) },
			&countingClient{body: archive}, ReasonChecksumMismatch,
		},
		{
			"library larger than the manifest says", func(m *OpenH264Manifest) { m.Entries[0].LibraryBytes-- },
			&countingClient{body: archive}, ReasonSizeMismatch,
		},
		{"not an archive", func(m *OpenH264Manifest) {
			junk := []byte("this is not bzip2 data at all, only plain bytes")
			m.Entries[0].Bytes, m.Entries[0].SHA256 = int64(len(junk)), hexSum(junk)
		}, &countingClient{body: []byte("this is not bzip2 data at all, only plain bytes")}, ReasonBadArchive},
		{"host answers 404", nil, &countingClient{body: archive, code: http.StatusNotFound}, ReasonDownloadFailed},
		{"connection fails", nil, &countingClient{err: errors.New("boom")}, ReasonDownloadFailed},
		{
			"entry not published", func(m *OpenH264Manifest) { m.Entries[0].Published = false },
			&countingClient{body: archive}, ReasonNotPublished,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mm := OpenH264Manifest{Hosts: m.Hosts, Entries: append([]OpenH264Entry(nil), m.Entries...)}
			if c.mutate != nil {
				c.mutate(&mm)
			}
			dir := t.TempDir()
			_, err := InstallOpenH264(
				context.Background(),
				InstallOpenH264Options{ToolsDir: dir, Manifest: mm, Client: c.client},
			)
			var fe *Error
			if !errors.As(err, &fe) || fe.Reason != c.reason || fe.Code != CodeNoEncoder {
				t.Fatalf("error = %v, want reason %s", err, c.reason)
			}
			noLibraryLeft(t, dir)
			if st := CheckOpenH264(dir, mm, "", ""); st.Installed {
				t.Error("a library is reported installed")
			}
		})
	}

	t.Run("the right file installs", func(t *testing.T) {
		dir := t.TempDir()
		var last int64
		st, err := InstallOpenH264(context.Background(), InstallOpenH264Options{
			ToolsDir: dir, Manifest: m, Client: &countingClient{body: archive},
			Progress: func(done, _ int64) { last = done },
		})
		if err != nil || !st.Installed {
			t.Fatalf("install: %+v %v", st, err)
		}
		if last != int64(len(archive)) {
			t.Errorf("progress ended at %d of %d", last, len(archive))
		}
		got, err := os.ReadFile(OpenH264Path(dir, m.Entries[0]))
		if err != nil || !bytes.Equal(got, library) {
			t.Fatalf("library on disk: %v", err)
		}
		if st := CheckOpenH264(
			dir,
			m,
			"",
			"",
		); !st.Installed ||
			st.Dir != filepath.Dir(OpenH264Path(dir, m.Entries[0])) {
			t.Errorf("check: %+v", st)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
			t.Errorf("download folder not empty: %v", entries)
		}

		// A file changed after the install is not trusted, with the same size or another.
		if err := os.WriteFile(OpenH264Path(dir, m.Entries[0]), flipByte(library), 0o600); err != nil {
			t.Fatal(err)
		}
		if st := CheckOpenH264(dir, m, "", ""); st.Installed || st.Reason != ReasonOpenH264Changed {
			t.Errorf("changed library: %+v", st)
		}
	})
}

func flipByte(b []byte) []byte {
	c := bytes.Clone(b)
	c[0] ^= 0xff
	return c
}

func TestOpenH264ManifestIsValidAndPinsEveryPlatform(t *testing.T) {
	m, err := DefaultOpenH264Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Hosts) != 1 || m.Hosts[0] != "ciscobinary.openh264.org" {
		t.Errorf("hosts = %v", m.Hosts)
	}
	for _, p := range [][2]string{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
	} {
		e, ok := m.For(p[0], p[1])
		if !ok || !e.Published || e.Bytes <= 0 || !isHex64(e.SHA256) || !isHex64(e.LibrarySHA256) ||
			e.License != "BSD-2-Clause" {
			t.Errorf("%v: %+v", p, e)
		}
	}
}

func TestOpenH264ManifestRefusesWhatItShould(t *testing.T) {
	bad := map[string]string{
		"http":         `{"hosts":["h.example"],"entries":[{"name":"openh264","version":"1","os":"linux","arch":"amd64","url":"http://h.example/a","file":"a"}]}`,
		"other host":   `{"hosts":["h.example"],"entries":[{"name":"openh264","version":"1","os":"linux","arch":"amd64","url":"https://evil.example/a","file":"a"}]}`,
		"path in file": `{"hosts":["h.example"],"entries":[{"name":"openh264","version":"1","os":"linux","arch":"amd64","url":"https://h.example/a","file":"../a"}]}`,
		"no digest":    `{"hosts":["h.example"],"entries":[{"name":"openh264","version":"1","os":"linux","arch":"amd64","url":"https://h.example/a","file":"a","bytes":1,"library_bytes":1,"published":true}]}`,
		"upper case": `{"hosts":["h.example"],"entries":[{"name":"openh264","version":"1","os":"linux","arch":"amd64","url":"https://h.example/a","file":"a","bytes":1,"library_bytes":1,"published":true,"sha256":"` + strings.Repeat(
			"A",
			64,
		) + `","library_sha256":"` + strings.Repeat(
			"a",
			64,
		) + `"}]}`,
	}
	for name, doc := range bad {
		if _, err := ParseOpenH264Manifest([]byte(doc)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestOpenH264NeverAutoDownloaded (RF-130, ADR-0020): the package builds no HTTP request anywhere but in
// InstallOpenH264, which only the explicit install step calls, and checking the library creates nothing.
func TestOpenH264NeverAutoDownloaded(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == `"net/http"` && name != "install_openh264.go" {
				t.Errorf("%s imports net/http: only install_openh264.go may talk HTTP", name)
			}
			if p := imp.Path.Value; p == `"net"` || p == `"net/http/httputil"` || p == `"crypto/tls"` {
				t.Errorf("%s imports %s", name, p)
			}
		}
	}

	m, _, _ := fakeLibrary(t)
	dir := t.TempDir()
	if st := CheckOpenH264(dir, m, "", ""); st.Installed || st.Reason != ReasonOpenH264NotInstalled {
		t.Fatalf("check: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, OpenH264Tool)); !os.IsNotExist(err) {
		t.Errorf("something was created under tools/: %v", err)
	}

	// An install without a client refuses and never invents one.
	if _, err := InstallOpenH264(context.Background(), InstallOpenH264Options{ToolsDir: dir, Manifest: m}); err == nil {
		t.Error("an install with no client succeeded")
	}
}
