package remote

import (
	"context"
	"net/url"

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
