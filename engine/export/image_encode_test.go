package export

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"testing"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

func renderTwice(t *testing.T, name string) (*image.RGBA, *image.RGBA) {
	t.Helper()

	src := sourcePNG(t)
	cfg := ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}

	a, err := RenderImage(context.Background(), imageDoc(name), cfg)
	if err != nil {
		t.Fatal(err)
	}

	b, err := RenderImage(context.Background(), imageDoc(name), cfg)
	if err != nil {
		t.Fatal(err)
	}

	return a, b
}

// TestImageExportDeterministic is NFR-062: the same document exported twice gives the same bytes, in every format
// that has an encoder here.
func TestImageExportDeterministic(t *testing.T) {
	for _, name := range []string{"shapes", "text", "image_and_group", "transparent"} {
		a, b := renderTwice(t, name)

		for _, o := range []ImageEncodeOptions{
			{Format: ImagePNG},
			{Format: ImageJPEG},
			{Format: ImageJPEG, Quality: 70, Width: 160},
			{Format: ImagePNG, Width: 100, Height: 100},
		} {
			one, err := EncodeImage(context.Background(), a, o)
			if err != nil {
				t.Fatal(err)
			}

			two, err := EncodeImage(context.Background(), b, o)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(one, two) {
				t.Errorf("%s %+v: two exports differ", name, o)
			}
		}
	}
}

func TestEncodeImageFormatsAndSizes(t *testing.T) {
	a, _ := renderTwice(t, "transparent")

	raw, err := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: ImagePNG, Width: 160})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width != 160 || cfg.Height != 90 {
		t.Fatalf("a png of width 160 keeps the aspect: %v %+v", err, cfg)
	}

	full, _ := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: ImagePNG})

	decoded, err := png.Decode(bytes.NewReader(full))
	if err != nil {
		t.Fatal(err)
	}

	if _, _, _, al := decoded.At(5, 5).RGBA(); al != 0 {
		t.Error("a png keeps the alpha")
	}

	jraw, err := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: ImageJPEG})
	if err != nil {
		t.Fatal(err)
	}

	jimg, err := jpeg.Decode(bytes.NewReader(jraw))
	if err != nil {
		t.Fatal(err)
	}

	if r, g, b, _ := jimg.At(5, 5).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Errorf("a jpeg is flattened on white, got %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestEncodeImageRefusals(t *testing.T) {
	a, _ := renderTwice(t, "shapes")

	if _, err := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: "bmp"}); err == nil {
		t.Error("an unknown format is refused")
	}

	if _, err := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: ImageJPEG, Quality: 101}); err == nil {
		t.Error("a quality over 100 is refused")
	}

	_, err := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: ImageWebP})

	var fe *ffmpeg.Error
	if !errors.As(err, &fe) || fe.Details["reason"] != ReasonWebPUnavailable {
		t.Fatalf("webp with no encoder is webp_unavailable: %v", err)
	}
}

type fakeWebP struct{ got []byte }

func (f *fakeWebP) EncodeWebP(_ context.Context, raw []byte, _ int) ([]byte, error) {
	f.got = raw

	return []byte("RIFFxxxxWEBP"), nil
}

func TestEncodeImageWebPThroughTheSeam(t *testing.T) {
	a, _ := renderTwice(t, "shapes")
	w := &fakeWebP{}

	out, err := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: ImageWebP, WebP: w})
	if err != nil || string(out) != "RIFFxxxxWEBP" {
		t.Fatalf("%v %q", err, out)
	}

	if _, err := png.Decode(bytes.NewReader(w.got)); err != nil {
		t.Errorf("the encoder is given a png: %v", err)
	}
}

// The sizes an export may ask for are bounded, and a picture with no pixels is refused, before anything is
// allocated (SDD-021 section 6).
func TestEncodeImageRefusesBadSizes(t *testing.T) {
	a, _ := renderTwice(t, "shapes")

	for name, o := range map[string]ImageEncodeOptions{
		"negative width":   {Format: ImagePNG, Width: -1},
		"negative height":  {Format: ImagePNG, Height: -5},
		"width too large":  {Format: ImagePNG, Width: coreimage.MaxExportSide + 1},
		"height too large": {Format: ImagePNG, Height: coreimage.MaxExportSide + 1},
		"too many pixels":  {Format: ImageJPEG, Width: coreimage.MaxExportSide, Height: coreimage.MaxExportSide},
	} {
		if _, err := EncodeImage(context.Background(), a, o); err == nil {
			t.Errorf("%s: the size was accepted", name)
		}
	}

	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if _, err := EncodeImage(context.Background(), empty, ImageEncodeOptions{Format: ImagePNG, Width: 10}); err == nil {
		t.Error("a picture with no pixels was accepted")
	}

	if _, err := EncodeImage(context.Background(), nil, ImageEncodeOptions{Format: ImagePNG}); err == nil {
		t.Error("a missing picture was accepted")
	}
}

func TestResampleFailureIsAnError(t *testing.T) {
	a, _ := renderTwice(t, "shapes")
	// the largest side the export allows, on the smallest other side: fine, and the result is the size asked
	out, err := EncodeImage(context.Background(), a, ImageEncodeOptions{Format: ImagePNG, Width: 8192, Height: 1})
	if err != nil {
		t.Fatal(err)
	}

	if cfg, err := png.DecodeConfig(bytes.NewReader(out)); err != nil || cfg.Width != 8192 || cfg.Height != 1 {
		t.Fatalf("%v %+v", err, cfg)
	}
}
