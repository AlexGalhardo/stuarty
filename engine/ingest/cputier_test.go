package ingest

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNormalizeIsTheSameOnEveryCPUTier runs the normalize encodes (mpeg4 with the scale of the plan, and ProRes
// 4444 with alpha) under every CPU tier of an x86-64 machine from SSE2 up (the default, no AVX-512, no AVX2, no
// AVX, SSSE3 and SSE2 only) and checks that the bytes are the same (NFR-010, SDD-011 4.3.2). A machine with
// AVX-512 also has the tiers below it, and plain C differs from all of them (swscale), so the mask of
// ffargs.CPUMask, which the product adds on x86-64, puts every runner on a tier shown equal here.
func TestNormalizeIsTheSameOnEveryCPUTier(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("the CPU flags of x86-64")
	}

	in, _ := pinnedInstall(t)

	dir := t.TempDir()
	gen := func(args ...string) {
		t.Helper()

		if _, err := in.FFmpeg.Run(
			context.Background(),
			dir,
			append([]string{"-v", "error", "-y"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}

	gen("-f", "lavfi", "-i", "testsrc2=s=1280x720:r=30:d=3", "-c:v", "mpeg4", "-dct", "int", "-q:v", "4", "src.mp4")

	pipelines := map[string][]string{
		"mpeg4": {
			"-vf", "setpts=PTS-STARTPTS,fps=30,scale=640:360:flags=bicubic+accurate_rnd,setsar=1,format=yuv420p",
			"-c:v", "mpeg4", "-q:v", "2", "-threads", "1", "-dct", "int", "-g", "30", "-bf", "0",
			"-pix_fmt", "yuv420p", "-fps_mode", "cfr",
		},
		"prores": {
			"-vf", "setpts=PTS-STARTPTS,fps=30,scale=640:360:flags=bicubic+accurate_rnd,format=yuva444p10le",
			"-c:v", "prores_ks", "-profile:v", "4444", "-threads", "1", "-pix_fmt", "yuva444p10le",
		},
	}

	const noAVX = "-avx-avx2-fma3-fma4-xop-bmi1-bmi2"

	// Every tier an x86-64 machine can have from SSE2 up. Plain C (-cpuflags 0) and no SSE2 are not here: their
	// swscale bytes differ from the SIMD ones, and no x86-64 CPU lacks SSE2.
	tiers := [][]string{
		nil,
		{"-cpuflags", "-avx512-avx512icl"},
		{"-cpuflags", "-avx2"},
		{"-cpuflags", "-avx-avx2"},
		{"-cpuflags", noAVX + "-sse4.2-sse4.1"},
		{"-cpuflags", noAVX + "-sse4.2-sse4.1-ssse3-sse3"},
	}

	for name, opts := range pipelines {
		var first [32]byte

		for i, tier := range tiers {
			out := filepath.Join(dir, name+string(rune('a'+i))+".mov")
			args := append([]string{"-v", "error", "-y", "-filter_threads", "1"}, tier...)
			args = append(args, "-i", "src.mp4")
			args = append(args, opts...)
			args = append(args, "-fflags", "+bitexact", "-flags", "+bitexact", "-f", "mov", out)

			if _, err := in.FFmpeg.Run(context.Background(), dir, args...); err != nil {
				t.Fatalf("%s %v: %v", name, tier, err)
			}

			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}

			sum := sha256.Sum256(b)
			if i == 0 {
				first = sum
			} else if sum != first {
				t.Errorf("%s: the bytes under %v differ from the default tier", name, tier[1:])
			}
		}
	}
}
