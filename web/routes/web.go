package routes

import (
	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/middleware"
	"github.com/AlexGalhardo/vukem/router"
)

// maxBody is the largest request body: 1 MiB (SDD-003 section 6.1). A batch is
// further limited to 200 operations and 512 KiB by the service.
const maxBody = 1 << 20

// ContentSecurityPolicy is the policy of SDD-007 section 8.5.5, with {nonce}
// replaced per request by vukem. It names every fetch directive and closes
// base-uri and object-src instead of leaving them to the framework default.
const ContentSecurityPolicy = "default-src 'self'; script-src 'self' 'nonce-{nonce}'; " +
	"style-src 'self' 'nonce-{nonce}'; img-src 'self' data:; media-src 'self'; connect-src 'self'; " +
	"font-src 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'"

// Web installs the default middleware without vukem sessions: the editor
// authenticates with its own local token and in-memory browser sessions
// (SDD-007 section 3, web/app/auth), which the API routes enforce.
// GET /up is registered by vukem itself. The browser pages are in pages.go.
func Web(r *router.Router, app *vukem.App) error {
	return vukem.DefaultMiddlewareWith(app, r, vukem.MiddlewareOptions{
		Stateless: true,
		MaxBody:   maxBody,
		SecureHeaders: func(cfg *middleware.SecureHeadersConfig) {
			cfg.CSP = ContentSecurityPolicy
			cfg.ReferrerPolicy = "no-referrer" // SDD-007 section 3.6
		},
	})
}
