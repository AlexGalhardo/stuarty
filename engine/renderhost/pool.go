package renderhost

import (
	"context"
	"sync"
	"time"
)

// Pool keeps idle instances of the module for the next render. An instance that has drawn a large picture owns
// the memory it grew to, and a fresh one pays for every page of it again (the pages are faulted in one at a time,
// and the module's memory is grown by copying): the render of an image document uses up to eight instances at
// once, so the next one is faster on instances that are already as large as it needs.
//
// A pool bounds what it keeps: at most maxIdle instances and maxIdleBytes of their memory, and it closes them all
// when none was taken for ttl. An instance that failed is never kept. It is safe for concurrent use; every
// instance it gives out is the caller's alone until it comes back.
type Pool struct {
	cfg          Config
	maxIdle      int
	maxIdleBytes int64
	ttl          time.Duration

	mu    sync.Mutex
	idle  []*Host
	timer *time.Timer

	// drained, set by a test before the first Put, runs after a Drain: the timer gives a test nothing else to wait on.
	drained func()
}

// NewPool makes a pool of instances made with cfg.
func NewPool(cfg Config, maxIdle int, maxIdleBytes int64, ttl time.Duration) *Pool {
	return &Pool{cfg: cfg, maxIdle: maxIdle, maxIdleBytes: maxIdleBytes, ttl: ttl}
}

// Get gives an idle instance, with nothing stored in it, or a new one.
func (p *Pool) Get(ctx context.Context) (*Host, error) {
	p.mu.Lock()

	if n := len(p.idle); n > 0 {
		h := p.idle[n-1]
		p.idle = p.idle[:n-1]

		p.mu.Unlock()

		return h, nil
	}

	p.mu.Unlock()

	return New(ctx, p.cfg)
}

// Put gives an instance back. It is cleared first; it is closed instead of kept when it failed, when the pool is
// full, or when it would hold more memory than the pool allows.
func (p *Pool) Put(h *Host) {
	if h == nil {
		return
	}

	if h.dead != nil || h.Reset(context.Background()) != nil {
		_ = h.Close(context.Background())

		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	held := h.MemoryBytes()
	for _, o := range p.idle {
		held += o.MemoryBytes()
	}

	if len(p.idle) >= p.maxIdle || held > p.maxIdleBytes {
		_ = h.Close(context.Background())

		return
	}

	p.idle = append(p.idle, h)

	if p.timer == nil {
		p.timer = time.AfterFunc(p.ttl, p.Drain)
	} else {
		p.timer.Reset(p.ttl)
	}
}

// Drain closes every idle instance.
func (p *Pool) Drain() {
	p.mu.Lock()
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()

	for _, h := range idle {
		_ = h.Close(context.Background())
	}

	if p.drained != nil {
		p.drained()
	}
}

// Idle is how many instances the pool holds.
func (p *Pool) Idle() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.idle)
}
