package capturehost

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// Inspect reads a recording that lies outside the content store, under the argument policy of FFmpeg: the folder of
// the recording is the one extra input directory (FF-03).
//
// verify:env-skip needs the pinned FFmpeg (STUARTY_TEST_FFMPEG_DIR); without it the test says why and passes.
func TestLiveInspectReadsAFileOutsideTheStore(t *testing.T) {
	dir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")
	if dir == "" {
		t.Skip("verify:env-skip STUARTY_TEST_FFMPEG_DIR names no pinned FFmpeg")
	}

	exe := "ffmpeg"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}

	in, err := ffmpeg.Discover(context.Background(), ffmpeg.Config{FFmpegPath: filepath.Join(dir, exe), CPUCount: 2})
	if err != nil {
		t.Skipf("verify:env-skip the FFmpeg of STUARTY_TEST_FFMPEG_DIR cannot be used: %v", err)
	}

	ws := t.TempDir()

	r, err := ffmpeg.NewRunner(in, ws)
	if err != nil {
		t.Fatal(err)
	}

	folder := filepath.Join(ws, "recordings", "rec_0000000001")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(folder, "capture.mp4")
	if out, err := exec.CommandContext(t.Context(), in.FFmpeg.Path(), "-v", "error", "-f", "lavfi", "-i",
		"testsrc=size=320x240:rate=30:duration=1", "-pix_fmt", "yuv420p", "-movflags",
		"+frag_keyframe+empty_moov+default_base_moof", file).CombinedOutput(); err != nil {
		t.Fatalf("making the sample: %v: %s", err, out)
	}

	live := &Live{Runner: r}

	got, err := live.Inspect(t.Context(), file)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if got.Width != 320 || got.Height != 240 || got.DurationMS < 900 || got.DurationMS > 1200 {
		t.Errorf("Inspect = %+v", got)
	}

	if err := os.WriteFile(filepath.Join(folder, "empty.mp4"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := live.Inspect(t.Context(), filepath.Join(folder, "empty.mp4")); err == nil {
		t.Error("an empty file has nothing to decode")
	}
}

func TestLiveRefusesAHelperThatIsNotThere(t *testing.T) {
	for _, path := range []string{"", "relative/stuarty-capture.exe", filepath.Join(t.TempDir(), "missing.exe")} {
		l := &Live{HelperPath: path}
		if err := l.check(); err == nil {
			t.Errorf("%q must be refused", path)
		}
	}
}

func TestLiveSourcesUsesTheConfiguredHelper(t *testing.T) {
	env := newEnv(t, "")
	live := &Live{Runner: env.r, HelperPath: env.path}

	sources, err := live.Sources(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if len(sources.Displays) != 1 || sources.Displays[0].ID != "d1" {
		t.Fatalf("unexpected displays: %+v", sources.Displays)
	}

	live.HelperPath = ""
	if _, err := live.Sources(t.Context()); !errors.Is(err, ErrHelperMissing) {
		t.Fatalf("Sources without a helper returned %v", err)
	}
}
