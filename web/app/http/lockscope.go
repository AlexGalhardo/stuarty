package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// lockScope ends the requests in progress when the studio locks (SDD-007 section 3.9 item 4). An event stream
// was authorized when it opened and is asked again only once a minute, so without this it would go on sending to
// a browser that has just logged out.
type lockScope struct {
	mu   sync.Mutex
	next uint64
	live map[uint64]context.CancelFunc
}

func newLockScope() *lockScope { return &lockScope{live: map[uint64]context.CancelFunc{}} }

// bind returns a context that ends when the studio locks, and the function that releases it.
func (l *lockScope) bind(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	l.mu.Lock()
	id := l.next
	l.next++
	l.live[id] = cancel
	l.mu.Unlock()

	return ctx, func() {
		l.mu.Lock()
		delete(l.live, id)
		l.mu.Unlock()
		cancel()
	}
}

// end cancels every bound request.
func (l *lockScope) end() {
	l.mu.Lock()
	live := l.live
	l.live = map[uint64]context.CancelFunc{}
	l.mu.Unlock()

	for _, cancel := range live {
		cancel()
	}
}

// boundToLock is the context of a request that ends when the studio locks. The account routes are left out: the
// logout locks the studio and still has its answer to write.
func (h *apiHandler) boundToLock(r *http.Request) (context.Context, func()) {
	if h.Account == nil || strings.HasPrefix(r.URL.Path, BasePathAuth+"/account") {
		return r.Context(), func() {}
	}

	return h.streams.bind(r.Context())
}
