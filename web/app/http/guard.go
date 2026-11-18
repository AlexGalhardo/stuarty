package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/service"
)

// relayHeaders reveal a tunnel or proxy: the peer is loopback while the client
// may not be (SDD-007 section 3.6 item 2).
var relayHeaders = []string{
	"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Port", "X-Real-Ip", "Via",
}

// credentialParams are the query parameters that never carry a credential here
// (SDD-007 section 3.7, T-04).
var credentialParams = []string{"token", "key", "access_token", "api_key"}

const healthPath = "/up"

func refused() *service.Error {
	return &service.Error{Code: service.CodeForbidden, Message: "Request refused."}
}

// guard is the first thing every request meets in local mode (SDD-007 section
// 3.6): peer, relay, Host, Sec-Fetch-Site and Origin, then the rules that hold
// for every response (no CORS, OPTIONS is 405, no credential in a query). In
// network mode n replaces the first three and the origin values (4.3).
func guard(n *Network) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return guardWith(n, next) }
}

func guardWith(n *Network, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")

		if strings.HasPrefix(r.URL.Path, BasePath+"/") || strings.HasPrefix(r.URL.Path, "/auth/") ||
			r.URL.Path == "/openapi.json" {
			h.Set("Cache-Control", "no-store")
		}

		// Errors on the API are always problem+json, whatever the client prefers.
		// The handlers that answer with an image read what the client asked for
		// through requestedAccept, because the header below is rewritten for them.
		accept := r.Header.Get("Accept")
		if jsonPath(r.URL.Path) && !strings.Contains(accept, "json") {
			r.Header.Set("Accept", "application/json")
		}

		r = r.WithContext(context.WithValue(r.Context(), acceptKey{}, accept))

		if !requestAllowed(n, r) {
			web.Fail(w, r, refused())
			return
		}

		if r.Method == http.MethodOptions {
			h.Set("Allow", "GET, HEAD, POST")
			web.Fail(w, r, errs.ErrMethodNotAllowed)

			return
		}

		q := r.URL.Query()
		for _, name := range credentialParams {
			if q.Has(name) {
				web.Fail(w, r, &service.Error{
					Code:    "E_VALIDATION",
					Message: "a credential in the query string is refused.",
					Hint:    "send it in the Authorization header",
					Path:    name,
				})

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func requestAllowed(n *Network, r *http.Request) bool {
	if n != nil {
		return n.allowed(r)
	}

	return peerAllowed(r) && !hasRelay(r) && hostAllowed(r) && fetchSiteAllowed(r) && originAllowed(r)
}

type acceptKey struct{}

// requestedAccept is the Accept header as the client sent it.
func requestedAccept(r *http.Request) string {
	if a, ok := r.Context().Value(acceptKey{}).(string); ok {
		return a
	}

	return r.Header.Get("Accept")
}

// jsonPath tells the routes whose errors are always problem+json.
func jsonPath(p string) bool {
	return strings.HasPrefix(p, BasePath+"/") || strings.HasPrefix(p, "/auth/") || p == "/openapi.json"
}

func peerAddr(r *http.Request) (netip.Addr, bool) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}

	a := ap.Addr()
	if a.Zone() != "" {
		return netip.Addr{}, false
	}

	return a.Unmap(), true
}

func peerAllowed(r *http.Request) bool {
	a, ok := peerAddr(r)

	return ok && a.IsLoopback()
}

func hasRelay(r *http.Request) bool {
	for _, name := range relayHeaders {
		if _, ok := r.Header[http.CanonicalHeaderKey(name)]; ok {
			return true
		}
	}

	return false
}

// listenPort is the port of the socket that accepted the request.
func listenPort(r *http.Request) string {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return ""
	}

	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}

	return port
}

// browserPort is the port the browser used to reach this server (the Host header), which is not the
// listening port behind a published container port; it falls back to the listening port.
func browserPort(r *http.Request) string {
	if _, port, err := net.SplitHostPort(r.Host); err == nil && port != "" {
		return port
	}

	return listenPort(r)
}

func hostAllowed(r *http.Request) bool {
	port := listenPort(r)
	if port == "" {
		return false
	}

	// The container health probe calls /up as 127.0.0.1:7878 whatever APP_URL
	// says; a loopback peer without a proxy header is exempt (3.7 item 6).
	if r.URL.Path == healthPath && peerAllowed(r) && !hasRelay(r) {
		return true
	}

	switch strings.ToLower(r.Host) {
	case "127.0.0.1:" + port, "localhost:" + port, "[::1]:" + port:
		return true
	}

	return false
}

func fetchSiteAllowed(r *http.Request) bool {
	site, present := r.Header["Sec-Fetch-Site"]
	if !present {
		return true
	}

	return len(site) == 1 && (site[0] == "same-origin" || site[0] == "none")
}

func originAllowed(r *http.Request) bool {
	origin, present := r.Header["Origin"]
	if !present {
		return true
	}

	port := listenPort(r)
	if len(origin) != 1 || port == "" {
		return false
	}

	switch origin[0] {
	case "http://127.0.0.1:" + port, "http://localhost:" + port, "http://[::1]:" + port:
		return true
	}

	return false
}
