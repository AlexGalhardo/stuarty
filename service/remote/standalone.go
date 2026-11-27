package remote

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

// StandaloneVideoList implements service.Service.
func (c *Client) StandaloneVideoList(
	ctx context.Context,
	req service.StandaloneVideoListRequest,
) (*service.ProjectListResult, error) {
	q := url.Values{}
	setIf(q, "state", req.State)

	return get[service.ProjectListResult](ctx, c, "/standalone/videos", pageQuery(q, req.Page))
}

// StandaloneVideoCreate implements service.Service.
func (c *Client) StandaloneVideoCreate(
	ctx context.Context,
	req service.StandaloneVideoCreateRequest,
) (*service.StandaloneVideoResult, error) {
	return post[service.StandaloneVideoResult](ctx, c, "/standalone/videos", req)
}

// StandaloneVideoUpload implements service.Service.
func (c *Client) StandaloneVideoUpload(
	ctx context.Context,
	req service.StandaloneVideoUploadRequest,
) (*service.StandaloneVideoResult, error) {
	q := url.Values{}
	setIf(q, "file_name", req.FileName)
	setIf(q, "name", req.Name)
	setIf(q, "preset", req.Preset)
	setIf(q, "fps", req.FPS)

	if req.WaitMS > 0 {
		q.Set("wait_ms", strconv.Itoa(req.WaitMS))
	}

	if req.DryRun {
		q.Set("dry_run", "true")
	}

	out := new(service.StandaloneVideoResult)
	if err := c.do(
		ctx,
		http.MethodPost,
		"/standalone/videos/upload",
		q,
		fileBody{r: req.Body, size: req.Size},
		out,
	); err != nil {
		return nil, err
	}

	return out, nil
}

// StandalonePromote implements service.Service.
func (c *Client) StandalonePromote(
	ctx context.Context,
	req service.StandalonePromoteRequest,
) (*service.ProjectUpdateResult, error) {
	body := struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		Goal        service.ProjectGoal `json:"goal"`
		Kind        string              `json:"kind,omitempty"`
		DryRun      bool                `json:"dry_run,omitempty"`
	}{req.Name, req.Description, req.Goal, req.Kind, req.DryRun}

	return post[service.ProjectUpdateResult](ctx, c, "/standalone/videos/"+segment(req.ProjectID)+"/promote", body)
}

// ImageOpenFromAsset implements service.Service.
func (c *Client) ImageOpenFromAsset(
	ctx context.Context,
	req service.ImageOpenFromAssetRequest,
) (*service.ImageImportResult, error) {
	return post[service.ImageImportResult](ctx, c, "/images/from-asset", req)
}
