package httpapi

import (
	"os"
	"time"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The HTTP encoding of the blog operations (SDD-019 section 13, SDD-020 sections 47 and 48). They carry no rule: the
// Service validates (ADR-0008), core/writing renders, and the routes of the published posts read one table.

type postCheckIn struct {
	ID     string `json:"id"      validate:"required" doc:"The project of the post, prj_ and 10 characters."`
	ItemID string `json:"item_id" validate:"required" doc:"A text of the kind post."`
}

type postsListIn struct {
	Limit  int    `json:"limit,omitempty"  doc:"The page size, 1 to 50; the default is 20."`
	Cursor string `json:"cursor,omitempty" doc:"The next_cursor of the previous page."`
}

type postGetIn struct {
	Slug string `json:"slug" validate:"required" doc:"The slug of a published post."`
}

type postMediaIn struct {
	Slug string `json:"slug" validate:"required" doc:"The slug of a published post."`
	Name string `json:"name" validate:"required" doc:"The file name of an image of the post, as the post lists it."`
}

func (h *apiHandler) postCheck(c *web.Ctx, in postCheckIn) (service.PostCheckResult, error) {
	res, err := h.Service.PostCheck(c.Context(), service.PostCheckRequest{ProjectID: in.ID, ItemID: in.ItemID})

	return deref(res), err
}

func (h *apiHandler) postsList(c *web.Ctx, in postsListIn) (service.PublishedPostListResult, error) {
	res, err := h.Service.PublishedPostList(
		c.Context(),
		service.PublishedPostListRequest{Limit: in.Limit, Cursor: in.Cursor},
	)
	if err != nil {
		return service.PublishedPostListResult{}, err
	}

	if c.NotModified(res.ETag, time.Time{}) {
		return service.PublishedPostListResult{}, nil
	}

	return deref(res), nil
}

func (h *apiHandler) postsGet(c *web.Ctx, in postGetIn) (service.PublishedPostResult, error) {
	res, err := h.Service.PublishedPostGet(c.Context(), service.PublishedPostGetRequest{Slug: in.Slug})
	if err != nil {
		return service.PublishedPostResult{}, err
	}

	if c.NotModified(res.ETag, time.Time{}) {
		return service.PublishedPostResult{}, nil
	}

	return deref(res), nil
}

// postsMedia serves the bytes of an image of a published post: the type the service recorded, never sniffed, and
// never rendered by a browser as anything else.
func (h *apiHandler) postsMedia(c *web.Ctx, in postMediaIn) (noIn, error) {
	m, err := h.Service.PublishedPostMedia(c.Context(), service.PublishedPostMediaRequest{Slug: in.Slug, Name: in.Name})
	if err != nil {
		return noIn{}, err
	}

	if c.NotModified(`"`+m.SHA256+`"`, time.Time{}) {
		return noIn{}, nil
	}

	hd := c.Writer().Header()
	hd.Set("Content-Type", m.ContentType)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Disposition", "inline")
	hd.Set("Cache-Control", "private, max-age=0, must-revalidate")

	if err := c.File(os.DirFS(string(h.Workspace)), m.Path); err != nil {
		return noIn{}, err
	}

	return noIn{}, nil
}
