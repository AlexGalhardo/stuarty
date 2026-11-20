package destinations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The headers of a delivery and the limits of SDD-020 section 46.
const (
	HeaderDelivery  = "X-Stuarty-Delivery"
	HeaderTimestamp = "X-Stuarty-Timestamp"
	HeaderSignature = "X-Stuarty-Signature"
	// SignatureVersion starts the value of the signature header.
	SignatureVersion = "v1="
	// ReplayWindow is how far from its own clock a receiver accepts the timestamp of a delivery.
	ReplayWindow = 5 * time.Minute

	// PayloadVersion is the schema_version of the payload.
	PayloadVersion = 1

	maxRetryAfter  = 30 * time.Second
	maxHookAnswer  = 4 << 10
	minSigningLen  = 16
	maxSigningLen  = 256
	maxMediaBase   = 2048
	webhookRetries = 3
)

// retryDelays are the waits between the attempts of one send.
var retryDelays = [webhookRetries]time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// webhookConfig is the secret of a connection of the kind blog_webhook or blog_webhook_local.
type webhookConfig struct {
	SigningSecret string `json:"signing_secret"`
	MediaBaseURL  string `json:"media_base_url,omitempty"`
}

func parseWebhookConfig(secret []byte) (webhookConfig, error) {
	var c webhookConfig
	if err := strictJSON(secret, &c); err != nil {
		return c, err
	}

	if n := len(c.SigningSecret); n < minSigningLen || n > maxSigningLen ||
		strings.ContainsFunc(c.SigningSecret, unicode.IsControl) {
		return c, errors.New("the signing secret is 16 to 256 characters")
	}

	if c.MediaBaseURL != "" {
		u, err := url.Parse(c.MediaBaseURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" ||
			u.Fragment != "" || len(c.MediaBaseURL) > maxMediaBase || strings.ContainsFunc(c.MediaBaseURL, unicode.IsControl) {
			return c, errors.New("the media base address is an https URL with no user, query or fragment")
		}
	}

	return c, nil
}

// PublicHost refuses a host that is not on the public internet: localhost, and an IP address that is loopback,
// private, link-local, shared, unspecified or multicast. A name is judged again, at dial time, by the client of the
// connection (SDD-020 section 46, T-92).
func PublicHost(host string) error {
	host = strings.TrimSuffix(strings.ToLower(host), ".")

	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New(
			"the address is this machine: use a connection of the kind blog_webhook_local to deliver to it",
		)
	}

	if ip, err := netip.ParseAddr(host); err == nil && outbound.NotPublicAddr(ip) {
		return errors.New("the address is on this machine or a private network: use a connection of the kind " +
			"blog_webhook_local to deliver to it")
	}

	return nil
}

type webhook struct {
	cfg   Config
	kind  string
	local bool
}

func newWebhook(cfg Config, kind string) *webhook {
	return &webhook{cfg: cfg, kind: kind, local: kind == KindWebhookLocal}
}

func (w *webhook) Describe() Description {
	name := "Blog: webhook"
	if w.local {
		name = "Blog: webhook (this machine)"
	}

	return Description{
		Kind: w.kind, Name: name, Outbound: true,
		Hints: hints(map[string]string{
			social.KindRejected: "your receiver answered with an error status: look at its log, fix it, then send again",
		}),
	}
}

func (w *webhook) ValidateConfig(secret []byte, endpoint string) error {
	if _, err := parseWebhookConfig(secret); err != nil {
		return err
	}

	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return errors.New("the endpoint is the URL that receives the post")
	}

	if !w.local {
		return PublicHost(u.Hostname())
	}

	return nil
}

// Test sends nothing: a delivery would run the automation of the person. It checks the settings and the address
// rule, and says what will happen.
func (w *webhook) Test(_ context.Context, c connections.Credential) (connections.TestResult, error) {
	if _, ok := webhookSettings(c.Secret); !ok {
		return connections.TestResult{Message: "the settings of the connection are not valid."}, nil
	}

	if !w.addressOK(c.Connection.Endpoint) {
		return connections.TestResult{Message: "the address is not a public one: use a webhook of this machine."}, nil
	}

	return connections.TestResult{
		OK:      true,
		Message: "the settings are valid; a post is sent only after it is approved.",
	}, nil
}

// addressOK says whether the endpoint is a URL the kind may deliver to.
func (w *webhook) addressOK(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}

	return w.local || PublicHost(u.Hostname()) == nil
}

func webhookSettings(secret []byte) (webhookConfig, bool) {
	c, err := parseWebhookConfig(secret)

	return c, err == nil
}

// Sign is the signature of a delivery: HMAC-SHA256 of the timestamp, the delivery id and the body, joined by dots.
func Sign(secret, timestamp, delivery string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp + "." + delivery + "."))
	m.Write(body)

	return SignatureVersion + hex.EncodeToString(m.Sum(nil))
}

// Verify is what a receiver does: it checks the signature in constant time over the exact body, and refuses a
// timestamp further than ReplayWindow from now, so a captured delivery cannot be replayed later (T-93).
func Verify(secret, timestamp, delivery, signature string, body []byte, now time.Time) error {
	secs, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("the timestamp is not a Unix time")
	}

	if d := now.Sub(time.Unix(secs, 0)); d > ReplayWindow || d < -ReplayWindow {
		return errors.New("the timestamp is too far from now")
	}

	want := Sign(secret, timestamp, delivery, body)
	if subtle.ConstantTimeCompare([]byte(want), []byte(signature)) != 1 {
		return errors.New("the signature does not match")
	}

	return nil
}

type webhookPost struct {
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Tags         []string `json:"tags"`
	Language     string   `json:"language"`
	CanonicalURL string   `json:"canonical_url"`
	Cover        string   `json:"cover"`
	Markdown     string   `json:"markdown"`
	HTML         string   `json:"html"`
}

type webhookImage struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	SHA256      string `json:"sha256"`
	URL         string `json:"url,omitempty"`
}

type webhookPayload struct {
	SchemaVersion int            `json:"schema_version"`
	Event         string         `json:"event"`
	DeliveryID    string         `json:"delivery_id"`
	SentAt        string         `json:"sent_at"`
	Status        string         `json:"status"`
	Post          webhookPost    `json:"post"`
	Images        []webhookImage `json:"images"`
}

func (w *webhook) payload(in *SendInput, cfg *webhookConfig) ([]byte, error) {
	p := &in.Package

	event := "post.published"
	if in.Prev != nil {
		event = "post.updated"
	}

	body := webhookPayload{
		SchemaVersion: PayloadVersion, Event: event, DeliveryID: in.Delivery,
		SentAt: w.cfg.now().UTC().Format(time.RFC3339), Status: p.Status,
		Post: webhookPost{
			Slug: p.Slug, Title: p.Meta.Title, Summary: p.Meta.Summary, Tags: append([]string{}, p.Meta.Tags...),
			Language: p.Meta.Language, CanonicalURL: p.Meta.Canonical, Markdown: p.Markdown, HTML: p.HTML,
		},
		Images: []webhookImage{},
	}

	for i := range p.Images {
		im := &p.Images[i]
		wi := webhookImage{
			Name:        im.Name,
			Role:        im.Role,
			ContentType: im.ContentType,
			Bytes:       im.Bytes,
			SHA256:      im.SHA256,
		}

		if cfg.MediaBaseURL != "" {
			wi.URL = strings.TrimRight(cfg.MediaBaseURL, "/") + "/api/v1/posts/" + p.Slug + "/media/" + im.Name
		}

		if im.Role == "cover" {
			body.Post.Cover = "images/" + im.Name
		}

		body.Images = append(body.Images, wi)
	}

	return json.Marshal(body)
}

// Send delivers the post once, retrying a transient failure (SDD-020 section 46).
func (w *webhook) Send(ctx context.Context, in *SendInput) (Result, error) {
	cfg, err := parseWebhookConfig(in.Cred.Secret)
	if err != nil {
		return Result{}, fail(social.KindInvalid, w.kind, "bad_settings", 0, err)
	}

	if cerr := in.Package.Check(); cerr != nil {
		return Result{}, fail(social.KindInvalid, w.kind, "bad_package", 0, cerr)
	}

	if in.Delivery == "" {
		return Result{}, fail(social.KindInvalid, w.kind, "bad_package", 0, errors.New("a delivery has no id"))
	}

	body, err := w.payload(in, &cfg)
	if err != nil {
		return Result{}, fail(social.KindInvalid, w.kind, "bad_package", 0, err)
	}

	for attempt := 0; ; attempt++ {
		wait, err := w.once(ctx, in, &cfg, body)
		if err == nil {
			return Result{RemoteID: in.Delivery, State: in.Package.Status}, nil
		}

		se, isSocial := social.AsError(err)
		if !isSocial || se.Kind != social.KindUnavailable || attempt >= webhookRetries {
			return Result{}, err
		}

		if wait <= 0 {
			wait = retryDelays[attempt]
		}

		if serr := w.cfg.wait(ctx, wait); serr != nil {
			return Result{}, serr
		}
	}
}

// once makes one attempt. The wait it returns is the Retry-After of the receiver, capped.
func (w *webhook) once(ctx context.Context, in *SendInput, cfg *webhookConfig, body []byte) (time.Duration, error) {
	ts := strconv.FormatInt(w.cfg.now().Unix(), 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, in.Cred.Connection.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fail(social.KindInvalid, w.kind, "bad_endpoint", 0, errors.New("the endpoint is not a URL"))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Stuarty")
	req.Header.Set(HeaderDelivery, in.Delivery)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, Sign(cfg.SigningSecret, ts, in.Delivery, body))

	res, err := in.Cred.Client.Do(req)
	if err != nil {
		if errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
			errors.Is(err, outbound.ErrNotPublic) || errors.Is(err, context.Canceled) {
			return 0, err
		}

		return 0, fail(social.KindUnavailable, w.kind, "network", 0, errors.New("the receiver could not be reached"))
	}

	defer func() { _ = res.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxHookAnswer))

	code := res.StatusCode
	cause := errors.New("the receiver answered " + strconv.Itoa(code))

	switch {
	case code >= 200 && code <= 299:
		return 0, nil
	case code >= 300 && code <= 399:
		return 0, fail(social.KindRejected, w.kind, "redirect", code,
			errors.New("the receiver redirected the request; no redirect is followed: use its final address"))
	case code == http.StatusRequestTimeout || code == http.StatusTooEarly || code == http.StatusTooManyRequests || code >= 500:
		return retryAfter(
				res.Header.Get("Retry-After"),
			), fail(
				social.KindUnavailable,
				w.kind,
				"http_"+strconv.Itoa(code),
				code,
				cause,
			)
	}

	return 0, fail(social.KindRejected, w.kind, "http_"+strconv.Itoa(code), code, cause)
}

// retryAfter reads a Retry-After in seconds, capped; anything else is no wait of its own.
func retryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}

	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}
