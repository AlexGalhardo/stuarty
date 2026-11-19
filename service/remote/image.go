package remote

import (
	"context"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The image operations of SDD-021 section 11 over HTTP.

func imagePath(id, suffix string) string { return "/images/" + url.PathEscape(id) + suffix }

// ImageCreate implements service.Service.
func (c *Client) ImageCreate(ctx context.Context, req service.ImageCreateRequest) (*service.ImageCreateResult, error) {
	return post[service.ImageCreateResult](ctx, c, "/images", req)
}

// ImageList implements service.Service.
func (c *Client) ImageList(ctx context.Context, req service.ImageListRequest) (*service.ImageListResult, error) {
	q := url.Values{}
	setIf(q, "templates", req.Templates)

	return get[service.ImageListResult](ctx, c, "/images", pageQuery(q, req.Page))
}

// ImageDescribe implements service.Service.
func (c *Client) ImageDescribe(
	ctx context.Context,
	req service.ImageDescribeRequest,
) (*service.ImageDescribeResult, error) {
	return get[service.ImageDescribeResult](ctx, c, imagePath(req.ImageID, ""), nil)
}

// ImageEdit implements service.Service.
func (c *Client) ImageEdit(ctx context.Context, req service.ImageEditRequest) (*service.ImageEditResult, error) {
	return post[service.ImageEditResult](ctx, c, imagePath(req.ImageID, "/edit"), editBody{
		BaseRevision:   req.BaseRevision,
		Operations:     req.Operations,
		IdempotencyKey: req.IdempotencyKey,
		DryRun:         req.DryRun,
	})
}

// ImageUndo implements service.Service.
func (c *Client) ImageUndo(ctx context.Context, req service.ImageStepRequest) (*service.ImageStepResult, error) {
	return post[service.ImageStepResult](ctx, c, imagePath(req.ImageID, "/undo"), dryBody{req.DryRun})
}

// ImageRedo implements service.Service.
func (c *Client) ImageRedo(ctx context.Context, req service.ImageStepRequest) (*service.ImageStepResult, error) {
	return post[service.ImageStepResult](ctx, c, imagePath(req.ImageID, "/redo"), dryBody{req.DryRun})
}

// ImageExport implements service.Service.
func (c *Client) ImageExport(ctx context.Context, req service.ImageExportRequest) (*service.ImageExportResult, error) {
	return post[service.ImageExportResult](ctx, c, imagePath(req.ImageID, "/export"), req)
}

// ImageFrame implements service.Service. The picture is binary and the screen reads it from its URL.
func (*Client) ImageFrame(context.Context, service.ImageFrameRequest) (*service.ImageFrameResult, error) {
	return nil, &service.Error{
		Code:    command.CodeInternal,
		Message: "the picture of an image is not read over this client.",
		Hint:    "read it from GET /images/{id}/frame",
	}
}

// ImageHistory implements service.Service.
func (c *Client) ImageHistory(
	ctx context.Context,
	req service.ImageHistoryRequest,
) (*service.ImageHistoryResult, error) {
	q := url.Values{}

	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	return get[service.ImageHistoryResult](ctx, c, imagePath(req.ImageID, "/history"), q)
}

// ImageFile implements service.Service. A stored file is a path inside the workspace of the server.
func (*Client) ImageFile(context.Context, service.ImageFileRequest) (*service.File, error) {
	return nil, &service.Error{
		Code:    command.CodeInternal,
		Message: "stored files are not opened over HTTP.",
		Hint:    "read the file from GET /images/{id}/exports/{name}",
	}
}
