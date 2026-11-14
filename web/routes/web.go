package routes

import (
	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/router"
)

// Web installs the default middleware without vukem sessions: the editor
// authenticates with its own local token and in-memory browser sessions
// (SDD-007 section 3), which arrive with the service and auth tasks.
// GET /up is registered by vukem itself. The editor pages, the API and the
// event stream arrive with SDD-003 and SDD-012.
func Web(r *router.Router, app *vukem.App) error {
	return vukem.DefaultMiddlewareWith(app, r, vukem.MiddlewareOptions{Stateless: true})
}
