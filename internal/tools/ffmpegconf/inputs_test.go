package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The inputs of the own build are read by shell scripts and by the engine, so a drift between them is
// found only at the end of a long build. These tests read the same files and fail early.

func repoFile(t *testing.T, rel string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.FromSlash("../../../" + rel))
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func sourcesValue(t *testing.T, key string) string {
	t.Helper()

	env := string(repoFile(t, ".config/ffmpeg-build/sources.env"))
	m := regexp.MustCompile(`(?m)^` + key + `="([^"]*)"`).FindStringSubmatch(env)
	if m == nil {
		t.Fatalf("%s is not in sources.env", key)
	}

	return m[1]
}

func TestPatchDigestIsTheOneSourcesEnvPins(t *testing.T) {
	t.Parallel()

	patch := repoFile(t, ".config/ffmpeg-build/patches/"+sourcesValue(t, "FFMPEG_PATCH_FILE"))
	sum := sha256.Sum256(patch)

	if got, want := hex.EncodeToString(sum[:]), sourcesValue(t, "FFMPEG_PATCH_SHA256"); got != want {
		t.Fatalf("the patch hashes to %s but sources.env pins %s (FFMPEG_BUILD_TAG changes with the patch)", got, want)
	}
}

// TestOwnBuildNamesAreTheStuartyOnes pins VV-231: the patch reads STUARTY_OPENH264_DIR, the build marks itself
// stuarty<N>, and the verify script asks for both. The former names live only in the engine, which still accepts
// the builds installed before.
func TestOwnBuildNamesAreTheStuartyOnes(t *testing.T) {
	t.Parallel()

	patch := string(repoFile(t, ".config/ffmpeg-build/patches/"+sourcesValue(t, "FFMPEG_PATCH_FILE")))
	if !strings.Contains(patch, `getenv("STUARTY_OPENH264_DIR")`) || strings.Contains(patch, "VUKEM_VIDEO") {
		t.Error("the patch must read STUARTY_OPENH264_DIR and no former name")
	}

	build := string(repoFile(t, "scripts/ffmpeg-build.sh"))
	if !strings.Contains(build, `--extra-version=stuarty${FFMPEG_BUILD_TAG##*-}`) {
		t.Error("ffmpeg-build.sh must mark the build with --extra-version=stuarty<N>")
	}

	verify := string(repoFile(t, "scripts/ffmpeg-verify.sh"))
	hasMarker := strings.Contains(verify, "--extra-version=stuarty*")
	if !hasMarker || !strings.Contains(verify, "grep -aq STUARTY_OPENH264_DIR") {
		t.Error("ffmpeg-verify.sh must require the stuarty marker and STUARTY_OPENH264_DIR")
	}
}

// TestBuildTagNumberIsTheMarkerNumber: the marker is the last number of the tag, so a new tag is a new marker.
func TestBuildTagNumberIsTheMarkerNumber(t *testing.T) {
	t.Parallel()

	tag := sourcesValue(t, "FFMPEG_BUILD_TAG")
	version := regexp.QuoteMeta(sourcesValue(t, "FFMPEG_BUILD_VERSION"))
	if !regexp.MustCompile(`^ffmpeg-` + version + `-[0-9]+$`).MatchString(tag) {
		t.Fatalf("FFMPEG_BUILD_TAG %q is not ffmpeg-<version>-<number>", tag)
	}
}

// TestEveryFilterTheEngineAllowsIsRequiredOfTheBuild: the filters of engine/ffmpeg/ffargs/filter.go that the
// build must provide are the lines of scripts/ffmpeg-filters.txt.
func TestEveryFilterTheEngineAllowsIsRequiredOfTheBuild(t *testing.T) {
	t.Parallel()

	src := string(repoFile(t, "engine/ffmpeg/ffargs/filter.go"))
	start := strings.Index(src, "var filters = map[string]filterSpec{")
	end := strings.Index(src, "// lavfiSources")

	if start < 0 || end < start {
		t.Fatal("the filter table of ffargs was not found")
	}

	listed := map[string]bool{}

	for line := range strings.SplitSeq(string(repoFile(t, "scripts/ffmpeg-filters.txt")), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			listed[line] = true
		}
	}

	for _, m := range regexp.MustCompile(`(?m)^\t"([a-z0-9]+)":\s*\{`).FindAllStringSubmatch(src[start:end], -1) {
		if !listed[m[1]] {
			t.Errorf("the engine allows the filter %s but scripts/ffmpeg-filters.txt does not require it", m[1])
		}
	}

	for _, gpl := range []string{"eq", "boxblur"} {
		if listed[gpl] {
			t.Errorf("%s is GPL only and must not be required", gpl)
		}
	}
}

// TestHeaderOnlyLicencesShipWithTheBuild: the NVENC and AMF header archives carry no licence file, so the build
// copies the MIT texts of licenses/programs into the package; both files must exist and be named by the script.
func TestHeaderOnlyLicencesShipWithTheBuild(t *testing.T) {
	t.Parallel()

	build := string(repoFile(t, "scripts/ffmpeg-build.sh"))

	for _, rel := range []string{"nv-codec-headers/LICENSE", "amf/LICENSE.txt"} {
		if !strings.Contains(build, rel) {
			t.Errorf("ffmpeg-build.sh does not copy licenses/programs/%s into the package", rel)
		}

		if len(repoFile(t, "licenses/programs/"+rel)) == 0 {
			t.Errorf("licenses/programs/%s is empty", rel)
		}
	}
}
