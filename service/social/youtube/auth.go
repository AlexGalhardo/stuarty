package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
)

const maxBody = 1 << 20

// gateError says whether err is the refusal of the outbound gate, which the service maps to its own
// code and which a connector never rewrites.
func gateError(err error) bool {
	return errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
		errors.Is(err, outbound.ErrNotPublic)
}

// transportError turns a failure to reach the network into a connector error; the refusal of the gate
// passes through.
func transportError(err error) error {
	if gateError(err) {
		return err
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return social.NewError(social.KindUnavailable, publish.NetworkYouTube, "", 0, err)
}

// tokenError turns a refusal of the token endpoint into a connector error.
func tokenError(err error) error {
	te, ok := errors.AsType[*oauth.TokenError](err)
	if !ok {
		return transportError(err)
	}

	switch {
	case te.Status >= http.StatusInternalServerError:
		return social.NewError(social.KindUnavailable, publish.NetworkYouTube, "", te.Status, err)
	case te.Status == http.StatusTooManyRequests:
		return social.NewError(social.KindQuota, publish.NetworkYouTube, "rateLimitExceeded", te.Status, err)
	default:
		// invalid_grant: revoked, expired (a project in the Testing status), or replaced by a newer one.
		// invalid_client: the client id or the secret is wrong.
		reason := te.Code
		if reason == "" {
			reason = "refused"
		}

		return social.NewError(social.KindAuth, publish.NetworkYouTube, reason, te.Status, err)
	}
}

// accessToken asks for an access token with the refresh token of the connection. The token is held for
// the length of one operation and never stored.
func (c *Connector) accessToken(ctx context.Context, cred connections.Credential) (string, error) {
	s, err := parseSecret(cred.Secret)
	if err != nil {
		return "", err
	}

	if s.RefreshToken == "" {
		return "", social.NewError(social.KindAuth, publish.NetworkYouTube, "needs_sign_in", 0, nil)
	}

	tok, err := oauth.Refresh(ctx, cred.Client, c.cfg.TokenURL, s.ClientID, s.ClientSecret, s.RefreshToken)
	if err != nil {
		return "", tokenError(err)
	}

	return tok.AccessToken, nil
}

type channelList struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
	} `json:"items"`
}

// channel reads the channel the access token belongs to (channels.list with mine=true, 1 unit of quota).
func (c *Connector) channel(ctx context.Context, cl connections.Doer, token string) (social.Account, []byte, error) {
	q := url.Values{"part": {"id,snippet"}, "mine": {"true"}}

	res, err := c.get(ctx, cl, token, c.cfg.APIBase+"/youtube/v3/channels?"+q.Encode())
	if err != nil {
		return social.Account{}, nil, transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode != http.StatusOK {
		return social.Account{}, body, classify(res.StatusCode, body, res.Header)
	}

	var out channelList
	if json.Unmarshal(body, &out) != nil || len(out.Items) == 0 || out.Items[0].ID == "" {
		return social.Account{}, body, social.NewError(
			social.KindNotEligible,
			publish.NetworkYouTube,
			"noChannel",
			res.StatusCode,
			nil,
		)
	}

	return social.Account{ID: out.Items[0].ID, Title: out.Items[0].Snippet.Title}, body, nil
}

// Exchange implements social.Connector: the authorization code for a refresh token, and the channel it
// belongs to. The new secret is returned to the service, which puts it in the vault.
func (c *Connector) Exchange(
	ctx context.Context, cred connections.Credential, code, verifier, redirectURI string,
) ([]byte, error) {
	s, err := parseSecret(cred.Secret)
	if err != nil {
		return nil, err
	}

	tok, err := oauth.Exchange(
		ctx,
		cred.Client,
		c.cfg.TokenURL,
		s.ClientID,
		s.ClientSecret,
		code,
		verifier,
		redirectURI,
	)
	if err != nil {
		return nil, tokenError(err)
	}

	if tok.RefreshToken == "" {
		// Google gives a refresh token only the first time a person consents, unless the request said
		// prompt=consent; with it a missing one means the consent was cut short.
		return nil, social.NewError(social.KindAuth, publish.NetworkYouTube, "no_refresh_token", http.StatusOK, nil)
	}

	acc, _, err := c.channel(ctx, cred.Client, tok.AccessToken)
	if err != nil {
		return nil, err
	}

	s.RefreshToken, s.AccountID, s.AccountTitle = tok.RefreshToken, acc.ID, acc.Title

	return s.marshal()
}

// Account implements social.Connector: the channel the stored refresh token signs in as.
func (c *Connector) Account(ctx context.Context, cred connections.Credential) (social.Account, error) {
	token, err := c.accessToken(ctx, cred)
	if err != nil {
		return social.Account{}, err
	}

	acc, _, err := c.channel(ctx, cred.Client, token)

	return acc, err
}

// Test implements connections.Connector: one channels.list request, the harmless call that also proves
// the sign-in works. The answer is shown as untrusted text, and never holds the credential.
func (c *Connector) Test(ctx context.Context, cred connections.Credential) (connections.TestResult, error) {
	token, err := c.accessToken(ctx, cred)
	if err != nil {
		if gateError(err) {
			return connections.TestResult{}, err
		}

		if se, ok := social.AsError(err); ok {
			return connections.TestResult{
				Message: "the sign-in does not work: " + se.Kind + ".",
			}, nil
		}

		return connections.TestResult{}, err
	}

	_, body, err := c.channel(ctx, cred.Client, token)
	if err != nil && gateError(err) {
		return connections.TestResult{}, err
	}

	if err != nil {
		return connections.TestResult{ //nolint:nilerr // a refusal is the answer of the test
			Message: "the request was refused.",
		}, nil
	}

	return connections.TestResult{OK: true, Status: http.StatusOK, Message: connections.SafeMessage(cred, body)}, nil
}
