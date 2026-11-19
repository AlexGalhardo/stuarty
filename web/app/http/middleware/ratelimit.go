// Package middleware holds the HTTP middleware that the API adds to vukem's.
package middleware

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/web"
)

// Denied is the error of a request that a limiter refused: E_RATE_LIMITED in the
// common shape, with details.retry_after in whole seconds (SDD-007 section 4.4).
func Denied(retryAfter time.Duration) *service.Error {
	seconds := max(int(math.Ceil(retryAfter.Seconds())), 1)

	return &service.Error{
		Code: service.CodeRateLimited, Message: "too many requests.", Retryable: true,
		Hint:    "wait " + strconv.Itoa(seconds) + " seconds and try again",
		Details: map[string]any{"retry_after": seconds},
	}
}

// deniedLimit is Denied with the limit of SDD-007 section 9.1: the name, and the size of the bucket
// as both the value counted and the ceiling, because a refused request found it empty. A limit of
// zero (a store that failed) adds nothing.
func deniedLimit(retryAfter time.Duration, name string, limit int) *service.Error {
	e := Denied(retryAfter)
	if limit > 0 {
		e.Details["limit"], e.Details["value"], e.Details["ceiling"] = name, limit, limit
	}

	return e
}

// Allow asks l about key. A refusal, and a store that fails, both deny: the
// limiter fails closed. It returns nil when the request may go on.
func Allow(r *http.Request, l ratelimit.Limiter, key string) *service.Error {
	res, err := l.Allow(r.Context(), key)
	if err != nil {
		return Denied(time.Second)
	}

	if !res.Allowed {
		return deniedLimit(res.RetryAfter, "requests", res.Limit)
	}

	return nil
}

// Limit is Limiter.Allow called directly instead of ratelimit.Middleware, whose
// 429 has a body of its own: the product's error shape must be the same on
// every surface. A key of "" passes the request without counting it.
func Limit(l ratelimit.Limiter, key ratelimit.KeyFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				// ThrottleKeys may parse a multipart form.
				if f := r.MultipartForm; f != nil {
					_ = f.RemoveAll()
				}
			}()

			k := key(r)
			if k == "" {
				next.ServeHTTP(w, r)
				return
			}

			if err := Allow(r, l, k); err != nil {
				web.SecurityEvent(r, "ratelimit.exceeded")
				web.Fail(w, r, err)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// FailedAuth counts the failed authentications of an address (SDD-007 section
// 4.4): every 401 spends one of its tokens. vukem's Limiter only spends, it
// cannot be asked without spending, so an address whose token ran out is
// remembered until the limiter would let it through again; the failures of that
// address are then refused with E_RATE_LIMITED instead of 401. A request with a
// valid credential is not a failure and is never refused by this.
type FailedAuth struct {
	l   ratelimit.Limiter
	now func() time.Time

	mu      sync.Mutex
	blocked map[string]time.Time
	limit   int // the bucket of l, as the last answer of l gave it
}

// maxBlocked bounds the memory of refused addresses; at the bound the expired
// ones go first.
const maxBlocked = 10_000

// failedAuthLimit names the limit of the failed authentications of an address.
const failedAuthLimit = "failed_authentications"

// NewFailedAuth counts on l; now may be nil.
func NewFailedAuth(l ratelimit.Limiter, now func() time.Time) *FailedAuth {
	if now == nil {
		now = time.Now
	}

	return &FailedAuth{l: l, now: now, blocked: map[string]time.Time{}}
}

// Blocked is the refusal owed to an address that spent its failures, or nil.
func (f *FailedAuth) Blocked(key string) *service.Error {
	f.mu.Lock()
	defer f.mu.Unlock()

	until, ok := f.blocked[key]
	if !ok {
		return nil
	}

	if wait := until.Sub(f.now()); wait > 0 {
		return deniedLimit(wait, failedAuthLimit, f.limit)
	}

	delete(f.blocked, key)

	return nil
}

// Record spends a token for a failed authentication. It returns the refusal to
// give in place of the 401 when there is none left, or when the store fails.
func (f *FailedAuth) Record(r *http.Request, key string) *service.Error {
	res, err := f.l.Allow(r.Context(), key)
	if err == nil && res.Allowed {
		return nil
	}

	wait := time.Second
	if err == nil {
		wait = max(res.RetryAfter, time.Second)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if err == nil {
		f.limit = res.Limit
	}

	if len(f.blocked) >= maxBlocked {
		for k, until := range f.blocked {
			if !until.After(f.now()) {
				delete(f.blocked, k)
			}
		}
	}

	if len(f.blocked) < maxBlocked {
		f.blocked[key] = f.now().Add(wait)
	}

	return deniedLimit(wait, failedAuthLimit, res.Limit)
}

// ServiceLimiter adapts a vukem limiter to service.RateLimiter.
type ServiceLimiter struct{ L ratelimit.Limiter }

// Allow implements service.RateLimiter.
func (s ServiceLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	res, err := s.L.Allow(ctx, key)

	return res.Allowed, res.RetryAfter, err
}
