package transcribe

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

func TestManifestHasEntryForEverySupportedPlatform(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range SupportedPlatforms {
		if _, ok := m.For(HelperName, DefaultModel, p[0], p[1]); !ok {
			t.Errorf("%s/%s: no helper (with the default model) in the manifest", p[0], p[1])
		}
	}

	if got := m.Models(HelperName); !slices.Equal(got, Models) {
		t.Errorf("models %v, want %v", got, Models)
	}

	if _, ok := m.For(HelperName, "custom", "linux", "amd64"); ok {
		t.Error("a model outside the manifest was accepted")
	}

	for i := range m.Entries {
		e := &m.Entries[i]
		if e.Kind == KindHelper && !slices.Contains(SupportedPlatforms, [2]string{e.OS, e.Arch}) {
			t.Errorf("entry %s/%s is not a supported platform", e.OS, e.Arch)
		}
	}
}

// A-18: every URL is https on a listed host, every published hash is 64 hex characters, and the notice
// text and licence are there for the notices step.
func TestManifestPinsAreWellFormed(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	published := 0

	for i := range m.Entries {
		e := &m.Entries[i]
		if e.License == "" || len(e.Notice) < 100 {
			t.Errorf("%s: licence or notice missing", e.File)
		}

		if !e.Published {
			if e.Kind != KindHelper ||
				!strings.Contains(e.URL, "/AlexGalhardo/vukem-video/releases/download/helpers-") {
				t.Errorf("%s: only a helper built by this repository may be unpublished", e.File)
			}

			continue
		}

		published++

		if !isHex64(e.SHA256) || e.Bytes <= 0 {
			t.Errorf("%s: size %d, sha %q", e.File, e.Bytes, e.SHA256)
		}
	}

	if published < 7 {
		t.Errorf("%d published entries, want 3 upstream helpers and 4 models", published)
	}

	bad := map[string]string{
		"an http URL":           `{"hosts":["x"],"entries":[{"kind":"model","model":"tiny","url":"http://x/y","file":"f"}]}`,
		"a host outside hosts":  `{"hosts":["x"],"entries":[{"kind":"model","model":"tiny","url":"https://y/y","file":"f"}]}`,
		"a custom model":        `{"hosts":["x"],"entries":[{"kind":"model","model":"mine","url":"https://x/y","file":"f"}]}`,
		"a traversal in a name": `{"hosts":["x"],"entries":[{"kind":"model","model":"tiny","url":"https://x/y","file":"../f"}]}`,
		"a short digest":        `{"hosts":["x"],"entries":[{"kind":"model","model":"tiny","url":"https://x/y","file":"f","published":true,"bytes":1,"sha256":"ab"}]}`,
		"a member that is a path": `{"hosts":["x"],"entries":[{"kind":"helper","os":"linux","arch":"amd64","url":"https://x/y","file":"w","published":true,"bytes":1,"sha256":"` +
			strings.Repeat(
				"a",
				64,
			) + `","format":"zip","members":[{"from":"a","to":"d/w"}]}]}`,
		"a program that is not a member": `{"hosts":["x"],"entries":[{"kind":"helper","os":"linux","arch":"amd64","url":"https://x/y","file":"w","published":true,"bytes":1,"sha256":"` +
			strings.Repeat(
				"a",
				64,
			) + `","format":"zip","members":[{"from":"a","to":"other"}]}]}`,
	}
	for name, doc := range bad {
		if _, err := ParseManifest([]byte(doc)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func sumHex(b []byte) string {
	s := sha256.Sum256(b)

	return hex.EncodeToString(s[:])
}

func writeZip(t *testing.T, path string, files map[string][]byte, order []string) {
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

	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
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

// Archives are extracted by the names of the manifest: an entry the manifest does not list is never
// written, whatever its name says.
func TestExtractHelperTakesOnlyTheMembersOfTheManifest(t *testing.T) {
	work := t.TempDir()
	archive := filepath.Join(work, "h.zip")
	writeZip(t, archive, map[string][]byte{
		"Release/whisper-cli": []byte("program"),
		"Release/lib.dll":     []byte("library"),
		"Release/extra.exe":   []byte("not listed"),
		"../evil":             []byte("traversal"),
		"/abs/evil":           []byte("absolute"),
	}, []string{"../evil", "/abs/evil", "Release/extra.exe", "Release/lib.dll", "Release/whisper-cli"})

	e := &ManifestEntry{Format: "zip", File: "whisper-cli", Members: []Member{
		{From: "Release/whisper-cli", To: "whisper-cli", Executable: true},
		{From: "Release/lib.dll", To: "lib.dll"},
	}}
	dst := filepath.Join(work, "dst")

	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}

	sum, err := ExtractHelper(archive, dst, e)
	if err != nil || sum != sumHex([]byte("program")) {
		t.Fatalf("sum %q, err %v", sum, err)
	}

	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
	}

	if !slices.Equal(names, []string{"lib.dll", "whisper-cli"}) {
		t.Errorf("extracted %v, want only the listed members", names)
	}

	for _, p := range []string{filepath.Join(work, "evil"), filepath.Join(dst, "evil"), filepath.Join(filepath.Dir(work), "evil")} {
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

func TestExtractHelperRefusesAMissingMember(t *testing.T) {
	work := t.TempDir()
	archive := filepath.Join(work, "h.zip")
	writeZip(t, archive, map[string][]byte{"a": []byte("x")}, []string{"a"})

	e := &ManifestEntry{
		Format:  "zip",
		File:    "whisper-cli",
		Members: []Member{{From: "whisper-cli", To: "whisper-cli", Executable: true}},
	}

	if _, err := ExtractHelper(archive, work, e); reasonOf(t, err) != ReasonBadArchive {
		t.Errorf("err = %v", err)
	}
}

func TestExtractHelperFromATarWithLinks(t *testing.T) {
	work := t.TempDir()

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

	archive := filepath.Join(work, "h.tgz")
	if err := os.WriteFile(archive, raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	e := &ManifestEntry{Format: "tar.gz", File: "whisper-cli", Members: []Member{
		{From: "pkg/whisper-cli", To: "whisper-cli", Executable: true},
		{From: "pkg/libx.so.1.0", To: "libx.so.1.0"},
	}, Links: []Link{{Name: "libx.so", Target: "libx.so.1.0"}}}
	dst := filepath.Join(work, "dst")

	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := ExtractHelper(archive, dst, e); err != nil {
		t.Fatal(err)
	}

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

// Starting a transcription with no helper installed makes no connection and says how to install it.
func TestJobNeverDownloads(t *testing.T) {
	var dials int

	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error {
		dials++

		return errors.New("no connection is allowed")
	})
	defer restore()

	w := &Whisper{
		ModelName:  DefaultModel,
		Helper:     filepath.Join(t.TempDir(), "none"),
		ModelPath:  filepath.Join(t.TempDir(), "none.bin"),
		HelperItem: Item{Name: HelperName, URLHost: "github.com", Bytes: 8194445},
		ModelItem:  Item{Name: DefaultModel, URLHost: "huggingface.co", Bytes: 487601967},
	}

	_, err := w.Transcribe(context.Background(), Input{Audio: "x.wav", Duration: 1}, nil)

	missing, ok := errors.AsType[*MissingError](err)
	if !ok {
		t.Fatalf("err = %v, want *MissingError", err)
	}

	fe := UnavailableError(missing, w.ModelName)
	if fe.Code != CodeUnavailable ||
		fe.Details["install_command"] != "vukem-video tools:install whisper --model small" ||
		fe.Details["tool"] != "whisper" || fe.Details["model"] != "small" {
		t.Errorf("error %+v", fe)
	}

	if b, _ := fe.Details["download_bytes"].(int64); b != 8194445+487601967 {
		t.Errorf("download_bytes %v", fe.Details["download_bytes"])
	}

	if dials != 0 {
		t.Errorf("%d dials, want 0", dials)
	}
}

func TestAnnounceStatesSizeAndSource(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	entries, ok := m.For(HelperName, "small", "windows", "amd64")
	if !ok {
		t.Fatal("no entries")
	}

	got := Announce(entries...)
	for _, want := range []string{"whisper-cli.exe", "github.com", "ggml-small.bin", "huggingface.co", "495.8 MB"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
}

// NFR-043: every component of the manifest is under a permissive licence the product may redistribute,
// and the text of that licence is in licenses/programs for the notices step.
func TestManifestLicencesAreRedistributable(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	dirs := map[string]string{KindHelper: "whisper.cpp", KindModel: "whisper-model"}
	for i := range m.Entries {
		e := &m.Entries[i]
		if e.License != "MIT" {
			t.Errorf("%s: licence %q is not on the allowlist (MIT)", e.File, e.License)
		}

		text, err := os.ReadFile(filepath.Join("..", "..", "licenses", "programs", dirs[e.Kind], "LICENSE"))
		if err != nil || !strings.HasPrefix(string(text), "MIT License") {
			t.Errorf("%s: the licence text of %s is missing or not MIT: %v", e.File, dirs[e.Kind], err)
		}
	}
}
