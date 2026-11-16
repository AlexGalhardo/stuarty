package service

import (
	"context"
	"math"
	"strings"
	"time"
)

// heavyOperations are the operations that cost a render, an ingest or an archive
// read; together they share 10 calls a minute per credential (SDD-007 section 4.4).
const (
	heavyPerMinute = 10
	// HeavyKey is the part of the limiter key that names the shared bucket.
	HeavyKey = "heavy"
)

// RateLimiter decides whether one more call of a heavy operation may run. The
// web layer implements it over vukem's Limiter (the service imports no vukem
// package). A key is "heavy:<bucket>", see heavyBucket.
type RateLimiter interface {
	// Allow spends a token for key. When it is not allowed, retryAfter says how
	// long to wait. An error is a failing store, and the caller is refused.
	Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error)
}

// HeavyPerMinute is the number of calls the limiter of heavy operations should
// allow a minute; the web layer builds the limiter with it.
const HeavyPerMinute = heavyPerMinute

// heavyBucket is the limiter key of a caller: the credential, never a label
// the client chose (SDD-007 section 4.4). Local agents share the local token.
func heavyBucket(c Caller) string {
	switch {
	case !strings.HasPrefix(c.Actor, "local:"):
		return HeavyKey + ":" + c.Actor
	case c.Browser:
		return HeavyKey + ":local:ui"
	default:
		return HeavyKey + ":local:agents"
	}
}

// heavyLimit refuses the call of a heavy operation over the limit of its
// credential with E_RATE_LIMITED. Without a limiter nothing is counted.
func (s *svc) heavyLimit(ctx context.Context, c Caller) error {
	if s.heavy == nil {
		return nil
	}

	ok, wait, err := s.heavy.Allow(ctx, heavyBucket(c))
	if err != nil {
		ok, wait = false, time.Second // fails closed
	}

	if ok {
		return nil
	}

	seconds := max(int(math.Ceil(wait.Seconds())), 1)

	limit := s.heavyLimitN
	if limit <= 0 {
		limit = heavyPerMinute
	}

	e := newError(CodeRateLimited, "more than %d heavy calls a minute.", limit).
		withHint("wait %d seconds and try again", seconds)
	e.Details = map[string]any{"retry_after": seconds}

	return e.withLimit("heavy_per_minute", limit, limit)
}
