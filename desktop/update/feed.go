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
)

// Errors of the feed and of the artifact checks.
var (
	ErrBadFeed              = errors.New("update: the feed is not valid")
	ErrBadFeedSignature     = errors.New("update: the signature of the feed does not verify")
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
	Schema    int              `json:"schema"`
	Version   string           `json:"version"`
	Platforms map[string]entry `json:"platforms"`
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
func parseFeed(raw []byte, key ed25519.PublicKey, platform string) (*Offer, error) {
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

	e, ok := p.Platforms[platform]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoPlatform, platform)
	}

	return offerOf(p.Version, platform, e)
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
