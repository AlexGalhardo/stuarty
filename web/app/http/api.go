package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/api"
	"github.com/AlexGalhardo/vukem/apidocs"
	"github.com/AlexGalhardo/vukem/openapi"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	"github.com/AlexGalhardo/vukem-video/web/app/events"
)

// Deps is what the adapter needs from the process that serves it.
type Deps struct {
	Service   service.Service
	Registry  *command.Registry // the commands whose argument schemas the OpenAPI document carries
	Workspace workspace.Dir
	Token     string // the local token, as read from <workspace>/token; empty in network mode
	// Network is the guard of network mode (SDD-007 section 4.3); nil is local mode.
	Network  *Network
	Sessions *auth.Sessions
	Launch   *auth.Launch
	// Auth is the credential side of network mode: users, the session guard and
	// the keys. It is required with Network and unused without it.
	Auth *auth.Network
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

		r.Use(h.preAuthLimit, h.authenticate, h.credentialLimit)
		h.registerAll()

		if h.MCP != nil {
			h.mountMCP()
		}

		return nil
	}
}

// openPaths need no credential: the handshake proves the server to the client,
// the launch route turns a code into a session, and the health probe is vukem's.
var openPaths = []string{BasePath + "/handshake", "/auth/launch", healthPath}

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

		var caller service.Caller

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
		} else if ck, err := r.Cookie(auth.CookieName); err == nil && h.Sessions.Valid(ck.Value) {
			if !ambientAllowed(r) {
				web.Fail(w, r, refused())
				return
			}

			caller = service.Caller{Actor: "local:" + auth.UILabel, Scopes: service.AllScopes, Browser: true}
		} else {
			h.unauthorized(w, r, &service.Error{
				Code: service.CodeUnauthorized, Message: "the request carries no credentials.",
				Hint: "send the local token as Authorization: Bearer <token>",
			})

			return
		}

		next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), caller)))
	})
}

// authenticateNetwork is the authentication of network mode (SDD-007 sections
// 4.1 and 5.1 item 3): the session cookie of a user who logged in. The local
// token, the handshake and the launch code do not exist in this mode. With no
// user yet, nothing authenticates and every request but the health probe and
// the login is 401, which the start-up log says.
func (h *apiHandler) authenticateNetwork(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.URL.Path == healthPath || r.URL.Path == "/auth/login" || r.URL.Path == "/auth/logout" {
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
		deny()
		return
	}

	if !ambientAllowed(r) {
		web.Fail(w, r, refused())
		return
	}

	next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), user.Caller())))
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
