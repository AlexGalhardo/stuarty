package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	"github.com/AlexGalhardo/stuarty/web/app/http/middleware"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/api"
	"github.com/AlexGalhardo/vukem/apidocs"
	"github.com/AlexGalhardo/vukem/openapi"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"
)

// Deps is what the adapter needs from the process that serves it.
type Deps struct {
	Service   service.Service
	Registry  *command.Registry // the commands whose argument schemas the OpenAPI document carries
	Workspace workspace.Dir
	Token     string // the local token, as read from <workspace>/token; empty in network mode
	// RuntimeSecret is the per-run secret of network mode (SDD-007 section 3.8), the key of the
	// handshake there; empty in local mode, and in network mode it leaves the handshake unanswered.
	RuntimeSecret string
	// Network is the guard of network mode (SDD-007 section 4.3); nil is local mode.
	Network  *Network
	Sessions *auth.Sessions
	// LauncherSecret is the per-process secret the desktop shell passed in the environment; only a
	// launch request that proves it yields a session with the browser-only privileges (SDD-007
	// section 5.3). Empty when no launcher started the server.
	LauncherSecret string
	Launch         *auth.Launch
	// Auth is the credential side of network mode: users, the session guard and
	// the keys. It is required with Network and unused without it.
	Auth *auth.Network
	// Google is the optional Google sign-in of network mode (SDD-007 section 4.7); nil leaves its routes
	// answering E_NOT_FOUND, as in local mode.
	Google *auth.Google
	// LoginLimits replaces the limits of the login route; nil is the defaults.
	LoginLimits *LoginLimits
	// RateLimits are the per-credential limits of SDD-007 section 4.4.
	RateLimits RateLimits
	Now        func() time.Time
	// MCP is the Streamable HTTP handler of the MCP server, mounted at POST /mcp
	// behind the guard and the authentication; nil mounts nothing.
	MCP http.Handler
	// Events carries the event stream of GET /projects/{id}/events.
	Events *events.Events
	// Version is the binary version the editor page shows in bug reports.
	Version string
}

type apiHandler struct {
	Deps
	app           *vukem.App
	r             *router.Router
	loginThrottle loginThrottles
	limits        *limits
}

// Routes installs the guard, the error envelope and every route of Endpoints.
// It must run before the routes that install vukem's default middleware, so
// that the guard is the outermost layer (SDD-007 section 5.1 item 3).
func Routes(d *Deps) func(r *router.Router, app *vukem.App) error {
	return func(r *router.Router, app *vukem.App) error {
		if d.Service == nil || (d.Token == "" && d.Network == nil) || d.Sessions == nil || d.Launch == nil {
			return errors.New("httpapi: Service, Token (local mode), Sessions and Launch are required")
		}

		if d.Network != nil && d.Auth == nil {
			return errors.New("httpapi: Auth is required in network mode")
		}

		if d.Registry == nil {
			d.Registry = command.Default()
		}

		if d.Now == nil {
			d.Now = time.Now
		}

		installErrors(r.Services())
		r.UseServer(guard(d.Network))

		if d.Network != nil {
			users, err := d.Auth.Users.Count(context.Background())
			if err != nil {
				return err
			}

			d.Network.logStartup(users == 0)
			r.Use(d.Auth.Sessions.Middleware())
		}

		h := &apiHandler{
			Deps: *d, app: app, r: r, loginThrottle: newLoginThrottles(d.LoginLimits),
			limits: newLimits(d.RateLimits, d.Now),
		}

		r.Use(h.preAuthLimit, h.authenticate, middleware.AccessLog, h.credentialLimit)
		h.registerAll()

		if h.MCP != nil {
			h.mountMCP()
		}

		return nil
	}
}

// openPaths need no credential: the handshake proves the server to the client,
// the launch route turns a code into a session, and the health probe is vukem's.
var openPaths = []string{handshakePath, "/auth/launch", healthPath}

const handshakePath = BasePath + "/handshake"

func (h *apiHandler) op(id string) api.Op {
	ep := endpointOf(id)

	op := api.Op{Name: ep.ID, Summary: ep.Summary, Status: ep.Status, Security: []string{"bearer"}}
	if ep.Pending != "" {
		op.Description = "Not available in this build yet (" + ep.Pending + "): the route answers E_INTERNAL."
	}

	return op
}

// authenticate is step "authentication" of SDD-007 section 5.1 item 3 for
// local mode (3.7): the bearer token first, with no fall back to the cookie
// when it is wrong, then the session cookie.
func (h *apiHandler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.Network != nil {
			h.authenticateNetwork(w, r, next)
			return
		}

		if slices.Contains(openPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		var (
			caller     service.Caller
			valid      bool
			privileged bool
		)

		if ck, err := r.Cookie(
			auth.LocalCookieName(browserPort(r)),
		); err == nil &&
			r.Header.Get("Authorization") == "" {
			valid, privileged = h.Sessions.Lookup(ck.Value)
		}

		if header := r.Header.Get("Authorization"); header != "" {
			token, ok := auth.Bearer(header)
			if !ok || !auth.TokenMatches(h.Token, token) {
				h.unauthorized(w, r, &service.Error{Code: service.CodeUnauthorized, Message: "the token is not valid."})

				return
			}

			actor, err := auth.Actor(r.Header.Get(auth.ClientHeader))
			if err != nil {
				web.Fail(
					w,
					r,
					&service.Error{Code: command.CodeValidation, Message: err.Error(), Path: auth.ClientHeader},
				)
				return
			}

			caller = service.Caller{Actor: actor, Scopes: service.AllScopes}
		} else if valid {
			if !ambientAllowed(r) {
				web.Fail(w, r, refused())
				return
			}

			caller = service.Caller{Actor: "local:" + auth.UILabel, Scopes: service.AllScopes, Browser: privileged}
		} else {
			// The consent screen of a social network sends the browser back to the sign-in page by a
			// cross-site navigation, which carries no SameSite=Strict session cookie. The page is drawn
			// with no caller: it holds no data, calls no operation, and hands the state and the code to
			// POST /api/v1/social/complete, a same-origin call that does carry the cookie and is
			// authenticated as every other one (SDD-007 section 3.7, SDD-020 section 6).
			if r.Method == http.MethodGet && r.URL.Path == SignInCallbackPath {
				next.ServeHTTP(w, r)

				return
			}

			// A browser that opens a page with no session is told how to get in (VV-157):
			// the page is plain HTML and carries no secret, since the launch code is
			// issued only to a caller that proves the token (SDD-007 section 3.4).
			if isPageNavigation(r) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeSignInPage(w)

				return
			}

			h.unauthorized(w, r, &service.Error{
				Code: service.CodeUnauthorized, Message: "the request carries no credentials.",
				Hint: signInHint,
			})

			return
		}

		next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), caller)))
	})
}

// authenticateNetwork is the authentication of network mode (SDD-007 sections
// 4.1 and 5.1 item 3): the session cookie of a user who logged in. The local
// token and the launch code do not exist in this mode, and the handshake exists
// only as the proof of the per-run secret. With no user yet, nothing
// authenticates and every request but the health probe, the handshake and the
// login is 401, which the start-up log says.
func (h *apiHandler) authenticateNetwork(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.URL.Path == healthPath || r.URL.Path == "/auth/login" || r.URL.Path == "/auth/logout" ||
		(r.URL.Path == LoginPath && r.Method == http.MethodGet) || googleOpen(r) ||
		(r.URL.Path == handshakePath && h.RuntimeSecret != "") {
		next.ServeHTTP(w, r)
		return
	}

	if h.blockedAddress(w, r) {
		return
	}

	deny := func() {
		h.unauthorized(w, r, &service.Error{
			Code: service.CodeUnauthorized, Message: "the request carries no valid credentials.",
		})
	}

	// A header that is not a valid key is never a fall back to the cookie (3.7).
	if header := r.Header.Get("Authorization"); header != "" {
		secret, ok := auth.Bearer(header)
		if !ok {
			deny()
			return
		}

		caller, err := h.Auth.KeyCaller(r.Context(), secret)

		switch {
		case errors.Is(err, auth.ErrBadKey):
			deny()
		case err != nil:
			web.Fail(w, r, &service.Error{Code: command.CodeInternal, Message: "the key could not be checked."})
		default:
			next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), caller)))
		}

		return
	}

	user, ok, err := h.Auth.SessionUser(web.NewCtx(w, r, h.r.Services()))
	if err != nil {
		web.Fail(w, r, &service.Error{Code: command.CodeInternal, Message: "the session could not be checked."})
		return
	}

	if !ok {
		// A browser that opens a page with no session is sent to the login page and
		// back (SDD-012 section 2.1); it is not a failed authentication to count.
		if isPageNavigation(r) {
			http.Redirect(w, r, LoginPath+"?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}

		deny()

		return
	}

	if !ambientAllowed(r) {
		web.Fail(w, r, refused())
		return
	}

	next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), user.Caller())))
}

// LoginPath is the page of network mode that turns a password into a session; it is
// the only page that needs no session.
const LoginPath = "/login"

// isPageNavigation tells a browser opening a page from a call of the API: a GET that
// accepts HTML, outside the paths whose errors are always problem+json.
func isPageNavigation(r *http.Request) bool {
	return r.Method == http.MethodGet && !jsonPath(r.URL.Path) &&
		strings.Contains(requestedAccept(r), "text/html")
}

// ambientAllowed is step 6 of SDD-007 section 3.6: a request that only the
// cookie authenticates and that changes state must show where it comes from
// and be JSON, which a cross-origin form cannot send without a preflight.
func ambientAllowed(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return true
	}

	if r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") == "" {
		return false
	}

	return hasJSONContentType(r)
}

func hasJSONContentType(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	for i := range len(ct) {
		if ct[i] == ';' {
			ct = ct[:i]
			break
		}
	}

	return ct == "application/json"
}

// need is the scope check of a route; the Service checks again (ADR-0008), this
// one covers the routes that are not service operations.
func need(c *web.Ctx, scope string) error {
	caller, ok := service.CallerFrom(c.Context())
	if !ok && scope == "" {
		return nil // handshake and launch: no credential by design
	}

	if !ok {
		return &service.Error{Code: service.CodeUnauthorized, Message: "the request carries no credentials."}
	}

	if scope != "" && !slices.Contains(caller.Scopes, scope) {
		return &service.Error{Code: service.CodeForbidden, Message: "the caller lacks the " + scope + " scope."}
	}

	return nil
}

// registerDocs adds the OpenAPI document and the reference page.
func (h *apiHandler) registerDocs() {
	ep := endpointOf("openapi.get")
	h.r.Get(ep.Path, h.serveOpenAPI).Name(ep.ID).Doc(openapi.Skip{})

	apidocs.Register(h.r, endpointOf("docs.get").Path, apidocs.Config{Name: "docs"})
}

// mountMCP serves the MCP handler at POST /mcp. The guard and authenticate run
// before it, so the Caller is in the context that the tool handlers read; any
// other method on the path answers 405 from the router.
func (h *apiHandler) mountMCP() {
	ep := endpointOf("mcp.post")

	h.r.Handle(ep.Method+" "+ep.Path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, ok := service.CallerFrom(r.Context())
		if !ok || !slices.Contains(caller.Scopes, ep.Scope) {
			web.Fail(
				w,
				r,
				&service.Error{Code: service.CodeForbidden, Message: "the caller lacks the " + ep.Scope + " scope."},
			)

			return
		}

		h.MCP.ServeHTTP(w, r)
	})).Name(ep.ID).Doc(openapi.Skip{})
}
