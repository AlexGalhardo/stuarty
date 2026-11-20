package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/cache"
)

// The processed sound of the timeline preview (SDD-004 section 7.1, epic 51): a file per source, rendered by the mixer
// of the export before the envelope and the loudness gain, kept in the cache folder audio-previews/ and served by
// key. The plan hands the key out; the route asks for the file; a file that is not there is rendered, a window of two
// seconds first and then the whole item.
const (
	// PreviewWindowSeconds is the length of the window that is rendered first.
	PreviewWindowSeconds = 2

	maxKnownPreviews     = 4096
	maxQueuedPreviews    = 16
	previewRetryAfter    = 30 * time.Second
	previewRenderTimeout = 30 * time.Minute
	previewMaxAge        = 31536000
)

// AudioPreviewInput is one render of the preview: the source to render, from the document of its item.
type AudioPreviewInput struct {
	Doc    *timeline.Document
	Source plan.AudioSource
	JobID  string
	// OutPath is a path that does not exist yet: the render writes the 16-bit WAV there.
	OutPath string
}

// AudioPreviewRenderer is implemented by an Exporter that can render the processed sound of one source.
type AudioPreviewRenderer interface {
	RenderAudioPreview(ctx context.Context, in AudioPreviewInput) error
}

// audioPreviews remembers, for each key that a plan window handed out, what is needed to render it later: the route
// receives only the key. A key belongs to the project whose plan handed it out: it is content-addressed, so it is not
// a secret, and another project's plan must not unlock it (SDD-007 T-90).
type audioPreviews struct {
	mu       sync.Mutex
	known    map[bitmapID]knownPreview
	order    []bitmapID
	inflight map[string]bool
	failed   map[string]time.Time
	queued   int

	once    sync.Once
	windows chan struct{} // one window is rendered at a time, and one whole item: two renders at most
	whole   chan struct{}
}

func (p *audioPreviews) setup() {
	p.once.Do(func() {
		p.inflight, p.failed = map[string]bool{}, map[string]time.Time{}
		p.windows, p.whole = make(chan struct{}, 1), make(chan struct{}, 1)
	})
}

// knownPreview is the item of a key: the key of its whole file, a document that holds only that item
// (plan.PreviewSubject), where the file starts on the timeline and how many frames it holds.
type knownPreview struct {
	key          string
	doc          *timeline.Document
	item         string
	start, count int
	// nested is set for an item inside a composition: doc then holds the composition item, and the document of the
	// item's own timeline is found with the lookup when a target is resolved (SDD-004 section 7.1).
	nested *nestedPreview
}

// nestedPreview is what resolving the file of an item inside a composition needs besides the composition's document:
// where to find the timelines that the composition uses, and the whole source that the key was made from.
type nestedPreview struct {
	opts  plan.Options
	whole plan.AudioSource
	owner *timeline.Document // the document of the item's timeline, with that item only (plan.PreviewSubject)
}

func (p *audioPreviews) remember(project, key, item string, doc *timeline.Document, start, count int) {
	p.store(project, key, knownPreview{
		key: key, doc: plan.PreviewSubject(doc, item), item: item, start: start, count: count,
	})
}

// rememberNested is remember for an entry of the plan that comes from a composition.
func (p *audioPreviews) rememberNested(
	project, key, item string, doc *timeline.Document, opts plan.Options, start, count int,
) {
	top, _, _ := strings.Cut(item, "/")
	comp := doc.Items[top]
	k := knownPreview{
		key: key, doc: plan.CompositionSubject(doc, top), item: item, start: start, count: count,
		nested: &nestedPreview{opts: plan.Options{Nested: opts.Nested}},
	}

	// The key was made from the sound of the whole composition item, not from the frames of one window.
	whole, owner, ok := plan.NestedPreviewSource(k.doc, k.nested.opts, item, comp.Start, comp.Start+comp.Duration)
	if !ok {
		return
	}

	// The item's own id: a nested item of a nested composition has more than one slash.
	k.nested.whole, k.nested.owner = whole, plan.PreviewSubject(owner, item[strings.LastIndex(item, "/")+1:])
	p.store(project, key, k)
}

func (p *audioPreviews) store(project, key string, k knownPreview) {
	id := bitmapID{project, key}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.known == nil {
		p.known = map[bitmapID]knownPreview{}
	}

	if _, ok := p.known[id]; ok {
		return
	}

	if len(p.order) >= maxKnownPreviews {
		delete(p.known, p.order[0])
		p.order = p.order[1:]
	}

	p.known[id] = k
	p.order = append(p.order, id)
}

func (p *audioPreviews) lookup(project, key string) (knownPreview, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	k, ok := p.known[bitmapID{project, key}]

	return k, ok
}

// registerAudioPreviews makes the processed sound of the items of a plan window askable by key.
func (s *svc) registerAudioPreviews(project string, doc *timeline.Document, opts plan.Options, w *plan.WindowPlan) {
	for i := range w.Audio {
		pa := w.Audio[i].Processed
		if pa == nil {
			continue
		}

		if w.Audio[i].Timeline != "" {
			s.previews.rememberNested(project, pa.Key, w.Audio[i].Item, doc, opts, pa.Start, pa.Frames)

			continue
		}

		s.previews.remember(project, pa.Key, w.Audio[i].Item, doc, pa.Start, pa.Frames)
	}
}

// previewTarget is what one request asks for: the source of the file, its name in the cache and the frames it covers.
type previewTarget struct {
	src         plan.AudioSource
	key         string // the key of the file: the one of the request, or the one of the window
	first, last int    // the frames of the whole file that this one covers, counted from its first, the last inclusive
	window      bool
	job         string             // the id of the work folder of the render
	doc         *timeline.Document // the document that holds the asset of the source
}

// target resolves a request: the whole item, or the window of PreviewWindowSeconds that starts at the frame from of
// the file. from counts from the first frame of the file, not of the timeline: a key is shared by every item with the
// same source wherever it sits, and the item that handed it out first may since have moved, so only a frame of the
// file names one sound for all of them (SDD-004 section 7.1).
func (k *knownPreview) target(key, from string) (*previewTarget, error) {
	doc := k.doc

	if from == "" {
		if k.nested != nil {
			return &previewTarget{
				src: k.nested.whole, key: key, first: 0, last: k.count - 1, doc: k.nested.owner,
			}, nil
		}

		src, owner, ok := k.source(k.start, k.start+k.count)
		if !ok {
			return nil, newError(command.CodeNotFound, "there is no processed sound with that key.")
		}

		return &previewTarget{src: src, key: key, first: 0, last: k.count - 1, doc: owner}, nil
	}

	n, err := strconv.Atoi(from)
	if err != nil || n < 0 || n >= k.count {
		return nil, newError(
			command.CodeValidation, "from %q is not a frame of the file, 0 to %d.", from, k.count-1,
		).withPath("from")
	}

	fps := doc.Settings.FPS
	window := (PreviewWindowSeconds*fps.Num + fps.Den - 1) / fps.Den
	to := min(n+window, k.count)

	src, owner, ok := k.source(k.start+n, k.start+to)
	if !ok {
		return nil, newError(command.CodeNotFound, "there is no processed sound with that key.")
	}

	return &previewTarget{
		src:    src,
		key:    plan.PreviewKey(owner, &src),
		first:  n,
		last:   to - 1,
		window: true,
		doc:    owner,
	}, nil
}

// source is the source of the file over the timeline frames [from, to) and the document that holds its asset: the
// item's own, or for an item inside a composition the one of the timeline it sits in.
func (k *knownPreview) source(from, to int) (plan.AudioSource, *timeline.Document, bool) {
	if k.nested != nil {
		src, owner, ok := plan.NestedPreviewSource(k.doc, k.nested.opts, k.item, from, to)
		if !ok {
			return plan.AudioSource{}, nil, false
		}

		return src, plan.PreviewSubject(owner, k.item[strings.LastIndex(k.item, "/")+1:]), true
	}

	src, ok := plan.PreviewSource(k.doc, k.item, from, to)
	if !ok {
		return plan.AudioSource{}, nil, false
	}

	return src, k.doc, true
}

// openAudioPreview serves the processed sound of an item by key (SDD-003 section 6.5): the file when it is stored,
// else it starts the render and answers that the file is not ready.
func (s *svc) openAudioPreview(project, key, from string) (*File, error) {
	notFound := func() error {
		return newError(command.CodeNotFound, "there is no processed sound with that key.").
			withHint("read the plan of the window again")
	}

	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		return nil, notFound()
	}

	known, ok := s.previews.lookup(project, key)
	if !ok {
		return nil, notFound()
	}

	if s.media.Frames == nil || s.media.Frames.Cache == nil {
		return nil, newError(command.CodeInternal, "the processed sound is not available in this build.")
	}

	t, err := known.target(key, from)
	if err != nil {
		return nil, err
	}

	store, name := s.media.Frames.Cache, cache.AudioPreviewKey(t.key)

	if store.Has(name) {
		return s.previewFile(name, t), nil
	}

	if err := s.startPreview(store, &known, t); err != nil {
		return nil, err
	}

	return &File{Pending: true}, nil
}

func (s *svc) previewFile(name string, t *previewTarget) *File {
	f := &File{
		Path: name, ContentType: "audio/wav", ETag: `"ap-` + t.key + `"`, ModTime: time.Unix(0, 0).UTC(),
		Immutable: true, MaxAge: previewMaxAge,
	}

	if info, err := os.Stat(filepath.Join(s.workspace, filepath.FromSlash(name))); err == nil {
		f.Size = info.Size()
	}

	if t.window {
		f.Header = map[string]string{"X-Stuarty-Window": strconv.Itoa(t.first) + "-" + strconv.Itoa(t.last)}
	}

	return f
}

// startPreview starts the render of a target once, however many ask for it. A window is followed by the render of the
// whole item.
func (s *svc) startPreview(store *cache.Store, known *knownPreview, t *previewTarget) error {
	renderer, ok := s.exporter.(AudioPreviewRenderer)
	if !ok {
		return newError(command.CodeInternal, "the processed sound is not available in this build.")
	}

	// The file is a 48 kHz stereo 16-bit WAV: 4 bytes a sample after a 44-byte header. One the cache could never hold
	// would be rendered in full, refused by Commit, and rendered again after every pause of previewRetryAfter.
	if 44+4*t.src.Samples > store.Max() {
		return newError(command.CodeLimit, "the processed sound of the item is larger than the cache.").
			withHint("raise STUARTY_CACHE_MAX_BYTES; the item plays its windows until then")
	}

	p := &s.previews
	p.setup()

	lane := p.whole

	if t.window {
		lane = p.windows
	}

	p.mu.Lock()

	if p.inflight[t.key] {
		p.mu.Unlock()

		return nil
	}

	if until, bad := p.failed[t.key]; bad && s.now().Before(until) {
		p.mu.Unlock()

		return newError(command.CodeInternal, "the processed sound could not be rendered.").
			withHint("change the item or try again in a moment")
	}

	if p.queued >= maxQueuedPreviews {
		p.mu.Unlock()

		return newError(CodeQueueFull, "too many processed sounds are waiting to be rendered.").
			withHint("try again in a moment")
	}

	p.queued++
	p.inflight[t.key] = true
	// Made here, under the lock: the id generator of a service is not asked to be safe for two renders at once.
	t.job = s.newIDs("job")
	p.mu.Unlock()

	go s.renderPreview(renderer, store, known, *t, lane)

	return nil
}

func (s *svc) renderPreview(
	renderer AudioPreviewRenderer,
	store *cache.Store,
	known *knownPreview,
	t previewTarget,
	lane chan struct{},
) {
	lane <- struct{}{}

	err := s.renderPreviewFile(renderer, store, &t)

	<-lane

	p := &s.previews

	p.mu.Lock()
	p.queued--
	delete(p.inflight, t.key)

	if err != nil {
		// The failures that may be tried again are forgotten, so that a caller who asks for one window after another
		// of a source that cannot be decoded does not grow the map without end.
		now := s.now()
		for key, until := range p.failed {
			if !now.Before(until) {
				delete(p.failed, key)
			}
		}

		p.failed[t.key] = now.Add(previewRetryAfter)
	} else {
		delete(p.failed, t.key)
	}
	p.mu.Unlock()

	if err != nil {
		s.log.Warn("audio preview render failed", "key", t.key, "error", err.Error())

		return
	}

	// The window was heard first; now the rest of the item.
	if t.window {
		if whole, werr := known.target(known.key, ""); werr == nil && !store.Has(cache.AudioPreviewKey(whole.key)) {
			_ = s.startPreview(store, known, whole)
		}
	}
}

func (s *svc) renderPreviewFile(
	renderer AudioPreviewRenderer,
	store *cache.Store,
	t *previewTarget,
) error {
	name := cache.AudioPreviewKey(t.key)

	part, err := store.PartPath(name)
	if err != nil {
		return err
	}

	// A leftover of a render that was cut short: the render creates its file new.
	_ = os.Remove(part)

	ctx, cancel := context.WithTimeout(context.Background(), previewRenderTimeout)
	defer cancel()

	if err := renderer.RenderAudioPreview(ctx, AudioPreviewInput{
		Doc: t.doc, Source: t.src, JobID: t.job, OutPath: part,
	}); err != nil {
		_ = os.Remove(part)

		return err
	}

	return store.Commit(name)
}
