package x

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

// basicClient adds the HTTP Basic header X asks of a confidential client to every request of the token
// endpoint; the client secret travels there and never in the form (SDD-020 section 34).
type basicClient struct {
	connections.Doer
	id, secret string
}

func (b basicClient) Do(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(url.QueryEscape(b.id), url.QueryEscape(b.secret))

	return b.Doer.Do(req)
}

// accessToken asks for an access token with the refresh token of the connection, under the lock of the
// connection. When X answers a new refresh token it is written into the vault at once, before the access token
// is used. The access token is held for the length of one operation and never stored.
func (c *Connector) accessToken(ctx context.Context, cred connections.Credential) (string, error) {
	s, err := parseSecret(cred.Secret)
	if err != nil {
		return "", err
	}

	if s.RefreshToken == "" {
		return "", social.NewError(social.KindAuth, publish.NetworkX, "needs_sign_in", 0, nil)
	}

	unlock := c.rot.Lock(cred.Connection.ID)
	defer unlock()

	presented := c.rot.Current(cred.Connection.ID, s.RefreshToken)

	tok, err := oauth.Request(ctx, basicClient{cred.Client, s.ClientID, s.ClientSecret}, c.cfg.TokenURL, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {presented}, "client_id": {s.ClientID},
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
		return social.NewError(social.KindUnavailable, publish.NetworkX, "vault_write_failed", 0, rerr)
	}

	return nil
}

type meAnswer struct {
	Data struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"data"`
}

// me reads who the access token belongs to: GET /2/users/me, a User: Read that costs $0.010 (SDD-020 section
// 34). It is called once, at the sign-in.
func (c *Connector) me(ctx context.Context, cl connections.Doer, token string) (social.Account, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIBase+"/2/users/me", http.NoBody)
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
		return social.Account{}, classify(res.StatusCode, body, res.Header, c.cfg.Now())
	}

	var out meAnswer
	if json.Unmarshal(body, &out) != nil || out.Data.ID == "" {
		return social.Account{}, social.NewError(
			social.KindNotEligible,
			publish.NetworkX,
			"no_account",
			res.StatusCode,
			nil,
		)
	}

	title := out.Data.Name
	if out.Data.Username != "" {
		title = "@" + out.Data.Username
	}

	return social.Account{ID: out.Data.ID, Title: title}, nil
}

// Exchange implements social.Connector: the authorization code for a refresh token, and the account it
// belongs to. The new secret is returned to the service, which puts it in the vault.
func (c *Connector) Exchange(
	ctx context.Context, cred connections.Credential, code, verifier, redirectURI string,
) ([]byte, error) {
	s, err := parseSecret(cred.Secret)
	if err != nil {
		return nil, err
	}

	tok, err := oauth.Request(ctx, basicClient{cred.Client, s.ClientID, s.ClientSecret}, c.cfg.TokenURL, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier},
		"client_id": {s.ClientID}, "redirect_uri": {redirectURI},
	})
	if err != nil {
		return nil, tokenError(err)
	}

	if tok.RefreshToken == "" {
		// X issues a refresh token only with the scope offline.access: a missing one means the person did not
		// grant it, or the app does not allow it.
		return nil, social.NewError(social.KindAuth, publish.NetworkX, "no_refresh_token", http.StatusOK, nil)
	}

	acc, err := c.me(ctx, cred.Client, tok.AccessToken)
	if err != nil {
		return nil, err
	}

	s.RefreshToken, s.AccountID, s.AccountTitle = tok.RefreshToken, acc.ID, acc.Title

	return s.marshal()
}

// Account implements social.Connector. It answers from the secret and makes no request: reading the account
// from X is a billed request, made once at the sign-in.
func (c *Connector) Account(_ context.Context, cred connections.Credential) (social.Account, error) {
	acc, ok := c.SignedIn(cred.Secret)
	if !ok {
		return social.Account{}, social.NewError(social.KindAuth, publish.NetworkX, "needs_sign_in", 0, nil)
	}

	return acc, nil
}

// Test implements connections.Connector: it asks for an access token with the refresh token, which proves
// the sign-in works and is not billed. No request that costs money is made.
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
		OK: true, Status: http.StatusOK,
		Message: "the sign-in works: X granted an access token. No request that costs money was made.",
	}, nil
}

// Status implements social.Connector. A post on X is published when the request that created it answers: it
// has no later state, and reading it back would be one more billed request, so none is made.
func (c *Connector) Status(
	_ context.Context, _ connections.Credential, _ string,
) (social.Remote, error) {
	return social.Remote{State: publish.StatePublished}, nil
}
