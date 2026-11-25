package export

import (
	"context"
	"crypto/sha256"
	"image"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// The instances of a drag (SDD-021 section 6.3). A person who drags a slider edits one layer again and again, and
// the window of each edit is the same, so the bands of one edit hold what the bands of the next need: the layers
// that did not change, already cut to the window. A session keeps the instances of the bands of the latest
// document shape, and each render hands them only the layers that are not in them yet. It holds at most one
// shape, up to maxImageWorkers instances (the pool's own bound), and gives them back after sessionIdle with no
// edit, when another shape asks for the place, or when an instance fails.
const (
	sessionIdle = 15 * time.Second
	// maxHeldPictures bounds what one instance keeps: past it the instance starts again from nothing.
	maxHeldPictures = 96
)

// heldPicture is a prepared picture an instance holds: which one, and the part of it that was sent.
type heldPicture struct {
	serial uint64
	crop   [4]int
}

// bandSlot is the instance of one band, with what it holds.
type bandSlot struct {
	host     *renderhost.Host
	held     map[string]heldPicture
	base     *image.RGBA
	baseRect [4]int
}

// get gives the instance, a new one from the pool when there is none.
func (s *bandSlot) get(ctx context.Context) (*renderhost.Host, error) {
	if s.host != nil && len(s.held) > maxHeldPictures {
		s.free()
	}

	if s.host == nil {
		h, err := imagePool.Get(ctx)
		if err != nil {
			return nil, err
		}

		s.host, s.held, s.base = h, map[string]heldPicture{}, nil
	}

	return s.host, nil
}

// free gives the instance back to the pool, which clears it.
func (s *bandSlot) free() {
	if s.host != nil {
		imagePool.Put(s.host)
	}

	s.host, s.held, s.base = nil, nil, nil
}

// windowSession is the instances of the bands of one document shape. gate admits one render at a time.
type windowSession struct {
	gate  chan struct{}
	slots []*bandSlot
	timer *time.Timer
}

// slot is the instance of band b; nil for no session, in which case the band takes one from the pool.
func (s *windowSession) slot(b int) *bandSlot {
	if s == nil {
		return nil
	}

	for len(s.slots) <= b {
		s.slots = append(s.slots, &bandSlot{})
	}

	return s.slots[b]
}

// acquire waits for the render that holds the session, or for the end of ctx.
func (s *windowSession) acquire(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		if s.timer != nil {
			s.timer.Stop()
		}

		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release lets the next render in and starts the idle time.
func (s *windowSession) release() {
	s.timer = time.AfterFunc(sessionIdle, func() {
		select {
		case s.gate <- struct{}{}:
			s.freeAll()

			<-s.gate
		default: // a render holds it: it starts the idle time again when it is done
		}
	})

	<-s.gate
}

// freeAll gives every instance back. The caller holds the gate.
func (s *windowSession) freeAll() {
	for _, sl := range s.slots {
		sl.free()
	}
}

// sessionFor gives the session of the shape, and ends the session of any other shape that no render holds; nil
// when another shape's render holds the place (the caller then takes its instances from the pool).
func (c *ImageCache) sessionFor(scope string, shape [sha256.Size]byte) *windowSession {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := lastKey{scope, shape}
	if s, ok := c.sessions[key]; ok {
		return s
	}

	for k, other := range c.sessions {
		select {
		case other.gate <- struct{}{}:
			other.freeAll()
			delete(c.sessions, k)

			<-other.gate
		default:
			return nil
		}
	}

	s := &windowSession{gate: make(chan struct{}, 1)}
	c.sessions[key] = s

	return s
}
