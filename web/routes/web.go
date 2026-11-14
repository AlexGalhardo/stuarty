package routes

import (
	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/middleware"
	"github.com/AlexGalhardo/vukem/router"
)

// maxBody is the largest request body: 1 MiB (SDD-003 section 6.1). A batch is
// further limited to 200 operations and 512 KiB by the service.
const maxBody = 1 << 20

// Web installs the default middleware without vukem sessions: the editor
// authenticates with its own local token and in-memory browser sessions
// (SDD-007 section 3, web/app/auth), which the API routes enforce.
// GET /up is registered by vukem itself. The editor pages and the event stream
// arrive with SDD-012 and SDD-006.
func Web(r *router.Router, app *vukem.App) error {
	return vukem.DefaultMiddlewareWith(app, r, vukem.MiddlewareOptions{
		Stateless: true,
		MaxBody:   maxBody,
		SecureHeaders: func(cfg *middleware.SecureHeadersConfig) {
			cfg.ReferrerPolicy = "no-referrer" // SDD-007 section 3.6
		},
	})
}
