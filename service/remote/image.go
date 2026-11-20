package remote

import (
	"context"
	"net/http"
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

func imageCheckpointPath(id, name string) string {
	return imagePath(id, "/checkpoints/"+url.PathEscape(name))
}

// ImageRevert implements service.Service.
func (c *Client) ImageRevert(ctx context.Context, req service.ImageRevertRequest) (*service.ImageRevertResult, error) {
	body := struct {
		Revision     int64  `json:"revision,omitempty"`
		Checkpoint   string `json:"checkpoint,omitempty"`
		BaseRevision int64  `json:"base_revision,omitempty"`
		DryRun       bool   `json:"dry_run,omitempty"`
	}{req.Revision, req.Checkpoint, req.BaseRevision, req.DryRun}

	return post[service.ImageRevertResult](ctx, c, imagePath(req.ImageID, "/revert"), body)
}

// ImageDiff implements service.Service.
func (c *Client) ImageDiff(ctx context.Context, req service.ImageDiffRequest) (*service.ImageDiffResult, error) {
	q := url.Values{}
	setIf(q, "from_checkpoint", req.FromCheckpoint)

	if req.From > 0 {
		q.Set("from", strconv.FormatInt(req.From, 10))
	}

	if req.Until > 0 {
		q.Set("until", strconv.FormatInt(req.Until, 10))
	}

	return get[service.ImageDiffResult](ctx, c, imagePath(req.ImageID, "/diff"), q)
}

// ImageCheckpointCreate implements service.Service.
func (c *Client) ImageCheckpointCreate(
	ctx context.Context,
	req service.ImageCheckpointCreateRequest,
) (*service.Checkpoint, error) {
	body := struct {
		Name     string `json:"name"`
		Note     string `json:"note,omitempty"`
		Revision int64  `json:"revision,omitempty"`
	}{req.Name, req.Note, req.Revision}

	return post[service.Checkpoint](ctx, c, imagePath(req.ImageID, "/checkpoints"), body)
}

// ImageCheckpointList implements service.Service.
func (c *Client) ImageCheckpointList(
	ctx context.Context,
	req service.ImageCheckpointListRequest,
) (*service.ImageCheckpointListResult, error) {
	return get[service.ImageCheckpointListResult](ctx, c, imagePath(req.ImageID, "/checkpoints"), nil)
}

// ImageCheckpointRename implements service.Service.
func (c *Client) ImageCheckpointRename(
	ctx context.Context,
	req service.ImageCheckpointRenameRequest,
) (*service.Checkpoint, error) {
	body := struct {
		NewName string  `json:"new_name"`
		Note    *string `json:"note,omitempty"`
	}{req.NewName, req.Note}

	out := new(service.Checkpoint)
	if err := c.do(ctx, http.MethodPatch, imageCheckpointPath(req.ImageID, req.Name), nil, body, out); err != nil {
		return nil, err
	}

	return out, nil
}

// ImageCheckpointRemove implements service.Service.
func (c *Client) ImageCheckpointRemove(
	ctx context.Context,
	req service.ImageCheckpointRemoveRequest,
) (*service.Checkpoint, error) {
	out := new(service.Checkpoint)
	if err := c.do(ctx, http.MethodDelete, imageCheckpointPath(req.ImageID, req.Name), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// ImageExport implements service.Service.
func (c *Client) ImageExport(ctx context.Context, req service.ImageExportRequest) (*service.ImageExportResult, error) {
	// The document is in the path: the route refuses a body that names it as an unknown member.
	body := struct {
		Format  string `json:"format,omitempty"`
		Quality int    `json:"quality,omitempty"`
		Preset  string `json:"preset,omitempty"`
		Width   int    `json:"width,omitempty"`
		Height  int    `json:"height,omitempty"`
	}{req.Format, req.Quality, req.Preset, req.Width, req.Height}

	return post[service.ImageExportResult](ctx, c, imagePath(req.ImageID, "/export"), body)
}

// ImageImport implements service.Service.
func (c *Client) ImageImport(ctx context.Context, req service.ImageImportRequest) (*service.ImageImportResult, error) {
	return post[service.ImageImportResult](ctx, c, "/images/import", req)
}

// ImageImportFile implements service.Service: the bytes of the file are the body, the other members the query.
func (c *Client) ImageImportFile(
	ctx context.Context,
	req service.ImageImportFileRequest,
) (*service.ImageImportResult, error) {
	q := url.Values{}
	setIf(q, "file_name", req.FileName)
	setIf(q, "image_id", req.ImageID)
	setIf(q, "name", req.Name)
	setIf(q, "preset", req.Preset)
	setIf(q, "idempotency_key", req.IdempotencyKey)

	if req.Width > 0 {
		q.Set("width", strconv.Itoa(req.Width))
	}

	if req.Height > 0 {
		q.Set("height", strconv.Itoa(req.Height))
	}

	if req.WaitMS > 0 {
		q.Set("wait_ms", strconv.Itoa(req.WaitMS))
	}

	if req.BaseRevision > 0 {
		q.Set("base_revision", strconv.FormatInt(req.BaseRevision, 10))
	}

	if req.DryRun {
		q.Set("dry_run", "true")
	}

	out := new(service.ImageImportResult)
	if err := c.do(
		ctx,
		http.MethodPost,
		"/images/import/file",
		q,
		fileBody{r: req.Body, size: req.Size},
		out,
	); err != nil {
		return nil, err
	}

	return out, nil
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
