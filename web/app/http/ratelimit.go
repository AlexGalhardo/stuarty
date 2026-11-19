package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/http/middleware"

	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/web"
)

// Defaults of SDD-007 section 4.4.
const (
	DefaultRatePerMinute        = 300
	DefaultRateBurst            = 60
	DefaultSessionRatePerMinute = 1800
	DefaultSessionRateBurst     = 300
	preAuthPerMinute            = 600
	failedAuthPerMinute         = 20
)

// RateLimits are the per-credential limits in requests a minute; zero is the
// default (STUARTY_RATE_LIMIT_PER_MINUTE and
// STUARTY_RATE_LIMIT_SESSION_PER_MINUTE). The burst is added to the rate: a
// credential may spend its minute at once and then 60 more (a session 300 more).
type RateLimits struct {
	PerCredential int
	PerSession    int
	// Heavy is the per-actor limit of JobStart, MediaImport and TemplateImport;
	// zero is service.HeavyPerMinute.
	Heavy int
}

// limits is the state of the limiters of one server.
type limits struct {
	preAuth    ratelimit.Limiter
	credential ratelimit.Limiter
	session    ratelimit.Limiter
	failed     *middleware.FailedAuth

	uncountedOnce sync.Once
	uncounted     []uncountedRoute

	inFlightMu sync.Mutex
	inFlight   map[string]int
}

type uncountedRoute struct {
	re          *regexp.Regexp
	maxInFlight int
}

func newLimits(r RateLimits, now func() time.Time) *limits {
	if r.PerCredential <= 0 {
		r.PerCredential = DefaultRatePerMinute
	}

	if r.PerSession <= 0 {
		r.PerSession = DefaultSessionRatePerMinute
	}

	return &limits{
		preAuth:    ratelimit.NewMemory(preAuthPerMinute, time.Minute),
		credential: ratelimit.NewMemoryBurst(r.PerCredential, time.Minute, r.PerCredential+DefaultRateBurst),
		session:    ratelimit.NewMemoryBurst(r.PerSession, time.Minute, r.PerSession+DefaultSessionRateBurst),
		failed:     middleware.NewFailedAuth(ratelimit.NewMemory(failedAuthPerMinute, time.Minute), now),
		inFlight:   map[string]int{},
	}
}

// uncountedRoute finds the route, among those the limiters do not count (SDD-007
// section 4.4), that a request is for: the open event stream, the byte-serving
// routes the editor hits many times a second while it plays or scrubs, the
// cached frames and the plan windows. Static assets and pages are outside
// /api/v1 and /mcp and are never counted per credential.
func (l *limits) uncountedRoute(method, path string) (uncountedRoute, bool) {
	l.uncountedOnce.Do(func() {
		for i := range Endpoints {
			if !Endpoints[i].Uncounted {
				continue
			}

			re := regexp.MustCompile("^" + regexp.MustCompile(`\\\{[^}]*\\\}`).
				ReplaceAllString(regexp.QuoteMeta(Endpoints[i].FullPath()), `[^/]+`) + "$")
			l.uncounted = append(l.uncounted, uncountedRoute{re: re, maxInFlight: Endpoints[i].MaxInFlight})
		}
	})

	if method != http.MethodGet && method != http.MethodHead {
		return uncountedRoute{}, false
	}

	for _, u := range l.uncounted {
		if u.re.MatchString(path) {
			return u, true
		}
	}

	return uncountedRoute{}, false
}

func (l *limits) uncountedPath(method, path string) bool {
	_, ok := l.uncountedRoute(method, path)

	return ok
}

// acquire takes one of the in-flight places of an actor on a route; false when
// the actor already has max calls running.
func (l *limits) acquire(actor string, route uncountedRoute) bool {
	l.inFlightMu.Lock()
	defer l.inFlightMu.Unlock()

	key := actor + "|" + route.re.String()
	if l.inFlight[key] >= route.maxInFlight {
		return false
	}

	l.inFlight[key]++

	return true
}

func (l *limits) release(actor string, route uncountedRoute) {
	l.inFlightMu.Lock()
	defer l.inFlightMu.Unlock()

	key := actor + "|" + route.re.String()
	l.inFlight[key]--

	if l.inFlight[key] <= 0 {
		delete(l.inFlight, key)
	}
}

func counted(path string) bool {
	return strings.HasPrefix(path, BasePath+"/") || path == "/mcp"
}

// preAuthLimit is the limiter by address before authentication, every request
// of network mode (4.4). Local mode shares one address and skips it.
func (h *apiHandler) preAuthLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.Network == nil || r.URL.Path == healthPath || h.limits.uncountedPath(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if e := middleware.Allow(r, h.limits.preAuth, ratelimit.ByIP(r)); e != nil {
			web.SecurityEvent(r, "ratelimit.exceeded", "limiter", "pre-authentication")
			web.Fail(w, r, e)

			return
		}

		next.ServeHTTP(w, r)
	})
}

// unauthorized answers a failed authentication with 401. In network mode it is
// counted against the address of the client, which is refused with 429 instead
// once it has spent its failures (SDD-007 section 4.4). Local mode does not
// count: every client is the loopback address, and the local token has 256 bits.
func (h *apiHandler) unauthorized(w http.ResponseWriter, r *http.Request, e *service.Error) {
	if h.Network != nil {
		ip := ratelimit.ByIP(r)

		denied := h.limits.failed.Blocked(ip)
		if denied == nil {
			denied = h.limits.failed.Record(r, ip)
		}

		if denied != nil {
			web.Fail(w, r, denied)
			return
		}
	}

	w.Header().Set("WWW-Authenticate", "Bearer")
	web.Fail(w, r, e)
}

// blockedAddress refuses, before any bearer key is verified, a request from an
// address that spent its failed authentications: verifying it would tell a
// guesser which guess was right. A request that carries only a session cookie
// may go on, because the cookie is checked against the session store and the
// session is already established; if it is not valid it fails like any other.
func (h *apiHandler) blockedAddress(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") == "" {
		return false
	}

	if e := h.limits.failed.Blocked(ratelimit.ByIP(r)); e != nil {
		web.Fail(w, r, e)
		return true
	}

	return false
}

// credentialLimit is the limiter after authentication: per bearer credential,
// or per browser session, on the routes under /api/v1 and /mcp (4.4).
func (h *apiHandler) credentialLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := service.CallerFrom(r.Context())
		if !ok || !counted(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// The credential, never the actor: in local mode the actor carries a label the client
		// chooses, and rotating labels must not lift a cap (SR1-03).
		limiter, key := h.limits.credential, ""

		if header := r.Header.Get("Authorization"); header != "" {
			tok, _ := auth.Bearer(header)
			key = "token:" + digest(tok)
		} else {
			limiter, key = h.limits.session, "session:"+digest(h.sessionID(r))
		}

		// A route that is not counted but renders on a miss (the cached frame) has
		// a cap on what one caller runs at once instead.
		if route, exempt := h.limits.uncountedRoute(r.Method, r.URL.Path); exempt {
			if route.maxInFlight > 0 {
				if !h.limits.acquire(key, route) {
					web.Fail(w, r, &service.Error{
						Code: service.CodeQueueFull, Message: "this caller already has frames being rendered.",
						Retryable: true, Hint: "wait for a frame to arrive before asking for another",
						Details: map[string]any{
							"limit": "frames", "value": route.maxInFlight, "ceiling": route.maxInFlight,
						},
					})

					return
				}

				defer h.limits.release(key, route)
			}

			next.ServeHTTP(w, r)

			return
		}

		if e := middleware.Allow(r, limiter, key); e != nil {
			web.SecurityEvent(r, "ratelimit.exceeded", "limiter", "credential")
			web.Fail(w, r, e)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *apiHandler) sessionID(r *http.Request) string {
	name := auth.LocalCookieName(browserPort(r))
	if h.Auth != nil {
		name = h.Auth.Sessions.CookieName()
	}

	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}

	return c.Value
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:16])
}
