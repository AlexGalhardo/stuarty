package remote

import (
	"context"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service"
)

// SocialNetworkList implements service.Service.
func (c *Client) SocialNetworkList(
	ctx context.Context, _ service.SocialNetworkListRequest,
) (*service.SocialNetworkListResult, error) {
	return get[service.SocialNetworkListResult](ctx, c, "/social/networks", nil)
}

// SocialConnectBegin implements service.Service.
func (c *Client) SocialConnectBegin(
	ctx context.Context, req service.SocialConnectBeginRequest,
) (*service.SocialConnectBeginResult, error) {
	return post[service.SocialConnectBeginResult](ctx, c, "/social/connect", req)
}

// SocialConnectComplete implements service.Service.
func (c *Client) SocialConnectComplete(
	ctx context.Context, req service.SocialConnectCompleteRequest,
) (*service.SocialAccount, error) {
	return post[service.SocialAccount](ctx, c, "/social/complete", req)
}

// SocialAccountList implements service.Service.
func (c *Client) SocialAccountList(
	ctx context.Context, _ service.SocialAccountListRequest,
) (*service.SocialAccountListResult, error) {
	return get[service.SocialAccountListResult](ctx, c, "/social/accounts", nil)
}

// SocialAccountSelect implements service.Service.
func (c *Client) SocialAccountSelect(
	ctx context.Context, req service.SocialAccountSelectRequest,
) (*service.SocialAccount, error) {
	return post[service.SocialAccount](ctx, c, "/social/select", req)
}

// PublishValidate implements service.Service.
func (c *Client) PublishValidate(
	ctx context.Context, req service.PublishValidateRequest,
) (*service.PublishValidateResult, error) {
	return post[service.PublishValidateResult](ctx, c, "/publish/validate", req)
}

// PublishNow implements service.Service.
func (c *Client) PublishNow(ctx context.Context, req service.PublishNowRequest) (*service.Job, error) {
	return post[service.Job](ctx, c, "/publish", req)
}

// PublishStatus implements service.Service.
func (c *Client) PublishStatus(
	ctx context.Context, req service.PublishStatusRequest,
) (*service.PublishStatusResult, error) {
	return get[service.PublishStatusResult](ctx, c, "/publish/"+url.PathEscape(req.JobID), nil)
}
