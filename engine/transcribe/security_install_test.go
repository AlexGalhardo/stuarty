package transcribe

import (
	"bufio"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// unpublishedHelpers are the helpers that wait for a run of helpers.yml: their size and digest are not
// known, and the install refuses them with reason not_published. Anything else must be published. Empty
// since the helpers-v1.9.2 release (run 43729041820) published darwin/amd64, darwin/arm64 and windows/arm64.
var unpublishedHelpers = [][2]string{}

// ffmpegPinPlatforms are the platforms of .config/ffmpeg.env (FFMPEG_ASSET_* and FFMPEG_SHA256_*). The
// other supported platforms have no pinned FFmpeg build and ship none.
var ffmpegPinPlatforms = []string{"LINUX_AMD64", "LINUX_ARM64", "WINDOWS_AMD64"}

func readFFmpegEnv(t *testing.T) map[string]string {
	t.Helper()

	f, err := os.Open(filepath.Join("..", "..", ".config", "ffmpeg.env"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	env := map[string]string{}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("ffmpeg.env: unreadable line %q", line)
		}

		env[k] = strings.Trim(v, `"`)
	}

	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	return env
}

// pinProblems lists what is missing from the pins of the helpers, the models and the FFmpeg build (which
// holds ffprobe in the same archive, so one asset and one digest pin both).
func pinProblems(m Manifest, ffEnv map[string]string) []string {
	var out []string

	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	hostOK := func(raw string) bool {
		u, err := url.Parse(raw)

		return err == nil && u.Scheme == "https" && slices.Contains(m.Hosts, u.Host)
	}

	for _, p := range SupportedPlatforms {
		var helper *ManifestEntry

		for i := range m.Entries {
			if e := &m.Entries[i]; e.Kind == KindHelper && e.OS == p[0] && e.Arch == p[1] {
				helper = e
			}
		}

		if helper == nil {
			add("helper %s/%s: no entry", p[0], p[1])

			continue
		}

		if helper.Published == slices.Contains(unpublishedHelpers, p) {
			add("helper %s/%s: published = %v, expected the opposite", p[0], p[1], helper.Published)
		}

		if !helper.Published {
			if !strings.Contains(helper.URL, "/AlexGalhardo/stuarty/releases/download/helpers-") {
				add("helper %s/%s: unpublished and not from this repository's helpers release", p[0], p[1])
			}

			continue
		}

		checkPin(add, hostOK, "helper "+p[0]+"/"+p[1], helper)
	}

	for _, name := range Models {
		var found *ManifestEntry

		for i := range m.Entries {
			if e := &m.Entries[i]; e.Kind == KindModel && e.Model == name {
				found = e
			}
		}

		if found == nil {
			add("model %s: no entry", name)

			continue
		}

		checkPin(add, hostOK, "model "+name, found)
	}

	for _, p := range ffmpegPinPlatforms {
		if !isHex64(ffEnv["FFMPEG_SHA256_"+p]) {
			add("ffmpeg %s: sha256 %q", p, ffEnv["FFMPEG_SHA256_"+p])
		}

		if ffEnv["FFMPEG_ASSET_"+p] == "" {
			add("ffmpeg %s: no asset name", p)
		}
	}

	if ffEnv["FFMPEG_RELEASE_TAG"] == "" || !strings.HasPrefix(ffEnv["FFMPEG_SOURCE_URL"], "https://") {
		add("ffmpeg: release tag or source URL missing")
	}

	return out
}

func checkPin(add func(string, ...any), hostOK func(string) bool, what string, e *ManifestEntry) {
	if !hostOK(e.URL) {
		add("%s: url %q is not https on a listed host", what, e.URL)
	}

	if e.Bytes <= 0 {
		add("%s: size %d", what, e.Bytes)
	}

	if !isHex64(e.SHA256) {
		add("%s: sha256 %q", what, e.SHA256)
	}
}

// SDD-007 13, T-27: the embedded manifest has a URL, size and SHA-256 for every helper and model, and
// .config/ffmpeg.env has an asset and SHA-256 for FFmpeg (and so ffprobe) on every platform that ships one.
// The helpers of the three platforms waiting for helpers.yml are asserted unpublished, never silently empty.
// ffmpeg.env records no size: the digest of the archive is the pin, and the install script checks it.
func TestSecurity_T27_ManifestPinsEveryHelper(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	env := readFFmpegEnv(t)

	for _, p := range pinProblems(m, env) {
		t.Error(p)
	}

	t.Run("a blanked digest fails", func(t *testing.T) {
		for i := range m.Entries {
			if !m.Entries[i].Published {
				continue
			}

			cp := m
			cp.Entries = slices.Clone(m.Entries)
			cp.Entries[i].SHA256 = ""

			if len(pinProblems(cp, env)) == 0 {
				t.Errorf("entry %s with a blank sha256 passed", m.Entries[i].File)
			}

			cp.Entries[i].SHA256 = m.Entries[i].SHA256
			cp.Entries[i].Bytes = 0

			if len(pinProblems(cp, env)) == 0 {
				t.Errorf("entry %s with a zero size passed", m.Entries[i].File)
			}
		}

		for _, p := range ffmpegPinPlatforms {
			cp := maps.Clone(env)
			cp["FFMPEG_SHA256_"+p] = ""

			if len(pinProblems(m, cp)) == 0 {
				t.Errorf("ffmpeg %s with a blank sha256 passed", p)
			}
		}
	})
}
