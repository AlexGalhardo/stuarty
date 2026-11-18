package servicetest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
)

// cisco stands in for ciscobinary.openh264.org: it serves the compressed stand-in library of
// engine/ffmpeg/testdata and counts the requests.
type cisco struct {
	calls   atomic.Int32
	archive []byte
	library []byte
	body    []byte
}

func (c *cisco) Do(*http.Request) (*http.Response, error) {
	c.calls.Add(1)

	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(c.body))}, nil
}

func newCisco(t *testing.T) *cisco {
	t.Helper()

	dir := filepath.Join("..", "..", "..", "engine", "ffmpeg", "testdata", "openh264")

	archive, err := os.ReadFile(filepath.Join(dir, "library.bin.bz2"))
	if err != nil {
		t.Fatal(err)
	}

	library, err := os.ReadFile(filepath.Join(dir, "library.bin"))
	if err != nil {
		t.Fatal(err)
	}

	return &cisco{archive: archive, library: bytes.ReplaceAll(library, []byte("\r\n"), []byte("\n")), body: archive}
}

func (c *cisco) tools(dir, goos string) *service.Tools {
	return &service.Tools{
		Dir: dir, Client: c, GOOS: goos, GOARCH: "testarch",
		Manifest: transcribe.Manifest{Hosts: []string{"github.com"}},
		OpenH264: ffmpeg.OpenH264Manifest{
			Hosts: []string{"ciscobinary.openh264.org"},
			Entries: []ffmpeg.OpenH264Entry{{
				Name: ffmpeg.OpenH264Tool, Version: "2.6.0", OS: "testos", Arch: "testarch",
				URL: "https://ciscobinary.openh264.org/fake.bz2", Bytes: int64(len(c.archive)), SHA256: sum(c.archive),
				File: "libopenh264.so.8", LibraryBytes: int64(len(c.library)), LibrarySHA256: sum(c.library),
				License: "BSD-2-Clause", Published: true,
			}},
		},
	}
}

var openH264Person = service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true}

// P3-RND-001, SDD-007 section 6.8 (OpenH264): the install step of a person announces the size and the
// host, downloads through the client it was given, and leaves the verified library where the own
// build looks for it.
func TestToolsInstallOpenH264InstallsTheVerifiedLibrary(t *testing.T) {
	cs := newCisco(t)
	dir := filepath.Join(t.TempDir(), "tools")
	m := newMediaEnv(t, mediaCfg{tools: cs.tools(dir, "testos")})
	person := service.WithCaller(context.Background(), openH264Person)

	res, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "openh264"})
	if err != nil {
		t.Fatal(err)
	}

	if res.State != "running" || len(res.Files) != 1 || res.Files[0].Host != "ciscobinary.openh264.org" ||
		res.DownloadBytes != int64(len(cs.archive)) {
		t.Errorf("announcement = %+v", res)
	}

	if st := m.waitInstall(person); st.State != "idle" || st.Error != nil {
		t.Fatalf("status = %+v", st)
	}

	got, rerr := os.ReadFile(filepath.Join(dir, "openh264", "2.6.0", "libopenh264.so.8"))
	if rerr != nil || !bytes.Equal(got, cs.library) {
		t.Errorf("library = %v, %v", len(got), rerr)
	}

	state := ffmpeg.CheckOpenH264(dir, cs.tools(dir, "testos").OpenH264, "testos", "testarch")
	if !state.Installed {
		t.Errorf("the installed library does not verify: %+v", state)
	}
}

// Every refusal of the install is E_NO_ENCODER with details.reason, and keeps nothing.
func TestToolsInstallOpenH264RefusalsAreNoEncoderWithAReason(t *testing.T) {
	flipped := func(c *cisco) { c.body = bytes.Clone(c.archive); c.body[len(c.body)-1] ^= 0xff }

	for name, tc := range map[string]struct {
		mutate func(*cisco)
		reason string
	}{
		"another file with the right name": {flipped, ffmpeg.ReasonChecksumMismatch},
		"a shorter body":                   {func(c *cisco) { c.body = c.archive[:len(c.archive)-1] }, ffmpeg.ReasonSizeMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			cs := newCisco(t)
			tc.mutate(cs)

			dir := filepath.Join(t.TempDir(), "tools")
			m := newMediaEnv(t, mediaCfg{tools: cs.tools(dir, "testos")})
			person := service.WithCaller(context.Background(), openH264Person)

			if _, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "openh264"}); err != nil {
				t.Fatal(err)
			}

			st := m.waitInstall(person)
			if st.State != "failed" || st.Error == nil || st.Error.Code != "E_NO_ENCODER" ||
				st.Error.Details["reason"] != tc.reason {
				t.Fatalf("status = %+v", st)
			}

			if _, err := os.Stat(filepath.Join(dir, "openh264")); err == nil {
				t.Error("a refused install left a library directory")
			}
		})
	}

	t.Run("a platform the manifest does not list", func(t *testing.T) {
		cs := newCisco(t)
		m := newMediaEnv(t, mediaCfg{tools: cs.tools(filepath.Join(t.TempDir(), "tools"), "otheros")})

		_, err := m.svc.ToolsInstall(service.WithCaller(context.Background(), openH264Person),
			service.ToolsInstallRequest{Name: "openh264"})

		se := serviceErr(err)
		if se == nil || se.Code != "E_NO_ENCODER" || se.Details["reason"] != ffmpeg.ReasonOpenH264Platform ||
			cs.calls.Load() != 0 {
			t.Errorf("err = %v, calls %d", err, cs.calls.Load())
		}
	})
}

// T-26: no agent surface starts the download, and neither does any other call: the client of the
// install is never used by anything but the install step of a person.
func TestSecurity_T26_OpenH264NeverTriggeredByAnAgentSurface(t *testing.T) {
	cs := newCisco(t)
	m := newMediaEnv(t, mediaCfg{tools: cs.tools(filepath.Join(t.TempDir(), "tools"), "testos")})
	req := service.ToolsInstallRequest{Name: "openh264"}

	for name, c := range map[string]service.Caller{
		"the local token":     {Actor: "local:cli", Scopes: service.AllScopes},
		"a key without admin": {Actor: "key:k", Scopes: []string{service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender}},
		"a read key":          {Actor: "key:r", Scopes: []string{service.ScopeRead}},
		"an admin key of one project": {
			Actor: "key:p", Scopes: service.AllScopes, ProjectID: "prj_0000000001",
		},
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

	// The reads and the full diagnostic report of a person do not download either.
	operatorCtx := service.WithCaller(context.Background(), operator())
	if _, err := m.svc.ToolsStatus(operatorCtx, service.ToolsStatusRequest{}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.Doctor(operatorCtx, service.DoctorRequest{Full: true}); err != nil {
		t.Fatal(err)
	}

	if n := cs.calls.Load(); n != 0 {
		t.Errorf("%d downloads without an install by a person", n)
	}
}
