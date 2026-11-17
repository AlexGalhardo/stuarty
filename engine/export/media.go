package export

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/compositor"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/engine/lut"
)

// layerInfo is what the pre-pass learns about a media layer over its whole item: whether its drawn
// size, rotation and crop never change, which lets the decoder deliver frames already cropped and
// scaled (SDD-004 section 6, pre-scaling, and section 12 rule 1).
type layerInfo struct {
	w, h, rot float64
	cx, cy    float64
	crop      plan.Rect
	constant  bool
	// cropVaries says Crop is not the same rectangle on every frame of the item (VV-120). The decoder
	// then delivers the whole frame and each frame is cropped in Go; one crop is not baked in.
	cropVaries bool
	// plain says no frame of the item flips, blurs, masks, transitions or applies an effect to it, and the
	// centre never moves: the visible part of the layer is the same on every frame, so the decoder may
	// deliver only that part (compositor.Image.FullW).
	plain bool
}

func isPlain(l *plan.Layer) bool {
	return !l.FlipX && !l.FlipY && l.Blur == 0 && l.Mask == nil && l.Transition == nil && len(l.Effects) == 0 &&
		l.Shadow == nil && l.Outline == nil
}

// scanLayers plans every frame once and records, for each media item, whether its size, rotation and
// crop are constant. A changing crop is recorded like a changing size: nothing may assume one crop.
// The plans are not kept: they are cheap to make again and a long project has many frames.
func (s *state) scanLayers() map[string]*layerInfo {
	infos := map[string]*layerInfo{}
	for t := s.offset; t < s.offset+s.frames; t++ {
		fp := plan.FrameWith(s.job.Doc, t, s.opts)
		for i := range fp.Layers {
			l := &fp.Layers[i]
			if l.Kind != plan.Media {
				continue
			}
			in, ok := infos[l.ItemID]
			if !ok {
				infos[l.ItemID] = &layerInfo{
					w: l.W, h: l.H, rot: l.Rotation, cx: l.CX, cy: l.CY, crop: l.Crop,
					constant: l.Rotation == 0, plain: isPlain(l),
				}
				continue
			}
			if l.Crop != in.crop {
				in.cropVaries = true
				in.constant = false
			}
			if l.W != in.w || l.H != in.h || l.Rotation != in.rot {
				in.constant = false
			}
			if l.CX != in.cx || l.CY != in.cy || !isPlain(l) {
				in.plain = false
			}
		}
	}
	return infos
}

// frameSource gives the compositor the pixels of media layers: decoded video frames, read in order
// from one FFmpeg process per active clip, and decoded images.
type frameSource struct {
	s     *state
	infos map[string]*layerInfo

	vids   map[string]*videoDecoder
	used   map[string]bool
	images map[string]compositor.Image    // decoded PNG by asset id
	placed map[placedKey]compositor.Image // an image cropped and scaled for one crop of its item

	luts map[string]*lut.Table // parsed lookup tables by asset id

	anims      map[string]*ingest.Anim      // open frame sequence of an animated image by asset id
	animFrames map[animKey]compositor.Image // its frames cropped and scaled for their item
	animBytes  int64
}

func newFrameSource(s *state, infos map[string]*layerInfo) *frameSource {
	return &frameSource{
		s: s, infos: infos,
		vids: map[string]*videoDecoder{}, used: map[string]bool{},
		images: map[string]compositor.Image{}, placed: map[placedKey]compositor.Image{},
		luts:  map[string]*lut.Table{},
		anims: map[string]*ingest.Anim{}, animFrames: map[animKey]compositor.Image{},
	}
}

// media is the compositor.MediaSource.
func (f *frameSource) media(l *plan.Layer) (compositor.Image, error) {
	a, ok := f.s.job.Doc.Assets[l.AssetID]
	if !ok {
		return compositor.Image{}, fmt.Errorf("the asset %s is not in the document", l.AssetID)
	}
	if a.IsAnimated() {
		return f.animImage(l, &a)
	}
	if a.Kind == timeline.AssetImage {
		return f.image(l, &a)
	}
	return f.video(l, &a)
}

// cropRect is the crop of the layer in whole source pixels, inside the asset.
func cropRect(l *plan.Layer, a *timeline.Asset) (x, y, w, h int) {
	x, y = max(round(l.Crop.X), 0), max(round(l.Crop.Y), 0)
	x, y = min(x, max(a.Width-1, 0)), min(y, max(a.Height-1, 0))
	w, h = max(round(l.Crop.W), 1), max(round(l.Crop.H), 1)
	return x, y, min(w, a.Width-x), min(h, a.Height-y)
}

func round(v float64) int { return int(math.Floor(v + 0.5)) }

// target is the size the decoder or the image scaler delivers: the drawn size when the layer is constant
// and not rotated (the compositor then copies), otherwise the cropped size of the source.
func (f *frameSource) target(l *plan.Layer, cw, ch int) (w, h int, scaled bool) {
	in := f.infos[l.ItemID]
	if (f.s.single || (in != nil && in.constant)) && l.Rotation == 0 {
		return max(round(l.W*f.s.scale), 1), max(round(l.H*f.s.scale), 1), true
	}
	return cw, ch, false
}

// placedKey is one delivered image of an item: the crop in source pixels and the size it was scaled
// to. A static crop is one key, so the cache still holds one picture; a changing crop is a key per
// frame and the first rectangle is never reused for a later one.
type placedKey struct {
	item       string
	x, y, w, h int
	tw, th     int
}

func (f *frameSource) image(l *plan.Layer, a *timeline.Asset) (compositor.Image, error) {
	src, ok := f.images[l.AssetID]
	if !ok {
		path, err := f.s.artifact(a, "png")
		if err != nil {
			return compositor.Image{}, err
		}
		if src, err = loadImage(path); err != nil {
			return compositor.Image{}, fmt.Errorf("image %s: %w", l.AssetID, err)
		}
		f.images[l.AssetID] = src
	}

	cx, cy, cw, ch := cropRect(l, a)
	cw, ch = min(cw, max(src.W-cx, 0)), min(ch, max(src.H-cy, 0))
	tw, th, scaled := f.target(l, cw, ch)
	key := placedKey{l.ItemID, cx, cy, cw, ch, tw, th}
	if img, ok := f.placed[key]; ok {
		return img, nil
	}
	img := cropImage(src, cx, cy, cw, ch)
	if scaled && (tw != cw || th != ch) {
		img = compositor.Resize(img, tw, th)
	}
	f.placed[key] = img
	return img, nil
}

func cropImage(src compositor.Image, x, y, w, h int) compositor.Image {
	if x == 0 && y == 0 && w == src.W && h == src.H {
		return src
	}
	pix := make([]byte, w*h*4)
	for row := range h {
		copy(pix[row*w*4:(row+1)*w*4], src.Pix[((y+row)*src.W+x)*4:((y+row)*src.W+x+w)*4])
	}
	return compositor.Image{Pix: pix, W: w, H: h, Opaque: src.Opaque}
}

// loadImage decodes a normalized PNG into premultiplied RGBA.
func loadImage(path string) (compositor.Image, error) {
	fh, err := os.Open(path)
	if err != nil {
		return compositor.Image{}, err
	}
	defer func() { _ = fh.Close() }()
	im, err := png.Decode(fh)
	if err != nil {
		return compositor.Image{}, err
	}
	b := im.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), im, b.Min, draw.Src)
	opaque := true
	for i := 3; i < len(rgba.Pix); i += 4 {
		if rgba.Pix[i] != 255 {
			opaque = false
			break
		}
	}
	return compositor.Image{Pix: rgba.Pix, W: b.Dx(), H: b.Dy(), Opaque: opaque}, nil
}

func (f *frameSource) video(l *plan.Layer, a *timeline.Asset) (compositor.Image, error) {
	d, err := f.decoder(l, a)
	if err != nil {
		return compositor.Image{}, err
	}
	pix, err := d.frame(l.SourceFrame)
	if err != nil {
		return compositor.Image{}, err
	}
	img := compositor.Image{Pix: pix, W: d.w, H: d.h, Opaque: !a.HasAlpha}
	if d.win.full {
		img.FullW, img.FullH, img.OffX, img.OffY = d.win.fullW, d.win.fullH, d.win.x, d.win.y
	}
	// A changing crop was not applied by FFmpeg: cut this frame's rectangle out of the whole picture.
	if d.each {
		cx, cy, cw, ch := cropRect(l, a)
		cw, ch = min(cw, max(img.W-cx, 0)), min(ch, max(img.H-cy, 0))
		img = cropImage(img, cx, cy, cw, ch)
	}

	return img, nil
}

// visibleWindow is the part of a drawn layer that lies inside the frame, and the part of the (already
// cropped) source that covers it.
type visibleWindow struct {
	full         bool // false: deliver the whole drawn size
	fullW, fullH int  // the drawn size
	x, y, w, h   int  // the window in drawn pixels
	sx, sy       int  // the source rectangle that covers it, in pixels of the cropped source
	sw, sh       int
}

// window is the visible window of a layer the decoder delivers at its drawn size w by h, when the layer
// is static for its whole item and more than a fifth of it falls outside the frame. The window is
// scaled from the source pixels under it, so the pixels differ from scaling the whole layer and cutting
// it only in the sub-pixel phase of the scaler.
func (f *frameSource) window(l *plan.Layer, cw, ch, w, h int) visibleWindow {
	in := f.infos[l.ItemID]
	if f.s.single || in == nil || !in.constant || !in.plain || f.s.job.PixelRenderer == PixelRendererRust ||
		f.s.job.PixelRenderer == PixelRendererGPU || w < 1 || h < 1 {
		return visibleWindow{}
	}

	st := f.s.job.Doc.Settings
	fw, fh := max(round(float64(st.Width)*f.s.scale), 1), max(round(float64(st.Height)*f.s.scale), 1)
	x0 := round(l.CX*f.s.scale - float64(w)/2)
	y0 := round(l.CY*f.s.scale - float64(h)/2)
	vx0, vy0 := max(-x0, 0), max(-y0, 0)
	vx1, vy1 := min(w, fw-x0), min(h, fh-y0)

	if vx1 <= vx0 || vy1 <= vy0 || 5*(vx1-vx0)*(vy1-vy0) > 4*w*h {
		return visibleWindow{}
	}

	kx, ky := float64(w)/float64(cw), float64(h)/float64(ch)
	sx0, sx1 := round(float64(vx0)/kx), min(round(float64(vx1)/kx), cw)
	sy0, sy1 := round(float64(vy0)/ky), min(round(float64(vy1)/ky), ch)

	if sx1 <= sx0 || sy1 <= sy0 {
		return visibleWindow{}
	}

	return visibleWindow{
		full: true, fullW: w, fullH: h, x: vx0, y: vy0, w: vx1 - vx0, h: vy1 - vy0,
		sx: sx0, sy: sy0, sw: sx1 - sx0, sh: sy1 - sy0,
	}
}

// deliverCrop is the rectangle the decoder cuts, in source pixels. When the crop changes across the
// item it is the whole frame (each is true) and video crops per frame; a static crop is applied once
// in the filter, which is what the existing goldens are built from.
func (f *frameSource) deliverCrop(l *plan.Layer, a *timeline.Asset) (x, y, w, h int, each bool) {
	if in := f.infos[l.ItemID]; in != nil && in.cropVaries {
		return 0, 0, a.Width, a.Height, true
	}
	x, y, w, h = cropRect(l, a)
	return x, y, w, h, false
}

// decoder is the decoder of the clip of the layer, made on the first use. A later frame whose crop
// is not the one this process was built for starts a new process, so a single-frame renderer that is
// asked twice does not keep the first crop.
func (f *frameSource) decoder(l *plan.Layer, a *timeline.Asset) (*videoDecoder, error) {
	f.used[l.ItemID] = true
	cx, cy, cw, ch, each := f.deliverCrop(l, a)
	d := f.vids[l.ItemID]
	if d != nil && (d.each != each || d.cx != cx || d.cy != cy || d.cw != cw || d.ch != ch) {
		d.stop()
		delete(f.vids, l.ItemID)
		d = nil
	}
	if d == nil {
		w, h, _ := f.target(l, cw, ch)
		win := f.window(l, cw, ch, w, h)
		path, err := f.s.artifact(a, "mov")
		if err != nil {
			return nil, err
		}
		var vf []ffargs.Filter
		if !each && (cx != 0 || cy != 0 || cw != a.Width || ch != a.Height) {
			vf = append(vf, ffargs.CropRect(cw, ch, cx, cy)...)
		}
		if win.full {
			// Scaling the whole drawn size costs in proportion to its pixels (a landscape clip covering a
			// portrait frame is 2.5 times the frame), so only the visible part of the source is scaled.
			vf = append(vf, ffargs.CropRect(win.sw, win.sh, win.sx, win.sy)...)
			w, h = win.w, win.h
		}
		vf = append(vf, ffargs.ScaleFrameToRGBA(w, h), ffargs.Format("rgba"))
		d = &videoDecoder{
			s:     f.s,
			path:  path,
			w:     w,
			h:     h,
			win:   win,
			vf:    ffargs.Chain1(vf...),
			rate:  a.FrameRate,
			item:  l.ItemID,
			alpha: a.HasAlpha,
			each:  each,
			cx:    cx,
			cy:    cy,
			cw:    cw,
			ch:    ch,
		}
		f.vids[l.ItemID] = d
	}
	return d, nil
}

// prestart starts the decoders of every video layer of a single frame before the first one is read.
// Starting an FFmpeg process costs tens of milliseconds before it delivers a byte, and the compositor
// asks for the layers one after the other, so a frame of two clips (a picture in picture, a
// transition) would wait for two starts in a row; with the processes started together the waits
// overlap (NFR-004).
func (f *frameSource) prestart(fp *plan.FramePlan) {
	for i := range fp.Layers {
		l := &fp.Layers[i]
		if l.Kind != plan.Media || math.Round(min(max(l.Opacity, 0), 1)*255) == 0 {
			continue
		}

		a, ok := f.s.job.Doc.Assets[l.AssetID]
		if !ok || a.Kind == timeline.AssetImage {
			continue
		}

		d, err := f.decoder(l, &a)
		if err != nil || (d.begun && d.curIdx == l.SourceFrame) {
			continue
		}

		d.pending = d.start(l.SourceFrame)
	}
}

// endFrame stops the decoders of the clips that were not drawn in the frame that just ended.
func (f *frameSource) endFrame() {
	for id, d := range f.vids {
		if !f.used[id] {
			d.stop()
			delete(f.vids, id)
		}
	}
	clear(f.used)
}

func (f *frameSource) close() {
	for id, d := range f.vids {
		d.stop()
		delete(f.vids, id)
	}
	for id, seq := range f.anims {
		_ = seq.Close()
		delete(f.anims, id)
	}
}

// restartGap is how many source frames a decoder reads and drops before it is cheaper to start a new
// process at the wanted frame.
const restartGap = 120

// videoDecoder is one FFmpeg process delivering the raw RGBA frames of a normalized clip in order, from the
// frame it was started at. A reader goroutine fills a few buffers ahead so that decoding overlaps composing.
type videoDecoder struct {
	s    *state
	item string
	path string
	w, h int
	vf   string
	rate timeline.Rational
	// alpha says the decoded pixels carry an alpha channel: they are premultiplied as they arrive.
	alpha bool
	win   visibleWindow
	// each says the filter delivers the whole frame and video crops per frame. cx, cy, cw, ch are the
	// rectangle the filter was built with, the whole frame when each is set.
	each   bool
	cx, cy int
	cw, ch int

	cancel context.CancelFunc
	out    chan []byte
	free   chan []byte
	exited chan struct{} // closed when the process has ended and the pipe holds all it will hold
	read   chan struct{} // closed when the reader goroutine has ended
	dir    *ffmpeg.RunDir
	runErr error
	stderr string

	cur    []byte
	curIdx int
	// begun is true while a process is running or has been started for this decoder, and pending is
	// the error of a start made ahead of the first frame (see prestart).
	begun   bool
	pending error
}

const decoderBuffers = 3

// singleExitGrace is how long the decoder of a single frame has to end by itself before it is killed.
const singleExitGrace = 300 * time.Millisecond

func (d *videoDecoder) frameBytes() int { return d.w * d.h * 4 }

// frame returns the pixels of source frame sf. A frame past the end of the media repeats the last one.
func (d *videoDecoder) frame(sf int) ([]byte, error) {
	if err := d.pending; err != nil {
		d.pending = nil

		return nil, err
	}
	if !d.begun || sf < d.curIdx || sf-d.curIdx > restartGap || (d.s.single && sf != d.curIdx) {
		if err := d.start(sf); err != nil {
			return nil, err
		}
	}
	for d.curIdx < sf {
		buf, ok := <-d.out
		if !ok {
			<-d.exited
			if d.cur != nil && d.runErr == nil {
				d.curIdx = sf
				break
			}
			return nil, d.failure()
		}
		if d.cur != nil {
			d.free <- d.cur
		}
		d.cur = buf
		d.curIdx++
	}
	return d.cur, nil
}

func (d *videoDecoder) failure() error {
	if err := d.s.ctx.Err(); err != nil {
		return err
	}
	cause := d.runErr
	if cause == nil {
		cause = io.ErrUnexpectedEOF
	}
	return wrapRun(cause, ReasonDecoder, "A clip of the project could not be decoded.", d.stderr)
}

// start (re)starts the process at source frame sf.
func (d *videoDecoder) start(sf int) error {
	d.stop()
	r := d.s.job.Runner
	dir, err := d.s.childDir()
	if err != nil {
		return err
	}
	ms := int64(sf) * 1000 * int64(d.rate.Den) / int64(d.rate.Num)
	opts := []ffargs.Opt{ffargs.Enum("-pix_fmt", "rgba")}
	if d.s.single {
		// A single frame is all that will be read: the process ends by itself after delivering it, so
		// closing the renderer has no process to kill and the answer does not wait for one.
		opts = append(opts, ffargs.Int("-frames:v", 1))
	}
	argv, err := ffargs.Cmd{
		// One filter thread: the scaler slices a frame by thread count (NFR-010, same pixels everywhere).
		Threads: r.In.ThreadLimit(), FilterThreads: 1,
		Inputs: []ffargs.Input{ffargs.FileInput(d.path).At(ms)},
		Outputs: []ffargs.Output{{
			Format: "rawvideo", Pipe: true, Maps: []string{"0:v:0"}, NoAudio: true, VF: d.vf,
			Opts: opts,
		}},
	}.Argv(r.Policy(dir, d.s.inputDirs...))
	if err != nil {
		_ = dir.Remove()
		return exportErr(ReasonDecoder, "The decoder command was refused.", err, "")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		_ = dir.Remove()
		return exportErr(ReasonDecoder, "A pipe for the decoder could not be made.", err, "")
	}

	ctx, cancel := context.WithCancel(d.s.ctx)
	d.cancel, d.dir = cancel, dir
	out, free := make(chan []byte, decoderBuffers), make(chan []byte, decoderBuffers)
	exited, read := make(chan struct{}), make(chan struct{})
	d.out, d.free, d.exited, d.read = out, free, exited, read
	d.runErr, d.stderr = nil, ""
	d.cur, d.curIdx, d.begun = nil, sf-1, true
	for range decoderBuffers {
		free <- make([]byte, d.frameBytes())
	}

	// The goroutines use their own copies of the channels: the one that closes the pipe can outlive
	// stop, and the next start replaces the fields.
	go func() {
		res, err := r.RunFFmpeg(ctx, ffmpeg.Request{
			Dir: dir, Argv: argv, InputDirs: d.s.inputDirs, Deadline: d.s.deadline,
			DeadlineCeiling: ffmpeg.ExportDeadlineCeiling, Stdout: pw,
		})
		_ = pw.Close()
		d.runErr, d.stderr = err, res.Stderr
		close(exited)
	}()
	alpha := d.alpha
	go func() {
		defer close(read)
		defer close(out)
		for {
			var buf []byte
			select {
			case buf = <-free:
			case <-ctx.Done():
				return
			}
			if err := readFrame(pr, buf); err != nil {
				return
			}
			if alpha {
				premultiply(buf)
			}
			select {
			case out <- buf:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		<-read
		<-exited
		_ = pr.Close()
	}()
	return nil
}

// premultiply turns straight RGBA into the premultiplied form the compositor blends, rounding to
// the nearest like the image/draw conversion of the stills.
func premultiply(pix []byte) {
	for i := 0; i+3 < len(pix); i += 4 {
		a := uint32(pix[i+3])
		if a == 255 {
			continue
		}

		for c := i; c < i+3; c++ {
			pix[c] = byte((uint32(pix[c])*a + 127) / 255) //nolint:gosec // at most 255 by construction
		}
	}
}

// pipeReadChunk is the most one read asks of the decoder pipe. The pipe of os.Pipe holds 4 KiB on
// Windows, and a read into a whole frame (33 MB at 4K, 105 MB when a portrait clip covers a landscape
// canvas) is slow there: every 4 KiB that arrives costs a call that handles the entire buffer, so a
// 3840x6827 frame took 1 s instead of 0.15 s. Reading the frame in pieces keeps each call small and
// is as fast on the other systems.
const pipeReadChunk = 64 << 10

// readFrame fills buf from r, pipeReadChunk bytes at a time.
func readFrame(r io.Reader, buf []byte) error {
	for len(buf) > 0 {
		n, err := io.ReadFull(r, buf[:min(len(buf), pipeReadChunk)])
		buf = buf[n:]
		if err != nil {
			return err
		}
	}
	return nil
}

// stop ends the process tree and waits until nothing of the decoder is left.
func (d *videoDecoder) stop() {
	if d.cancel == nil {
		return
	}
	if d.s.single {
		// The decoder of a single frame was told to stop after it, so it is already ending: give it a
		// moment before the kill, which on Windows starts taskkill and costs 150 ms of the answer.
		select {
		case <-d.exited:
		case <-time.After(singleExitGrace):
		}
	}
	d.cancel()
	<-d.exited
	<-d.read
	_ = d.dir.Remove()
	d.cancel, d.dir, d.cur, d.begun = nil, nil, nil, false
}

// childDirs hands out the run directories of the child processes of one export.
type childDirs struct {
	mu sync.Mutex
	n  int
}

// childDir makes the run directory of one child: every child has its own (SDD-007 6.4).
func (s *state) childDir() (*ffmpeg.RunDir, error) {
	s.children.mu.Lock()
	s.children.n++
	n := s.children.n
	s.children.mu.Unlock()
	d, err := s.job.Runner.NewJobRunDir(fmt.Sprintf("%s-c%d", s.job.JobID, n))
	if err != nil {
		return nil, wrapRun(err, ReasonRender, "A work folder for the export could not be made.", "")
	}
	return d, nil
}

// artifact is the path of the normalized artifact of an asset with the given extension.
func (s *state) artifact(a *timeline.Asset, ext string) (string, error) {
	return s.artifactOf(a, "normalized", ext)
}

// artifactOf is the path of the artifact of an asset with the given kind and extension, at the normalized profile.
func (s *state) artifactOf(a *timeline.Asset, kind, ext string) (string, error) {
	hash := strings.TrimPrefix(a.Hash, "sha256:")
	if !strings.HasPrefix(a.Hash, "sha256:") || len(hash) != 64 {
		return "", fmt.Errorf("the asset %s has no content hash", a.ID)
	}
	p := filepath.Join(s.job.ArtifactsDir, hash, ingest.ArtifactName(kind, ingest.ProfileNormalized, "", ext))
	if _, err := os.Stat(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", exportErr(
				ReasonRender, "The prepared media of "+a.Name+" is missing; it must be prepared again.", err, "",
			)
		}
		return "", err
	}
	return p, nil
}
