package routes

import (
	"regexp"

	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/inertia"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"
)

var imageID = regexp.MustCompile(`^img_[0-9a-hjkmnp-tv-z]{10}$`)

// imagePages installs the two pages of the image editor (SDD-012 section 24.1): the list of documents and
// templates, and the editor of one document. The pages read and change the document through /api/v1/images,
// so a handler only names the page (and the id, after checking its shape).
func imagePages(r *router.Router) {
	r.Get("/edit-image", func(c *web.Ctx) error {
		return inertia.Render(c, "EditImage/Index", inertia.Props{})
	}).Name("edit_image.index")

	r.Get("/edit-image/{image_id}", func(c *web.Ctx) error {
		id := c.Param("image_id")
		if !imageID.MatchString(id) {
			return errs.ErrNotFound
		}

		return inertia.Render(c, "EditImage/Show", inertia.Props{"imageId": id})
	}).Name("edit_image.show")
}
