package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type providerSignInIn struct {
	Provider     string `json:"provider"                validate:"required" doc:"The provider to sign in to: openai (Sign in with ChatGPT) or openrouter."`
	Label        string `json:"label,omitempty"                             doc:"A name for a new connection, 1 to 60 characters; the default name of the provider when left out."`
	ConnectionID string `json:"connection_id,omitempty"                     doc:"Sign in again an existing connection made by a sign-in, instead of making one."`
	RedirectURI  string `json:"redirect_uri"            validate:"required" doc:"The address of this app followed by /auth/callback, at the address you opened the app with. OpenAI accepts only http://127.0.0.1:<port>/auth/callback."`
}

type providerSignInCompleteIn struct {
	State    string `json:"state"               validate:"required" doc:"The state the provider sent back to the redirect address."`
	Code     string `json:"code,omitempty"                          doc:"The authorization code the provider sent back."`
	Error    string `json:"error,omitempty"                         doc:"The error the provider sent back instead of a code, for example access_denied."`
	ClientID string `json:"client_id,omitempty"                     doc:"The client id the provider issued at the first sign-in (OpenAI sends it in the redirect)."`
}

func (h *apiHandler) providersSignInBegin(
	c *web.Ctx,
	in providerSignInIn,
) (service.ProviderSignInBeginResult, error) {
	res, err := h.Service.ProviderSignInBegin(c.Context(), service.ProviderSignInBeginRequest{
		Provider: in.Provider, Label: in.Label, ConnectionID: in.ConnectionID, RedirectURI: in.RedirectURI,
	})

	return deref(res), err
}

func (h *apiHandler) providersSignInComplete(
	c *web.Ctx,
	in providerSignInCompleteIn,
) (service.ProviderSignIn, error) {
	res, err := h.Service.ProviderSignInComplete(c.Context(), service.ProviderSignInCompleteRequest(in))

	return deref(res), err
}

func (h *apiHandler) providersSignIns(c *web.Ctx, _ noIn) (service.ProviderSignInListResult, error) {
	res, err := h.Service.ProviderSignInList(c.Context(), service.ProviderSignInListRequest{})

	return deref(res), err
}
