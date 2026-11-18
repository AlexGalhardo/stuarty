package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

const ciscoHost = "ciscobinary.openh264.org"

// ciscoPlatforms are the platforms that ship the own build and so need the Cisco library. They equal
// transcribe.SupportedPlatforms; the package cannot import it (transcribe imports this package).
var ciscoPlatforms = [][2]string{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// ciscoPinProblems lists what is wrong with the pins of the Cisco library: every platform has an entry,
// published, on https on the one Cisco host with no credentials, with both sizes and both digests.
func ciscoPinProblems(m OpenH264Manifest) []string {
	var out []string
	add := func(s string) { out = append(out, s) }

	if !slices.Equal(m.Hosts, []string{ciscoHost}) {
		add("hosts are not exactly " + ciscoHost)
	}
	seenDigest := map[string]bool{}
	for _, p := range ciscoPlatforms {
		e, ok := m.For(p[0], p[1])
		if !ok {
			add(p[0] + "/" + p[1] + ": no entry")
			continue
		}
		who := p[0] + "/" + p[1]
		u, err := url.Parse(e.URL)
		switch {
		case err != nil || u.Scheme != "https" || u.Host != ciscoHost || u.User != nil || u.RawQuery != "" ||
			u.Fragment != "":
			add(who + ": url is not a plain https URL on the Cisco host: " + e.URL)
		case !strings.HasSuffix(path.Base(u.Path), ".bz2") || !strings.Contains(u.Path, e.Version):
			add(who + ": url does not name a versioned .bz2 file")
		}
		if !e.Published || e.Bytes <= 0 || e.LibraryBytes <= 0 || !isHex64(e.SHA256) || !isHex64(e.LibrarySHA256) {
			add(who + ": not published with exact sizes and digests")
		}
		if !plainName(e.File) || e.License != "BSD-2-Clause" || e.Name != OpenH264Tool {
			add(who + ": file name, licence or tool name")
		}
		for _, d := range []string{e.SHA256, e.LibrarySHA256} {
			if seenDigest[d] {
				add(who + ": a digest is shared with another entry")
			}
			seenDigest[d] = true
		}
	}
	return out
}

// TestSecurity_T27_CiscoLibraryPinnedOnEveryPlatform extends T-27 to the Cisco library of the own build
// (SDD-007 6.8): the embedded manifest pins every supported platform, and a blanked or altered pin is found.
func TestSecurity_T27_CiscoLibraryPinnedOnEveryPlatform(t *testing.T) {
	m, err := DefaultOpenH264Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ciscoPinProblems(m) {
		t.Error(p)
	}

	mutate := map[string]func(e *OpenH264Entry){
		"blank archive digest": func(e *OpenH264Entry) { e.SHA256 = "" },
		"blank library digest": func(e *OpenH264Entry) { e.LibrarySHA256 = "" },
		"upper case digest":    func(e *OpenH264Entry) { e.SHA256 = strings.ToUpper(e.SHA256) },
		"zero size":            func(e *OpenH264Entry) { e.Bytes = 0 },
		"zero library size":    func(e *OpenH264Entry) { e.LibraryBytes = 0 },
		"http":                 func(e *OpenH264Entry) { e.URL = strings.Replace(e.URL, "https://", "http://", 1) },
		"other host":           func(e *OpenH264Entry) { e.URL = "https://evil.example/" + path.Base(e.URL) },
		"credentials":          func(e *OpenH264Entry) { e.URL = strings.Replace(e.URL, "https://", "https://u:p@", 1) },
		"lookalike host":       func(e *OpenH264Entry) { e.URL = strings.Replace(e.URL, ciscoHost, ciscoHost+".evil.io", 1) },
		"unpublished":          func(e *OpenH264Entry) { e.Published = false },
		"path in file name":    func(e *OpenH264Entry) { e.File = "../" + e.File },
	}
	for name, f := range mutate {
		for i := range m.Entries {
			cp := OpenH264Manifest{Hosts: m.Hosts, Entries: slices.Clone(m.Entries)}
			f(&cp.Entries[i])
			if len(ciscoPinProblems(cp)) == 0 {
				t.Errorf("%s on entry %d passed the pin check", name, i)
			}
		}
	}
	for i := range m.Entries {
		cp := OpenH264Manifest{Hosts: m.Hosts, Entries: slices.Delete(slices.Clone(m.Entries), i, i+1)}
		if len(ciscoPinProblems(cp)) == 0 {
			t.Errorf("a manifest without entry %d passed", i)
		}
	}
	if len(ciscoPinProblems(OpenH264Manifest{Hosts: []string{ciscoHost, "github.com"}, Entries: m.Entries})) == 0 {
		t.Error("an extra host passed")
	}
}

// ownPinKeys are the keys .config/ffmpeg.env must hold for the own build of D-017 once a build is released.
var ownPinPlatforms = []string{"LINUX_AMD64", "LINUX_ARM64", "WINDOWS_AMD64", "DARWIN_AMD64", "DARWIN_ARM64"}

// ownPinProblems checks the own-build block of ffmpeg.env. An empty FFMPEG_OWN_TAG means no build has been
// released: the block is then only allowed to be inactive (FFMPEG_SOURCE is not "own").
func ownPinProblems(env map[string]string) []string {
	var out []string
	add := func(s string) { out = append(out, s) }

	if env["FFMPEG_OWN_TAG"] == "" {
		if env["FFMPEG_SOURCE"] == "own" {
			add("FFMPEG_SOURCE is own but FFMPEG_OWN_TAG is empty")
		}
		return out
	}
	if env["FFMPEG_OWN_VERSION"] == "" || !strings.HasPrefix(env["FFMPEG_OWN_SOURCE_URL"], "https://") ||
		!isHex64(env["FFMPEG_OWN_SOURCE_SHA256"]) {
		add("own build: version, https source URL or source sha256 missing")
	}
	for _, p := range ownPinPlatforms {
		if env["FFMPEG_OWN_ASSET_"+p] == "" {
			add("own build " + p + ": no asset name")
		}
		for _, k := range []string{"FFMPEG_OWN_SHA256_", "FFMPEG_OWN_BIN_SHA256_", "FFPROBE_OWN_BIN_SHA256_"} {
			if !isHex64(env[k+p]) {
				add("own build " + p + ": " + k + " is not a SHA-256")
			}
		}
	}
	return out
}

func readEnvFile(t *testing.T, name string) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", ".config", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s: unreadable line %q", name, line)
		}
		env[k] = strings.Trim(v, `"`)
	}
	return env
}

// TestSecurity_T27_OwnBuildPinned extends T-27 to the own FFmpeg build: once .config/ffmpeg.env names a
// released own build, every platform has an asset, the archive digest and the digests of both binaries. The
// checker is proven on a synthetic block here; the real block waits for the first release of the build.
func TestSecurity_T27_OwnBuildPinned(t *testing.T) {
	full := map[string]string{
		"FFMPEG_SOURCE": "own", "FFMPEG_OWN_TAG": "ffmpeg-own-1", "FFMPEG_OWN_VERSION": "n9.0.2-vukem1",
		"FFMPEG_OWN_SOURCE_URL": "https://example.test/src.tar.gz", "FFMPEG_OWN_SOURCE_SHA256": strings.Repeat("a", 64),
	}
	for _, p := range ownPinPlatforms {
		full["FFMPEG_OWN_ASSET_"+p] = "ffmpeg-" + p + ".tar.xz"
		for _, k := range []string{"FFMPEG_OWN_SHA256_", "FFMPEG_OWN_BIN_SHA256_", "FFPROBE_OWN_BIN_SHA256_"} {
			full[k+p] = strings.Repeat("b", 64)
		}
	}
	if p := ownPinProblems(full); len(p) != 0 {
		t.Fatalf("a complete block has problems: %v", p)
	}
	for k := range full {
		if k == "FFMPEG_SOURCE" {
			continue
		}
		broken := maps.Clone(full)
		broken[k] = ""
		if len(ownPinProblems(broken)) == 0 {
			t.Errorf("a blank %s passed", k)
		}
		if strings.Contains(k, "SHA256") {
			broken[k] = strings.Repeat("B", 64)
			if len(ownPinProblems(broken)) == 0 {
				t.Errorf("an upper-case %s passed", k)
			}
		}
	}
	if len(ownPinProblems(map[string]string{"FFMPEG_SOURCE": "own"})) == 0 {
		t.Error("the own source with no tag passed")
	}

	env := readEnvFile(t, "ffmpeg.env")
	for _, p := range ownPinProblems(env) {
		t.Error(p)
	}
	if env["FFMPEG_OWN_TAG"] == "" {
		t.Log("no own FFmpeg build is released yet: the pins of the real block are not checked")
	}
}

// bodyOf is a download whose body is the given reader and whose request URL is reqURL (the last hop).
type bodyClient struct {
	calls  atomic.Int32
	body   io.Reader
	reqURL string
}

func (c *bodyClient) Do(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	final := req
	if c.reqURL != "" {
		u, _ := url.Parse(c.reqURL)
		final = req.Clone(req.Context())
		final.URL = u
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(c.body), Request: final}, nil
}

type endless struct{ read atomic.Int64 }

func (e *endless) Read(b []byte) (int, error) {
	e.read.Add(int64(len(b)))
	clear(b)
	return len(b), nil
}

// entryWith returns the manifest of fakeLibrary with the mutation applied to its entry.
func entryWith(t *testing.T, f func(*OpenH264Entry)) (OpenH264Manifest, []byte) {
	t.Helper()
	m, archive, _ := fakeLibrary(t)
	f(&m.Entries[0])
	return m, archive
}

// TestSecurity_T26_CiscoDownloadRefusals extends T-26 to the Cisco install: an http:// URL, a host outside
// the allowlist, a redirect to one, an answer from another host, a body that never ends and a
// decompression bomb are each refused, and nothing is left under tools/.
func TestSecurity_T26_CiscoDownloadRefusals(t *testing.T) {
	ctx := context.Background()
	install := func(m OpenH264Manifest, c Downloader) (string, error) {
		dir := t.TempDir()
		_, err := InstallOpenH264(ctx, InstallOpenH264Options{ToolsDir: dir, Manifest: m, Client: c})
		return dir, err
	}
	wantReason := func(t *testing.T, err error, reason string) {
		t.Helper()
		var fe *Error
		if !errors.As(err, &fe) || fe.Code != CodeNoEncoder || fe.Reason != reason {
			t.Fatalf("error = %v, want %s", err, reason)
		}
	}

	t.Run("an http URL in the manifest is never requested", func(t *testing.T) {
		m, archive := entryWith(t, func(e *OpenH264Entry) { e.URL = "http://" + ciscoHost + "/fake.bz2" })
		c := &countingClient{body: archive}
		dir, err := install(m, c)
		wantReason(t, err, ReasonDownloadFailed)
		if c.calls.Load() != 0 {
			t.Errorf("%d requests were made", c.calls.Load())
		}
		noLibraryLeft(t, dir)
	})

	t.Run("a host outside the manifest hosts is never requested", func(t *testing.T) {
		m, archive := entryWith(t, func(e *OpenH264Entry) { e.URL = "https://evil.example/fake.bz2" })
		c := &countingClient{body: archive}
		dir, err := install(m, c)
		wantReason(t, err, ReasonDownloadFailed)
		if c.calls.Load() != 0 {
			t.Errorf("%d requests were made", c.calls.Load())
		}
		noLibraryLeft(t, dir)
	})

	t.Run("an answer that came from another host is refused", func(t *testing.T) {
		m, archive := entryWith(t, func(*OpenH264Entry) {})
		for _, final := range []string{"https://evil.example/x.bz2", "http://" + ciscoHost + "/x.bz2"} {
			dir, err := install(m, &bodyClient{body: bytes.NewReader(archive), reqURL: final})
			wantReason(t, err, ReasonDownloadFailed)
			noLibraryLeft(t, dir)
			if st := CheckOpenH264(dir, m, "", ""); st.Installed {
				t.Errorf("%s: a library is installed", final)
			}
		}
	})

	t.Run("the real install client refuses a redirect out of the allowlist", func(t *testing.T) {
		m, _ := entryWith(t, func(e *OpenH264Entry) { e.URL = "https://" + ciscoHost + "/fake.bz2" })
		for _, target := range []string{
			"https://evil.example/fake.bz2",
			"http://" + ciscoHost + "/fake.bz2",
			"https://" + ciscoHost + ".evil.io/fake.bz2",
		} {
			client := outbound.HelperDownload(m.Hosts, 2*time.Second)
			var hops atomic.Int32
			base := client.Transport
			// The Cisco host answers with a redirect without any network; every other request goes to the
			// client's own checked transport, which refuses it before dialling.
			client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == ciscoHost && r.URL.Scheme == "https" && hops.Add(1) == 1 {
					return &http.Response{
						StatusCode: http.StatusFound, Header: http.Header{"Location": {target}},
						Body: http.NoBody, Request: r,
					}, nil
				}
				return base.RoundTrip(r)
			})
			dir, err := install(m, client)
			wantReason(t, err, ReasonDownloadFailed)
			noLibraryLeft(t, dir)
		}
	})

	t.Run("a body that never ends is cut at the pinned size", func(t *testing.T) {
		m, _ := entryWith(t, func(*OpenH264Entry) {})
		e := &endless{}
		dir, err := install(m, &bodyClient{body: e})
		wantReason(t, err, ReasonSizeMismatch)
		noLibraryLeft(t, dir)
		if got, limit := e.read.Load(), m.Entries[0].Bytes+1+64<<10; got > limit {
			t.Errorf("read %d bytes of an endless body, pinned size %d", got, m.Entries[0].Bytes)
		}
	})

	t.Run("a bzip2 bomb is cut at the library size", func(t *testing.T) {
		bomb, err := os.ReadFile(filepath.Join("testdata", "openh264", "bomb.bz2"))
		if err != nil {
			t.Fatal(err)
		}
		m, _ := entryWith(t, func(e *OpenH264Entry) {
			e.Bytes, e.SHA256 = int64(len(bomb)), hexSum(bomb)
		})
		start := time.Now()
		dir, err := install(m, &countingClient{body: bomb})
		wantReason(t, err, ReasonSizeMismatch)
		noLibraryLeft(t, dir)
		// The bomb expands to 1 GiB; cut at the pinned size it takes milliseconds, uncut it takes tens of seconds.
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("the bomb took %v: it was not cut", d)
		}
	})

	t.Run("a context cancelled during the download leaves nothing", func(t *testing.T) {
		m, archive := entryWith(t, func(*OpenH264Entry) {})
		cctx, cancel := context.WithCancel(ctx)
		dir := t.TempDir()
		_, err := InstallOpenH264(cctx, InstallOpenH264Options{
			ToolsDir: dir, Manifest: m, Client: &countingClient{body: archive},
			Progress: func(int64, int64) { cancel() },
		})
		cancel()
		// Whether the cancel lands before or after the last byte, no library may remain unless it verified.
		if err != nil {
			noLibraryLeft(t, dir)
		}
	})

	t.Run("a failed attempt keeps a good install intact", func(t *testing.T) {
		m, archive, library := fakeLibrary(t)
		dir := t.TempDir()
		if _, err := InstallOpenH264(
			ctx,
			InstallOpenH264Options{ToolsDir: dir, Manifest: m, Client: &countingClient{body: archive}},
		); err != nil {
			t.Fatal(err)
		}
		bad := bytes.Clone(archive)
		bad[len(bad)-1] ^= 0xff
		if _, err := InstallOpenH264(
			ctx,
			InstallOpenH264Options{ToolsDir: dir, Manifest: m, Client: &countingClient{body: bad}},
		); err == nil {
			t.Fatal("a tampered archive installed")
		}
		got, err := os.ReadFile(OpenH264Path(dir, m.Entries[0]))
		if err != nil || !bytes.Equal(got, library) {
			t.Errorf("the installed library changed: %v", err)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, ".download")); len(entries) != 0 {
			t.Errorf("download folder not empty: %v", entries)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// NFR-043: the Cisco library and the own build are under licences the product may redistribute or
// download, and the text of each is in licenses/programs for the notices step.
func TestSecurity_T27_ShippedComponentsHaveLicenceText(t *testing.T) {
	m, err := DefaultOpenH264Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Entries {
		if e.License != "BSD-2-Clause" {
			t.Errorf("%s: licence %q is not on the allowlist (BSD-2-Clause)", e.File, e.License)
		}
	}

	for dir, want := range map[string]string{
		"openh264": "Cisco Systems",
		"ffmpeg":   "GNU LESSER GENERAL PUBLIC LICENSE",
	} {
		files, err := filepath.Glob(filepath.Join("..", "..", "licenses", "programs", dir, "*"))
		if err != nil || len(files) == 0 {
			t.Errorf("licenses/programs/%s has no licence text", dir)
			continue
		}
		text, err := os.ReadFile(files[0])
		if err != nil || !strings.Contains(string(text), want) {
			t.Errorf("licenses/programs/%s: the text does not name %q: %v", dir, want, err)
		}
	}
}
