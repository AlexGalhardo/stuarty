// Package meta is what the Instagram and the Facebook connectors share (SDD-020 part 2): the sign-in with the
// user's own Meta app, the account the person chooses, and the client of the Graph API. Everything it knows
// of Meta is here and in the two connector packages. Every request goes through the gated client of its
// connection, and a token travels only in a header, never in an address.
package meta

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Version is the version of the Graph API the pages showed on 2026-11-19 (SDD-020 section 15).
const Version = "v27.0"

// Config holds the addresses a Meta connector uses. The product uses Default; a test points them at a fake
// server, which then is the one host the connection of the test lists.
type Config struct {
	// GraphBase is the Graph host, for example https://graph.facebook.com.
	GraphBase string
	// UploadBase is the host of the resumable upload, https://rupload.facebook.com.
	UploadBase string
	// AuthURL is the consent screen the person opens.
	AuthURL string
	// Version is the Graph API version in the paths.
	Version string
	// Polls and PollEvery bound how long a publication waits for Meta to finish processing.
	Polls     int
	PollEvery time.Duration
	// Resumes is how many times an upload is sent again after a failed attempt before giving up.
	Resumes int
	// Wait pauses between attempts; nil is a real, context-aware sleep.
	Wait func(ctx context.Context, d time.Duration)
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Default is the production configuration. Meta advises one status query a minute for at most five minutes.
// The connector keeps the five-minute ceiling and asks every 10 seconds, so that a short Reel, which is
// ready in seconds, is not made to wait a minute.
func Default() Config {
	return Config{
		GraphBase: "https://graph.facebook.com", UploadBase: "https://rupload.facebook.com",
		AuthURL: "https://www.facebook.com/" + Version + "/dialog/oauth", Version: Version,
		Polls: 30, PollEvery: 10 * time.Second, Resumes: 3,
	}
}

// AtBase is the configuration of a test or of the end-to-end suite: every address is a path of one server on
// loopback, and the polling is quick.
func AtBase(base string) Config {
	base = strings.TrimRight(base, "/")

	return Config{
		GraphBase: base, UploadBase: base, AuthURL: base + "/dialog/oauth", Version: Version,
		Polls: 20, PollEvery: 200 * time.Millisecond, Resumes: 2,
	}
}

// WithDefaults fills every zero field with its default.
func (c Config) WithDefaults() Config {
	d := Default()

	for _, f := range []struct {
		dst *string
		def string
	}{
		{&c.GraphBase, d.GraphBase}, {&c.UploadBase, d.UploadBase}, {&c.AuthURL, d.AuthURL}, {&c.Version, d.Version},
	} {
		if *f.dst == "" {
			*f.dst = f.def
		}
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

// Hosts are the hosts a connection of a Meta network reaches: the Graph host and the upload host.
func (c Config) Hosts() []string {
	var hosts []string

	for _, raw := range []string{c.GraphBase, c.UploadBase} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}

		if h := strings.ToLower(u.Hostname()); !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}

	return hosts
}

// graph is the address of a Graph path: <base>/<version>/<path>.
func (c Config) graph(path string) string {
	return strings.TrimRight(c.GraphBase, "/") + "/" + c.Version + "/" + strings.TrimLeft(path, "/")
}
