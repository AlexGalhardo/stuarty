package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const allowFile = "# c\n--enable-zlib\n--enable-libvpx\n"

func TestCheckAcceptsAnAllowedLine(t *testing.T) {
	t.Parallel()

	line := "configuration: --disable-autodetect --enable-zlib --enable-libvpx --prefix=/x --disable-libx264"
	if got := check(line, allowedFlags(allowFile)); len(got) != 0 {
		t.Fatalf("unexpected problems: %v", got)
	}
}

func TestCheckRefusesGPLNonfreeAndOpenH264(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{
		"--enable-gpl", "--enable-nonfree", "--enable-libopenh264", "--enable-libx264",
		"--enable-libx265", "--enable-libfdk-aac", "--enable-openssl",
	} {
		got := check("--enable-zlib "+flag+"=x", allowedFlags(allowFile))
		if len(got) != 1 || !strings.Contains(got[0], "is refused") {
			t.Errorf("%s: want one refusal, got %v", flag, got)
		}
	}
}

func TestCheckRefusesAFlagOutsideTheAllowlist(t *testing.T) {
	t.Parallel()

	got := check("--enable-zlib --enable-libwebp --enable-libwebp", allowedFlags(allowFile))
	if len(got) != 1 || !strings.Contains(got[0], "not in the allowlist") {
		t.Fatalf("want one allowlist problem, got %v", got)
	}
}

func TestRunOnTheFlagFileOfTheRepository(t *testing.T) {
	t.Parallel()

	var out, errb bytes.Buffer

	code := run([]string{
		"-allow", filepath.FromSlash("../../../.config/ffmpeg-build/configure-allowlist.txt"),
		"-flags", filepath.FromSlash("../../../.config/ffmpeg-build/configure-flags.txt"),
	}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}

func TestRunReadsALineFromStandardInput(t *testing.T) {
	t.Parallel()

	allow := filepath.Join(t.TempDir(), "allow.txt")
	if err := os.WriteFile(allow, []byte(allowFile), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer

	code := run([]string{"-allow", allow, "-line", "-"}, strings.NewReader("configuration: --enable-gpl"), &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "--enable-gpl is refused") {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}

	if code = run([]string{"-allow", allow}, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("no input flag: exit %d, want 2", code)
	}
}
