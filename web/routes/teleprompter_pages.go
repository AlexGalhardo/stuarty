package routes

import (
	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/inertia"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"
)

// teleprompterPage installs the page of the teleprompter (SDD-025 section 20.3): the script of one project, drawn
// in a window of its own. The page reads the script through /api/v1 with the texts operations, so a handler only
// names the page and the id, after checking its shape.
func teleprompterPage(r *router.Router) {
	r.Get("/projects/{project_id}/teleprompter", func(c *web.Ctx) error {
		id := c.Param("project_id")
		if !projectID.MatchString(id) {
			return errs.ErrNotFound
		}

		return inertia.Render(c, "Teleprompter/Show", inertia.Props{"projectId": id})
	}).Name("teleprompter.show")
}
