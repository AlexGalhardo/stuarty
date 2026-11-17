package ingest

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

// AnimKind and AnimExt name the frame sequence artifact of an animated image: frames-1.anim
// (SDD-011 4.7, 6.1).
const (
	AnimKind = "frames"
	AnimExt  = "anim"
)

// animMagic starts a frame sequence file: the layout is
//
//	"VKAN" | u32 version (1) | u32 count | u32 width | u32 height |
//	count x (u32 delay in ms, u32 length of the PNG) | the PNG files back to back
//
// all integers little endian. Every frame is a straight-alpha PNG of the normalized size, so a reader
// decodes the frame it needs with one seek and no other frame is read.
const (
	animMagic      = "VKAN"
	animVersion    = 1
	animHeaderSize = 20
	animEntrySize  = 8
	// animMaxFrame is the most bytes of one stored frame a reader accepts: raw RGBA of the largest frame.
	animMaxFrame = ffmpeg.MaxAnimPixels * 4
)

// minAnimDelay and maxAnimDelay are the delay bounds of SDD-001 3.2.
const (
	minAnimDelay = 10
	maxAnimDelay = 60000
)

// parseFrameDelays reads the csv of ProbeAnimPackets (pts_time,duration_time, in seconds) into the
// display time of each frame in whole milliseconds, held to the bounds of the schema; a packet whose
// duration is not known (N/A) takes the 100 ms that browsers give a frame with no usable delay.
func parseFrameDelays(out string) []int {
	var delays []int
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, dur, _ := strings.Cut(line, ",")
		d := 100
		if f, err := strconv.ParseFloat(strings.TrimSpace(dur), 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			d = int(math.Round(f * 1000))
		}
		delays = append(delays, min(max(d, minAnimDelay), maxAnimDelay))
	}
	return delays
}

// animated is the frame sequence of an animated image (SDD-011 4.7): the decode of every frame at the
// normalized size to raw RGBA by one FFmpeg process, packed into the sequence file.
func (j *job) animated(ctx context.Context, plan ImagePlan) error {
	sum := j.res.Summary
	delays := sum.FrameDelays
	g := ffargs.Graph{
		{
			In: []string{linkOf(plan.StreamIndex)},
			Filters: []ffargs.Filter{
				ffargs.Scale(plan.W, plan.H, "bicubic+accurate_rnd+full_chroma_int"),
				ffargs.Format("rgba"),
			},
			Out: []string{"n"},
		},
	}
	cmd := ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(j.original)}, Graph: g,
		Outputs: []ffargs.Output{
			{
				Format: "rawvideo",
				Pipe:   true,
				Maps:   []string{"[n]"},
				Opts: cat(
					[]ffargs.Opt{
						ffargs.Enum("-pix_fmt", "rgba"),
						ffargs.Enum("-fps_mode", "vfr"),
						ffargs.Int("-frames:v", len(delays)),
					},
					bitexact(),
				),
			},
		},
	}
	if err := j.runToFile(
		ctx,
		cmd,
		j.path("frames.rgba"),
		min(int64(plan.W)*int64(plan.H)*4*int64(len(delays)), ffmpeg.MaxAnimPixels*4),
	); err != nil {
		return err
	}
	frameBytes := int64(plan.W) * int64(plan.H) * 4
	st, err := os.Stat(j.path("frames.rgba"))
	if err != nil {
		return ingestErr(ReasonDecodeFailed, "The decoded frames could not be read back.",
			map[string]any{"step": StepVideo}, err)
	}
	if want := frameBytes * int64(len(delays)); st.Size() != want {
		return ingestErr(
			ReasonVerificationFailed,
			"The animated image did not decode to the frames it declares.",
			map[string]any{
				"step":           StepVideo,
				"frames":         len(delays),
				"expected_bytes": want,
				"actual_bytes":   st.Size(),
			},
			nil,
		)
	}
	if err := writeAnim(j.path("frames.rgba"), j.path("frames.anim"), plan.W, plan.H, delays); err != nil {
		return ingestErr(ReasonDecodeFailed, "The frames of the animated image could not be packed.",
			map[string]any{"step": StepVideo}, err)
	}
	_ = os.Remove(j.path("frames.rgba"))
	j.add(workFile{kind: AnimKind, profile: ProfileNormalized, ext: AnimExt, name: "frames.anim"})
	return nil
}

// writeAnim packs the raw RGBA frames of rawPath into the sequence file at dst. The PNG encoder is the
// one of the Go standard library with one setting, so the same frames give the same pixels back on every run.
func writeAnim(rawPath, dst string, w, h int, delays []int) error {
	raw, err := os.Open(rawPath)
	if err != nil {
		return err
	}
	defer func() { _ = raw.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// The table is written last, when the length of every PNG is known; its place is reserved first.
	head := make([]byte, animHeaderSize+animEntrySize*len(delays))
	copy(head, animMagic)
	binary.LittleEndian.PutUint32(head[4:], animVersion)
	binary.LittleEndian.PutUint32(head[8:], uint32(len(delays))) //nolint:gosec // at most MaxAnimFrames
	binary.LittleEndian.PutUint32(head[12:], uint32(w))          //nolint:gosec // at most MaxSourceEdge
	binary.LittleEndian.PutUint32(head[16:], uint32(h))          //nolint:gosec // at most MaxSourceEdge
	err = writeAnimFrames(raw, out, head, w, h, delays)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

func writeAnimFrames(raw io.Reader, out *os.File, head []byte, w, h int, delays []int) error {
	bw := bufio.NewWriter(out)
	if _, err := bw.Write(head); err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	buf := make([]byte, w*h*4)
	img := &image.NRGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	for i, d := range delays {
		if _, err := io.ReadFull(raw, buf); err != nil {
			return err
		}
		cw := &sizeWriter{w: bw}
		if err := enc.Encode(cw, img); err != nil {
			return err
		}
		binary.LittleEndian.PutUint32(
			head[animHeaderSize+animEntrySize*i:],
			uint32(d), //nolint:gosec // 10 to 60000
		)
		binary.LittleEndian.PutUint32(
			head[animHeaderSize+animEntrySize*i+4:],
			uint32(cw.n), //nolint:gosec // one PNG of at most 4 bytes a pixel
		)
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	_, err := out.WriteAt(head, 0)
	return err
}

type sizeWriter struct {
	w io.Writer
	n int64
}

func (c *sizeWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	return n, err
}

// Anim is an open frame sequence: the header and the table of the frames are read, the frames are not.
type Anim struct {
	f       *os.File
	W, H    int
	Delays  []int // display time of each frame, in milliseconds
	offsets []int64
	lengths []int64
}

// ErrAnim is wrapped by every error of a damaged or hostile sequence file.
var ErrAnim = errors.New("invalid animated image sequence")

// OpenAnim opens the frame sequence artifact at path and checks its table against the file size and the
// ceilings of SDD-007 9.1 before any frame is read.
func OpenAnim(path string) (*Anim, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a, err := readAnimHeader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return a, nil
}

func readAnimHeader(f *os.File) (*Anim, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	head := make([]byte, animHeaderSize)
	if _, err := io.ReadFull(f, head); err != nil || string(head[:4]) != animMagic ||
		binary.LittleEndian.Uint32(head[4:]) != animVersion {
		return nil, fmt.Errorf("%w: bad header", ErrAnim)
	}
	n := int64(binary.LittleEndian.Uint32(head[8:]))
	w, h := int64(binary.LittleEndian.Uint32(head[12:])), int64(binary.LittleEndian.Uint32(head[16:]))
	if n < 1 || n > ffmpeg.MaxAnimFrames || w < 1 || h < 1 || w > ffmpeg.MaxSourceEdge || h > ffmpeg.MaxSourceEdge ||
		n*w*h > ffmpeg.MaxAnimPixels {
		return nil, fmt.Errorf("%w: size %dx%d with %d frames is over the ceilings", ErrAnim, w, h, n)
	}
	table := make([]byte, n*animEntrySize)
	if _, err := io.ReadFull(f, table); err != nil {
		return nil, fmt.Errorf("%w: short table", ErrAnim)
	}
	a := &Anim{f: f, W: int(w), H: int(h), Delays: make([]int, n), offsets: make([]int64, n), lengths: make([]int64, n)}
	pos := int64(animHeaderSize) + n*animEntrySize
	for i := range n {
		d := binary.LittleEndian.Uint32(table[i*animEntrySize:])
		l := int64(binary.LittleEndian.Uint32(table[i*animEntrySize+4:]))
		if d < minAnimDelay || d > maxAnimDelay || l < 1 || l > animMaxFrame {
			return nil, fmt.Errorf("%w: frame %d has delay %d and length %d", ErrAnim, i, d, l)
		}
		a.Delays[i], a.offsets[i], a.lengths[i] = int(d), pos, l
		pos += l
	}
	if pos != st.Size() {
		return nil, fmt.Errorf("%w: the frames take %d bytes and the file has %d", ErrAnim, pos, st.Size())
	}
	return a, nil
}

// Frames is the number of frames of the sequence.
func (a *Anim) Frames() int { return len(a.Delays) }

// Frame decodes frame i into straight-alpha pixels (R, G, B, A, row by row, W*H*4 bytes). An index past
// the end is taken modulo the frame count, so the animation loops whatever the caller asks.
func (a *Anim) Frame(i int) ([]byte, error) {
	n := len(a.Delays)
	i = ((i % n) + n) % n
	im, err := png.Decode(io.NewSectionReader(a.f, a.offsets[i], a.lengths[i]))
	if err != nil {
		return nil, fmt.Errorf("%w: frame %d: %w", ErrAnim, i, err)
	}
	if b := im.Bounds(); b.Dx() != a.W || b.Dy() != a.H {
		return nil, fmt.Errorf("%w: frame %d is %dx%d, not %dx%d", ErrAnim, i, b.Dx(), b.Dy(), a.W, a.H)
	}
	nr, ok := im.(*image.NRGBA)
	if !ok {
		// An opaque frame is stored without an alpha channel and decodes to *image.RGBA.
		nr = image.NewNRGBA(image.Rect(0, 0, a.W, a.H))
		draw.Draw(nr, nr.Bounds(), im, im.Bounds().Min, draw.Src)
	}
	if nr.Stride == a.W*4 {
		return nr.Pix[:a.W*a.H*4], nil
	}
	pix := make([]byte, a.W*a.H*4)
	for y := range a.H {
		copy(pix[y*a.W*4:], nr.Pix[y*nr.Stride:y*nr.Stride+a.W*4])
	}
	return pix, nil
}

// Close closes the file.
func (a *Anim) Close() error { return a.f.Close() }

// verifyAnim checks the frame sequence just written against the plan: its table, its size, the delays
// of the probe, and a full decode of the first and the last frame.
func (j *job) verifyAnim(c *checks, plan ImagePlan, delays []int) {
	a, err := OpenAnim(j.path("frames.anim"))
	if err != nil {
		c.fail("frames.anim", "a readable frame sequence", err.Error())
		return
	}
	defer func() { _ = a.Close() }()
	c.eq("anim_size", fmt.Sprintf("%dx%d", plan.W, plan.H), fmt.Sprintf("%dx%d", a.W, a.H))
	c.eq("anim_delays", fmt.Sprint(delays), fmt.Sprint(a.Delays))
	for _, i := range []int{0, a.Frames() - 1} {
		if _, err := a.Frame(i); err != nil {
			c.fail("frames.anim", "a decodable frame", err.Error())
		}
	}
}

// runToFile runs an FFmpeg command whose only output is pipe:1 and keeps what it writes in the file at dst. The size
// watchdog kills the run once the file passes maxBytes, so a file with more frames than it declared cannot fill the disk (T-17).
func (j *job) runToFile(ctx context.Context, c ffargs.Cmd, dst string, maxBytes int64) error {
	argv, err := c.Argv(j.pol)
	if err != nil {
		return &ffmpeg.Error{
			Code:    ffmpeg.CodeInternal,
			Reason:  "argv_policy",
			Message: "the FFmpeg arguments were refused before any start",
			Err:     err,
		}
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return ingestErr(
			ReasonDecodeFailed,
			"A work file could not be written.",
			map[string]any{"step": StepVideo},
			err,
		)
	}
	res, err := j.r.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: j.wd, Argv: argv, Deadline: ffmpeg.IngestDeadline(0), DeadlineCeiling: ffmpeg.IngestDeadlineCeiling,
		Stdout:     out,
		OutputPath: dst, MaxOutputBytes: maxBytes,
	})
	if cerr := out.Close(); err == nil && cerr != nil {
		err = cerr
	}
	return mapRunErr(err, StepVideo, res.Stderr)
}
