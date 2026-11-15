package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/engine/cache"
	"github.com/AlexGalhardo/vukem-video/engine/raster"
)

// PlanWindowRequest asks for the render plan of frames From up to, not
// including, To (SDD-004 section 10).
type PlanWindowRequest struct {
	ProjectID string `json:"project_id"`
	From      int    `json:"from"`
	To        int    `json:"to"`
}

// PlanWindowResult is the compact plan of a window; Revision is the revision it
// was read at.
type PlanWindowResult = plan.WindowPlan

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
type bitmaps struct {
	mu    sync.Mutex
	specs map[bitmapID]plan.BitmapSpec
	order []bitmapID
	cache *raster.Cache
}

type bitmapID struct{ project, key string }

func (b *bitmaps) remember(project string, spec plan.BitmapSpec) {
	key := bitmapID{project, spec.Key()}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.specs == nil {
		b.specs = map[bitmapID]plan.BitmapSpec{}
	}

	if _, ok := b.specs[key]; ok {
		return
	}

	if len(b.order) >= maxKnownBitmaps {
		delete(b.specs, b.order[0])
		b.order = b.order[1:]
	}

	b.specs[key] = spec
	b.order = append(b.order, key)
}

func (b *bitmaps) spec(project, key string) (plan.BitmapSpec, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.specs[bitmapID{project, key}]

	return s, ok
}

// PlanWindow reads the render plan of a window of the current revision. The
// editor draws what it says and computes nothing (ADR-0007).
func (s *svc) PlanWindow(ctx context.Context, req PlanWindowRequest) (*PlanWindowResult, error) {
	c, err := authorize(ctx, "PlanWindow", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.From < 0 || req.To <= req.From || req.To-req.From > plan.MaxWindow {
		return nil, newError(command.CodeValidation, "the window %d to %d is not 1 to %d frames from frame 0.",
			req.From, req.To, plan.MaxWindow).withPath("to")
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	opts := plan.Options{Measure: raster.Measure(raster.Bundled())}

	w := plan.WindowWith(doc, req.From, req.To, opts)
	w.Revision = row.Revision

	// The bitmaps of the window can be asked for by key from now on.
	for t := req.From; t < req.To; t++ {
		layers := plan.FrameWith(doc, t, opts).Layers
		for i := range layers {
			if layers[i].Bitmap != nil {
				s.bitmaps.remember(row.ID, *layers[i].Bitmap)
			}
		}
	}

	return &w, nil
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

	spec, ok := s.bitmaps.spec(project, key)
	if !ok {
		return nil, notFound()
	}

	if s.media.Frames == nil || s.media.Frames.Cache == nil {
		return nil, newError(command.CodeInternal, "bitmaps are not available in this build.")
	}

	store, name := s.media.Frames.Cache, cache.BitmapKey(key)

	body, found := store.Get(name)
	if !found {
		s.bitmaps.mu.Lock()
		if s.bitmaps.cache == nil {
			s.bitmaps.cache = raster.NewCache(raster.Bundled(), 0)
		}
		rc := s.bitmaps.cache
		s.bitmaps.mu.Unlock()

		bm, err := rc.Get(spec)
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
