package ingest

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

func TestParseFrameDelays(t *testing.T) {
	got := parseFrameDelays("0.000000,0.070000\n0.070000,0.030000\n0.1,N/A\n0.2,0.001\n0.3,90\n\n")
	want := []int{70, 30, 100, 10, 60000}
	if !slices.Equal(got, want) {
		t.Fatalf("delays %v, want %v", got, want)
	}
}

// TestAnimatedGIFIngest is the three frame GIF of P3-RND-004: delays 70, 30 and 100 ms are read from the
// file, the sequence keeps the alpha of each frame, and the frame the plan picks at 30 fps is the one decoded.
func TestAnimatedGIFIngest(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AnimatedGIF("a.gif", 16, 12, []int{7, 3, 10})
	_, res := e.mustIngest(mustGen(t, p, err))

	wantDelays := []int{70, 30, 100}
	if !slices.Equal(res.Facts.FrameDelays, wantDelays) || res.Facts.FrameCount != 3 {
		t.Fatalf("facts %+v", res.Facts)
	}
	if res.Facts.Width != 16 || res.Facts.Height != 12 {
		t.Errorf("size %dx%d", res.Facts.Width, res.Facts.Height)
	}
	if res.find(AnimKind, "") == nil || res.find("normalized", "") == nil {
		t.Fatalf("artifacts %v", res.names())
	}

	a, err := OpenAnim(filepath.Join(e.ws, res.find(AnimKind, "").RelPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	if a.Frames() != 3 || a.W != 16 || a.H != 12 || !slices.Equal(a.Delays, wantDelays) {
		t.Fatalf("sequence %dx%d %v", a.W, a.H, a.Delays)
	}

	colours := [][3]byte{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}}
	fps := timeline.Rational{Num: 30, Den: 1}
	for r := range 12 {
		i := plan.AnimImageSourceFrame(wantDelays, fps, r)
		pix, err := a.Frame(i)
		if err != nil {
			t.Fatal(err)
		}
		if got := [3]byte(pix[(5*16+5)*4:][:3]); got != colours[i] || pix[(5*16+5)*4+3] != 255 {
			t.Fatalf("item frame %d: source frame %d is %v", r, i, pix[(5*16+5)*4:][:4])
		}
		if pix[3] != 0 {
			t.Fatalf("source frame %d lost its transparent corner: alpha %d", i, pix[3])
		}
	}
}

func TestAnimatedAPNGAndWebP(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for name, gen := range map[string]func() (string, error){
		"a.png":  func() (string, error) { return e.gen.AnimatedAPNG(ctx, "a.png", 24, 16, 5) },
		"a.webp": func() (string, error) { return e.gen.AnimatedWebP(ctx, "a.webp", 24, 16, 5) },
	} {
		p, err := gen()
		_, res := e.mustIngest(mustGen(t, p, err))
		if res.Facts.FrameCount < 2 || len(res.Facts.FrameDelays) != res.Facts.FrameCount {
			t.Errorf("%s: facts %+v", name, res.Facts)
			continue
		}
		a, err := OpenAnim(filepath.Join(e.ws, res.find(AnimKind, "").RelPath))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if a.W != 24 || a.H != 16 || a.Frames() != res.Facts.FrameCount {
			t.Errorf("%s: sequence %dx%d with %d frames", name, a.W, a.H, a.Frames())
		}
		_ = a.Close()
	}
}

// TestAnimatedSequenceDeterministic: the same file gives the same sequence bytes on two ingests.
func TestAnimatedSequenceDeterministic(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AnimatedGIF("d.gif", 20, 10, []int{4, 4, 8, 12})
	p = mustGen(t, p, err)
	var sums []string
	for range 2 {
		hash, res, ierr := e.ingest(context.Background(), p, nil)
		if ierr != nil {
			t.Fatal(ierr)
		}
		sums = append(sums, res.find(AnimKind, "").SHA256)
		if err := RemoveArtifacts(e.artifacts(), hash); err != nil {
			t.Fatal(err)
		}
	}
	if sums[0] != sums[1] {
		t.Fatalf("the sequence differs between runs: %v", sums)
	}
}

// TestAnimatedBombRefused: a GIF that declares too many frames, too large a screen or too many pixels
// over all its frames is refused with E_LIMIT_EXCEEDED from the probe, before any frame is decoded (T-17).
func TestAnimatedBombRefused(t *testing.T) {
	e := newEnv(t)
	cases := map[string]func() (string, error){
		"anim_frames": func() (string, error) { return e.gen.BombGIFFrames("frames.gif", 100_000) },
		"source_edge": func() (string, error) { return e.gen.BombGIFSize("edge.gif", 10_000, 10_000, 2) },
		"anim_pixels": func() (string, error) { return e.gen.BombGIFSize("pixels.gif", 5000, 5000, 6) },
	}
	for reason, gen := range cases {
		p, err := gen()
		_, _, ierr := e.ingest(context.Background(), mustGen(t, p, err), nil)
		var fe *ffmpeg.Error
		if !errors.As(ierr, &fe) || fe.Code != ffmpeg.CodeLimitExceeded || fe.Reason != reason {
			t.Errorf("%s: %v", reason, ierr)
		}
	}
}

func TestOpenAnimRefusesHostileFiles(t *testing.T) {
	dir := t.TempDir()
	head := func(n, w, h uint32) []byte {
		b := make([]byte, animHeaderSize)
		copy(b, animMagic)
		binary.LittleEndian.PutUint32(b[4:], animVersion)
		binary.LittleEndian.PutUint32(b[8:], n)
		binary.LittleEndian.PutUint32(b[12:], w)
		binary.LittleEndian.PutUint32(b[16:], h)
		return b
	}
	cases := map[string][]byte{
		"empty":          nil,
		"magic":          append([]byte("NOPE"), make([]byte, 40)...),
		"frames":         head(100_000, 1, 1),
		"pixels":         head(1000, 4000, 4000),
		"edge":           head(2, 9000, 9000),
		"table past eof": head(5, 2, 2),
	}
	for name, b := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := OpenAnim(p)
		if err == nil {
			_ = a.Close()
			t.Errorf("%s: opened", name)
		} else if !errors.Is(err, ErrAnim) && name != "empty" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestSecurity_T17_AnimatedDecodeStopsAtProbedCount: a file that really holds more frames than the probe counted
// is decoded for the probed count only (-frames:v), so the raw output cannot pass the size the size watchdog
// is armed with (frames x width x height x 4) and nothing past it is written or stored.
func TestSecurity_T17_AnimatedDecodeStopsAtProbedCount(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AnimatedGIF("big.gif", 64, 64, []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5})
	hash := e.put(mustGen(t, p, err))
	wd, err := e.r.NewRunDir("job_cap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = wd.Remove() }()
	j := &job{
		in: Input{Runner: e.r, Hash: hash}, r: e.r, wd: wd, pol: e.r.Policy(wd),
		original: filepath.Join(e.r.StoreDir, hash[:2], hash),
		res:      &Result{Summary: &Summary{FrameDelays: []int{50, 50}}},
		files:    nil,
	}
	if err := j.animated(context.Background(), ImagePlan{StreamIndex: 0, W: 64, H: 64}); err != nil {
		t.Fatal(err)
	}
	a, err := OpenAnim(j.path("frames.anim"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	if a.Frames() != 2 {
		t.Fatalf("%d frames stored, 2 probed", a.Frames())
	}
}
