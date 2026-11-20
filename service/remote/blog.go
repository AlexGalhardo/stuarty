package remote

import (
	"context"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// PostCheck implements service.Service.
func (c *Client) PostCheck(ctx context.Context, req service.PostCheckRequest) (*service.PostCheckResult, error) {
	return get[service.PostCheckResult](ctx, c, textPath(req.ProjectID, req.ItemID, "/post"), nil)
}

// PublishedPostList implements service.Service.
func (c *Client) PublishedPostList(
	ctx context.Context, req service.PublishedPostListRequest,
) (*service.PublishedPostListResult, error) {
	q := url.Values{}
	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	if req.Cursor != "" {
		q.Set("cursor", req.Cursor)
	}

	return get[service.PublishedPostListResult](ctx, c, "/posts", q)
}

// PublishedPostGet implements service.Service.
func (c *Client) PublishedPostGet(
	ctx context.Context, req service.PublishedPostGetRequest,
) (*service.PublishedPostResult, error) {
	return get[service.PublishedPostResult](ctx, c, "/posts/"+url.PathEscape(req.Slug), nil)
}

// PublishedPostMedia implements service.Service. The bytes of an image are read from its route; a path in the
// workspace of the server is of no use to a client.
func (*Client) PublishedPostMedia(
	context.Context,
	service.PublishedPostMediaRequest,
) (*service.PublishedPostMediaResult, error) {
	return nil, &service.Error{
		Code:    command.CodeInternal,
		Message: "stored files are not opened over HTTP.",
		Hint:    "read the image from /posts/{slug}/media/{name}",
	}
}
