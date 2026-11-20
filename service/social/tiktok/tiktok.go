// Package tiktok is the TikTok connector (SDD-020 part 4): the user's own TikTok developer app signs in with
// Login Kit, and a video goes to the person's inbox as a draft through the upload endpoint of the Content
// Posting API, to be finished and published in the TikTok app. The connector offers nothing a draft cannot
// have: no caption, no privacy level, no AI flag, no public post and never a pull from a public address.
// Everything it knows of TikTok is in this package; every request goes through the gated client of its
// connection.
package tiktok

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The scopes the sign-in asks for (SDD-020 section 32): the name of the account, and the upload of a draft.
const (
	ScopeUserInfo = "user.info.basic"
	ScopeUpload   = "video.upload"
)

// Scopes is the list the sign-in asks for.
func Scopes() []string { return []string{ScopeUserInfo, ScopeUpload} }

// Config holds the addresses the connector uses. The product uses Default; a test points them at a fake
// server, which then is the one host the connection of the test lists.
type Config struct {
	// APIBase is https://open.tiktokapis.com; UploadBase the host the upload address is on,
	// https://open-upload.tiktokapis.com. The connection reaches both and nothing else.
	APIBase    string
	UploadBase string
	// AuthURL is the consent screen the person opens; TokenURL the token endpoint.
	AuthURL  string
	TokenURL string
	// ChunkBytes is the size of a chunk of the upload: 5 MB to 64 MB, the last one up to 128 MB.
	ChunkBytes int64
	// Polls and PollEvery bound how long Publish waits for TikTok to put the draft in the inbox.
	Polls     int
	PollEvery time.Duration
	// Resumes is how many times a chunk is sent again after a transient failure before giving up.
	Resumes int
	// Wait pauses between attempts; nil is a real, context-aware sleep.
	Wait func(ctx context.Context, d time.Duration)
}

// Default is the production configuration.
func Default() Config {
	return Config{ //nolint:gosec // G101: public addresses of TikTok, no credential
		APIBase: "https://open.tiktokapis.com", UploadBase: "https://open-upload.tiktokapis.com",
		AuthURL: "https://www.tiktok.com/v2/auth/authorize/", TokenURL: "https://open.tiktokapis.com/v2/oauth/token/",
		ChunkBytes: 10_000_000, Polls: 15, PollEvery: 2 * time.Second, Resumes: 3,
	}
}

// AtBase is the configuration of a test or of the end-to-end suite: every address is a path of one server on
// loopback, and the polling is quick.
func AtBase(base string) Config {
	base = strings.TrimRight(base, "/")

	return Config{
		APIBase: base, UploadBase: base, AuthURL: base + "/tiktok/authorize", TokenURL: base + "/v2/oauth/token/",
		ChunkBytes: 10_000_000, Polls: 20, PollEvery: 100 * time.Millisecond, Resumes: 2,
	}
}

// WithDefaults fills every zero field with its default.
func (c Config) WithDefaults() Config {
	d := Default()

	for _, f := range []struct {
		dst *string
		def string
	}{
		{&c.APIBase, d.APIBase}, {&c.UploadBase, d.UploadBase}, {&c.AuthURL, d.AuthURL}, {&c.TokenURL, d.TokenURL},
	} {
		if *f.dst == "" {
			*f.dst = f.def
		}
	}

	if c.ChunkBytes == 0 {
		c.ChunkBytes = d.ChunkBytes
	}

	if c.Polls == 0 {
		c.Polls = d.Polls
	}

	if c.PollEvery == 0 {
		c.PollEvery = d.PollEvery
	}

	if c.Resumes == 0 {
		c.Resumes = d.Resumes
	}

	if c.Wait == nil {
		c.Wait = func(ctx context.Context, d time.Duration) {
			t := time.NewTimer(d)
			defer t.Stop()

			select {
			case <-ctx.Done():
			case <-t.C:
			}
		}
	}

	return c
}

// Connector is the TikTok connector.
type Connector struct {
	cfg Config
	rot *social.Rotation
}

var (
	_ social.Connector  = (*Connector)(nil)
	_ social.Authorizer = (*Connector)(nil)
)

// New returns the connector; a zero field of cfg takes its default.
func New(cfg Config) *Connector {
	return &Connector{cfg: cfg.WithDefaults(), rot: social.NewRotation()}
}

func host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return strings.ToLower(u.Hostname())
}

// Describe implements social.Connector.
func (c *Connector) Describe() social.Description {
	var hosts []string

	for _, h := range []string{host(c.cfg.APIBase), host(c.cfg.UploadBase)} {
		if h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}

	return social.Description{
		Network: publish.NetworkTikTok, Name: "TikTok", Publishes: []string{"video"},
		Limits: publish.TikTokLimits(), Draft: true, Endpoint: c.cfg.APIBase, Hosts: hosts, Scopes: Scopes(),
		Guide:     "https://developers.tiktok.com/doc/content-posting-api-get-started-upload-content",
		GuideRead: "2026-11-20", AIDisclosure: false, Hints: hints,
	}
}

// hints are what a person does about an error of TikTok, by the kind of the error (SDD-020 section 37).
var hints = map[string]string{
	social.KindQuota: "TikTok allows 5 drafts waiting in the inbox in 24 hours through its API, and a rate of " +
		"requests; finish or delete the drafts in the TikTok app, wait, then publish again",
	social.KindAuth: "sign in to TikTok again from the Connections page; check that your app has the " +
		"scope video.upload and that your account is a target user of the app",
	social.KindNotEligible: "TikTok does not let this account post through the API: check the account in the " +
		"TikTok app",
	social.KindInvalid:     "fix the video named in the reason (length, size, frame rate or format) and publish again",
	social.KindRejected:    "TikTok refused the video itself; read its reason in the TikTok app before trying again",
	social.KindUnavailable: "try again later; a file that already reached TikTok is not sent again",
}

// Validate implements social.Connector.
func (c *Connector) Validate(p publish.Post) []publish.Issue {
	return publish.Validate(p, publish.TikTokLimits())
}

// AuthConfig implements social.Connector. The consent address is built by AuthorizeURL, since TikTok's is not
// the generic one; the addresses and the scopes are here for the guide and the token exchange.
func (c *Connector) AuthConfig() social.AuthConfig {
	return social.AuthConfig{AuthURL: c.cfg.AuthURL, TokenURL: c.cfg.TokenURL, Scopes: Scopes()}
}

// AuthorizeURL implements social.Authorizer: the address of the desktop kit of Login Kit (SDD-020 section 34).
// The parameter is client_key, the scopes are separated by commas and the PKCE challenge is the hex encoding of
// the SHA-256 of the verifier. The challenge it is given is the same SHA-256 in base64url, which is turned
// into hex here, so the flow keeps no second verifier.
func (c *Connector) AuthorizeURL(clientKey, redirectURI, state, challenge string) string {
	sum, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil {
		sum = []byte(challenge)
	}

	q := url.Values{
		"client_key": {clientKey}, "response_type": {"code"}, "scope": {strings.Join(Scopes(), ",")},
		"redirect_uri": {redirectURI}, "state": {state},
		"code_challenge": {hex.EncodeToString(sum)}, "code_challenge_method": {"S256"},
	}

	sep := "?"
	if strings.Contains(c.cfg.AuthURL, "?") {
		sep = "&"
	}

	return c.cfg.AuthURL + sep + q.Encode()
}

// secret is the secret of a TikTok connection, the one document in the vault: ClientID is the client key of the
// app. AccountID is the open_id of the account.
type secret struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	AccountTitle string `json:"account_title,omitempty"`
}

func (s *secret) marshal() ([]byte, error) {
	return json.Marshal(s) //nolint:gosec // G117: this is the secret of a connection, made to go into the vault
}

func parseSecret(raw []byte) (secret, error) {
	var s secret
	if err := json.Unmarshal(raw, &s); err != nil || s.ClientID == "" || s.ClientSecret == "" {
		return secret{}, social.NewError(
			social.KindAuth, publish.NetworkTikTok, "bad_secret", 0,
			errors.New("tiktok: the secret of the connection is not a client"),
		)
	}

	return s, nil
}

// Begin implements social.Connector.
func (c *Connector) Begin(clientKey, clientSecret string) ([]byte, error) {
	clientKey, clientSecret = strings.TrimSpace(clientKey), strings.TrimSpace(clientSecret)

	if clientKey == "" || clientSecret == "" {
		return nil, errors.New("tiktok: the client key and the client secret are required")
	}

	s := secret{ClientID: clientKey, ClientSecret: clientSecret}

	return s.marshal()
}

// ClientID implements social.Connector.
func (c *Connector) ClientID(raw []byte) (string, bool) {
	s, err := parseSecret(raw)

	return s.ClientID, err == nil
}

// SignedIn implements social.Connector.
func (c *Connector) SignedIn(raw []byte) (social.Account, bool) {
	s, err := parseSecret(raw)
	if err != nil || s.RefreshToken == "" || s.AccountID == "" {
		return social.Account{}, false
	}

	return social.Account{ID: s.AccountID, Title: s.AccountTitle}, true
}
