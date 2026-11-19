package service

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

func ownBuild(flags ...string) *ffmpeg.Installation {
	return &ffmpeg.Installation{Info: ffmpeg.VersionInfo{
		Version: "9.0.2", Flags: append([]string{"--extra-version=vukem1"}, flags...),
		License: ffmpeg.License{Name: "lgpl"},
	}}
}

// SDD-011 section 8.6: ffmpeg.license names the flavour and the flags a distributed build must not
// carry, and is not ok when it has one.
func TestLicenseCheckNamesTheFlavourAndTheProblems(t *testing.T) {
	clean := licenseCheck(ownBuild(), false)
	if !clean.OK || !strings.Contains(clean.Detail, "flavour vukem") || strings.Contains(clean.Detail, "problems") {
		t.Errorf("clean own build = %+v", clean)
	}

	bad := licenseCheck(ownBuild("--enable-libopenh264"), false)
	if bad.OK || !strings.Contains(bad.Detail, "problems --enable-libopenh264") || bad.Hint == "" {
		t.Errorf("own build with OpenH264 compiled in = %+v", bad)
	}

	user := &ffmpeg.Installation{Info: ffmpeg.VersionInfo{
		Flags: []string{"--enable-gpl"}, License: ffmpeg.License{Name: "gpl"},
	}}
	if c := licenseCheck(user, false); !c.OK || !strings.Contains(c.Detail, "flavour other") {
		t.Errorf("a user build with GPL is the user's choice = %+v", c)
	}

	c := licenseCheck(user, false)
	if !strings.Contains(c.Hint, "--enable-gpl") || !strings.Contains(c.Hint, "STUARTY_FFMPEG") {
		t.Errorf("the GPL hint = %q", c.Hint)
	}

	text := doctorText(&DoctorResult{Checks: []DoctorCheck{c}})
	if !strings.Contains(text, "warn ffmpeg.license") {
		t.Errorf("a GPL build prints %q", text)
	}
}

// SDD-011 section 8.6: ffmpeg.openh264 exists for the own build only; it is ok when the library is
// installed and unchanged, or the machine has another working H.264 encoder, and its detail says
// the reason and the cost.
func TestOpenH264CheckReportsInstalledChangedAbsent(t *testing.T) {
	want := sha256.Sum256([]byte("0123456789"))
	m := ffmpeg.OpenH264Manifest{Hosts: []string{"ciscobinary.openh264.org"}, Entries: []ffmpeg.OpenH264Entry{{
		Name: ffmpeg.OpenH264Tool, Version: "2.6.0", OS: "testos", Arch: "testarch",
		URL: "https://ciscobinary.openh264.org/x.bz2", Bytes: 600000, SHA256: strings.Repeat("a", 64),
		File: "lib.so", LibraryBytes: 10, LibrarySHA256: hex.EncodeToString(want[:]), Published: true,
	}}}

	if _, ok := openH264Check(&ffmpeg.Installation{}, t.TempDir(), m, "testos", "testarch", false); ok {
		t.Error("a build that is not the own one has an openh264 check")
	}

	absent, ok := openH264Check(ownBuild(), t.TempDir(), m, "testos", "testarch", false)
	if !ok || absent.OK || absent.Name != "ffmpeg.openh264" ||
		!strings.Contains(absent.Detail, "openh264_not_installed") || !strings.Contains(absent.Detail, "0.6 MB") ||
		!strings.Contains(absent.Hint, "tools:install openh264") {
		t.Errorf("absent = %+v", absent)
	}

	if c, _ := openH264Check(ownBuild(), t.TempDir(), m, "testos", "testarch", true); !c.OK {
		t.Errorf("absent with another working encoder = %+v", c)
	}

	put := func(content string) string {
		dir := t.TempDir()
		lib := filepath.Join(dir, "openh264", "2.6.0")

		if err := os.MkdirAll(lib, 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(lib, "lib.so"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		return dir
	}

	if c, _ := openH264Check(ownBuild(), put("0123456789"), m, "testos", "testarch", false); !c.OK ||
		!strings.Contains(c.Detail, "installed") {
		t.Errorf("installed = %+v", c)
	}

	if c, _ := openH264Check(ownBuild(), put("9876543210"), m, "testos", "testarch", false); c.OK ||
		!strings.Contains(c.Detail, "openh264_changed") {
		t.Errorf("changed = %+v", c)
	}

	if c, _ := openH264Check(ownBuild(), t.TempDir(), m, "other", "testarch", false); c.OK ||
		!strings.Contains(c.Detail, "openh264_unsupported_platform") {
		t.Errorf("unsupported platform = %+v", c)
	}
}
