package export

import (
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/gpuhost"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// The GPU gate of the skill renderer-parity, part 2, through the child process: the frames stuarty-render draws
// on the GPU against the frames of the CPU path (equal to the Go compositor byte for byte, see
// TestRustCPUPathEqualsGoCompositor), as SSIM on luma. The displacement bound of half a pixel is held by the
// Rust test renderer/stuarty-renderer-gpu/tests/parity.rs, which has the quads.

const (
	envGPUHost  = "STUARTY_TEST_GPU_HOST"
	minGPUSSIM  = 0.99
	gpuHostWait = 2 * time.Minute
)

// gpuHostBinary is the stuarty-render the environment names or a fresh build; a machine without cargo skips.
func gpuHostBinary(t *testing.T) string {
	t.Helper()

	if p := os.Getenv(envGPUHost); p != "" {
		return p
	}

	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("no cargo on the PATH, and " + envGPUHost + " is not set")
	}

	root, err := filepath.Abs(rendererRoot)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), cargo, "build", "--locked", "--release", "--bin", "stuarty-render",
		"--manifest-path", filepath.Join(root, "Cargo.toml"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cargo build of stuarty-render: %v\n%s", err, out)
	}

	name := "stuarty-render"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	return filepath.Join(root, "target", "release", name)
}

// startGPUHost starts the child for a test, or skips when the machine has no adapter.
func startGPUHost(t *testing.T, job Job, id string) *gpuhost.Host {
	t.Helper()

	dir, err := job.Runner.NewRunDir(id)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Remove() })

	g, err := gpuhost.Start(t.Context(), job.Runner, gpuhost.Config{
		Path: gpuHostBinary(t), Dir: dir, Deadline: gpuHostWait,
	})
	if err != nil {
		t.Skipf("the GPU host did not start (no adapter?): %v", err)
	}

	t.Cleanup(func() { _ = g.Close() })

	return g
}

func TestGPUHostIsHeldToTheCPUPath(t *testing.T) {
	setup(t)

	var all []parityFrame

	for _, name := range referenceScenes {
		dir := filepath.Join("testdata", "scenes", name)

		var frames []int

		for i, f := range sceneFrames(endFrame(loadDir(t, dir))) {
			if i%5 == 0 {
				frames = append(frames, f)
			}
		}

		all = append(all, recordFrames(t, dir, "scene-"+name, frames)...)
	}

	job := newJob(t, loadScene(t, referenceScenes[0]), "gpu-parity", "unused.mp4")
	g := startGPUHost(t, job, "gpu-parity-host")
	t.Logf("adapter: %s", g.Adapter())

	worst := 1.0

	for i := range all {
		fr := &all[i]

		for _, tx := range fr.textures {
			if err := g.PutTexture(tx.id, tx.w, tx.h, tx.pix, tx.opaque); err != nil {
				t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
			}
		}

		for key, bm := range fr.bitmaps {
			if err := g.PutBitmap(key, bm); err != nil {
				t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
			}
		}

		dst := image.NewRGBA(image.Rect(0, 0, fr.w, fr.h))
		out := renderhost.Output{Width: fr.w, Height: fr.h, Scale: fr.scale}

		if err := g.Render(renderhost.RenderInput{Output: out, Plan: fr.plan}, dst); err != nil {
			t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
		}

		s := ssim(luma(dst.Pix), luma(fr.want), fr.w, fr.h)
		worst = min(worst, s)

		if s < minGPUSSIM {
			t.Errorf("%s frame %d: SSIM %.4f is under %.2f", fr.name, fr.frame, s, minGPUSSIM)
		}

		for _, tx := range fr.textures {
			_ = g.Release(tx.id)
		}
	}

	t.Logf("%d frames of %d scenes through the child, worst SSIM %.4f", len(all), len(referenceScenes), worst)
}

// The export with the GPU renderer draws through the child when it can and writes a valid file; without a
// GPU host it is the Rust CPU path and the same bytes as the Go compositor.
func TestExportWithTheGPUHost(t *testing.T) {
	setup(t)

	doc := loadScene(t, "clip-caption")

	t.Run("without a host the CPU path draws", func(t *testing.T) {
		job := newJob(t, doc, "export-gpu-none", "gpu-none.mp4")
		job.PixelRenderer = PixelRendererGPU

		if _, err := Run(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a host that cannot start falls back to the CPU path", func(t *testing.T) {
		job := newJob(t, doc, "export-gpu-missing", "gpu-missing.mp4")
		job.PixelRenderer = PixelRendererGPU
		job.GPUHost = filepath.Join(t.TempDir(), "no-such-stuarty-render")

		res, err := Run(context.Background(), job)
		if err != nil {
			t.Fatal(err)
		}

		want := newJob(t, doc, "export-gpu-missing-go", "gpu-missing-go.mp4")
		ref, err := Run(context.Background(), want)
		if err != nil {
			t.Fatal(err)
		}

		if fileSum(t, res.Path) != fileSum(t, ref.Path) {
			t.Error("the fallback did not draw the CPU path's bytes")
		}
	})

	t.Run("with the host the file is exported", func(t *testing.T) {
		probe := newJob(t, doc, "export-gpu-probe", "unused.mp4")
		startGPUHost(t, probe, "export-gpu-probe-host")

		job := newJob(t, doc, "export-gpu-host", "gpu-host.mp4")
		job.PixelRenderer = PixelRendererGPU
		job.GPUHost = gpuHostBinary(t)

		res, err := Run(context.Background(), job)
		if err != nil {
			t.Fatal(err)
		}

		if res.Frames == 0 || res.Size == 0 {
			t.Errorf("an empty export: %+v", res)
		}
	})
}

func fileSum(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return sum(raw)
}

// The child is chosen only when the job asks for the GPU, names a host and the host starts; every other case is
// the CPU path, and the choice is made once.
func TestGPUHostIsChosenOnlyWhenAllowed(t *testing.T) {
	setup(t)

	doc := loadScene(t, "clip-caption")
	bin := gpuHostBinary(t)
	probe := newJob(t, doc, "gpu-choice-probe", "unused.mp4")
	startGPUHost(t, probe, "gpu-choice-probe-host")

	for _, c := range []struct {
		name   string
		kind   PixelRenderer
		host   string
		single bool
		want   bool
	}{
		{"gpu with a host", PixelRendererGPU, bin, false, true},
		{"gpu without a host", PixelRendererGPU, "", false, false},
		{"gpu with a host that is missing", PixelRendererGPU, filepath.Join(t.TempDir(), "none"), false, false},
		{"gpu with a relative path", PixelRendererGPU, "stuarty-render", false, false},
		{"rust with a host", PixelRendererRust, bin, false, false},
		{"go with a host", PixelRendererGo, bin, false, false},
		{"gpu for a single frame", PixelRendererGPU, bin, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			job := newJob(t, doc, "gpu-choice", "unused.mp4")
			job.PixelRenderer, job.GPUHost = c.kind, c.host
			s := testState(t, job, "gpu-choice-state")
			s.single = c.single

			rd := newRustRenderer(s, nil, nil, 1)
			defer rd.close()

			got := rd.gpuHost() != nil
			if got != c.want {
				t.Errorf("gpu host chosen = %v, want %v", got, c.want)
			}

			if rd.gpuHost() != rd.gpu {
				t.Error("the choice changed between two frames")
			}
		})
	}
}
