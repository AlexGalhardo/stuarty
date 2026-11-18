package gioapp

import (
	"context"
	"image"
	"sync"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
	"github.com/AlexGalhardo/vukem-video/engine/export"
)

const previewWidth = 640

// stage is the live preview. One sequence renderer decodes video ahead, the
// same way an export does, and a second one mixes the audio. Frames are the
// composited picture, not a JPEG fetched per frame.
type stage struct {
	sess *Session
	wake func()

	vmu  sync.Mutex
	vren *export.Renderer
	vrev int64

	amu  sync.Mutex
	aren *export.Renderer
	arev int64
	w, h int

	audioWG sync.WaitGroup

	mu       sync.Mutex
	img      image.Image
	at       int
	seq      int
	playing  bool
	err      string
	audioMsg string
	token    int
	cancel   context.CancelFunc
	failed   bool
}

func newStage(sess *Session, wake func()) *stage {
	return &stage{sess: sess, wake: wake}
}

type stageSnap struct {
	img     image.Image
	at      int
	seq     int
	playing bool
	err     string
	audio   string
}

func (st *stage) snapshot() stageSnap {
	if st == nil {
		return stageSnap{}
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	return stageSnap{
		img: st.img, at: st.at, seq: st.seq, playing: st.playing, err: st.err, audio: st.audioMsg,
	}
}

func (st *stage) poke() {
	if st.wake != nil {
		st.wake()
	}
}

func (st *stage) pause() {
	if st == nil {
		return
	}

	st.mu.Lock()
	st.token++

	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}

	st.playing = false
	st.mu.Unlock()
	st.poke()
}

func (st *stage) show(doc *timeline.Document, rev int64, frame int) {
	if st == nil || doc == nil || st.failed {
		return
	}

	st.mu.Lock()
	st.token++
	token := st.token

	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}

	st.playing = false
	st.mu.Unlock()

	go func() {
		img, err := st.render(doc, rev, frame)
		st.finish(token, img, frame, false, err)
	}()
}

func (st *stage) play(doc *timeline.Document, rev int64, frame int) {
	if st == nil || doc == nil || st.failed {
		return
	}

	st.mu.Lock()
	st.token++
	token := st.token

	if st.cancel != nil {
		st.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	st.cancel = cancel
	st.playing = true
	st.mu.Unlock()

	go st.playLoop(ctx, token, doc, rev, frame)
}

func (st *stage) playLoop(ctx context.Context, token int, doc *timeline.Document, rev int64, origin int) {
	defer func() {
		st.mu.Lock()

		if token == st.token {
			st.playing = false

			if st.cancel != nil {
				st.cancel()
				st.cancel = nil
			}
		}

		st.mu.Unlock()
		st.poke()
	}()

	go st.audio(ctx, doc, rev, origin)

	fps := fpsOf(doc)
	start := time.Now()
	end := projectEnd(doc)
	last := -1

	for {
		if ctx.Err() != nil {
			return
		}

		target := min(frameAfter(origin, time.Since(start), fps), end)

		if target != last {
			img, err := st.render(doc, rev, target)
			if ctx.Err() != nil {
				return
			}

			st.finish(token, img, target, true, err)
			last = target
		}

		if target >= end {
			return
		}

		time.Sleep(8 * time.Millisecond)
	}
}

func (st *stage) finish(token int, img image.Image, frame int, playing bool, err error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if token != st.token {
		return
	}

	if err != nil {
		st.err = err.Error()

		return
	}

	st.img = img
	st.at = frame
	st.seq++
	st.err = ""
	st.playing = playing
	st.poke()
}

func (st *stage) render(doc *timeline.Document, rev int64, frame int) (image.Image, error) {
	st.vmu.Lock()
	defer st.vmu.Unlock()

	ren, err := st.ensureVideo(doc, rev)
	if err != nil {
		st.mu.Lock()
		st.failed = true
		st.err = err.Error()
		st.mu.Unlock()

		return nil, err
	}

	if frame < 0 {
		frame = 0
	}

	return ren.RenderSize(context.Background(), frame, st.w, st.h)
}

func (st *stage) ensureVideo(doc *timeline.Document, rev int64) (*export.Renderer, error) {
	if st.vren != nil && st.vrev == rev {
		return st.vren, nil
	}

	if st.vren != nil {
		_ = st.vren.Close()
		st.vren = nil
	}

	cfg, err := st.config(doc, "desk-preview")
	if err != nil {
		return nil, err
	}

	ren, err := export.NewSequenceRenderer(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	st.w, st.h = previewSize(doc)
	st.vren, st.vrev = ren, rev

	return ren, nil
}

func (st *stage) audio(ctx context.Context, doc *timeline.Document, rev int64, from int) {
	st.amu.Lock()
	st.audioWG.Wait()
	st.audioWG.Add(1)
	ren, err := st.ensureAudio(doc, rev)
	st.amu.Unlock()

	defer st.audioWG.Done()

	if err != nil || ren == nil {
		return
	}

	_, _, bin, err := st.sess.Engine(ctx)
	if err != nil {
		return
	}

	sp, err := openSpeaker(ctx, bin)
	if err != nil {
		st.mu.Lock()
		st.audioMsg = "Audio output is unavailable; the picture still plays."
		st.mu.Unlock()
		st.poke()

		return
	}

	defer sp.close()

	n := projectEnd(doc) - from
	if n < 1 {
		return
	}

	_ = ren.MixFrom(ctx, from, n, sp.write)
}

func (st *stage) ensureAudio(doc *timeline.Document, rev int64) (*export.Renderer, error) {
	if st.aren != nil && st.arev == rev {
		return st.aren, nil
	}

	if st.aren != nil {
		_ = st.aren.Close()
		st.aren = nil
	}

	cfg, err := st.config(doc, "desk-audio")
	if err != nil {
		return nil, err
	}

	ren, err := export.NewSequenceRenderer(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	st.aren, st.arev = ren, rev

	return ren, nil
}

func (st *stage) config(doc *timeline.Document, job string) (export.RenderConfig, error) {
	runner, artifacts, _, err := st.sess.Engine(context.Background())
	if err != nil {
		return export.RenderConfig{}, err
	}

	return export.RenderConfig{
		Runner: runner, Doc: doc, JobID: job, ArtifactsDir: artifacts,
	}, nil
}

// ShowSync renders one frame on this goroutine. The gallery uses it.
func (st *stage) ShowSync(doc *timeline.Document, rev int64, frame int) error {
	img, err := st.render(doc, rev, frame)
	st.finish(st.currentToken(), img, frame, false, err)

	return err
}

func (st *stage) currentToken() int {
	st.mu.Lock()
	defer st.mu.Unlock()

	return st.token
}

// ClosePreview stops playback and the decoders.
func (ui *UI) ClosePreview() {
	if ui != nil && ui.stage != nil {
		ui.stage.Close()
	}
}

// Close stops playback and the decoders.
func (st *stage) Close() {
	if st == nil {
		return
	}

	st.pause()
	st.audioWG.Wait()
	st.vmu.Lock()
	st.amu.Lock()

	if st.vren != nil {
		_ = st.vren.Close()
		st.vren = nil
	}

	if st.aren != nil {
		_ = st.aren.Close()
		st.aren = nil
	}

	st.amu.Unlock()
	st.vmu.Unlock()
}

func previewSize(doc *timeline.Document) (int, int) {
	w := previewWidth
	if doc.Settings.Width > 0 && doc.Settings.Width < w {
		w = doc.Settings.Width
	}

	if w%2 != 0 {
		w--
	}

	if w < 16 {
		w = 16
	}

	h := 360
	if doc.Settings.Width > 0 {
		h = w * doc.Settings.Height / doc.Settings.Width
	}

	if h%2 != 0 {
		h++
	}

	if h < 16 {
		h = 16
	}

	return w, h
}

func projectEnd(doc *timeline.Document) int {
	end := 0

	if doc != nil {
		for id := range doc.Items {
			it := doc.Items[id]
			if it.Start+it.Duration > end {
				end = it.Start + it.Duration
			}
		}
	}

	if end < 1 {
		return 1
	}

	return end
}

func fpsOf(doc *timeline.Document) timeval.FPS {
	if doc == nil || doc.Settings.FPS.Num <= 0 || doc.Settings.FPS.Den <= 0 {
		return timeval.FPS{Num: 30, Den: 1}
	}

	return timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}
}
