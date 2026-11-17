package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// EnvTestFFmpegDir names the bin directory of the pinned LGPL build for tests. scripts/install-ffmpeg.sh
// prints it. When it is unset the tests look where the script installs by default.
const envTestFFmpegDir = "VUKEM_VIDEO_TEST_FFMPEG_DIR"

// envTestToolsDir names the tools folder (<workspace>/tools) that holds the Cisco OpenH264 library the own
// build loads; the own build has no H.264 encoder without it.
const envTestToolsDir = "VUKEM_VIDEO_TEST_TOOLS_DIR"

var pinnedCache *Installation

// pinned returns the pinned LGPL FFmpeg, or skips the test. A test never downloads anything:
// run "bash scripts/install-ffmpeg.sh" first and export the printed folder as
// VUKEM_VIDEO_TEST_FFMPEG_DIR (it is also found in the default install location).
func pinned(t *testing.T) *Installation {
	t.Helper()
	if pinnedCache != nil {
		return pinnedCache
	}
	dir := os.Getenv(envTestFFmpegDir)
	if dir == "" {
		for _, base := range []string{os.Getenv("RUNNER_TEMP"), os.Getenv("TEMP"), os.Getenv("TMPDIR")} {
			if base == "" {
				continue
			}
			m, _ := filepath.Glob(filepath.Join(base, "ffmpeg", "*", "*", "bin", exeName("ffmpeg")))
			if len(m) > 0 {
				dir = filepath.Dir(m[len(m)-1])
				break
			}
		}
	}
	if dir == "" {
		t.Skip(
			"no pinned FFmpeg: run scripts/install-ffmpeg.sh and set " + envTestFFmpegDir + " to the folder it prints",
		)
	}
	in, err := Discover(
		context.Background(),
		Config{FFmpegPath: filepath.Join(dir, exeName("ffmpeg")), CPUCount: 2, ToolsDir: os.Getenv(envTestToolsDir)},
	)
	if err != nil {
		t.Skipf("the FFmpeg in %s is not usable: %v", envTestFFmpegDir, err)
	}
	if in.Info.License.Name != "lgpl" {
		t.Skipf("the FFmpeg in %s is %s, not the pinned LGPL build", envTestFFmpegDir, in.Info.License.Name)
	}
	pinnedCache = in
	return in
}

func pinnedRunner(t *testing.T) (*Runner, *RunDir) {
	t.Helper()
	in := pinned(t)
	r, err := NewRunner(in, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir, err := r.NewRunDir("job_1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Remove() })
	return r, dir
}

// putStore copies a file into the content store of the runner as <store>/<aa>/<sha256>.
func putStore(t *testing.T, r *Runner, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return putStoreBytes(t, r, b)
}

func putStoreBytes(t *testing.T, r *Runner, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	p := filepath.Join(r.StoreDir, h[:2], h)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// gen synthesizes a media fixture with the pinned FFmpeg (SDD-011 12): the arguments are
// literal test input, run straight through the tool, because they use muxers that the
// product itself never writes. out is a file name inside dir.
func gen(t *testing.T, in *Installation, dir *RunDir, out string, args ...string) string {
	t.Helper()
	full := append([]string{"-hide_banner", "-v", "error", "-nostdin"}, args...)
	full = append(full, out)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if res, err := in.FFmpeg.Run(ctx, dir.Dir(), full...); err != nil {
		t.Fatalf("generating %s: %v\n%s", out, err, res.Stderr)
	}
	return filepath.Join(dir.Dir(), out)
}
