package ffmpegpins

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestForKnownAndUnknownPlatforms(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"linux/amd64", "linux/arm64", "windows/amd64"} {
		goos, goarch, _ := strings.Cut(p, "/")

		pins := For(goos, goarch)
		for _, name := range []string{"ffmpeg", "ffprobe"} {
			if !hex64.MatchString(pins[name]) {
				t.Errorf("%s %s: %q is not a SHA-256", p, name, pins[name])
			}
		}
	}

	if For("darwin", "arm64") != nil {
		t.Error("darwin bundles no FFmpeg, so it has no pin")
	}

	For("linux", "amd64")["ffmpeg"] = "changed"

	if For("linux", "amd64")["ffmpeg"] == "changed" {
		t.Error("For returned the table itself")
	}
}

// The committed file is what the env file produces; CI also runs this through `task generate:check`.
func TestGeneratedFileMatchesTheEnvFile(t *testing.T) {
	t.Parallel()

	env := filepath.Join("..", "..", ".config", "ffmpeg.env")
	if _, err := os.Stat(env); err != nil {
		t.Skipf("no env file: %v", err)
	}

	cmd := exec.CommandContext(
		t.Context(), "go", "run", "../tools/ffmpegpins", "-env", env, "-out", "pins_gen.go", "-check",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
