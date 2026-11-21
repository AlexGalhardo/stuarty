package remote

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

// WebhookEventList implements service.Service.
func (c *Client) WebhookEventList(
	ctx context.Context, _ service.WebhookEventListRequest,
) (*service.WebhookEventListResult, error) {
	return get[service.WebhookEventListResult](ctx, c, "/webhook-events", nil)
}

// WebhookList implements service.Service.
func (c *Client) WebhookList(ctx context.Context, _ service.WebhookListRequest) (*service.WebhookListResult, error) {
	return get[service.WebhookListResult](ctx, c, "/webhooks", nil)
}

// WebhookGet implements service.Service.
func (c *Client) WebhookGet(ctx context.Context, req service.WebhookRef) (*service.Webhook, error) {
	return get[service.Webhook](ctx, c, "/webhooks/"+url.PathEscape(req.ID), nil)
}

// WebhookCreate implements service.Service.
func (c *Client) WebhookCreate(
	ctx context.Context, req service.WebhookCreateRequest,
) (*service.WebhookSecretResult, error) {
	return post[service.WebhookSecretResult](ctx, c, "/webhooks", req)
}

// WebhookUpdate implements service.Service.
func (c *Client) WebhookUpdate(ctx context.Context, req service.WebhookUpdateRequest) (*service.Webhook, error) {
	out := new(service.Webhook)
	if err := c.do(ctx, http.MethodPatch, "/webhooks/"+url.PathEscape(req.ID), nil, req, out); err != nil {
		return nil, err
	}

	return out, nil
}

// WebhookRemove implements service.Service.
func (c *Client) WebhookRemove(ctx context.Context, req service.WebhookRef) (*service.Webhook, error) {
	out := new(service.Webhook)
	if err := c.do(ctx, http.MethodDelete, "/webhooks/"+url.PathEscape(req.ID), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// WebhookRotateSecret implements service.Service.
func (c *Client) WebhookRotateSecret(ctx context.Context, req service.WebhookRef) (*service.WebhookSecretResult, error) {
	return post[service.WebhookSecretResult](ctx, c, "/webhooks/"+url.PathEscape(req.ID)+"/rotate-secret", struct{}{})
}

// WebhookTest implements service.Service.
func (c *Client) WebhookTest(ctx context.Context, req service.WebhookRef) (*service.WebhookDelivery, error) {
	return post[service.WebhookDelivery](ctx, c, "/webhooks/"+url.PathEscape(req.ID)+"/test", struct{}{})
}

// WebhookDeliveryList implements service.Service.
func (c *Client) WebhookDeliveryList(
	ctx context.Context, req service.WebhookDeliveriesRequest,
) (*service.WebhookDeliveriesResult, error) {
	q := url.Values{}
	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	if req.Offset > 0 {
		q.Set("offset", strconv.Itoa(req.Offset))
	}

	return get[service.WebhookDeliveriesResult](ctx, c, "/webhooks/"+url.PathEscape(req.ID)+"/deliveries", q)
}

// WebhookRedeliver implements service.Service.
func (c *Client) WebhookRedeliver(
	ctx context.Context, req service.WebhookRedeliverRequest,
) (*service.WebhookDelivery, error) {
	return post[service.WebhookDelivery](
		ctx, c, "/webhooks/"+url.PathEscape(req.ID)+"/deliveries/"+url.PathEscape(req.DeliveryID)+"/redeliver", struct{}{})
}
