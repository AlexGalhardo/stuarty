// Package update is the auto-update client of the desktop application (ADR-0023, RF-213, SDD-009
// section 10.5). It is off by default and opens no connection unless the person asks or turns the
// setting on; it fetches one signed feed from one HTTPS host, downloads the artifact for this system,
// verifies its SHA-256 and its signature against the key embedded in the build, and only then hands
// the file to the Installer that the shell implements. It never executes an artifact itself and never
// touches the workspace.
package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

// Build-time configuration (SDD-009 section 10.5), set with -ldflags -X. The production feed host and
// key are decided by P3-REL-003; until then the values below are placeholders and New refuses them.
var (
	// FeedURL is the https URL of feed.json.
	FeedURL = placeholderFeedURL
	// PublicKeyHex is the Ed25519 public key (64 hex characters) that signs the feed and the artifacts.
	PublicKeyHex = ""
)

const (
	placeholderHost    = "updates.example.invalid"
	placeholderFeedURL = "https://" + placeholderHost + "/stuarty/feed.json"
)

// Errors of the client.
var (
	ErrDisabled           = errors.New("update: checking for updates is turned off")
	ErrNotConfigured      = errors.New("update: this build has no feed URL and signing key (P3-REL-003)")
	ErrIncompleteDownload = errors.New("update: the download is not the size the feed declared")
	ErrInstallFailed      = errors.New("update: the update could not be installed; the running version is unchanged")
)

// Artifact is a verified installer file.
type Artifact struct {
	Path    string
	Version string
}

// Installer is the step the shell implements: run the platform installer on a verified artifact and
// restart. It is called only for an artifact whose SHA-256 and signature verified. A returned error
// means the running version stays in use.
type Installer interface {
	Install(ctx context.Context, a Artifact) error
}

// Offer is a verified newer version, ready to be applied.
type Offer struct {
	Version  string
	Platform string
	// Notes is the optional plain text summary of the release, from the signed feed.
	Notes  string
	URL    string
	SHA256 string
	Size   int64

	signature []byte
	// signedPlatform is the platform name the artifact signature covers: the plain platform of an
	// installer, "launcher:<platform>" of a launcher artifact.
	signedPlatform string
}

// Config is the client configuration.
type Config struct {
	FeedURL        string
	PublicKey      ed25519.PublicKey
	CurrentVersion string
	// Platform defaults to GOOS-GOARCH.
	Platform string
	// StagingDir is a private directory outside the workspace; it is created with mode 0700.
	StagingDir string
	// AutoCheck is the setting update.check, off by default.
	AutoCheck bool
	Installer Installer
	// Progress, when set, is called with the bytes written and the declared size while the artifact
	// downloads; the client calls it from the downloading goroutine.
	Progress func(done, total int64)
	// CheckTimeout bounds the feed request (default 30 s); DownloadTimeout the artifact (default 30 min).
	CheckTimeout    time.Duration
	DownloadTimeout time.Duration
	// Now is the clock the feed expiry is checked against; default time.Now.
	Now func() time.Time
	// Test lets a test trust a local server; production code never sets it.
	Test *outbound.TestOptions
}

// FromBuild returns the configuration of this build: the feed URL and the key set at build time.
func FromBuild(current, stagingDir string, autoCheck bool, installer Installer) (Config, error) {
	key, err := hex.DecodeString(PublicKeyHex)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return Config{}, ErrNotConfigured
	}

	return Config{
		FeedURL: FeedURL, PublicKey: key, CurrentVersion: current, StagingDir: stagingDir,
		AutoCheck: autoCheck, Installer: installer,
	}, nil
}

// Client checks for and applies updates.
type Client struct {
	cfg      Config
	current  version
	feed     doer
	download doer
}

// New validates the configuration. It opens no connection.
func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.FeedURL)
	if err != nil || u.Hostname() == "" || u.Hostname() == placeholderHost ||
		len(cfg.PublicKey) != ed25519.PublicKeySize {
		return nil, ErrNotConfigured
	}

	if cfg.Installer == nil || cfg.StagingDir == "" {
		return nil, errors.New("update: an installer and a staging directory are required")
	}

	current, err := parseVersion(cfg.CurrentVersion)
	if err != nil {
		return nil, fmt.Errorf("update: the running version is not a release: %w", err)
	}

	if cfg.Platform == "" {
		cfg.Platform = runtime.GOOS + "-" + runtime.GOARCH
	}

	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	if cfg.CheckTimeout == 0 {
		cfg.CheckTimeout = 30 * time.Second
	}

	if cfg.DownloadTimeout == 0 {
		cfg.DownloadTimeout = 30 * time.Minute
	}

	feed, err := outbound.DesktopUpdate(cfg.FeedURL, cfg.CheckTimeout, cfg.Test)
	if err != nil {
		return nil, err
	}

	download, err := outbound.DesktopUpdate(cfg.FeedURL, cfg.DownloadTimeout, cfg.Test)
	if err != nil {
		return nil, err
	}

	return &Client{cfg: cfg, current: current, feed: feed, download: download}, nil
}

// CheckOnStartup is the check made when the application starts. With the setting off it returns
// ErrDisabled and makes no connection.
func (c *Client) CheckOnStartup(ctx context.Context) (*Offer, error) {
	if !c.cfg.AutoCheck {
		return nil, ErrDisabled
	}

	return c.CheckNow(ctx)
}

// CheckNow is the check the person asked for. It returns a verified offer of a strictly newer
// version, or ErrNotNewer.
func (c *Client) CheckNow(ctx context.Context) (*Offer, error) {
	raw, err := c.get(ctx, c.feed, c.cfg.FeedURL, maxFeedBytes)
	if err != nil {
		return nil, err
	}

	offer, err := parseFeed(raw, c.cfg.PublicKey, c.cfg.Platform, c.cfg.Now())
	if err != nil {
		return nil, err
	}

	v, _ := parseVersion(offer.Version)
	if !v.newerThan(c.current) {
		return nil, fmt.Errorf("%w: offered %s, running %s", ErrNotNewer, offer.Version, c.cfg.CurrentVersion)
	}

	return offer, nil
}

var fileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Apply downloads the artifact of the offer, verifies it and hands it to the installer. The installer
// is not reached unless the size, the SHA-256 and the signature all verified. A failed install is
// ErrInstallFailed with the reason; the running version stays in use.
func (c *Client) Apply(ctx context.Context, offer *Offer) error {
	name := path.Base(offerPath(offer.URL))
	if !fileName.MatchString(name) {
		return fmt.Errorf("%w: file name", ErrBadFeed)
	}

	if err := os.MkdirAll(c.cfg.StagingDir, 0o700); err != nil {
		return fmt.Errorf("update: staging directory: %w", err)
	}

	root, err := os.OpenRoot(c.cfg.StagingDir)
	if err != nil {
		return fmt.Errorf("update: staging directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	part := name + ".part"
	defer func() { _ = root.Remove(part) }()

	if err := c.fetchTo(ctx, root, part, offer); err != nil {
		return err
	}

	if err := verifyArtifact(c.cfg.PublicKey, offer); err != nil {
		return err
	}

	_ = root.Remove(name)

	if err := root.Rename(part, name); err != nil {
		return fmt.Errorf("update: staging: %w", err)
	}

	if err := root.Chmod(name, 0o500); err != nil {
		_ = root.Remove(name)

		return fmt.Errorf("update: staging: %w", err)
	}

	// The file stays after a successful install: the installer may still be running from it.
	art := Artifact{Path: filepath.Join(c.cfg.StagingDir, name), Version: offer.Version}
	if err := c.cfg.Installer.Install(ctx, art); err != nil {
		_ = root.Remove(name)

		return fmt.Errorf("%w: %w", ErrInstallFailed, err)
	}

	return nil
}

func offerPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return u.Path
}

// fetchTo writes the download to name inside root, hashing it on the way, and checks the size and the
// digest of the feed.
func (c *Client) fetchTo(ctx context.Context, root *os.Root, name string, offer *Offer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, offer.URL, http.NoBody)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", userAgent(c.cfg.CurrentVersion))

	res, err := c.download.Do(req)
	if err != nil {
		return fmt.Errorf("update: download: %w", err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("update: download: %s", res.Status)
	}

	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("update: staging: %w", err)
	}

	h := sha256.New()
	w := io.MultiWriter(f, h)

	if c.cfg.Progress != nil {
		w = io.MultiWriter(f, h, &progressWriter{total: offer.Size, report: c.cfg.Progress})
	}

	n, copyErr := io.Copy(w, io.LimitReader(res.Body, offer.Size+1))

	if err := f.Close(); err != nil && copyErr == nil {
		copyErr = err
	}

	if copyErr != nil {
		return fmt.Errorf("%w: %w", ErrIncompleteDownload, copyErr)
	}

	if n != offer.Size {
		return fmt.Errorf("%w: got %d bytes, want %d", ErrIncompleteDownload, n, offer.Size)
	}

	if hex.EncodeToString(h.Sum(nil)) != offer.SHA256 {
		return ErrDigestMismatch
	}

	return nil
}

// progressWriter reports how much of the artifact has been written, at most once per percent.
type progressWriter struct {
	total, done int64
	lastPercent int64
	report      func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))

	if percent := p.done * 100 / p.total; percent != p.lastPercent {
		p.lastPercent = percent
		p.report(min(p.done, p.total), p.total)
	}

	return len(b), nil
}

// verifyArtifact checks the signature over the digest the download matched.
func verifyArtifact(key ed25519.PublicKey, o *Offer) error {
	if !ed25519.Verify(key, ArtifactMessage(o.Version, o.signedPlatform, o.SHA256), o.signature) {
		return ErrBadArtifactSignature
	}

	return nil
}

func (c *Client) get(ctx context.Context, client doer, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent(c.cfg.CurrentVersion))
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: feed: %w", err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: feed: %s", res.Status)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("update: feed: %w", err)
	}

	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrBadFeed, limit)
	}

	return body, nil
}

func userAgent(current string) string { return "stuarty-desktop/" + current }

// doer is the part of an HTTP client the update code uses; the clients come from internal/outbound.
type doer interface {
	Do(req *http.Request) (*http.Response, error)
}
