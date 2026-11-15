// Package middleware holds the HTTP middleware that the API adds to vukem's.
package middleware

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/service"
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

// Allow asks l about key. A refusal, and a store that fails, both deny: the
// limiter fails closed. It returns nil when the request may go on.
func Allow(r *http.Request, l ratelimit.Limiter, key string) *service.Error {
	res, err := l.Allow(r.Context(), key)
	if err != nil {
		return Denied(time.Second)
	}

	if !res.Allowed {
		return Denied(res.RetryAfter)
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
