// Package fake is a connector for tests (SDD-020 section 5). It talks to a server the test runs, through
// the gated client of its connection, exactly as a real connector does, so a test can count what leaves
// the machine, and when. It needs only this package and one Register line to be a network.
package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// Network is the default name of the fake network.
const Network = "fake"

// Secret is the secret of a connection of the fake network.
type Secret struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	AccountTitle string `json:"account_title,omitempty"`
}

// Connector is the fake network. Base is the address of the test server.
type Connector struct {
	Base string
	// Name is the network name; empty is Network.
	Name string
	// Fail, when set, is returned by Publish after it made its request.
	Fail error

	mu       sync.Mutex
	uploaded []string
}

// New returns a fake connector for the server at base.
func New(base string) *Connector { return &Connector{Base: base} }

// Uploaded lists the titles Publish sent.
func (c *Connector) Uploaded() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string(nil), c.uploaded...)
}

func (c *Connector) name() string {
	if c.Name != "" {
		return c.Name
	}

	return Network
}

// Describe implements social.Connector.
func (c *Connector) Describe() social.Description {
	u, _ := url.Parse(c.Base)

	return social.Description{
		Network: c.name(), Name: "Fake network", Publishes: []string{"video", "thumbnail"},
		Limits: publish.Limits{
			TitleMax:             100,
			DescriptionMaxBytes:  5000,
			TagsMaxChars:         500,
			FileMaxBytes:         1 << 30,
			ThumbnailMaxBytes:    10 << 20,
			Formats:              []string{"mp4"},
			ThumbnailFormats:     []string{"png", "jpg"},
			Visibilities:         []publish.Visibility{publish.Private, publish.Unlisted, publish.Public},
			NeedsKidsDeclaration: true,
			Forbidden:            "<>",
			Source:               "the test server",
			Read:                 "2026-11-19",
		},
		Endpoint: c.Base, Hosts: []string{u.Hostname()}, Scopes: []string{"fake.upload"},
		Guide: "https://example.com/fake", GuideRead: "2026-11-19",
	}
}

// Validate implements social.Connector.
func (c *Connector) Validate(p publish.Post) []publish.Issue {
	return publish.Validate(p, c.Describe().Limits)
}

// AuthConfig implements social.Connector.
func (c *Connector) AuthConfig() social.AuthConfig {
	return social.AuthConfig{
		AuthURL: c.Base + "/authorize", TokenURL: c.Base + "/token", Scopes: []string{"fake.upload"},
		Extra: url.Values{"access_type": {"offline"}},
	}
}

// Begin implements social.Connector.
func (c *Connector) Begin(clientID, clientSecret string) ([]byte, error) {
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("fake: the client id and secret are required")
	}

	return json.Marshal( //nolint:gosec // G117: this is the secret of a connection, made to go into the vault
		Secret{ClientID: clientID, ClientSecret: clientSecret})
}

// ClientID implements social.Connector.
func (c *Connector) ClientID(secret []byte) (string, bool) {
	var s Secret
	err := json.Unmarshal(secret, &s)

	return s.ClientID, err == nil && s.ClientID != ""
}

// SignedIn implements social.Connector.
func (c *Connector) SignedIn(secret []byte) (social.Account, bool) {
	var s Secret
	if json.Unmarshal(secret, &s) != nil || s.RefreshToken == "" {
		return social.Account{}, false
	}

	return social.Account{ID: s.AccountID, Title: s.AccountTitle}, true
}

func (c *Connector) post(
	ctx context.Context,
	cred connections.Credential,
	path string,
	body any,
	out any,
) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+path, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := cred.Client.Do(req)
	if err != nil {
		return 0, err
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))

	if out != nil && res.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			return res.StatusCode, err
		}
	}

	return res.StatusCode, nil
}

// Exchange implements social.Connector: the generic flow against the token address of the fake server,
// then the account the token belongs to.
func (c *Connector) Exchange(
	ctx context.Context, cred connections.Credential, code, verifier, redirectURI string,
) ([]byte, error) {
	var s Secret
	if err := json.Unmarshal(cred.Secret, &s); err != nil {
		return nil, err
	}

	tok, err := oauth.Exchange(
		ctx,
		cred.Client,
		c.Base+"/token",
		s.ClientID,
		s.ClientSecret,
		code,
		verifier,
		redirectURI,
	)
	if err != nil {
		return nil, social.NewError(social.KindAuth, c.name(), "invalid_grant", 0, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/account", http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	res, err := cred.Client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	var acc struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}

	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&acc); err != nil {
		return nil, err
	}

	s.RefreshToken, s.AccountID, s.AccountTitle = tok.RefreshToken, acc.ID, acc.Title

	return json.Marshal(s) //nolint:gosec // G117: the secret of a connection, made to go into the vault
}

// Account implements social.Connector.
func (c *Connector) Account(_ context.Context, cred connections.Credential) (social.Account, error) {
	acc, ok := c.SignedIn(cred.Secret)
	if !ok {
		return social.Account{}, social.NewError(social.KindAuth, c.name(), "needs_sign_in", 0, nil)
	}

	return acc, nil
}

// Publish implements social.Connector: one request to the fake server with the post.
func (c *Connector) Publish(
	ctx context.Context,
	cred connections.Credential,
	u social.Upload,
) (social.Published, error) {
	if _, ok := c.SignedIn(cred.Secret); !ok {
		return social.Published{}, social.NewError(social.KindAuth, c.name(), "needs_sign_in", 0, nil)
	}

	var out struct {
		ID    string `json:"id"`
		URL   string `json:"url"`
		State string `json:"state"`
	}

	status, err := c.post(ctx, cred, "/videos", map[string]any{
		"title": u.Post.Title, "visibility": u.Post.EffectiveVisibility(), "path": u.Path,
	}, &out)
	if err != nil {
		return social.Published{}, err
	}

	c.mu.Lock()
	c.uploaded = append(c.uploaded, u.Post.Title)
	c.mu.Unlock()

	if c.Fail != nil {
		return social.Published{}, c.Fail
	}

	if status != http.StatusOK {
		return social.Published{}, social.NewError(social.KindUnavailable, c.name(), "", status, nil)
	}

	state := publish.StatePublished
	if out.State == string(publish.StateProcessing) {
		state = publish.StateProcessing
	}

	return social.Published{RemoteID: out.ID, URL: out.URL, State: state}, nil
}

// Status implements social.Connector.
func (c *Connector) Status(ctx context.Context, cred connections.Credential, remoteID string) (social.Remote, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/videos/"+url.PathEscape(remoteID), http.NoBody)
	if err != nil {
		return social.Remote{}, err
	}

	res, err := cred.Client.Do(req)
	if err != nil {
		return social.Remote{}, err
	}

	defer func() { _ = res.Body.Close() }()

	var out struct {
		State string `json:"state"`
	}

	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&out); err != nil {
		return social.Remote{}, err
	}

	return social.Remote{State: publish.State(out.State)}, nil
}

// Test implements connections.Connector.
func (c *Connector) Test(ctx context.Context, cred connections.Credential) (connections.TestResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/ping", http.NoBody)
	if err != nil {
		return connections.TestResult{}, err
	}

	res, err := cred.Client.Do(req)
	if err != nil {
		return connections.TestResult{}, err
	}

	defer func() { _ = res.Body.Close() }()

	return connections.TestResult{
		OK: res.StatusCode == http.StatusOK, Status: res.StatusCode,
		Message: fmt.Sprintf("the fake network answered %d.", res.StatusCode),
	}, nil
}
