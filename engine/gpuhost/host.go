// Package gpuhost drives the native GPU host of the export, the child process vukem-render (ADR-0029 item 4,
// SDD-013 section 4.9). The child composes the frame plan on the GPU and is fed through its standard input and
// output, the way FFmpeg is fed frames. It runs under the process policy of FFmpeg (SDD-007 6.10): started by
// engine/ffmpeg, from an absolute path, with the minimal environment, a run directory, limits, a deadline and
// the kill of its whole process tree. This package is the only place that knows the pipe protocol of
// renderer/vukem-renderer-gpu/src/proto.rs.
package gpuhost

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/bitmap"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/lut"
	"github.com/AlexGalhardo/vukem-video/engine/renderhost"
)

// The kinds of request of the protocol.
const (
	kindInit    = 0
	kindTexture = 1
	kindBitmap  = 2
	kindRelease = 3
	kindRender  = 4
	kindLUT     = 5
)

// Statuses of a reply that are not the reasons of SDD-013 section 7.1 (1 to 7).
const (
	statusDevice = 8
)

const (
	maxIDBytes      = 64
	maxTextureSide  = 8192
	maxOutputSide   = 4096
	maxFrameBytes   = maxOutputSide * maxOutputSide * 4
	maxDetailBytes  = 256
	endGrace        = 3 * time.Second
	maxRequestBytes = maxTextureSide*maxTextureSide*4 + 4096
)

// ErrUnavailable means the GPU host could not be started: the binary is missing or refuses to run, there is
// no graphics adapter, or the sandbox hides the devices. The export then draws on the CPU path.
var ErrUnavailable = errors.New("gpuhost: the GPU host is not available")

var errChildEnded = errors.New("the GPU host ended")

// reasons maps the statuses 1 to 7 to their reasons (renderhost's statusReasons, with the sign dropped).
var reasons = [...]renderhost.Reason{
	1: renderhost.InvalidPlan, 2: renderhost.UnsupportedVersion, 3: renderhost.UnsupportedLayer,
	4: renderhost.MissingTexture, 5: renderhost.LimitExceeded, 6: renderhost.OutOfMemory,
	7: renderhost.InvalidArgument,
}

// Config is what Start needs.
type Config struct {
	// Path is the absolute path of vukem-render.
	Path string
	// Dir is the run directory: the child's working directory, HOME and temp directory.
	Dir *ffmpeg.RunDir
	// Deadline bounds the life of the child; the export deadline.
	Deadline time.Duration
	// MaxTextureBytes and MaxBitmapBytes are the budgets of the child; zero is its default
	// (renderhost.DefaultMaxTextureBytes, renderhost.DefaultMaxBitmapBytes).
	MaxTextureBytes, MaxBitmapBytes uint32
}

// Host is a running vukem-render. It is not safe for concurrent use.
type Host struct {
	in      *io.PipeWriter
	out     *bufio.Reader
	cancel  context.CancelFunc
	done    chan struct{}
	adapter string

	mu     sync.Mutex
	runErr error
	stderr string

	dead error
	buf  []byte
}

// Start runs vukem-render and waits for its answer to the init, which names the adapter. Any failure before
// that is ErrUnavailable. ctx ends the child with the kill of its process tree.
func Start(ctx context.Context, r *ffmpeg.Runner, cfg Config) (*Host, error) {
	if cfg.Dir == nil || cfg.Deadline <= 0 {
		return nil, fmt.Errorf("%w: a run directory and a deadline are required", ErrUnavailable)
	}

	tool, err := r.HelperTool(ctx, cfg.Path, cfg.Deadline)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h := &Host{in: inW, out: bufio.NewReaderSize(outR, 1<<20), cancel: cancel, done: make(chan struct{})}

	go func() {
		defer close(h.done)

		// The child's end is seen when its standard output closes: contrib/engine waits for the copy of
		// standard input, which would otherwise block in inR until the wait delay of the pipes.
		res, err := r.RunHelper(runCtx, ffmpeg.HelperRequest{
			Tool: tool, Dir: cfg.Dir, Deadline: cfg.Deadline, Stdin: inR,
			Stdout: &endWriter{w: outW, onEnd: func() { _ = inW.Close() }},
		})

		h.mu.Lock()
		h.runErr, h.stderr = err, res.Stderr
		h.mu.Unlock()

		_ = outW.CloseWithError(errChildEnded)
		_ = inR.CloseWithError(errChildEnded)
	}()

	init := binary.LittleEndian.AppendUint32(nil, cfg.MaxTextureBytes)
	init = binary.LittleEndian.AppendUint32(init, cfg.MaxBitmapBytes)

	body, err := h.call(kindInit, init)
	if err != nil {
		h.stop()

		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	h.adapter = string(body)

	return h, nil
}

// endWriter passes the child's standard output to the pipe and calls onEnd when that output ends, which is when
// the child has exited (or closed it). exec.Cmd copies with io.Copy, which uses ReadFrom.
type endWriter struct {
	w     *io.PipeWriter
	onEnd func()
}

func (e *endWriter) Write(p []byte) (int, error) { return e.w.Write(p) }

func (e *endWriter) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.Copy(struct{ io.Writer }{e.w}, r)
	e.onEnd()

	return n, err
}

// Adapter is the adapter the child drew on, name, backend and device type, to record next to a result.
func (h *Host) Adapter() string { return h.adapter }

// PutTexture uploads the premultiplied RGBA pixels of a media layer under its item id.
func (h *Host) PutTexture(item string, w, ht int, rgba []byte, opaque bool) error {
	if err := checkImage(item, w, ht, rgba); err != nil {
		return err
	}

	p := make([]byte, 0, 1+len(item)+9+w*ht*4)
	p = append(p, byte(len(item))) //nolint:gosec // G115: checkImage bounds it to 64
	p = append(p, item...)
	p = binary.LittleEndian.AppendUint32(p, uint32(w))  //nolint:gosec // G115: checkImage bounds it
	p = binary.LittleEndian.AppendUint32(p, uint32(ht)) //nolint:gosec // G115: checkImage bounds it

	if opaque {
		p = append(p, 1)
	} else {
		p = append(p, 0)
	}

	p = append(p, rgba[:w*ht*4]...)
	_, err := h.call(kindTexture, p)

	return err
}

// PutBitmap uploads a bitmap of the rasterizer under its key; the child premultiplies it.
func (h *Host) PutBitmap(key string, bm bitmap.Bitmap) error {
	if err := checkImage(key, bm.Width, bm.Height, bm.Pix); err != nil {
		return err
	}

	p := make([]byte, 0, 1+len(key)+40+bm.Width*bm.Height*4)
	p = append(p, byte(len(key))) //nolint:gosec // G115: checkImage bounds it to 64
	p = append(p, key...)
	p = binary.LittleEndian.AppendUint32(p, uint32(bm.Width))  //nolint:gosec // G115: checkImage bounds it
	p = binary.LittleEndian.AppendUint32(p, uint32(bm.Height)) //nolint:gosec // G115: checkImage bounds it

	for _, f := range []float64{bm.NaturalW, bm.NaturalH, bm.CenterDX, bm.CenterDY} {
		p = binary.LittleEndian.AppendUint64(p, math.Float64bits(f))
	}

	p = append(p, bm.Pix[:bm.Width*bm.Height*4]...)
	_, err := h.call(kindBitmap, p)

	return err
}

// PutLUT uploads the lookup table of a lut asset under the asset id (SDD-004 section 3.6).
func (h *Host) PutLUT(asset string, t *lut.Table) error {
	if len(asset) > maxIDBytes || t == nil || t.Size < lut.MinSize || t.Size > lut.MaxSize ||
		len(t.Data) != 3*t.Size*t.Size*t.Size {
		return &renderhost.Error{Reason: renderhost.InvalidArgument, Detail: "PutLUT"}
	}

	p := make([]byte, 0, 1+len(asset)+4+2*len(t.Data))
	p = append(p, byte(len(asset))) //nolint:gosec // G115: bounded to 64 above
	p = append(p, asset...)
	p = binary.LittleEndian.AppendUint32(p, uint32(t.Size))
	p = append(p, t.Bytes()...)
	_, err := h.call(kindLUT, p)

	return err
}

// Release forgets the texture, bitmap or lookup table with that id or key; an unknown one is not an error.
func (h *Host) Release(id string) error {
	if len(id) > maxIDBytes {
		return &renderhost.Error{Reason: renderhost.InvalidArgument, Detail: "Release"}
	}

	req := append([]byte{byte(len(id))}, id...) //nolint:gosec // G115: bounded to 64 above
	_, err := h.call(kindRelease, req)

	return err
}

// Render draws the frame into dst: RGBA, no stride padding, exactly Output.Width by Output.Height. The child
// draws the whole frame; a band is not used on this path.
func (h *Host) Render(in renderhost.RenderInput, dst *image.RGBA) error {
	w, ht := in.Output.Width, in.Output.Height
	if dst.Rect.Min != (image.Point{}) || dst.Rect.Dx() != w || dst.Rect.Dy() != ht || dst.Stride != 4*w {
		return &renderhost.Error{Reason: renderhost.InvalidArgument, Detail: "dst is not the output size"}
	}

	if w > maxOutputSide || ht > maxOutputSide {
		return &renderhost.Error{Reason: renderhost.LimitExceeded, Detail: "render_frame_side"}
	}

	in.Output.Y0, in.Output.Y1 = 0, 0

	text, err := in.MarshalJSON()
	if err != nil {
		return fmt.Errorf("gpuhost: encoding the render input: %w", err)
	}

	frame, err := h.call(kindRender, text)
	if err != nil {
		return err
	}

	if len(frame) != len(dst.Pix) {
		return h.fatal(&renderhost.Error{Reason: renderhost.RenderTrap, Detail: "protocol: a frame of the wrong size"})
	}

	copy(dst.Pix, frame)

	return nil
}

// Close ends the child: end of input first, so it exits by itself, then the kill of its tree when it does not
// within a few seconds. It returns what the run reported, nil for a clean end.
func (h *Host) Close() error {
	_ = h.in.Close()

	select {
	case <-h.done:
	case <-time.After(endGrace):
	}

	h.cancel()
	<-h.done

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.dead == nil && h.runErr != nil && !errors.Is(h.runErr, context.Canceled) {
		return h.runErr
	}

	return nil
}

func (h *Host) stop() {
	h.cancel()
	_ = h.in.Close()
	<-h.done
}

func checkImage(id string, w, h int, pix []byte) error {
	switch {
	case len(id) > maxIDBytes || w < 0 || h < 0:
		return &renderhost.Error{Reason: renderhost.InvalidArgument, Detail: "image"}
	case w > maxTextureSide || h > maxTextureSide:
		return &renderhost.Error{Reason: renderhost.LimitExceeded, Detail: "render_texture_side"}
	case len(pix) < w*h*4:
		return &renderhost.Error{
			Reason: renderhost.InvalidArgument, Detail: "the pixels are shorter than width x height x 4",
		}
	}

	return nil
}

// call sends one request and reads its reply. A reply with a reason is a *renderhost.Error and the child goes
// on; a failure of the pipes, the device or the protocol ends it, and every later call fails with the same error.
func (h *Host) call(kind byte, payload []byte) ([]byte, error) {
	if h.dead != nil {
		return nil, h.dead
	}

	if len(payload) > maxRequestBytes {
		return nil, &renderhost.Error{Reason: renderhost.LimitExceeded, Detail: "render_request_bytes"}
	}

	head := binary.LittleEndian.AppendUint32([]byte{kind}, uint32(len(payload))) //nolint:gosec // G115: bounded above

	if _, err := h.in.Write(head); err != nil {
		return nil, h.fail(err)
	}

	if _, err := h.in.Write(payload); err != nil {
		return nil, h.fail(err)
	}

	var rh [5]byte
	if _, err := io.ReadFull(h.out, rh[:]); err != nil {
		return nil, h.fail(err)
	}

	n := binary.LittleEndian.Uint32(rh[1:])
	if n > maxFrameBytes {
		return nil, h.fatal(
			&renderhost.Error{Reason: renderhost.RenderTrap, Detail: "protocol: a reply over the largest frame"},
		)
	}

	if cap(h.buf) < int(n) {
		h.buf = make([]byte, n)
	}

	body := h.buf[:n]
	if _, err := io.ReadFull(h.out, body); err != nil {
		return nil, h.fail(err)
	}

	switch status := rh[0]; {
	case status == 0:
		return body, nil
	case status >= 1 && int(status) < len(reasons):
		return nil, &renderhost.Error{Reason: reasons[status], Detail: detail(body)}
	case status == statusDevice:
		return nil, h.fatal(&renderhost.Error{Reason: renderhost.RenderTrap, Detail: "gpu: " + detail(body)})
	default:
		return nil, h.fatal(&renderhost.Error{Reason: renderhost.RenderTrap, Detail: "protocol: " + detail(body)})
	}
}

// fail is the error of a pipe that broke: the child ended, with the reason the run reported.
func (h *Host) fail(err error) error {
	h.cancel()
	<-h.done

	h.mu.Lock()
	run, stderr := h.runErr, h.stderr
	h.mu.Unlock()

	switch {
	case errors.Is(run, context.Canceled), errors.Is(run, context.DeadlineExceeded):
		return h.fatal(run)
	case run != nil:
		return h.fatal(fmt.Errorf("gpuhost: %w (%s)", run, detail([]byte(stderr))))
	}

	return h.fatal(fmt.Errorf("gpuhost: the child ended: %w (%s)", err, detail([]byte(stderr))))
}

// fatal discards the child: its stored textures are gone, so the export fails once instead of drawing on a
// different state.
func (h *Host) fatal(err error) error {
	if h.dead == nil {
		h.dead = err
		h.cancel()
		_ = h.in.Close()
	}

	return err
}

// detail is a message of the child kept short and printable, for an error that reaches a log.
func detail(b []byte) string {
	if len(b) > maxDetailBytes {
		b = b[len(b)-maxDetailBytes:]
	}

	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}

		return r
	}, strings.TrimSpace(string(b)))
}
