package tiktok

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// accessToken asks for an access token with the refresh token of the connection, under the lock of the
// connection. TikTok answers a different refresh token at every refresh: the new secret is written into the
// vault at once, before the access token is used, or the sign-in would be lost. The access token is held for
// the length of one operation and never stored.
func (c *Connector) accessToken(ctx context.Context, cred connections.Credential) (string, error) {
	s, err := parseSecret(cred.Secret)
	if err != nil {
		return "", err
	}

	if s.RefreshToken == "" {
		return "", social.NewError(social.KindAuth, publish.NetworkTikTok, "needs_sign_in", 0, nil)
	}

	unlock := c.rot.Lock(cred.Connection.ID)
	defer unlock()

	presented := c.rot.Current(cred.Connection.ID, s.RefreshToken)

	tok, err := oauth.Request(ctx, cred.Client, c.cfg.TokenURL, url.Values{
		"client_key": {s.ClientID}, "client_secret": {s.ClientSecret},
		"grant_type": {"refresh_token"}, "refresh_token": {presented},
	})
	if err != nil {
		return "", tokenError(err)
	}

	if tok.RefreshToken != "" && tok.RefreshToken != presented {
		c.rot.Remember(cred.Connection.ID, s.RefreshToken, tok.RefreshToken)

		s.RefreshToken = tok.RefreshToken

		if kerr := keep(cred, &s); kerr != nil {
			return "", kerr
		}
	}

	return tok.AccessToken, nil
}

// keep writes the secret with the new refresh token over the one in the vault.
func keep(cred connections.Credential, s *secret) error {
	if cred.Rotate == nil {
		return nil
	}

	raw, err := s.marshal()
	if err != nil {
		return err
	}

	defer clear(raw)

	if rerr := cred.Rotate(raw); rerr != nil {
		return social.NewError(social.KindUnavailable, publish.NetworkTikTok, "vault_write_failed", 0, rerr)
	}

	return nil
}

type userInfo struct {
	Data struct {
		User struct {
			OpenID      string `json:"open_id"`
			DisplayName string `json:"display_name"`
		} `json:"user"`
	} `json:"data"`
}

// account reads who the access token belongs to: GET /v2/user/info/ with the scope user.info.basic.
func (c *Connector) account(ctx context.Context, cl connections.Doer, token string) (social.Account, error) {
	q := url.Values{"fields": {"open_id,display_name"}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIBase+"/v2/user/info/?"+q.Encode(), http.NoBody)
	if err != nil {
		return social.Account{}, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	res, err := cl.Do(req)
	if err != nil {
		return social.Account{}, transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode != http.StatusOK {
		return social.Account{}, classify(res.StatusCode, body)
	}

	var out userInfo
	if json.Unmarshal(body, &out) != nil || out.Data.User.OpenID == "" {
		return social.Account{}, social.NewError(
			social.KindAuth, publish.NetworkTikTok, ReasonScopeMissing, res.StatusCode, nil,
		)
	}

	title := out.Data.User.DisplayName
	if title == "" {
		title = out.Data.User.OpenID
	}

	return social.Account{ID: out.Data.User.OpenID, Title: title}, nil
}

// Exchange implements social.Connector: the authorization code for a refresh token, and the account it
// belongs to. The new secret is returned to the service, which puts it in the vault. The form is TikTok's: the
// client key, and the PKCE verifier a desktop app sends.
func (c *Connector) Exchange(
	ctx context.Context, cred connections.Credential, code, verifier, redirectURI string,
) ([]byte, error) {
	s, err := parseSecret(cred.Secret)
	if err != nil {
		return nil, err
	}

	tok, err := oauth.Request(ctx, cred.Client, c.cfg.TokenURL, url.Values{
		"client_key": {s.ClientID}, "client_secret": {s.ClientSecret}, "code": {code},
		"grant_type": {"authorization_code"}, "redirect_uri": {redirectURI}, "code_verifier": {verifier},
	})
	if err != nil {
		return nil, tokenError(err)
	}

	if tok.RefreshToken == "" {
		return nil, social.NewError(social.KindAuth, publish.NetworkTikTok, "no_refresh_token", http.StatusOK, nil)
	}

	acc, err := c.account(ctx, cred.Client, tok.AccessToken)
	if err != nil {
		return nil, err
	}

	s.RefreshToken, s.AccountID, s.AccountTitle = tok.RefreshToken, acc.ID, acc.Title

	return s.marshal()
}

// Account implements social.Connector: from the secret, with no request.
func (c *Connector) Account(_ context.Context, cred connections.Credential) (social.Account, error) {
	acc, ok := c.SignedIn(cred.Secret)
	if !ok {
		return social.Account{}, social.NewError(social.KindAuth, publish.NetworkTikTok, "needs_sign_in", 0, nil)
	}

	return acc, nil
}

// Test implements connections.Connector: it asks for an access token with the refresh token, which proves the
// sign-in works and posts nothing.
func (c *Connector) Test(ctx context.Context, cred connections.Credential) (connections.TestResult, error) {
	if _, err := c.accessToken(ctx, cred); err != nil {
		if gateError(err) {
			return connections.TestResult{}, err
		}

		if se, ok := social.AsError(err); ok {
			return connections.TestResult{Message: "the sign-in does not work: " + se.Kind + "."}, nil
		}

		return connections.TestResult{}, err
	}

	return connections.TestResult{
		OK: true, Status: http.StatusOK, Message: "the sign-in works: TikTok granted an access token.",
	}, nil
}
