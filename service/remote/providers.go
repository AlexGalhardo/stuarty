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

// VoiceGenerate implements service.Service.
func (c *Client) VoiceGenerate(ctx context.Context, req service.VoiceGenerateRequest) (*service.Job, error) {
	return jobText(post[service.Job](ctx, c, "/voice/generate", req))
}

// VoiceList implements service.Service.
func (c *Client) VoiceList(ctx context.Context, req service.VoiceListRequest) (*service.VoiceListResult, error) {
	return get[service.VoiceListResult](ctx, c, "/connections/"+url.PathEscape(req.ConnectionID)+"/voices", nil)
}
