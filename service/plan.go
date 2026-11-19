package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/cache"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

// PlanWindowRequest asks for the render plan of frames From up to, not
// including, Until (SDD-004 section 10).
type PlanWindowRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	From       int    `json:"from"`
	Until      int    `json:"until"`
	// Width is the output width in pixels the preview draws at; 0 is the project width. The
	// bitmaps of the plan are rasterized at that scale, as ViewFrame does (SDD-004 section 10).
	Width int `json:"width,omitempty"`
	// Pin, when not 0, must be the current revision (SDD-003 section 6.3). Set
	// by the HTTP adapter from rev; never on the wire of the Service.
	Pin int64 `json:"-"`
}

// PlanWindowResult is the compact plan of a window; Revision is the revision it
// was read at.
type PlanWindowResult = plan.WindowPlan

// The widths a plan window may ask the bitmaps to be drawn for.
const (
	minPlanWidth = 16
	maxPlanWidth = 4096
)

const (
	// maxKnownBitmaps bounds the specs kept to answer the bitmap route.
	maxKnownBitmaps = 8192
	bitmapMaxAge    = 31536000
)

// bitmaps remembers the spec behind each key that a plan window handed out: the
// bitmap route receives only the key. Each spec belongs to the project whose plan
// handed it out: a key is content-addressed, so it is not a secret, and another
// project's plan must not unlock it. A key the project's plan did not hand out
// (or the server forgot at a restart) is not found, and the client asks for the
// plan again.
//
// The key of a spec names the font family, never the font file. The plan therefore hands out a
// bitmap drawn with a user font under a key that also names the files of that family (PlanWindow
// renames it), and the bitmap is drawn by a cache that holds only documents with the same font
// files: two projects that name a family alike never share a bitmap, on disk or in memory, and a
// browser that caches a key for a year never shows a stale font after the family changes files.
type bitmaps struct {
	mu     sync.Mutex
	specs  map[bitmapID]knownBitmap
	order  []bitmapID
	cache  *rasterize.Cache
	caches map[string]*rasterize.Cache // by font fingerprint, user fonts only
}

type bitmapID struct{ project, key string }

type knownBitmap struct {
	spec        plan.BitmapSpec
	fonts       *rasterize.FontSet
	fingerprint string // of every user font of the document; empty with none
}

const (
	maxFontCaches   = 4
	fontCacheBudget = 16 << 20
)

func (b *bitmaps) remember(
	project, key string,
	spec plan.BitmapSpec,
	doc *timeline.Document,
	fonts *rasterize.FontSet,
) {
	id := bitmapID{project, key}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.specs == nil {
		b.specs = map[bitmapID]knownBitmap{}
	}

	if _, ok := b.specs[id]; ok {
		return
	}

	if len(b.order) >= maxKnownBitmaps {
		delete(b.specs, b.order[0])
		b.order = b.order[1:]
	}

	b.specs[id] = knownBitmap{spec: spec, fonts: fonts, fingerprint: fontFingerprint(doc)}
	b.order = append(b.order, id)
}

func (b *bitmaps) known(project, key string) (knownBitmap, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	k, ok := b.specs[bitmapID{project, key}]

	return k, ok
}

// rasterCache returns the in-process cache that draws bitmaps of documents with the same user
// font files as k.
func (b *bitmaps) rasterCache(k *knownBitmap) *rasterize.Cache {
	b.mu.Lock()
	defer b.mu.Unlock()

	if k.fingerprint == "" {
		if b.cache == nil {
			b.cache = rasterize.NewCache(rasterize.Bundled(), 0)
		}

		return b.cache
	}

	if c, ok := b.caches[k.fingerprint]; ok {
		return c
	}

	if b.caches == nil || len(b.caches) >= maxFontCaches {
		b.caches = map[string]*rasterize.Cache{}
	}

	c := rasterize.NewCache(k.fonts, fontCacheBudget)
	b.caches[k.fingerprint] = c

	return c
}

// planOutputScale is width / project width; 0 is the project width, scale 0 meaning 1.
func planOutputScale(width, projectWidth int) (float64, *Error) {
	if width == 0 {
		return 0, nil
	}

	if width < minPlanWidth || width > maxPlanWidth {
		return 0, newError(
			command.CodeValidation,
			"the width %d is not %d to %d pixels.",
			width,
			minPlanWidth,
			maxPlanWidth,
		).
			withPath("width")
	}

	return float64(width) / float64(projectWidth), nil
}

// PlanWindow reads the render plan of a window of the current revision. The
// editor draws what it says and computes nothing (ADR-0007).
func (s *svc) PlanWindow(ctx context.Context, req PlanWindowRequest) (*PlanWindowResult, error) {
	c, err := authorize(ctx, "PlanWindow", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	if req.Until == 0 {
		return nil, newError(command.CodeValidation,
			"until is required: the end of the window in whole frames, at most %d after from.", plan.MaxWindow).
			withPath("until").
			withHint("send from and until as frame numbers, for example from=0&until=30; a time such as at=1s is not read")
	}

	if req.From < 0 || req.Until <= req.From || req.Until-req.From > plan.MaxWindow {
		return nil, newError(command.CodeValidation, "the window %d to %d is not 1 to %d frames from frame 0.",
			req.From, req.Until, plan.MaxWindow).withPath("until")
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if req.Pin != 0 && row.Revision != req.Pin {
		return nil, pinMismatch(row.Revision)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	fonts := s.fontSet(doc)
	opts := plan.Options{Measure: rasterize.Measure(fonts)}

	scale, werr := planOutputScale(req.Width, doc.Settings.Width)
	if werr != nil {
		return nil, werr
	}

	opts.OutputScale = scale

	w := plan.WindowWith(doc, req.From, req.Until, opts)
	w.Revision = row.Revision

	if err := s.registerBitmaps(ctx, &row, doc, fonts, opts, &w, req.From, req.Until); err != nil {
		return nil, err
	}

	return &w, nil
}

// registerBitmaps makes the bitmaps of frames from to until askable by key. A bitmap drawn with a
// user font is handed out under a key that also names the font files (see bitmaps), and the window
// is rewritten to those keys.
func (s *svc) registerBitmaps(
	ctx context.Context,
	row *ProjectRow,
	doc *timeline.Document,
	fonts *rasterize.FontSet,
	opts plan.Options,
	w *plan.WindowPlan,
	from, until int,
) error {
	renamed := map[string]string{}

	for t := from; t < until; t++ {
		// A client that gave up (a newer revision) stops the planning (SDD-012 section 19.2 rule 8).
		if (t-from)%30 == 0 {
			if cerr := ctx.Err(); cerr != nil {
				return asError(cerr)
			}
		}

		layers := plan.FrameWith(doc, t, opts).Layers
		for i := range layers {
			if layers[i].Bitmap == nil {
				continue
			}

			spec := *layers[i].Bitmap
			key := spec.Key()

			if salt := userFontKey(doc, &spec); salt != "" {
				sum := sha256.Sum256([]byte(key + ":" + salt))
				key = hex.EncodeToString(sum[:])
				renamed[spec.Key()] = key
			}

			s.bitmaps.remember(row.ID, key, spec, doc, fonts)
		}
	}

	for i := range w.Layers {
		if w.Layers[i].Bitmaps == nil {
			continue
		}

		for j, k := range w.Layers[i].Bitmaps.Keys {
			if r, ok := renamed[k]; ok {
				w.Layers[i].Bitmaps.Keys[j] = r
			}
		}
	}

	return nil
}

// openBitmap serves a rasterized text or shape by its content key: the PNG is
// written once under bitmaps/ in the cache and served from there.
func (s *svc) openBitmap(project, key string) (*File, error) {
	notFound := func() error {
		return newError(command.CodeNotFound, "there is no bitmap with that key.").
			withHint("read the plan of the window again")
	}

	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		return nil, notFound()
	}

	known, ok := s.bitmaps.known(project, key)
	if !ok {
		return nil, notFound()
	}

	if s.media.Frames == nil || s.media.Frames.Cache == nil {
		return nil, newError(command.CodeInternal, "bitmaps are not available in this build.")
	}

	store, name := s.media.Frames.Cache, cache.BitmapKey(key)

	body, found := store.Get(name)
	if !found {
		bm, err := s.bitmaps.rasterCache(&known).Get(known.spec)
		if err != nil {
			return nil, newError(command.CodeValidation, "the bitmap cannot be drawn.")
		}

		if body, err = bm.PNG(); err != nil {
			return nil, internalError()
		}

		if err := store.Put(name, body); err != nil {
			return nil, internalError()
		}
	}

	return &File{
		Path: name, ContentType: "image/png", Size: int64(len(body)), ETag: `"bmp-` + key + `"`,
		ModTime: time.Unix(0, 0).UTC(), Immutable: true, MaxAge: bitmapMaxAge,
	}, nil
}
