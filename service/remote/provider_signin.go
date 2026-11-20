package remote

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

// ProviderSignInBegin implements service.Service.
func (c *Client) ProviderSignInBegin(
	ctx context.Context, req service.ProviderSignInBeginRequest,
) (*service.ProviderSignInBeginResult, error) {
	return post[service.ProviderSignInBeginResult](ctx, c, "/providers/signin", req)
}

// ProviderSignInComplete implements service.Service.
func (c *Client) ProviderSignInComplete(
	ctx context.Context, req service.ProviderSignInCompleteRequest,
) (*service.ProviderSignIn, error) {
	return post[service.ProviderSignIn](ctx, c, "/providers/signin/complete", req)
}

// ProviderSignInList implements service.Service.
func (c *Client) ProviderSignInList(
	ctx context.Context, _ service.ProviderSignInListRequest,
) (*service.ProviderSignInListResult, error) {
	return get[service.ProviderSignInListResult](ctx, c, "/providers/sign-ins", nil)
}
