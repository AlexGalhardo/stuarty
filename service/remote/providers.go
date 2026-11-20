package remote

import (
	"context"
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service"
)

// ProviderList implements service.Service.
func (c *Client) ProviderList(ctx context.Context, _ service.ProviderListRequest) (*service.ProviderListResult, error) {
	return get[service.ProviderListResult](ctx, c, "/providers", nil)
}

// ProviderModels implements service.Service.
func (c *Client) ProviderModels(
	ctx context.Context,
	req service.ProviderModelsRequest,
) (*service.ProviderModelsResult, error) {
	q := url.Values{}
	if req.Refresh {
		q.Set("refresh", "true")
	}

	return get[service.ProviderModelsResult](ctx, c, "/connections/"+url.PathEscape(req.ConnectionID)+"/models", q)
}

// ProviderPriceSet implements service.Service.
func (c *Client) ProviderPriceSet(
	ctx context.Context,
	req service.ProviderPriceSetRequest,
) (*service.ProviderPricesResult, error) {
	out := new(service.ProviderPricesResult)
	if err := c.do(ctx, http.MethodPut, "/providers/prices", nil, req, out); err != nil {
		return nil, err
	}

	return out, nil
}

// TextGenerate implements service.Service.
func (c *Client) TextGenerate(ctx context.Context, req service.TextGenerateRequest) (*service.Job, error) {
	return jobText(post[service.Job](ctx, c, "/text/generate", req))
}

// ImageGenerate implements service.Service.
func (c *Client) ImageGenerate(ctx context.Context, req service.ImageGenerateRequest) (*service.Job, error) {
	return jobText(post[service.Job](ctx, c, "/image/generate", req))
}

// VideoGenerate implements service.Service.
func (c *Client) VideoGenerate(ctx context.Context, req service.VideoGenerateRequest) (*service.Job, error) {
	return jobText(post[service.Job](ctx, c, "/video/generate", req))
}

// VoiceGenerate implements service.Service.
func (c *Client) VoiceGenerate(ctx context.Context, req service.VoiceGenerateRequest) (*service.Job, error) {
	return jobText(post[service.Job](ctx, c, "/voice/generate", req))
}

// modelEditBody is the body of POST /images/{id}/model-edit: the image is in the path.
type modelEditBody struct {
	ProjectID    string `json:"project_id"`
	ConnectionID string `json:"connection_id"`
	Operation    string `json:"operation"`
	Model        string `json:"model"`
	LayerID      string `json:"layer_id,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
	Size         string `json:"size,omitempty"`
	WaitMS       int    `json:"wait_ms,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
}

// ImageModelEdit implements service.Service.
func (c *Client) ImageModelEdit(ctx context.Context, req service.ImageModelEditRequest) (*service.Job, error) {
	return jobText(post[service.Job](ctx, c, imagePath(req.ImageID, "/model-edit"), modelEditBody{
		ProjectID: req.ProjectID, ConnectionID: req.ConnectionID, Operation: req.Operation, Model: req.Model,
		LayerID: req.LayerID, Prompt: req.Prompt, Size: req.Size, WaitMS: req.WaitMS, DryRun: req.DryRun,
	}))
}

// VoiceList implements service.Service.
func (c *Client) VoiceList(ctx context.Context, req service.VoiceListRequest) (*service.VoiceListResult, error) {
	return get[service.VoiceListResult](ctx, c, "/connections/"+url.PathEscape(req.ConnectionID)+"/voices", nil)
}
