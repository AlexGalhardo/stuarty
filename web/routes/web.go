package routes

import (
	"net/http"
	"strings"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/i18n"
	"github.com/AlexGalhardo/vukem/middleware"
	"github.com/AlexGalhardo/vukem/router"
)

// maxBody is the largest request body: 1 MiB (SDD-003 section 6.1). A batch is
// further limited to 200 operations and 512 KiB by the service.
const maxBody = 1 << 20

// ContentSecurityPolicy is the policy of SDD-007 section 8.5.5, with {nonce}
// replaced per request by vukem. It names every fetch directive and closes
// base-uri and object-src instead of leaving them to the framework default.
// 'wasm-unsafe-eval' lets the render worker compile the renderer module
// (SDD-013 section 4.7); it allows WebAssembly only, never eval of script.
const ContentSecurityPolicy = "default-src 'self'; script-src 'self' 'nonce-{nonce}' 'wasm-unsafe-eval'; " +
	"worker-src 'self'; style-src 'self' 'nonce-{nonce}'; img-src 'self' data:; media-src 'self'; connect-src 'self'; " +
	"font-src 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'"

// LangHeader carries the language the browser keeps in its own storage.
const LangHeader = "Stuarty-Lang"

// langHeader lets LangHeader stand for Accept-Language, which the locale
// middleware reads; without it the browser's own Accept-Language applies.
func langHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The API answers in English whatever the client asks for (SDD-003 section 1): leave the locale
		// middleware nothing to pick, or Content-Language would name a language the body is not in (VV-314).
		if strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/openapi.json" {
			r.Header.Del("Accept-Language")
			r.Header.Del(LangHeader)

			if q := r.URL.Query(); q.Has("lang") {
				q.Del("lang")
				r.URL.RawQuery = q.Encode()
			}

			next.ServeHTTP(w, r)

			return
		}

		if v := r.Header.Get(LangHeader); v != "" {
			r.Header.Set("Accept-Language", v)
		}

		w.Header().Add("Vary", LangHeader)
		next.ServeHTTP(w, r)
	})
}

// Web installs the default middleware without vukem sessions: the editor
// authenticates with its own local token and in-memory browser sessions
// (SDD-007 section 3, web/app/auth), which the API routes enforce.
// GET /up is registered by vukem itself. The browser pages are in pages.go.
func Web(r *router.Router, app *vukem.App) error {
	r.UseServer(langHeader)

	return vukem.DefaultMiddlewareWith(app, r, vukem.MiddlewareOptions{
		Stateless: true,
		// The language is a preference of each browser, not of the workspace
		// (SDD-012 section 16.3): the query parameter of the page load and the
		// header (langHeader) decide, and no cookie is read or written.
		I18n:    &i18n.MiddlewareConfig{Order: []i18n.Source{i18n.SourceQuery, i18n.SourceHeader}},
		MaxBody: maxBody,
		SecureHeaders: func(cfg *middleware.SecureHeadersConfig) {
			cfg.CSP = ContentSecurityPolicy
			cfg.ReferrerPolicy = "no-referrer" // SDD-007 section 3.6
		},
	})
}
