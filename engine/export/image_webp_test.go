package export

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// decodeWebP reads a WebP back with the FFmpeg of the test, as a straight RGBA picture.
func decodeWebP(t *testing.T, webp []byte) image.Image {
	t.Helper()

	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.webp"), filepath.Join(dir, "out.png")

	if err := os.WriteFile(in, webp, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := shared.runner.In.FFmpeg.Run(
		context.Background(), dir, "-nostdin", "-hide_banner", "-v", "error", "-i", in,
		"-frames:v", "1", "-c:v", "png", "-pix_fmt", "rgba", "-f", "image2", out,
	)
	if err != nil {
		t.Fatalf("ffmpeg cannot decode the WebP: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	return img
}

func alphaAt(img image.Image, x, y int) uint32 {
	_, _, _, a := img.At(x, y).RGBA()

	return a >> 8
}

func TestFFmpegWebPKeepsTheAlpha(t *testing.T) {
	setup(t)

	a, _ := renderTwice(t, "transparent")

	for name, q := range map[string]int{"lossless": 0, "lossy": 80} {
		t.Run(name, func(t *testing.T) {
			webp, err := EncodeImage(
				context.Background(), a,
				ImageEncodeOptions{Format: ImageWebP, Quality: q, WebP: FFmpegWebP{Runner: shared.runner}},
			)
			if err != nil {
				t.Fatal(err)
			}

			if len(webp) < 32 || string(webp[:4]) != "RIFF" || string(webp[8:12]) != "WEBP" {
				t.Fatalf("not a WebP file: %q", webp[:min(len(webp), 16)])
			}

			if len(webp) > 20_000 {
				t.Errorf("a 320 x 180 picture of one ellipse is %d bytes", len(webp))
			}

			img := decodeWebP(t, webp)
			if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 180 {
				t.Fatalf("the size is %v", b)
			}

			if got := alphaAt(img, 2, 2); got != 0 {
				t.Errorf("the corner is meant to be transparent, alpha %d", got)
			}

			if got := alphaAt(img, 150, 90); got != 255 {
				t.Errorf("the ellipse is meant to be opaque, alpha %d", got)
			}

			if q == 0 {
				r, g, bl, _ := img.At(150, 90).RGBA()
				if r>>8 != 0 || g>>8 != 255 || bl>>8 != 0 {
					t.Errorf("lossless must keep the colour, got %d %d %d", r>>8, g>>8, bl>>8)
				}
			}
		})
	}
}

func TestFFmpegWebPResizes(t *testing.T) {
	setup(t)

	a, _ := renderTwice(t, "shapes")

	webp, err := EncodeImage(
		context.Background(), a,
		ImageEncodeOptions{Format: ImageWebP, Quality: 70, Width: 160, WebP: FFmpegWebP{Runner: shared.runner}},
	)
	if err != nil {
		t.Fatal(err)
	}

	if b := decodeWebP(t, webp).Bounds(); b.Dx() != 160 || b.Dy() != 90 {
		t.Errorf("the size is %v", b)
	}
}

func TestFFmpegWebPWithoutARunnerIsUnavailable(t *testing.T) {
	_, err := FFmpegWebP{}.EncodeWebP(context.Background(), []byte("x"), 0)

	var fe *ffmpeg.Error
	if !errors.As(err, &fe) || fe.Details["reason"] != ReasonWebPUnavailable {
		t.Fatalf("no runner is webp_unavailable: %v", err)
	}
}

func TestMissingWebPEncoderIsRecognised(t *testing.T) {
	for _, s := range []string{
		"Unknown encoder 'libwebp'",
		"[vost#0:0] Encoder 'libwebp' not found",
	} {
		if !missingWebPEncoder(s) {
			t.Errorf("%q is a build with no libwebp", s)
		}
	}

	if missingWebPEncoder("Error while opening encoder") {
		t.Error("an encoder failure is not a missing encoder")
	}
}
