package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	"github.com/AlexGalhardo/vukem-video/web/app/http/middleware"
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
// default (VUKEM_VIDEO_RATE_LIMIT_PER_MINUTE and
// VUKEM_VIDEO_RATE_LIMIT_SESSION_PER_MINUTE). The burst is added to the rate: a
// credential may spend its minute at once and then 60 more (a session 300 more).
type RateLimits struct {
	PerCredential int
	PerSession    int
}

// limits is the state of the limiters of one server.
type limits struct {
	preAuth    ratelimit.Limiter
	credential ratelimit.Limiter
	session    ratelimit.Limiter
	failed     *middleware.FailedAuth

	uncountedOnce sync.Once
	uncounted     []*regexp.Regexp
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
	}
}

// uncountedPath is true for what the limiters do not count (SDD-007 section
// 4.4): the open event stream, the byte-serving routes the editor hits many
// times a second while it plays or scrubs, and the plan windows. Static assets
// and pages are outside /api/v1 and /mcp and are never counted per credential.
func (l *limits) uncountedPath(method, path string) bool {
	l.uncountedOnce.Do(func() {
		for i := range Endpoints {
			if !Endpoints[i].Uncounted {
				continue
			}

			re := regexp.MustCompile("^" + regexp.MustCompile(`\\{[^}]*\\}`).
				ReplaceAllString(regexp.QuoteMeta(Endpoints[i].FullPath()), `[^/]+`) + "$")
			l.uncounted = append(l.uncounted, re)
		}
	})

	if method != http.MethodGet && method != http.MethodHead {
		return false
	}

	for _, re := range l.uncounted {
		if re.MatchString(path) {
			return true
		}
	}

	return false
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

// unauthorized answers a failed authentication: 401, counted against the address
// of the client, which is refused instead once it has spent its failures.
func (h *apiHandler) unauthorized(w http.ResponseWriter, r *http.Request, e *service.Error) {
	if denied := h.limits.failed.Record(r, ratelimit.ByIP(r)); denied != nil {
		web.Fail(w, r, denied)
		return
	}

	w.Header().Set("WWW-Authenticate", "Bearer")
	web.Fail(w, r, e)
}

// blockedAddress refuses a request from an address that spent its failed
// authentications, before any credential is looked at.
func (h *apiHandler) blockedAddress(w http.ResponseWriter, r *http.Request) bool {
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
		if _, ok := service.CallerFrom(r.Context()); !ok || !counted(r.URL.Path) ||
			h.limits.uncountedPath(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		limiter, key := h.limits.credential, ""

		if header := r.Header.Get("Authorization"); header != "" {
			tok, _ := auth.Bearer(header)
			key = "token:" + digest(tok)
		} else {
			limiter, key = h.limits.session, "session:"+digest(h.sessionID(r))
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
	name := auth.CookieName
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
