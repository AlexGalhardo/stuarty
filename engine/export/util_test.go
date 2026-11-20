package export

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// luma is the BT.709 luma of an RGBA frame, one byte per pixel.
func luma(pix []byte) []byte {
	out := make([]byte, len(pix)/4)
	for i := range out {
		r, g, b := int(pix[i*4]), int(pix[i*4+1]), int(pix[i*4+2])
		out[i] = byte((2126*r + 7152*g + 722*b + 5000) / 10000)
	}
	return out
}

// ssim is the mean structural similarity of two luma planes over 8x8 windows every 4 pixels.
func ssim(a, b []byte, w, h int) float64 {
	const (
		c1 = (0.01 * 255) * (0.01 * 255)
		c2 = (0.03 * 255) * (0.03 * 255)
		n  = 64.0
	)
	var sum float64
	var count int
	for y := 0; y+8 <= h; y += 4 {
		for x := 0; x+8 <= w; x += 4 {
			var sa, sb, saa, sbb, sab float64
			for j := range 8 {
				row := (y+j)*w + x
				for i := range 8 {
					va, vb := float64(a[row+i]), float64(b[row+i])
					sa += va
					sb += vb
					saa += va * va
					sbb += vb * vb
					sab += va * vb
				}
			}
			ma, mb := sa/n, sb/n
			va, vb, cov := saa/n-ma*ma, sbb/n-mb*mb, sab/n-ma*mb
			sum += (2*ma*mb + c1) * (2*cov + c2) / ((ma*ma + mb*mb + c1) * (va + vb + c2))
			count++
		}
	}
	return sum / float64(count)
}

// decodeFrame decodes frame f of an exported file as RGBA with the pinned FFmpeg.
func decodeFrame(t *testing.T, file string, doc *timeline.Document, f int) []byte {
	t.Helper()
	r := shared.runner
	dir, err := r.NewRunDir("decode-" + strconv.FormatInt(time.Now().UnixNano(), 36))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Remove() }()
	st := doc.Settings
	ms := int64(f) * 1000 * int64(st.FPS.Den) / int64(st.FPS.Num)
	inDir := filepath.Dir(file)
	argv, err := ffargs.Cmd{
		Inputs: []ffargs.Input{ffargs.FileInput(file).At(ms)},
		Outputs: []ffargs.Output{{
			Format: "rawvideo", Pipe: true, Maps: []string{"0:v:0"}, NoAudio: true,
			VF:   ffargs.Chain1(ffargs.ScaleFrameToRGBA(st.Width, st.Height), ffargs.Format("rgba")),
			Opts: []ffargs.Opt{ffargs.Int("-frames:v", 1), ffargs.Enum("-pix_fmt", "rgba")},
		}},
	}.Argv(r.Policy(dir, inDir))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := r.RunFFmpeg(context.Background(), ffmpeg.Request{
		Dir: dir, Argv: argv, InputDirs: []string{inDir}, Deadline: time.Minute, Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}
	if out.Len() != st.Width*st.Height*4 {
		t.Fatalf("decoded %d bytes for frame %d", out.Len(), f)
	}
	return out.Bytes()
}

// ffmpegProcesses counts the FFmpeg processes whose command line mentions marker. ok is false when the
// machine cannot list processes.
func ffmpegProcesses(marker string) (n int, ok bool) {
	if runtime.GOOS == "windows" {
		script := "(Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -like '*" +
			strings.ReplaceAll(marker, "'", "''") + "*' -and $_.Name -like 'ffmpeg*' } | Measure-Object).Count"
		out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
		if err != nil {
			return 0, false
		}
		n, err = strconv.Atoi(strings.TrimSpace(string(out)))
		return n, err == nil
	}
	out, err := exec.Command("pgrep", "-fc", "ffmpeg.*"+marker).Output()
	if err != nil {
		if ee, isExit := errors.AsType[*exec.ExitError](err); isExit && ee.ExitCode() == 1 {
			return 0, true
		}
		return 0, false
	}
	n, err = strconv.Atoi(strings.TrimSpace(string(out)))
	return n, err == nil
}
