package export

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func TestBitrateScalesWithThePixelCount(t *testing.T) {
	t.Parallel()
	fps30, fps60 := timeline.Rational{Num: 30, Den: 1}, timeline.Rational{Num: 60, Den: 1}
	ntsc := timeline.Rational{Num: 30000, Den: 1001}
	cases := []struct {
		w, h int
		fps  timeline.Rational
		want int
	}{
		{1080, 1920, fps30, 8000},
		{1080, 1920, ntsc, 8000},
		{1080, 1920, fps60, 12000},
		{540, 960, fps30, 2000},    // a quarter of the pixels is 2000 kbit/s, which is also the floor
		{270, 480, fps30, 2000},    // below the floor
		{2160, 3840, fps30, 32000}, // four times the pixels
	}
	for _, c := range cases {
		if got := bitrateK(8, 12, c.w, c.h, c.fps); got != c.want {
			t.Errorf("%dx%d at %v: %d kbit/s, want %d", c.w, c.h, c.fps, got, c.want)
		}
	}
}

func TestEncoderOptionsPerEncoder(t *testing.T) {
	t.Parallel()
	fps := timeline.Rational{Num: 30, Den: 1}
	join := func(name string) string {
		var parts []string
		for _, o := range encoderOpts(name, 1080, 1920, fps) {
			name, value := o.Pair()
			parts = append(parts, name+" "+value)
		}
		return strings.Join(parts, " ")
	}
	want := map[string]string{
		"h264_nvenc":  "-c:v h264_nvenc -b:v 8000k -profile:v high -g 15 -bf 2",
		"h264_mf":     "-c:v h264_mf -b:v 12000k -g 15 -bf 2",
		"libopenh264": "-c:v libopenh264 -b:v 8000k -g 15",
		"h264_vaapi":  "-c:v h264_vaapi -b:v 8000k -g 15 -bf 2",
		"libx264":     "-c:v libx264 -crf 18 -preset medium -profile:v high -g 15 -bf 2",
	}
	for name, w := range want {
		if got := join(name); got != w {
			t.Errorf("%s: %q, want %q", name, got, w)
		}
	}
	// The GOP is half a second: 30000/1001 fps is 15 frames, 25 fps is 13.
	if got := join("libopenh264"); !strings.Contains(got, "-g 15") {
		t.Error(got)
	}
	opts := encoderOpts("libopenh264", 1080, 1920, timeline.Rational{Num: 25, Den: 1})
	if name, value := opts[2].Pair(); name != "-g" || value != "13" {
		t.Errorf("25 fps: %s %s", name, value)
	}
}

func TestTempoChain(t *testing.T) {
	t.Parallel()
	cases := map[float64]string{
		1:    "",
		2:    "atempo=2.000000",
		0.5:  "atempo=0.500000",
		1.5:  "atempo=1.500000",
		4:    "atempo=2.000000,atempo=2.000000",
		0.25: "atempo=0.500000,atempo=0.500000",
		3:    "atempo=2.000000,atempo=1.500000",
	}
	for tempo, want := range cases {
		var parts []string
		for _, f := range tempoChain(tempo) {
			parts = append(parts, string(f))
		}
		if got := strings.Join(parts, ","); got != want {
			t.Errorf("tempo %v: %q, want %q", tempo, got, want)
		}
	}
}

func TestSamplesAtFollowsTheFrameRate(t *testing.T) {
	t.Parallel()
	if got := samplesAt(timeline.Rational{Num: 30, Den: 1}, 90); got != 144000 {
		t.Errorf("90 frames at 30 fps: %d samples", got)
	}
	if got := samplesAt(timeline.Rational{Num: 30000, Den: 1001}, 30); got != 48048 {
		t.Errorf("30 frames at 30000/1001: %d samples", got)
	}
}

func TestCropRectStaysInsideTheAsset(t *testing.T) {
	t.Parallel()
	a := &timeline.Asset{Width: 640, Height: 360}
	l := &plan.Layer{Crop: plan.Rect{X: 160, Y: 0, W: 320, H: 360}}
	if x, y, w, h := cropRect(l, a); x != 160 || y != 0 || w != 320 || h != 360 {
		t.Errorf("crop %d %d %d %d", x, y, w, h)
	}
	l = &plan.Layer{Crop: plan.Rect{X: 600, Y: 300, W: 400, H: 400}}
	if x, y, w, h := cropRect(l, a); x != 600 || y != 300 || w != 40 || h != 60 {
		t.Errorf("an oversize crop gave %d %d %d %d", x, y, w, h)
	}
}

func box(kind string, payload int) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(8+payload))
	b = append(b, kind...)
	return append(b, make([]byte, payload)...)
}

func TestReadBoxes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name string, parts ...[]byte) string {
		var all []byte
		for _, p := range parts {
			all = append(all, p...)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, all, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	good, err := readBoxes(write("good.mp4", box("ftyp", 16), box("moov", 100), box("mdat", 1000)))
	if err != nil || !good.complete || good.moov != 24 || good.mdat != 132 {
		t.Errorf("faststart file: %+v %v", good, err)
	}

	slow, err := readBoxes(write("slow.mp4", box("ftyp", 16), box("mdat", 1000), box("moov", 100)))
	if err != nil || !slow.complete || slow.moov < slow.mdat {
		t.Errorf("moov after mdat: %+v %v", slow, err)
	}

	full := append(append(box("ftyp", 16), box("moov", 100)...), box("mdat", 1000)...)
	cut, err := readBoxes(write("cut.mp4", full[:len(full)-10]))
	if err != nil || cut.complete {
		t.Errorf("an mdat cut short counted as complete: %+v %v", cut, err)
	}

	tail, err := readBoxes(write("tail.mp4", box("ftyp", 16), []byte{0, 0, 0}))
	if err != nil || tail.complete {
		t.Errorf("bytes after the last box counted as complete: %+v %v", tail, err)
	}

	big := append(binary.BigEndian.AppendUint32(nil, 1), "mdat"...)
	big = binary.BigEndian.AppendUint64(big, 16+5)
	big = append(big, make([]byte, 5)...)
	if b, err := readBoxes(write("big.mp4", box("ftyp", 8), big)); err != nil || !b.complete || b.mdat != 16 {
		t.Errorf("a 64-bit box: %+v %v", b, err)
	}

	if _, err := readBoxes(write("bad.mp4", []byte{0, 0, 0, 4, 'f', 't', 'y', 'p'})); err == nil {
		t.Error("a box smaller than its header was accepted")
	}
}

func TestCheckDuration(t *testing.T) {
	t.Parallel()
	var failed []Check
	checkDuration(&failed, "d", "3.000000", 3.0, 1.0/30)
	checkDuration(&failed, "d", "3.030000", 3.0, 1.0/30)
	checkDuration(&failed, "d", "N/A", 3.0, 1.0/30)
	checkDuration(&failed, "d", "3.050000", 3.0, 1.0/30)
	if len(failed) != 2 || failed[0].Got != "N/A" || failed[1].Got != "3.050000" {
		t.Errorf("failed %+v", failed)
	}
}
