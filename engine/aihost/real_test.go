package aihost

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestRealHelperFindsTheDrawnFace runs the real stuarty-aihelper, the real ONNX Runtime and the real YuNet
// model over the drawn speaker of aihelper/testdata/make-face-fixture.sh. It runs only when
// STUARTY_TEST_AIHELPER_DIR names a folder holding stuarty-aihelper, onnxruntime.dll (or the library of the
// system) and yunet.onnx, because those are downloaded on request and never part of a build (SDD-024
// section 12).
func TestRealHelperFindsTheDrawnFace(t *testing.T) {
	dir := os.Getenv("STUARTY_TEST_AIHELPER_DIR")
	if dir == "" {
		t.Skip("set STUARTY_TEST_AIHELPER_DIR") // verify:env-skip on-request helper
	}

	r, artifacts, _, run := pinned(t)

	exe, lib := "stuarty-aihelper", "libonnxruntime.so"

	switch runtime.GOOS {
	case "windows":
		exe, lib = exe+".exe", "onnxruntime.dll"
	case "darwin":
		lib = "libonnxruntime.dylib"
	}

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is needed to draw the fixture") // verify:env-skip no bash on this machine
	}

	ffmpegDir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg")

	if runtime.GOOS == "windows" {
		ffmpegExe += ".exe"
	}

	script, err := filepath.Abs(filepath.Join("..", "..", "aihelper", "testdata", "make-face-fixture.sh"))
	if err != nil {
		t.Fatal(err)
	}

	// Drawing the fixture takes minutes on a busy machine (FFmpeg expressions on 100 frames), so a face.mp4
	// left in the folder by an earlier run is used as it is; it is the same bytes the script makes.
	clip := filepath.Join(artifacts, "face.mp4")

	if cached, err := os.ReadFile(filepath.Join(dir, "face.mp4")); err == nil {
		if err := os.WriteFile(clip, cached, 0o600); err != nil {
			t.Fatal(err)
		}
	} else if out, err := exec.Command(bash, script, filepath.ToSlash(ffmpegExe), filepath.ToSlash(clip)).CombinedOutput(); err != nil {
		t.Fatalf("the fixture was not drawn: %v\n%s", err, out)
	}

	h, err := New(Config{
		Runner: r, Helper: filepath.Join(dir, exe), Runtime: filepath.Join(dir, lib),
		Model: filepath.Join(dir, "yunet.onnx"), Provider: ProviderCPU,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	track, err := h.Detect(ctx, &Input{
		Source: clip, InputDirs: []string{artifacts}, WorkDir: run, Width: 1280, Height: 720,
		StartMS: 0, EndMS: 10_000, FPS: 10,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := track.Validate(); err != nil {
		t.Fatal(err)
	}

	if len(track.Samples) != 100 {
		t.Fatalf("%d samples, want 100", len(track.Samples))
	}

	near := 0

	for _, s := range track.Samples {
		if len(s.Faces) == 0 {
			continue
		}

		secs := float64(s.T) / 1000
		wantX := (240 + 80*secs) / 1280 * 10000
		wantY := (300 + 40*math.Sin(2*secs)) / 720 * 10000

		f := s.Faces[0]
		if math.Abs(float64(f.CenterX())-wantX) < 300 && math.Abs(float64(f.CenterY())-wantY) < 1500 {
			near++
		}
	}

	if near < 95 {
		t.Errorf("the face was found near where it was drawn in %d of 100 frames, want at least 95", near)
	}
}
