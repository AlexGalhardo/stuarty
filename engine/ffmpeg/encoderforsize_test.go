package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEncoderForSizeFallsBackToTheNextWorkingEncoder is D-034 for 4K: an encoder whose test encode at the
// size of the project fails is replaced by the next working candidate, down to the software encoder, and a
// forced encoder is never replaced.
func TestEncoderForSizeFallsBackToTheNextWorkingEncoder(t *testing.T) {
	big := SizeProbe{Width: 3840, Height: 2160, FPSNum: 60, FPSDen: 1}
	hd := SizeProbe{Width: 1080, Height: 1920, FPSNum: 30, FPSDen: 1}
	all := []string{"h264_nvenc", "h264_qsv", "h264_amf", "libopenh264"}

	cases := []struct {
		name  string
		small []string
		probe SizeProbe
		want  string
	}{
		{"the chosen one can do it", nil, big, "h264_nvenc"},
		{"NVENC cannot, Quick Sync can", []string{"h264_nvenc"}, big, "h264_qsv"},
		{"both cannot, AMF can", []string{"h264_nvenc", "h264_qsv"}, big, "h264_amf"},
		{"every GPU cannot", []string{"h264_nvenc", "h264_qsv", "h264_amf"}, big, "libopenh264"},
		{"a small project never needs the fallback", []string{"h264_nvenc"}, hd, "h264_nvenc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, caps, dir := hwFixtureDir(t, all...)
			if err := writeSmall(dir, c.small); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			det, err := r.DetectEncoder(ctx, caps, DetectOptions{})
			if err != nil || det.Chosen != "h264_nvenc" {
				t.Fatalf("%v: %+v", err, det)
			}
			got, err := r.EncoderForSize(ctx, det, c.probe)
			if err != nil || got.Name != c.want || got.Status != EncoderOK || got.Input.Filter == "" {
				t.Fatalf("got %+v, %v; want %s", got, err, c.want)
			}
		})
	}

	t.Run("nothing can", func(t *testing.T) {
		r, caps, dir := hwFixtureDir(t, all...)
		if err := writeSmall(dir, all); err != nil {
			t.Fatal(err)
		}
		det, err := r.DetectEncoder(context.Background(), caps, DetectOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.EncoderForSize(context.Background(), det, big)
		if !errors.Is(err, ErrEncoderSize) || got.Name != "h264_nvenc" ||
			!strings.Contains(err.Error(), "frame size too large") {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("a forced encoder stays", func(t *testing.T) {
		r, caps, dir := hwFixtureDir(t, all...)
		if err := writeSmall(dir, []string{"h264_nvenc"}); err != nil {
			t.Fatal(err)
		}
		det, err := r.DetectEncoder(context.Background(), caps, DetectOptions{Forced: "h264_nvenc"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.EncoderForSize(context.Background(), det, big)
		if !errors.Is(err, ErrEncoderSize) || got.Name != "h264_nvenc" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
}

// writeSmall tells the fake FFmpeg in dir which encoders cannot do a 4K picture.
func writeSmall(dir string, names []string) error {
	return os.WriteFile(filepath.Join(dir, "small.txt"), []byte(strings.Join(names, "\n")), 0o644)
}
