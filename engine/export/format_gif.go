package export

import (
	"bufio"
	"compress/lzw"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// The GIF the export writes is made here, not by FFmpeg (SDD-004 section 8): the palette is a fixed
// 6x6x6 colour cube, each frame is dithered with Floyd-Steinberg in integer arithmetic, and the frames
// are written one after the other, so the same frames give the same bytes (NFR-010) and the memory
// is one frame, not the whole animation. A plan that is not transparent writes opaque frames. A
// transparent plan (VV-172) reserves palette index 255 for pixels of alpha 0.

// gifLevels is the number of levels of each channel of the palette; the palette has 216 of its 256 entries in use.
const gifLevels = 6

// gifLevelOf maps a channel value to its nearest level; gifLevelValue is the value of a level.
var (
	gifLevelOf    [256]uint8
	gifLevelValue [gifLevels]int
)

func init() {
	for i := range gifLevelValue {
		gifLevelValue[i] = i * 255 / (gifLevels - 1)
	}

	for v := range gifLevelOf {
		gifLevelOf[v] = uint8((v*(gifLevels-1) + 127) / 255)
	}
}

// errGIFLimit means the GIF grew past the size cap while it was written.
var errGIFLimit = errors.New("the GIF passed the size limit")

// gifWriter writes a GIF89a stream: the header, the global palette, an infinite loop and one frame
// at a time. limit is the size cap in bytes, 0 for none.
type gifWriter struct {
	w           *bufio.Writer
	n           int64
	limit       int64
	err         error
	width       int
	height      int
	frames      int
	errRow      [2][]int16 // the Floyd-Steinberg error of the row above and of this row, three channels per pixel
	idx         []byte
	finished    bool
	transparent bool
	blockBytes  [256]byte
}

func newGIFWriter(w io.Writer, width, height int, limit int64, transparent bool) *gifWriter {
	g := &gifWriter{
		w: bufio.NewWriterSize(w, 64<<10), width: width, height: height, limit: limit,
		transparent: transparent,
	}
	g.errRow[0] = make([]int16, (width+2)*3)
	g.errRow[1] = make([]int16, (width+2)*3)
	g.idx = make([]byte, width*height)

	g.put([]byte("GIF89a"))
	g.put16(width)
	g.put16(height)
	// global colour table of 256 entries (size field 7), colour resolution 8 bits.
	// A transparent GIF names index 255 as the background; an opaque one keeps index 0.
	bg := byte(0)
	if transparent {
		bg = 255
	}
	g.put([]byte{0xF7, bg, 0})

	for i := range 256 {
		var r, gr, b int
		if i < gifLevels*gifLevels*gifLevels {
			r, gr, b = gifLevelValue[i/(gifLevels*gifLevels)], gifLevelValue[i/gifLevels%gifLevels], gifLevelValue[i%gifLevels]
		}
		g.put([]byte{byte(r), byte(gr), byte(b)})
	}
	// NETSCAPE2.0: loop forever
	g.put([]byte{0x21, 0xFF, 0x0B})
	g.put([]byte("NETSCAPE2.0"))
	g.put([]byte{0x03, 0x01, 0, 0, 0})

	return g
}

func (g *gifWriter) put(b []byte) {
	if g.err != nil {
		return
	}

	g.n += int64(len(b))
	if g.limit > 0 && g.n > g.limit {
		g.err = errGIFLimit

		return
	}

	_, g.err = g.w.Write(b)
}

// put16 writes v little endian; sizes are at most 4096 and delays at most 65535, so the cut is exact.
func (g *gifWriter) put16(v int) { g.put([]byte{byte(v & 0xFF), byte(v >> 8 & 0xFF)}) }

// quantize turns the frame into palette indexes with Floyd-Steinberg error diffusion, left to right on every row.
func (g *gifWriter) quantize(img *image.RGBA) {
	cur, next := g.errRow[0], g.errRow[1]
	clear(cur)
	clear(next)

	for y := range g.height {
		clear(next)

		row := img.Pix[y*img.Stride : y*img.Stride+g.width*4]
		for x := range g.width {
			if g.transparent && row[x*4+3] == 0 {
				g.idx[y*g.width+x] = 255
				continue
			}
			var lv [3]int

			for c := range 3 {
				v := int(row[x*4+c]) + int(cur[(x+1)*3+c])/16
				v = max(0, min(255, v))
				l := int(gifLevelOf[v])
				lv[c] = l
				e := int16(v - gifLevelValue[l]) //nolint:gosec // within +-127
				cur[(x+2)*3+c] += e * 7
				next[x*3+c] += e * 3
				next[(x+1)*3+c] += e * 5
				next[(x+2)*3+c] += e
			}

			g.idx[y*g.width+x] = byte((lv[0]*gifLevels*gifLevels + lv[1]*gifLevels + lv[2]) & 0xFF)
		}

		cur, next = next, cur
	}
}

// frame appends one frame shown for delayCS hundredths of a second.
func (g *gifWriter) frame(img *image.RGBA, delayCS int) error {
	g.quantize(img)

	// graphic control. Opaque: disposal 1 (do not dispose), no transparent index.
	// Transparent: disposal 2 (restore to the background, which is the transparent index) so a hole
	// in a later frame does not show the previous picture, and index 255 is transparent.
	packed, trans := byte(0x04), byte(0)
	if g.transparent {
		packed, trans = 0x09, 255
	}
	g.put([]byte{0x21, 0xF9, 0x04, packed, byte(delayCS & 0xFF), byte(delayCS >> 8 & 0xFF), trans, 0})
	// image descriptor: the whole canvas, no local table, not interlaced
	g.put([]byte{0x2C, 0, 0, 0, 0})
	g.put16(g.width)
	g.put16(g.height)
	g.put([]byte{0})
	g.put([]byte{8}) // minimum LZW code size

	bw := &gifBlocks{g: g}
	zw := lzw.NewWriter(bw, lzw.LSB, 8)

	if _, err := zw.Write(g.idx); err != nil && g.err == nil {
		g.err = err
	}

	if err := zw.Close(); err != nil && g.err == nil {
		g.err = err
	}

	bw.flush()
	g.put([]byte{0}) // block terminator
	g.frames++

	return g.err
}

// finish writes the trailer and flushes.
func (g *gifWriter) finish() error {
	if g.finished {
		return g.err
	}

	g.finished = true
	g.put([]byte{0x3B})

	if g.err == nil {
		g.err = g.w.Flush()
	}

	return g.err
}

// gifBlocks cuts a byte stream into the sub-blocks of at most 255 bytes of a GIF.
type gifBlocks struct {
	g   *gifWriter
	n   int
	buf [255]byte
}

func (b *gifBlocks) Write(p []byte) (int, error) {
	total := len(p)

	for len(p) > 0 {
		c := copy(b.buf[b.n:], p)
		b.n += c
		p = p[c:]

		if b.n == len(b.buf) {
			b.flush()
		}
	}

	return total, b.g.err
}

func (b *gifBlocks) flush() {
	if b.n == 0 {
		return
	}

	b.g.blockBytes[0] = byte(b.n & 0xFF)
	copy(b.g.blockBytes[1:], b.buf[:b.n])
	b.g.put(b.g.blockBytes[:b.n+1])
	b.n = 0
}

// buildGIF composes the frames of the project, every gifStep-th one, and writes them as a GIF with
// the delays of gifDelaysCS. Nothing is handed to FFmpeg: the file is written here, frame by frame,
// and the size cap is checked as it grows.
func (s *state) buildGIF(ctx context.Context, rd *renderer) (built, error) {
	st := s.job.Doc.Settings
	width, height, scale := gifSize(st)

	s.scale, s.opts.OutputScale = scale, scale

	partial, err := s.dir.Path(s.job.JobID + ".partial.gif")
	if err != nil {
		return built{}, err
	}

	f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return built{}, exportErr(ReasonEncoder, "The GIF file could not be created.", err, "")
	}

	defer func() { _ = f.Close() }()

	step := gifStep(st.FPS)
	delays := gifDelaysCS(st.FPS, s.frames)
	g := newGIFWriter(f, width, height, s.job.MaxBytes, plan.Frame(s.job.Doc, 0).Transparent)
	buf := image.NewRGBA(image.Rect(0, 0, width, height))
	last := time.Now()

	for j, delay := range delays {
		if err := ctx.Err(); err != nil {
			return built{}, err
		}

		if err := rd.render(buf, j*step); err != nil {
			return built{}, wrapRun(err, ReasonRender, "A frame of the project could not be composed.", "")
		}

		if err := g.frame(buf, delay); err != nil {
			return built{}, s.gifWriteError(g, err)
		}

		if s.job.OnProgress != nil && (time.Since(last) >= progressEvery || j == len(delays)-1) {
			last = time.Now()
			s.job.OnProgress(Progress{Frame: min((j+1)*step, s.frames), Total: s.frames})
		}
	}

	if err := g.finish(); err != nil {
		return built{}, s.gifWriteError(g, err)
	}

	if err := f.Close(); err != nil {
		return built{}, exportErr(ReasonEncoder, "The GIF file could not be closed.", err, "")
	}

	ver, err := s.verifyGIF(partial, delays)

	ver.Width, ver.Height = width, height

	return built{partial: partial, encoder: "gif", ver: ver}, err
}

func (s *state) gifWriteError(g *gifWriter, err error) error {
	if errors.Is(err, errGIFLimit) {
		return sizeLimitError(g.n, s.job.MaxBytes)
	}

	return exportErr(ReasonEncoder, "The GIF could not be written.", err, "")
}

// verifyGIF walks the blocks of the file, without decoding the pixels, and requires: the size of the
// project, one frame for each delay that was planned with that delay, an infinite loop, a trailer
// that is the last byte, and a total time within one frame (at least 0.02 s) of the timeline.
func (s *state) verifyGIF(path string, delays []int) (Verified, error) {
	st := s.job.Doc.Settings
	want := float64(s.frames) * float64(st.FPS.Den) / float64(st.FPS.Num)
	tol := max(float64(st.FPS.Den)/float64(st.FPS.Num), 0.02)

	info, err := scanGIF(path)
	if err != nil {
		return Verified{}, verifyError([]Check{{Name: "gif_blocks", Expected: "readable GIF blocks", Got: err.Error()}})
	}

	var failed []Check
	fail := func(name, expected, got string) { failed = append(failed, Check{name, expected, got}) }

	wantW, wantH, _ := gifSize(st)
	if info.width != wantW || info.height != wantH {
		fail("dimensions", fmt.Sprintf("%dx%d", wantW, wantH), fmt.Sprintf("%dx%d", info.width, info.height))
	}

	if !slices.Equal(info.delays, delays) {
		fail(
			"frames",
			fmt.Sprintf("%d frames with the planned delays", len(delays)),
			fmt.Sprintf("%d frames", len(info.delays)),
		)
	}

	if info.loops != 0 || !info.looped {
		fail("loop", "an infinite loop", "no infinite loop")
	}

	total := 0
	for _, d := range info.delays {
		total += d
	}

	got := float64(total) / 100
	if math.Abs(got-want) > tol {
		fail("duration", fmt.Sprintf("%.3f s +- %.3f s", want, tol), strconv.FormatFloat(got, 'f', 3, 64))
	}

	if len(failed) > 0 {
		return Verified{}, verifyError(failed)
	}

	return Verified{DurationMS: int64(total) * 10, Size: info.size}, nil
}

type gifInfo struct {
	width, height int
	delays        []int
	looped        bool
	loops         int
	size          int64
}

// gifScanner reads the blocks of a GIF the engine wrote.
type gifScanner struct {
	r       *bufio.Reader
	info    gifInfo
	pending int // delay of the graphic control block that waits for its frame, -1 for none
}

func (s *gifScanner) read(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(s.r, b)

	return b, err
}

// subBlocks reads the sub-blocks of an extension, which the engine writes small.
func (s *gifScanner) subBlocks() ([][]byte, error) {
	var out [][]byte

	for {
		n, err := s.r.ReadByte()
		if err != nil {
			return nil, err
		}

		if n == 0 {
			return out, nil
		}

		b, err := s.read(int(n))
		if err != nil {
			return nil, err
		}

		out = append(out, b)
	}
}

// skipBlocks skips the sub-blocks of the image data.
func (s *gifScanner) skipBlocks() error {
	for {
		n, err := s.r.ReadByte()
		if err != nil {
			return err
		}

		if n == 0 {
			return nil
		}

		if _, err := s.r.Discard(int(n)); err != nil {
			return err
		}
	}
}

func (s *gifScanner) extension() error {
	label, err := s.r.ReadByte()
	if err != nil {
		return err
	}

	blocks, err := s.subBlocks()
	if err != nil {
		return err
	}

	switch {
	case label == 0xF9 && len(blocks) == 1 && len(blocks[0]) == 4:
		s.pending = int(binary.LittleEndian.Uint16(blocks[0][1:3]))
	case label == 0xFF && len(blocks) == 2 && string(blocks[0]) == "NETSCAPE2.0" && len(blocks[1]) == 3:
		s.info.looped = blocks[1][0] == 1
		s.info.loops = int(binary.LittleEndian.Uint16(blocks[1][1:3]))
	}

	return nil
}

func (s *gifScanner) frame() error {
	if s.pending < 0 {
		return errors.New("a frame without a graphic control block")
	}

	d, err := s.read(9)
	if err != nil {
		return err
	}

	if d[8]&0x80 != 0 {
		return errors.New("a local colour table: the engine writes none")
	}

	if _, err := s.r.ReadByte(); err != nil { // minimum code size
		return err
	}

	if err := s.skipBlocks(); err != nil {
		return err
	}

	s.info.delays = append(s.info.delays, s.pending)
	s.pending = -1

	return nil
}

// scanGIF reads the structure of a GIF the engine wrote: the header, the global table, then extensions,
// frames and the trailer, which has to be the last byte.
func scanGIF(path string) (gifInfo, error) {
	fh, err := os.Open(path)
	if err != nil {
		return gifInfo{}, err
	}
	defer func() { _ = fh.Close() }()

	fi, err := fh.Stat()
	if err != nil {
		return gifInfo{}, err
	}

	s := &gifScanner{r: bufio.NewReaderSize(fh, 64<<10), info: gifInfo{size: fi.Size()}, pending: -1}

	h, err := s.read(13)
	if err != nil {
		return s.info, err
	}

	if string(h[:6]) != "GIF89a" {
		return s.info, errors.New("not a GIF89a header")
	}

	s.info.width, s.info.height = int(binary.LittleEndian.Uint16(h[6:])), int(binary.LittleEndian.Uint16(h[8:]))

	if h[10]&0x80 != 0 {
		if _, err := s.r.Discard(3 << (h[10]&7 + 1)); err != nil {
			return s.info, err
		}
	}

	for {
		kind, err := s.r.ReadByte()
		if err != nil {
			return s.info, errors.New("no trailer: the file ends inside a block")
		}

		switch kind {
		case 0x3B:
			if _, err := s.r.ReadByte(); !errors.Is(err, io.EOF) {
				return s.info, errors.New("bytes after the trailer")
			}

			return s.info, nil
		case 0x21:
			err = s.extension()
		case 0x2C:
			err = s.frame()
		default:
			err = fmt.Errorf("an unknown block 0x%02x", kind)
		}

		if err != nil {
			return s.info, err
		}
	}
}
