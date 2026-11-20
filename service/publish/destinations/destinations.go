// Package destinations sends a blog post to where a creator's blog lives: files for a static site, WordPress, a
// webhook of their own automation, or the read API of Stuarty itself (SDD-020 part 5). A destination is one more kind
// of connection and one more kind of target of the publication of SDD-020 part 3, so the calendar, the approval, the
// lease and the retries are the ones of any other target. The rules of a post are in core/writing; this package only
// carries a package that a person approved to a destination and reaches the network only through the credential and
// the gated client of one connection.
package destinations

import (
	"context"
	"errors"
	"io"
	"maps"
	"regexp"
	"time"

	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The kinds of connection that are blog destinations (SDD-020 section 42).
const (
	KindFiles        = "blog_files"
	KindWordPress    = "blog_wordpress"
	KindWebhook      = "blog_webhook"
	KindWebhookLocal = "blog_webhook_local"
	KindAPI          = "blog_api"
)

// The statuses of a post at its destination.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Limits of a package (SDD-020 section 43).
const (
	MaxImages     = writing.MaxPostImages
	MaxImageBytes = 10 << 20
	MaxTotalBytes = 50 << 20
)

// Image is one image of a post, as the person approved it: where it goes in the package, which asset it is and the
// digest the send checks again.
type Image struct {
	// Name is the file name in the folder images/ of the package.
	Name string `json:"name"`
	// Role is cover or body.
	Role    string `json:"role"`
	AssetID string `json:"asset_id"`
	// Hash is the hash of the asset in the content-addressed store; the service opens the file with it.
	Hash        string `json:"hash"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	SHA256      string `json:"sha256"`
}

// Package is a post as it is approved and sent: the metadata, the Markdown and the HTML with the images written as
// images/<name>, and the images.
type Package struct {
	Slug string `json:"slug"`
	// Status is draft or published.
	Status   string           `json:"status"`
	Meta     writing.PostMeta `json:"meta"`
	Markdown string           `json:"markdown"`
	HTML     string           `json:"html"`
	Images   []Image          `json:"images"`
}

// Cover is the cover image, or nil.
func (p *Package) Cover() *Image {
	for i := range p.Images {
		if p.Images[i].Role == "cover" {
			return &p.Images[i]
		}
	}

	return nil
}

// TotalBytes is the size of every image.
func (p *Package) TotalBytes() int64 {
	var n int64

	for i := range p.Images {
		n += p.Images[i].Bytes
	}

	return n
}

var (
	imageName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,60}\.(png|jpg|gif|webp|avif)$`)
	slugRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// Check refuses a package that could write or name something it should not: a slug or an image name that is not a
// plain one, more images or bytes than the limits. It is the defence in depth behind the validation of the post.
func (p *Package) Check() error {
	if p.Slug == "" || len(p.Slug) > writing.MaxPostSlug || !slugRe.MatchString(p.Slug) {
		return errors.New("the slug of the package is not a plain slug")
	}

	if p.Status != StatusDraft && p.Status != StatusPublished {
		return errors.New("the status of the package is not draft or published")
	}

	if len(p.Images) > MaxImages || p.TotalBytes() > MaxTotalBytes {
		return errors.New("the package has more images or bytes than a post may have")
	}

	seen := map[string]bool{}

	for i := range p.Images {
		im := &p.Images[i]
		if !imageName.MatchString(im.Name) || seen[im.Name] || im.Bytes < 0 || im.Bytes > MaxImageBytes {
			return errors.New("an image of the package has a name or a size that is not allowed")
		}

		seen[im.Name] = true
	}

	return nil
}

// Remote is what an earlier send of a post to a connection left, so that a second send updates the post.
type Remote struct {
	ConnectionID string
	PostItemID   string
	RemoteID     string
	URL          string
	UpdatedAt    time.Time
}

// Result is what a send produced.
type Result struct {
	RemoteID string
	URL      string
	// State is draft or published.
	State string
}

// Description says what a destination is.
type Description struct {
	// Kind is the kind of the connection and the network name of the targets.
	Kind string `json:"kind"`
	// Name is the name shown to a person.
	Name string `json:"name"`
	// Outbound is true when a send opens a connection to a host.
	Outbound bool `json:"outbound"`
	// Hints say what a person does about an error, by the kind of the error.
	Hints map[string]string `json:"hints,omitempty"`
}

// SendInput is what a destination is given for one send.
type SendInput struct {
	// Cred is the credential and the gated client of the connection; the settings are in its secret.
	Cred    connections.Credential
	Package Package
	// Open opens the file of an image of the package, already checked against its size and digest.
	Open func(Image) (io.ReadCloser, error)
	// Prev is what an earlier send left, or nil.
	Prev *Remote
	// Delivery identifies the send: the id of the target, the same in every retry.
	Delivery string
	// Publish stores the package in the workspace for the read API (the destination blog_api).
	Publish func(context.Context, Package) error
	Report  func(progress float64, message string)
}

// Destination is one kind of connection that takes a post.
type Destination interface {
	// Describe says what the destination is.
	Describe() Description
	// ValidateConfig checks the settings of a connection of the kind (the secret) and its endpoint, before it is
	// created. Nothing is sent.
	ValidateConfig(secret []byte, endpoint string) error
	// Send carries the package to the destination. It returns a *social.Error for what a person can act on, and the
	// errors of the gate as they are.
	Send(ctx context.Context, in *SendInput) (Result, error)
	// Test is the harmless request of the Connections page.
	connections.Connector
}

// EndpointError is a refusal of the endpoint of a connection, as opposed to its settings.
type EndpointError struct{ Message string }

func (e *EndpointError) Error() string { return e.Message }

// Config is what the destinations take from the build: the tests set the clock and the waits.
type Config struct {
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}

	return time.Now()
}

// wait sleeps for d, or until the context ends.
func (c Config) wait(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Registry maps a kind of connection to its destination.
type Registry struct {
	byKind map[string]Destination
	order  []string
}

// NewRegistry has the five destinations of the build.
func NewRegistry(cfg Config) *Registry {
	r := &Registry{byKind: map[string]Destination{}}

	for _, d := range []Destination{
		newFiles(cfg), newWordPress(cfg), newWebhook(cfg, KindWebhook), newWebhook(cfg, KindWebhookLocal), newAPI(cfg),
	} {
		k := d.Describe().Kind
		r.byKind[k] = d
		r.order = append(r.order, k)
	}

	return r
}

// Get returns the destination of a kind.
func (r *Registry) Get(kind string) (Destination, bool) {
	if r == nil {
		return nil, false
	}

	d, ok := r.byKind[kind]

	return d, ok
}

// Kinds lists the kinds, in the order of the specification.
func (r *Registry) Kinds() []string { return append([]string(nil), r.order...) }

// Bind registers every kind with the manager of the connections, so that a connection of a kind can be created,
// listed and tested like any other.
func (r *Registry) Bind(m *connections.Manager) {
	for _, k := range r.order {
		m.Register(k, r.byKind[k])
	}
}

// IsKind says whether a connection kind is a blog destination.
func IsKind(kind string) bool {
	switch kind {
	case KindFiles, KindWordPress, KindWebhook, KindWebhookLocal, KindAPI:
		return true
	}

	return false
}

// fail is a *social.Error of a destination: the kind, a stable reason code and the cause.
func fail(kind, dest, reason string, status int, cause error) error {
	return social.NewError(kind, dest, reason, status, cause)
}

// The hints every destination shares; a destination adds its own.
func hints(extra map[string]string) map[string]string {
	h := map[string]string{
		social.KindUnavailable: "the destination could not be reached or failed; the send is tried again by itself, or retry it",
		social.KindInvalid:     "fix the post, the connection settings or the folder named in the reason, then send again",
		social.KindAuth:        "the destination refused the credential; check it in the Connections page",
		social.KindRejected:    "the destination refused the post; read its side of the story, then send again",
	}

	maps.Copy(h, extra)

	return h
}
