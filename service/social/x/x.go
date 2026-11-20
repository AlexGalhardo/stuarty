// Package x is the X connector (SDD-020 part 4): the user's own X developer app signs in with OAuth 2.0 and
// PKCE, a video goes up through the chunked media upload, and the post is created with the text and the
// video. A post on X is paid, so the connector carries the price as data (core/publish.Price) and never
// repeats the one request that creates it. Everything it knows of X is in this package; every request goes
// through the gated client of its connection.
package x

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The scopes the sign-in asks for (SDD-020 section 32): the post and the account (tweet.read and users.read
// name it), the media upload, and a refresh token.
const (
	ScopeTweetRead  = "tweet.read"
	ScopeTweetWrite = "tweet.write"
	ScopeUsersRead  = "users.read"
	ScopeMediaWrite = "media.write"
	ScopeOffline    = "offline.access"
)

// Scopes is the list the sign-in asks for.
func Scopes() []string {
	return []string{ScopeTweetRead, ScopeTweetWrite, ScopeUsersRead, ScopeMediaWrite, ScopeOffline}
}

// Config holds the addresses the connector uses. The product uses Default; a test points them at a fake
// server, which then is the one host the connection of the test lists.
type Config struct {
	// APIBase is https://api.x.com.
	APIBase string
	// AuthURL is the consent screen the person opens; TokenURL the token endpoint.
	AuthURL  string
	TokenURL string
	// PostURL is the address a post is read at, with its id appended.
	PostURL string
	// SegmentBytes is the size of a segment of the upload; the page advises 5 MB and allows 8 MB.
	SegmentBytes int
	// Polls and PollEvery bound how long Publish waits for X to process the video.
	Polls     int
	PollEvery time.Duration
	// Resumes is how many times a segment is sent again after a transient failure before giving up.
	Resumes int
	// Wait pauses between attempts; nil is a real, context-aware sleep.
	Wait func(ctx context.Context, d time.Duration)
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Default is the production configuration.
func Default() Config {
	return Config{ //nolint:gosec // G101: public addresses of X, no credential
		APIBase: "https://api.x.com", AuthURL: "https://x.com/i/oauth2/authorize",
		TokenURL: "https://api.x.com/2/oauth2/token", PostURL: "https://x.com/i/status/",
		SegmentBytes: 4 << 20, Polls: 30, PollEvery: 2 * time.Second, Resumes: 3,
	}
}

// AtBase is the configuration of a test or of the end-to-end suite: every address is a path of one server on
// loopback, and the polling is quick.
func AtBase(base string) Config {
	base = strings.TrimRight(base, "/")

	return Config{
		APIBase: base, AuthURL: base + "/x/authorize", TokenURL: base + "/2/oauth2/token",
		PostURL: base + "/i/status/", SegmentBytes: 4 << 20, Polls: 20, PollEvery: 100 * time.Millisecond, Resumes: 2,
	}
}

// WithDefaults fills every zero field with its default.
func (c Config) WithDefaults() Config {
	d := Default()

	for _, f := range []struct {
		dst *string
		def string
	}{
		{&c.APIBase, d.APIBase}, {&c.AuthURL, d.AuthURL}, {&c.TokenURL, d.TokenURL}, {&c.PostURL, d.PostURL},
	} {
		if *f.dst == "" {
			*f.dst = f.def
		}
	}

	if c.SegmentBytes == 0 {
		c.SegmentBytes = d.SegmentBytes
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

	if c.Now == nil {
		c.Now = time.Now
	}

	return c
}

// Connector is the X connector.
type Connector struct {
	cfg Config
	rot *social.Rotation
}

var (
	_ social.Connector = (*Connector)(nil)
	_ social.Priced    = (*Connector)(nil)
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

	for _, h := range []string{host(c.cfg.APIBase), host(c.cfg.TokenURL)} {
		if h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}

	price := publish.XPrice()

	return social.Description{
		Network: publish.NetworkX, Name: "X", Publishes: []string{"video"},
		Limits: publish.XLimits(), Price: &price, Endpoint: c.cfg.APIBase, Hosts: hosts, Scopes: Scopes(),
		Guide: "https://docs.x.com/x-api/getting-started/pricing", GuideRead: "2026-11-20",
		AIDisclosure: true, Hints: hints,
	}
}

// hints are what a person does about an error of X, by the kind of the error (SDD-020 section 36).
var hints = map[string]string{
	social.KindQuota: "X refused for a limit: the rate of the app, or no credits left; wait, or add credits in the " +
		"X developer console, then publish again",
	social.KindAuth: "sign in to X again from the Connections page; check that the app has the permissions " +
		"tweet.write and media.write",
	social.KindNotEligible: "the app or the account may not post this: check the app's permissions and the " +
		"account in the X developer console",
	social.KindInvalid: "fix the text or the video named in the reason and publish again",
	social.KindRejected: "X refused the post, or its answer was lost and the post may exist: look at your X " +
		"timeline before publishing again, because a post costs money each time",
	social.KindUnavailable: "try again later; the video is not uploaded twice",
}

// Validate implements social.Connector.
func (c *Connector) Validate(p publish.Post) []publish.Issue {
	return publish.Validate(p, publish.XLimits())
}

// AuthConfig implements social.Connector: the generic flow of service/connections/oauth with the scopes X
// asks for (a space between them, as the generic builder does).
func (c *Connector) AuthConfig() social.AuthConfig {
	return social.AuthConfig{AuthURL: c.cfg.AuthURL, TokenURL: c.cfg.TokenURL, Scopes: Scopes()}
}

// secret is the secret of an X connection, the one document in the vault (SDD-020 section 34). Price is the
// price a person edited, kept here so that only this package reads it.
type secret struct {
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	AccountID    string         `json:"account_id,omitempty"`
	AccountTitle string         `json:"account_title,omitempty"`
	Price        *publish.Price `json:"price,omitempty"`
}

func (s *secret) marshal() ([]byte, error) {
	return json.Marshal(s) //nolint:gosec // G117: this is the secret of a connection, made to go into the vault
}

func parseSecret(raw []byte) (secret, error) {
	var s secret
	if err := json.Unmarshal(raw, &s); err != nil || s.ClientID == "" || s.ClientSecret == "" {
		return secret{}, social.NewError(
			social.KindAuth, publish.NetworkX, "bad_secret", 0,
			errors.New("x: the secret of the connection is not a client"),
		)
	}

	return s, nil
}

// Begin implements social.Connector.
func (c *Connector) Begin(clientID, clientSecret string) ([]byte, error) {
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)

	if clientID == "" || clientSecret == "" {
		return nil, errors.New("x: the client id and the client secret are required")
	}

	s := secret{ClientID: clientID, ClientSecret: clientSecret}

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
	if err != nil || s.RefreshToken == "" {
		return social.Account{}, false
	}

	return social.Account{ID: s.AccountID, Title: s.AccountTitle}, true
}

// Price implements social.Priced: the price a person edited, kept in the secret.
func (c *Connector) Price(raw []byte) (publish.Price, bool) {
	s, err := parseSecret(raw)
	if err != nil || s.Price == nil || s.Price.Valid() != nil {
		return publish.Price{}, false
	}

	return *s.Price, true
}

// SetPrice implements social.Priced: the amounts are kept with their currency, the page they were read from
// and the date a person edited them.
func (c *Connector) SetPrice(raw []byte, p publish.Price) ([]byte, error) {
	s, err := parseSecret(raw)
	if err != nil {
		return nil, err
	}

	if verr := p.Valid(); verr != nil {
		return nil, verr
	}

	base := publish.XPrice()
	p.Currency, p.Source, p.Read = base.Currency, base.Source, base.Read
	s.Price = &p

	return s.marshal()
}
