package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleEnv = `# comment
FFMPEG_RELEASE_TAG="autobuild-x"
FFMPEG_SHA256_LINUX_AMD64="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
FFMPEG_BIN_SHA256_LINUX_AMD64="1111111111111111111111111111111111111111111111111111111111111111"
FFPROBE_BIN_SHA256_LINUX_AMD64="2222222222222222222222222222222222222222222222222222222222222222"
FFMPEG_BIN_SHA256_WINDOWS_AMD64="3333333333333333333333333333333333333333333333333333333333333333"
FFPROBE_BIN_SHA256_WINDOWS_AMD64="4444444444444444444444444444444444444444444444444444444444444444"
`

func TestGenerateIsDeterministicAndCoversEveryPlatform(t *testing.T) {
	t.Parallel()

	a, err := generate([]byte(sampleEnv))
	if err != nil {
		t.Fatal(err)
	}

	b, err := generate([]byte(sampleEnv))
	if err != nil || !bytes.Equal(a, b) {
		t.Fatalf("two runs differ: %v", err)
	}

	for _, want := range []string{
		`"linux/amd64"`, `"windows/amd64"`, `Release = "autobuild-x"`,
		strings.Repeat("1", 64), strings.Repeat("4", 64),
	} {
		if !strings.Contains(string(a), want) {
			t.Errorf("generated source lacks %s", want)
		}
	}

	if strings.Contains(string(a), strings.Repeat("a", 64)) {
		t.Error("an archive digest leaked into the binary pins")
	}
}

func TestGenerateRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"no tag":        strings.Replace(sampleEnv, "FFMPEG_RELEASE_TAG", "OTHER", 1),
		"short digest":  strings.Replace(sampleEnv, strings.Repeat("1", 64), "abc", 1),
		"upper digest":  strings.Replace(sampleEnv, strings.Repeat("1", 64), strings.Repeat("A", 64), 1),
		"missing probe": strings.Replace(sampleEnv, "FFPROBE_BIN_SHA256_WINDOWS_AMD64", "X", 1),
		"no pins":       "FFMPEG_RELEASE_TAG=\"t\"\n",
		"not key value": "FFMPEG_RELEASE_TAG\n",
	}

	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := generate([]byte(in)); err == nil {
				t.Error("accepted")
			}
		})
	}
}

func TestCheckFlagDetectsADriftedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	env, out := filepath.Join(dir, "ffmpeg.env"), filepath.Join(dir, "pins_gen.go")

	if err := os.WriteFile(env, []byte(sampleEnv), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	if code := run([]string{"-env", env, "-out", out, "-check"}, &stdout, &stderr); code != 1 {
		t.Fatalf("check with no file exited %d", code)
	}

	if code := run([]string{"-env", env, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("write exited %d: %s", code, stderr.String())
	}

	if code := run([]string{"-env", env, "-out", out, "-check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("check on a fresh file exited %d: %s", code, stderr.String())
	}

	changed := strings.Replace(sampleEnv, strings.Repeat("1", 64), strings.Repeat("9", 64), 1)
	if err := os.WriteFile(env, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"-env", env, "-out", out, "-check"}, &stdout, &stderr); code != 1 {
		t.Fatalf("check after the env changed exited %d", code)
	}
}

// FFMPEG_SOURCE=own selects the own build block: its tag and its digests, darwin included, and none of the BtbN ones.
func TestGenerateFollowsTheSourceOfThePin(t *testing.T) {
	t.Parallel()

	own := sampleEnv + `FFMPEG_SOURCE="own"
FFMPEG_OWN_TAG="ffmpeg-9.0.2-1"
FFMPEG_OWN_BIN_SHA256_LINUX_AMD64="5555555555555555555555555555555555555555555555555555555555555555"
FFPROBE_OWN_BIN_SHA256_LINUX_AMD64="6666666666666666666666666666666666666666666666666666666666666666"
FFMPEG_OWN_BIN_SHA256_DARWIN_ARM64="7777777777777777777777777777777777777777777777777777777777777777"
FFPROBE_OWN_BIN_SHA256_DARWIN_ARM64="8888888888888888888888888888888888888888888888888888888888888888"
`

	got, err := generate([]byte(own))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{`Release = "ffmpeg-9.0.2-1"`, `"darwin/arm64"`, strings.Repeat("5", 64), strings.Repeat("8", 64)} {
		if !strings.Contains(string(got), want) {
			t.Errorf("own pins lack %s", want)
		}
	}

	for _, leaked := range []string{"autobuild-x", strings.Repeat("1", 64), `"windows/amd64"`} {
		if strings.Contains(string(got), leaked) {
			t.Errorf("a BtbN value leaked into the own pins: %s", leaked)
		}
	}

	for name, in := range map[string]string{
		"own without a tag":   strings.Replace(own, "FFMPEG_OWN_TAG", "OTHER", 1),
		"own without digests": sampleEnv + "FFMPEG_SOURCE=\"own\"\nFFMPEG_OWN_TAG=\"t\"\n",
		"unknown source":      strings.Replace(own, `FFMPEG_SOURCE="own"`, `FFMPEG_SOURCE="x"`, 1),
	} {
		if _, err := generate([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
