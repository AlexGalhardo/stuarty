package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
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
	// Account is the local account (ADR-0052); nil in network mode, where its routes answer E_NOT_FOUND. With
	// it, a launch code never privileges a session: only the password does (SDD-007 section 3.9 item 5).
	Account *account.Manager
	// AccountApply creates the account in a workspace that holds data (SDD-005 section 5.17.3); nil only stores
	// the record.
	AccountApply account.Apply
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
	// AgentTokens is the registry of the session tokens of the hosted agent chat (SDD-017 section 6.1); nil
	// means no hosted agent can authenticate.
	AgentTokens auth.AgentTokens
	// Chat is the host of the hosted agent, for the event stream of a session (SDD-017 section 8); nil serves the stored
	// events only.
	Chat ChatStreams
}

type apiHandler struct {
	Deps
	app           *vukem.App
	r             *router.Router
	loginThrottle loginThrottles
	limits        *limits
	// streams ends the requests in progress when the studio locks.
	streams *lockScope
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

		if d.Account != nil {
			// A locked studio has no session left (SDD-007 section 3.9 item 4).
			d.Account.OnLock(d.Sessions.Clear)
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
			limits: newLimits(d.RateLimits, d.Now), streams: newLockScope(),
		}

		if d.Account != nil {
			d.Account.OnLock(h.streams.end)
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

	return api.Op{Name: ep.ID, Summary: ep.Summary, Status: ep.Status, Security: []string{"bearer"}, MaxBody: ep.MaxBody}
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

		if slices.Contains(openPaths, r.URL.Path) || (h.Account != nil && accountOpen(r)) {
			next.ServeHTTP(w, r)
			return
		}

		if h.accountPageAnswered(w, r, next) {
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
			bearer, ok := h.bearerCaller(w, r, header)
			if !ok {
				return
			}

			caller = bearer
		} else if valid {
			if !ambientAllowed(r) {
				web.Fail(w, r, refused())
				return
			}

			caller = service.Caller{Actor: "local:" + auth.UILabel, Scopes: service.AllScopes, Browser: privileged}
		} else {
			h.noCredentials(w, r, next)

			return
		}

		// Bound before the check, so a lock that lands between them ends this request.
		ctx, release := h.boundToLock(r)
		defer release()

		if h.refuseWhileLocked(w, r) {
			return
		}

		next.ServeHTTP(w, r.WithContext(service.WithCaller(ctx, caller)))
	})
}

// accountPageAnswered answers a page navigation of a server that has an account: the account pages are drawn
// with no session, and any other page goes to the sign-in while the studio is locked. It reports whether it
// answered; false lets the authentication go on.
func (h *apiHandler) accountPageAnswered(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if h.Account == nil || !isPageNavigation(r) {
		return false
	}

	to, serve := h.accountPage(r)

	switch {
	case to != "":
		http.Redirect(w, r, to, http.StatusSeeOther)
	case serve:
		next.ServeHTTP(w, r)
	default:
		return false
	}

	return true
}

// noCredentials answers a local request that carries a valid credential of no kind.
func (h *apiHandler) noCredentials(w http.ResponseWriter, r *http.Request, next http.Handler) {
	// The consent screen of a social network sends the browser back to the sign-in page by a cross-site
	// navigation, which carries no SameSite=Strict session cookie. The page is drawn with no caller: it holds
	// no data, calls no operation, and hands the state and the code to POST /api/v1/social/complete, a
	// same-origin call that does carry the cookie and is authenticated as every other one (SDD-007 section
	// 3.7, SDD-020 section 6).
	if r.Method == http.MethodGet && isSignInCallbackPath(r.URL.Path) {
		next.ServeHTTP(w, r)

		return
	}

	// A browser that opens a page with no session is told how to get in (VV-157): the page is plain HTML and
	// carries no secret, since the launch code is issued only to a caller that proves the token (SDD-007
	// section 3.4).
	if isPageNavigation(r) {
		// With an account, a browser that has no session goes to the sign-in page; with none yet, it is told
		// how to open Stuarty.
		if h.Account != nil && h.Account.State() != account.StateNone {
			http.Redirect(w, r, AccountSignInPath, http.StatusSeeOther)

			return
		}

		w.Header().Set("WWW-Authenticate", "Bearer")
		writeSignInPage(w)

		return
	}

	h.unauthorized(w, r, &service.Error{
		Code: service.CodeUnauthorized, Message: "the request carries no credentials.", Hint: signInHint,
	})
}

// The pages of the local account (SDD-012 section 2.1.1).
const (
	AccountCreatePath  = "/account/create"
	AccountSignInPath  = "/account/signin"
	AccountRecoverPath = "/account/recover"
)

// accountPage decides a page navigation of a server that has an account manager: a redirect target, or
// whether the page is one of the account pages, which are drawn with no session (they hold no data). Any other
// page goes on to the authentication, after a locked studio has sent the browser to the sign-in.
func (h *apiHandler) accountPage(r *http.Request) (to string, serve bool) {
	state := h.Account.State()

	switch r.URL.Path {
	case AccountCreatePath:
		if state != account.StateNone {
			return "/", false
		}

		return "", true
	case AccountSignInPath, AccountRecoverPath:
		if state == account.StateNone {
			return AccountCreatePath, false
		}

		return "", true
	}

	if state == account.StateLocked {
		return AccountSignInPath, false
	}

	return "", false
}

// refuseWhileLocked answers E_LOCKED to a request that is not a page and not an account route while the studio
// is locked, and records the activity the idle lock counts from otherwise. It reports whether it answered.
func (h *apiHandler) refuseWhileLocked(w http.ResponseWriter, r *http.Request) bool {
	if h.Account == nil {
		return false
	}

	if h.Account.Check() != nil && !isPageNavigation(r) && !strings.HasPrefix(r.URL.Path, BasePathAuth+"/account") &&
		!launchRequest(r) {
		web.Fail(w, r, accountError(account.ErrLocked))

		return true
	}

	// A new workspace shows the page that creates the account before anything else (SDD-012 section 2.1.1).
	if h.Account.State() == account.StateNone && isPageNavigation(r) {
		http.Redirect(w, r, AccountCreatePath, http.StatusSeeOther)

		return true
	}

	h.Account.Touch()

	return false
}

// launchRequest is POST /api/v1/system/launch, which answers while the studio is locked: the desktop shell and
// `stuarty open` ask for it to open their window, and that window is the sign-in page. A launch code reads nothing
// and is never a person (SDD-007 section 3.9 items 5 and 6).
func launchRequest(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == BasePath+"/system/launch"
}

// bearerCaller is the caller of an Authorization header in local mode: the session token of a hosted
// agent, or the local token with the client header. A false ok means the refusal was written.
func (h *apiHandler) bearerCaller(w http.ResponseWriter, r *http.Request, header string) (service.Caller, bool) {
	token, ok := auth.Bearer(header)

	// The session token of a hosted agent: bound to one project, never a person (SDD-017 section 6.1).
	if g, found := h.agentGrant(token, ok); found {
		return auth.AgentCaller(g, false), true
	}

	if !ok || !auth.TokenMatches(h.Token, token) {
		h.unauthorized(w, r, &service.Error{Code: service.CodeUnauthorized, Message: "the token is not valid."})

		return service.Caller{}, false
	}

	actor, err := auth.Actor(r.Header.Get(auth.ClientHeader))
	if err != nil {
		web.Fail(w, r, &service.Error{Code: command.CodeValidation, Message: err.Error(), Path: auth.ClientHeader})

		return service.Caller{}, false
	}

	return service.Caller{Actor: actor, Scopes: service.AllScopes}, true
}

// agentGrant is the grant of a bearer credential that is a live session token.
func (h *apiHandler) agentGrant(token string, ok bool) (agenthost.Grant, bool) {
	if !ok || h.AgentTokens == nil {
		return agenthost.Grant{}, false
	}

	return h.AgentTokens.Lookup(token)
}

// authenticateNetwork is the authentication of network mode (SDD-007 sections
// 4.1 and 5.1 item 3): the session cookie of a user who logged in. The local
// token and the launch code do not exist in this mode, and the handshake exists
// only as the proof of the per-run secret. With no user yet, nothing
// authenticates and every request but the health probe, the handshake and the
// login is 401, which the start-up log says.
func (h *apiHandler) authenticateNetwork(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.URL.Path == healthPath || r.URL.Path == "/auth/login" || r.URL.Path == "/auth/logout" ||
		(r.URL.Path == LoginPath && r.Method == http.MethodGet) ||
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

		if g, found := h.agentGrant(secret, true); found {
			if g.Owner != "" && !h.Auth.UserActive(r.Context(), g.Owner) {
				deny()
				return
			}

			next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), auth.AgentCaller(g, true))))
			return
		}

		caller, err := h.Auth.KeyCaller(r.Context(), secret)

		switch {
		case errors.Is(err, auth.ErrBadKey):
			deny()
		case err != nil:
			web.Fail(w, r, &service.Error{Code: command.CodeInternal, Message: "the key could not be checked."})
		default:
			// A key made for the read API of the blog reaches its three routes and nothing else: not a page, a
			// stream or another operation (SDD-020 section 47, T-95).
			if postsKeyOnly(&caller) && !postsRoute(r) {
				web.Fail(w, r, &service.Error{
					Code:    service.CodeForbidden,
					Message: "this key can read published posts and nothing else.",
					Hint:    "use a key with the scope of the route",
					Details: map[string]any{"scope": service.ScopePostsRead},
				})

				return
			}

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

// discardResponseWriter is a no-op http.ResponseWriter used during read-only SSE session
// revalidation so session-guard checks never mutate headers on an already-flushed stream.
type discardResponseWriter struct{ hdr http.Header }

func (d *discardResponseWriter) Header() http.Header {
	if d.hdr == nil {
		d.hdr = make(http.Header)
	}

	return d.hdr
}

func (d *discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardResponseWriter) WriteHeader(int)             {}

// revalidateCaller re-verifies the request's active credential (bearer token, API key, hosted
// agent token, or session cookie) against live store/user state on SSE revalidation ticks,
// returning a fresh service.Caller so revocations, user disablement, password resets, and role
// demotions close open streams.
func (h *apiHandler) revalidateCaller(c *web.Ctx) (service.Caller, bool) {
	r := c.Request()
	if h.Network != nil {
		return h.revalidateNetworkCaller(r)
	}

	return h.revalidateLocalCaller(r)
}

func (h *apiHandler) revalidateNetworkCaller(r *http.Request) (service.Caller, bool) {
	if header := r.Header.Get("Authorization"); header != "" {
		secret, ok := auth.Bearer(header)
		if !ok {
			return service.Caller{}, false
		}

		if g, found := h.agentGrant(secret, true); found {
			if g.Owner != "" && !h.Auth.UserActive(r.Context(), g.Owner) {
				return service.Caller{}, false
			}

			return auth.AgentCaller(g, true), true
		}

		caller, err := h.Auth.KeyCaller(r.Context(), secret)
		if err != nil || postsKeyOnly(&caller) {
			return service.Caller{}, false
		}

		return caller, true
	}

	user, ok, err := h.Auth.SessionUser(web.NewCtx(&discardResponseWriter{}, r, h.r.Services()))
	if err != nil || !ok {
		return service.Caller{}, false
	}

	return user.Caller(), true
}

func (h *apiHandler) revalidateLocalCaller(r *http.Request) (service.Caller, bool) {
	if h.Account != nil && h.Account.Check() != nil {
		return service.Caller{}, false
	}

	if header := r.Header.Get("Authorization"); header != "" {
		token, ok := auth.Bearer(header)
		if g, found := h.agentGrant(token, ok); found {
			return auth.AgentCaller(g, false), true
		}

		if !ok || !auth.TokenMatches(h.Token, token) {
			return service.Caller{}, false
		}

		actor, err := auth.Actor(r.Header.Get(auth.ClientHeader))
		if err != nil {
			return service.Caller{}, false
		}

		return service.Caller{Actor: actor, Scopes: service.AllScopes}, true
	}

	ck, err := r.Cookie(auth.LocalCookieName(browserPort(r)))
	if err != nil {
		return service.Caller{}, false
	}

	valid, privileged := h.Sessions.Lookup(ck.Value)
	if !valid {
		return service.Caller{}, false
	}

	return service.Caller{Actor: "local:" + auth.UILabel, Scopes: service.AllScopes, Browser: privileged}, true
}

// postsKeyOnly says whether the caller holds the scope of the read API of the blog and no other.
func postsKeyOnly(c *service.Caller) bool {
	return len(c.Scopes) == 1 && c.Scopes[0] == service.ScopePostsRead
}

// postsRoute says whether the request is a read of the published posts: GET or HEAD of /api/v1/posts, of one post, or
// of one image of it. The path is cleaned the way the router does, so a dot segment reaches no other route.
func postsRoute(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}

	p := r.URL.Path
	if p != path.Clean(p) {
		return false
	}

	base := BasePath + "/posts"

	return p == base || strings.HasPrefix(p, base+"/")
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
