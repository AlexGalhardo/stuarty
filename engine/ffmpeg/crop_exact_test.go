package ffmpeg

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

// decodeGray decodes frame 0 of the file through the chain as 8-bit luma.
func decodeGray(t *testing.T, r *Runner, dir *RunDir, path string, vf ...ffargs.Filter) []byte {
	t.Helper()
	argv, err := ffargs.Cmd{
		Inputs: []ffargs.Input{ffargs.FileInput(path)},
		Outputs: []ffargs.Output{{
			Format: "rawvideo", Pipe: true, Maps: []string{"0:v:0"},
			Opts: []ffargs.Opt{ffargs.Int("-frames:v", 1), ffargs.Enum("-pix_fmt", "gray")},
			VF:   ffargs.Chain1(append(vf, ffargs.Format("gray"))...),
		}},
	}.Argv(r.Policy(dir))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := r.RunFFmpeg(
		context.Background(),
		Request{Dir: dir, Argv: argv, Deadline: time.Minute, Stdout: &out},
	); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func rect(full []byte, fw, x, y, w, h int) []byte {
	out := make([]byte, 0, w*h)
	for row := range h {
		out = append(out, full[(y+row)*fw+x:(y+row)*fw+x+w]...)
	}
	return out
}

// TestCropIsExactOnSubsampledSource: a 4:2:0 source cropped at an odd x or y is cut at that x and y (the
// golden crop case: left 45 of 180). Luma is compared, so the check does not depend on chroma
// interpolation: the crop must equal that rectangle of the whole frame, and a crop without exact=1 is
// shown to be cut one pixel to the left, which is the defect this fixes.
func TestCropIsExactOnSubsampledSource(t *testing.T) {
	r, dir := pinnedRunner(t)
	const fw, fh = 96, 64
	src := gen(t, r.In, dir, "src.mov", "-f", "lavfi", "-i", "testsrc2=s=96x64:r=1:d=1",
		"-c:v", "mpeg4", "-q:v", "1", "-pix_fmt", "yuv420p", "-frames:v", "1")

	full := decodeGray(t, r, dir, src)
	if len(full) != fw*fh {
		t.Fatalf("full frame is %d bytes", len(full))
	}
	const x, y, w, h = 45, 21, 30, 20

	got := decodeGray(t, r, dir, src, ffargs.Crop(w, h, x, y))
	if !bytes.Equal(got, rect(full, fw, x, y, w, h)) {
		t.Error("the crop is not the plan rectangle")
	}

	rounded := decodeGray(t, r, dir, src, "crop=30:20:45:21")
	if !bytes.Equal(rounded, rect(full, fw, x-1, y-1, w, h)) {
		t.Skip("this FFmpeg does not round the crop to the chroma grid, so the case is not reproducible")
	}
}
