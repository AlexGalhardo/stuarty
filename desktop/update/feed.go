package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Domain separation labels of the two signed messages (SDD-009 section 10.5).
const (
	feedLabel     = "vukem-video-update-feed-v1\n"
	artifactLabel = "vukem-video-update-artifact-v1\n"
)

const (
	feedSchema       = 1
	maxFeedBytes     = 1 << 20
	maxArtifactBytes = 1 << 30

	// DefaultFeedValidity is how long the feed tool signs a feed for, and MaxFeedValidity the longest
	// window a client accepts: a feed that claims more is refused as mis-signed (SDD-009 section 10.5).
	DefaultFeedValidity = 180 * 24 * time.Hour
	// MaxFeedValidity is documented with DefaultFeedValidity above.
	MaxFeedValidity = 400 * 24 * time.Hour

	// feedClockSkew is how far in the future published_at may be before the feed is refused.
	feedClockSkew = 10 * time.Minute
	maxNotesBytes = 4096
)

// Errors of the feed and of the artifact checks.
var (
	ErrBadFeed              = errors.New("update: the feed is not valid")
	ErrBadFeedSignature     = errors.New("update: the signature of the feed does not verify")
	ErrStaleFeed            = errors.New("update: the feed has expired; the update server has not been refreshed")
	ErrNoPlatform           = errors.New("update: the feed has no build for this system")
	ErrNotNewer             = errors.New("update: the offered version is not newer than the running one")
	ErrDigestMismatch       = errors.New("update: the SHA-256 of the download is not the one of the feed")
	ErrBadArtifactSignature = errors.New("update: the signature of the download does not verify")
)

// envelope is the feed as served: the payload bytes and the signature over them.
type envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type payload struct {
	Schema  int    `json:"schema"`
	Version string `json:"version"`
	// PublishedAt and Expires are RFC 3339 UTC instants inside the signed payload. They are what makes a
	// replayed or frozen feed detectable; the schema stays 1 so a client without them still reads the
	// feed and ignores both.
	PublishedAt string           `json:"published_at"`
	Expires     string           `json:"expires"`
	Platforms   map[string]entry `json:"platforms"`
	Notes       string           `json:"notes"`
	// Launchers are the artifacts the desktop launcher replaces itself with (SDD-009 section 10.5), by
	// platform; absent in an older feed.
	Launchers map[string]entry `json:"launchers"`
}

type entry struct {
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

// FeedMessage is the exact byte string the feed signature covers.
func FeedMessage(payloadBytes []byte) []byte {
	return append([]byte(feedLabel), payloadBytes...)
}

// ArtifactMessage is the exact byte string the artifact signature covers. It binds the digest to the
// version and the platform, so an older or foreign artifact cannot be offered as the new one.
func ArtifactMessage(version, platform, sha256Hex string) []byte {
	return []byte(artifactLabel + version + "\n" + platform + "\n" + sha256Hex)
}

// parseFeed verifies the envelope against key and only then reads the payload for platform.
func parseFeed(raw []byte, key ed25519.PublicKey, platform string, now time.Time) (*Offer, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadFeed, err)
	}

	body, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: payload is not base64", ErrBadFeed)
	}

	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(key, FeedMessage(body), sig) {
		return nil, ErrBadFeedSignature
	}

	var p payload

	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadFeed, err)
	}

	if p.Schema != feedSchema {
		return nil, fmt.Errorf("%w: schema %d", ErrBadFeed, p.Schema)
	}

	if _, err := parseVersion(p.Version); err != nil {
		return nil, fmt.Errorf("%w: version %q", ErrBadFeed, p.Version)
	}

	if err := checkFreshness(p.PublishedAt, p.Expires, now); err != nil {
		return nil, err
	}

	e, signed, ok := pickEntry(&p, platform)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoPlatform, platform)
	}

	offer, err := offerOf(p.Version, platform, e)
	if err != nil {
		return nil, err
	}

	offer.signedPlatform = signed

	offer.Notes = cutNotes(p.Notes)

	return offer, nil
}

// launcherLabel prefixes the platform in the signed message of a launcher artifact, so the signature of an
// installer never verifies as a launcher artifact and the other way round.
const launcherLabel = "launcher:"

// pickEntry chooses what the client downloads and the platform name its signature covers. The launcher
// of macOS and Linux (an .app.zip, an AppImage) is preferred, because the installers of those systems
// (.dmg, .deb) cannot be applied by the application. Windows and a feed without launchers use the
// installer of platforms, signed with the plain platform.
func pickEntry(p *payload, platform string) (entry, string, bool) {
	if !strings.HasPrefix(platform, "windows-") {
		if e, ok := p.Launchers[platform]; ok {
			return e, launcherLabel + platform, true
		}
	}

	e, ok := p.Platforms[platform]

	return e, platform, ok
}

// cutNotes keeps the optional release notes within maxNotesBytes, cut on a character boundary.
func cutNotes(s string) string {
	if len(s) <= maxNotesBytes {
		return s
	}

	s = s[:maxNotesBytes]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}

	return s
}

// checkFreshness enforces the signed published_at and expires. A feed without them is refused: the
// client that reads them ships in the same release as the tool that writes them, and the feed holds
// only the latest release, so no feed such a client meets lacks them, while an old signed feed
// replayed by an attacker does. Clients already installed ignore both fields and keep working.
func checkFreshness(publishedAt, expires string, now time.Time) error {
	if publishedAt == "" || expires == "" {
		return fmt.Errorf("%w: published_at and expires are not signed", ErrBadFeed)
	}

	pub, err := time.Parse(time.RFC3339, publishedAt)
	if err != nil {
		return fmt.Errorf("%w: published_at", ErrBadFeed)
	}

	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return fmt.Errorf("%w: expires", ErrBadFeed)
	}

	if !exp.After(pub) || exp.Sub(pub) > MaxFeedValidity {
		return fmt.Errorf("%w: expires is not within %s after published_at", ErrBadFeed, MaxFeedValidity)
	}

	if pub.After(now.Add(feedClockSkew)) {
		return fmt.Errorf("%w: published_at is in the future", ErrBadFeed)
	}

	if !now.Before(exp) {
		return fmt.Errorf("%w: expired at %s", ErrStaleFeed, expires)
	}

	return nil
}

func offerOf(version, platform string, e entry) (*Offer, error) {
	if e.Size <= 0 || e.Size > maxArtifactBytes {
		return nil, fmt.Errorf("%w: size %d", ErrBadFeed, e.Size)
	}

	if d, err := hex.DecodeString(e.SHA256); err != nil || len(d) != 32 || e.SHA256 != strings.ToLower(e.SHA256) {
		return nil, fmt.Errorf("%w: sha256", ErrBadFeed)
	}

	sig, err := base64.StdEncoding.DecodeString(e.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: artifact signature", ErrBadFeed)
	}

	if u, err := url.Parse(e.URL); err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: url", ErrBadFeed)
	}

	return &Offer{
		Version: version, Platform: platform, URL: e.URL, SHA256: e.SHA256, Size: e.Size, signature: sig,
	}, nil
}

// version is MAJOR.MINOR.PATCH, numbers without leading zeros and no suffix.
type version [3]uint64

func parseVersion(s string) (version, error) {
	var v version

	parts := strings.Split(s, ".")
	if len(parts) != len(v) {
		return v, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", s)
	}

	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return v, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", s)
		}

		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return v, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", s)
		}

		v[i] = n
	}

	return v, nil
}

func (v version) newerThan(o version) bool {
	for i := range v {
		if v[i] != o[i] {
			return v[i] > o[i]
		}
	}

	return false
}
