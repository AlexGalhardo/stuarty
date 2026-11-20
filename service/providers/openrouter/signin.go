package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
)

// The addresses of the sign-in of OpenRouter (SDD-016 section 12.1).
const (
	// DefaultAuthURL is where the person is sent to consent.
	DefaultAuthURL = "https://openrouter.ai/auth"
	// DefaultKeysURL is where the code is traded for a key.
	DefaultKeysURL = "https://openrouter.ai/api/v1/auth/keys"
)

const (
	maxAnswer = 64 << 10
	// minKey and maxKey bound the key taken from an answer.
	minKey = 8
	maxKey = 4096
)

// SignInConfig points the sign-in at other addresses, for the tests and the fake of the end-to-end suite.
// The zero value is OpenRouter itself.
type SignInConfig struct {
	AuthURL, KeysURL, Endpoint string
}

// SignIn is the sign-in of OpenRouter: the PKCE flow that yields a key of the person, which is kept in the
// vault and used like a key that was pasted.
type SignIn struct{ cfg SignInConfig }

// NewSignIn returns the sign-in; an empty address is the one of OpenRouter.
func NewSignIn(cfg SignInConfig) *SignIn {
	if cfg.AuthURL == "" {
		cfg.AuthURL = DefaultAuthURL
	}

	if cfg.KeysURL == "" {
		cfg.KeysURL = DefaultKeysURL
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}

	return &SignIn{cfg: cfg}
}

// Describe implements oauth.SignIn.
func (s *SignIn) Describe() oauth.Description {
	hosts := []string{hostOf(s.cfg.Endpoint)}
	if h := hostOf(s.cfg.KeysURL); h != hosts[0] {
		hosts = append(hosts, h)
	}

	return oauth.Description{
		Provider: ID, Name: "OpenRouter", Label: "OpenRouter", Endpoint: s.cfg.Endpoint, Hosts: hosts,
		Accepts: []string{connections.AcceptText},
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return u.Hostname()
}

// ValidateRedirect implements oauth.SignIn: this server's callback page over https, or on a loopback
// host over http.
func (*SignIn) ValidateRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != oauth.CallbackPath {
		return errors.New("must be the address of this app followed by " + oauth.CallbackPath)
	}

	switch u.Scheme {
	case "https":
	case "http":
		h := strings.ToLower(u.Hostname())

		if ip, perr := netip.ParseAddr(h); h != "localhost" && (perr != nil || !ip.Unmap().IsLoopback()) {
			return errors.New("http is only for localhost; use https")
		}
	default:
		return errors.New("the scheme must be https")
	}

	return nil
}

// AuthorizeURL implements oauth.SignIn.
func (s *SignIn) AuthorizeURL(in oauth.BeginInput) (string, error) {
	q := url.Values{
		"callback_url": {in.RedirectURI}, "code_challenge": {in.Challenge}, "code_challenge_method": {"S256"},
		"state": {in.State},
	}

	return s.cfg.AuthURL + "?" + q.Encode(), nil
}

// validKey says whether s can be a key: printable ASCII with no space, of a sane length. An answer that is
// not one is not stored.
func validKey(s string) bool {
	if len(s) < minKey || len(s) > maxKey {
		return false
	}

	for i := range len(s) {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}

	return true
}

// Complete implements oauth.SignIn: POST the code and the verifier as JSON; the answer has the key.
func (s *SignIn) Complete(ctx context.Context, d oauth.Doer, in oauth.CompleteInput) (oauth.Envelope, error) {
	raw, err := json.Marshal(map[string]string{
		"code": in.Code, "code_verifier": in.Verifier, "code_challenge_method": "S256",
	})
	if err != nil {
		return oauth.Envelope{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.KeysURL, bytes.NewReader(raw))
	if err != nil {
		return oauth.Envelope{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := d.Do(req)
	if err != nil {
		return oauth.Envelope{}, err
	}

	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxAnswer))
	if err != nil {
		return oauth.Envelope{}, err
	}

	var out struct {
		Key string `json:"key"`
	}

	_ = json.Unmarshal(body, &out)

	if res.StatusCode != http.StatusOK || !validKey(out.Key) {
		return oauth.Envelope{}, &oauth.TokenError{Status: res.StatusCode}
	}

	// The key does not expire and is not renewed: the person revokes it on their account.
	return oauth.Envelope{Provider: ID, AccessToken: out.Key, Account: ""}, nil
}

// Refresh implements oauth.SignIn: a key of OpenRouter has no refresh token and never expires, so it is
// never asked to refresh.
func (*SignIn) Refresh(context.Context, oauth.Doer, oauth.Envelope) (oauth.Token, error) {
	return oauth.Token{}, &oauth.TokenError{Status: http.StatusBadRequest, Code: "invalid_grant"}
}
