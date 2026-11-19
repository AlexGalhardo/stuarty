// Package youtube is the YouTube connector (SDD-020 section 7): the user's own Google project signs in
// with the generic flow of service/connections/oauth, and a video goes up through the resumable upload
// of the YouTube Data API. Everything it knows of YouTube is in this package; every request goes through
// the gated client of its connection.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The scopes the sign-in asks for (SDD-020 section 2): upload, and read to name the channel and the
// state of a video.
const (
	ScopeUpload   = "https://www.googleapis.com/auth/youtube.upload"
	ScopeReadonly = "https://www.googleapis.com/auth/youtube.readonly"
)

// Config holds the addresses the connector uses. The product uses Default; a test points them at a fake
// server, which then is the one host the connection of the test lists.
type Config struct {
	APIBase    string // https://www.googleapis.com
	UploadBase string // https://www.googleapis.com/upload
	AuthURL    string
	TokenURL   string
	WatchURL   string // the address a video is watched at, with the id appended
	// Polls and PollEvery bound how long Publish waits for YouTube to finish processing.
	Polls     int
	PollEvery time.Duration
	// Resumes is how many times an upload is resumed after a failed attempt before giving up.
	Resumes int
	// Wait pauses between attempts; nil is a real, context-aware sleep.
	Wait func(ctx context.Context, d time.Duration)
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Default is the production configuration.
func Default() Config {
	return Config{ //nolint:gosec // G101: public addresses of Google, no credential
		APIBase: "https://www.googleapis.com", UploadBase: "https://www.googleapis.com/upload",
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		WatchURL: "https://www.youtube.com/watch?v=",
		Polls:    5, PollEvery: 2 * time.Second, Resumes: 5,
	}
}

// Connector is the YouTube connector.
type Connector struct {
	cfg Config
}

var _ social.Connector = (*Connector)(nil)

// New returns the connector; a zero field of cfg takes its default.
func New(cfg Config) *Connector {
	d := Default()

	for _, f := range []struct {
		dst *string
		def string
	}{
		{&cfg.APIBase, d.APIBase},
		{&cfg.UploadBase, d.UploadBase},
		{&cfg.AuthURL, d.AuthURL},
		{&cfg.TokenURL, d.TokenURL},
		{&cfg.WatchURL, d.WatchURL},
	} {
		if *f.dst == "" {
			*f.dst = f.def
		}
	}

	if cfg.Polls == 0 {
		cfg.Polls = d.Polls
	}

	if cfg.PollEvery == 0 {
		cfg.PollEvery = d.PollEvery
	}

	if cfg.Resumes == 0 {
		cfg.Resumes = d.Resumes
	}

	if cfg.Wait == nil {
		cfg.Wait = func(ctx context.Context, d time.Duration) {
			t := time.NewTimer(d)
			defer t.Stop()

			select {
			case <-ctx.Done():
			case <-t.C:
			}
		}
	}

	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	return &Connector{cfg: cfg}
}

// Limits are the limits of a YouTube post, read on 2026-11-19 (SDD-020 section 2).
func Limits() publish.Limits {
	return publish.Limits{
		TitleMax: 100, DescriptionMaxBytes: 5000, TagsMaxChars: 500,
		FileMaxBytes: 256 << 30, ThumbnailMaxBytes: 50 << 20,
		Formats: []string{"mp4"}, ThumbnailFormats: []string{"png", "jpg", "jpeg"},
		Visibilities:         []publish.Visibility{publish.Private, publish.Unlisted, publish.Public},
		NeedsKidsDeclaration: true, Forbidden: "<>",
		Source: "https://developers.google.com/youtube/v3/docs/videos", Read: "2026-11-19",
	}
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

	for _, h := range []string{host(c.cfg.APIBase), host(c.cfg.UploadBase), host(c.cfg.TokenURL)} {
		if h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}

	return social.Description{
		Network: publish.NetworkYouTube, Name: "YouTube", Publishes: []string{"video", "thumbnail"},
		Limits: Limits(), Endpoint: c.cfg.APIBase, Hosts: hosts, Scopes: []string{ScopeUpload, ScopeReadonly},
		Guide: "https://developers.google.com/youtube/v3/guides/auth/server-side-web-apps", GuideRead: "2026-11-19",
		AIDisclosure: true, Hints: hints,
	}
}

// hints are what a person does about an error of YouTube, by the kind of the error (SDD-020 section 7.3).
var hints = map[string]string{
	social.KindQuota: "the limit of the day was used and it comes back at midnight Pacific Time; publish the same " +
		"post again then, and the upload resumes where it stopped",
	social.KindAuth: "sign in to the account again from the Connections page; if your Google project is in the " +
		"Testing status, its sign-in expires after 7 days",
	social.KindNotEligible: "check the channel in YouTube Studio: verification, restrictions and the privacy " +
		"setting; then publish again",
	social.KindInvalid: "fix the title, the description, the tags or the files named in the reason and publish again",
	social.KindRejected: "the network refused the video itself; read its reason in YouTube Studio before trying " +
		"again",
	social.KindUnavailable: "try again later; publishing the same post again resumes the upload",
}

// Validate implements social.Connector.
func (c *Connector) Validate(p publish.Post) []publish.Issue { return publish.Validate(p, Limits()) }

// AuthConfig implements social.Connector.
func (c *Connector) AuthConfig() social.AuthConfig {
	return social.AuthConfig{
		AuthURL: c.cfg.AuthURL, TokenURL: c.cfg.TokenURL, Scopes: []string{ScopeUpload, ScopeReadonly},
		// offline: a refresh token. consent: Google gives a refresh token again when the person signs in
		// a second time.
		Extra: url.Values{"access_type": {"offline"}, "prompt": {"consent"}, "include_granted_scopes": {"true"}},
	}
}

// secret is the secret of a YouTube connection, the one document in the vault (SDD-020 section 7.1).
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
			social.KindAuth,
			publish.NetworkYouTube,
			"bad_secret",
			0,
			errors.New("youtube: the secret of the connection is not a client"),
		)
	}

	return s, nil
}

// Begin implements social.Connector.
func (c *Connector) Begin(clientID, clientSecret string) ([]byte, error) {
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)

	if clientID == "" || clientSecret == "" {
		return nil, errors.New("youtube: the client id and the client secret are required")
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

func (c *Connector) get(ctx context.Context, cl connections.Doer, token, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	return cl.Do(req)
}
