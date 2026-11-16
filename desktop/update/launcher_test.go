package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const testSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// launcherFeed signs a feed. Each map gives, per platform key, the platform name its artifact signature
// covers, so a test can sign with the wrong label.
// launcherNow is the clock of the launcher feed tests: the feeds they sign are fresh at this instant.
var launcherNow = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

func launcherFeed(t *testing.T, priv ed25519.PrivateKey, platforms, launchers map[string]string) []byte {
	t.Helper()

	mk := func(url, signedAs string) map[string]any {
		sig := ed25519.Sign(priv, ArtifactMessage("0.21.0", signedAs, testSHA))

		return map[string]any{
			"url": url, "sha256": testSHA, "size": 10, "signature": base64.StdEncoding.EncodeToString(sig),
		}
	}

	body := map[string]any{
		"schema": feedSchema, "version": "0.21.0", "platforms": map[string]any{},
		"published_at": launcherNow.Add(-time.Hour).Format(time.RFC3339),
		"expires":      launcherNow.Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}

	for key, signedAs := range platforms {
		body["platforms"].(map[string]any)[key] = mk("https://h.example/installer-"+key, signedAs)
	}

	if launchers != nil {
		l := map[string]any{}
		for key, signedAs := range launchers {
			l[key] = mk("https://h.example/launcher-"+key, signedAs)
		}

		body["launchers"] = l
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	sig := ed25519.Sign(priv, FeedMessage(raw))
	out, err := json.Marshal(envelope{
		Payload: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(sig),
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func offerFor(t *testing.T, feed []byte, pub ed25519.PublicKey, platform string) (*Offer, error) {
	t.Helper()

	return parseFeed(feed, pub, platform, launcherNow)
}

func TestFeedWithLaunchersOffersTheLauncherOnMacAndLinux(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	for _, key := range []string{"linux-amd64", "darwin-arm64"} {
		feed := launcherFeed(t, priv, map[string]string{key: key}, map[string]string{key: "launcher:" + key})

		o, err := offerFor(t, feed, pub, key)
		if err != nil || !strings.Contains(o.URL, "launcher-") {
			t.Fatalf("%s: %v %+v", key, err, o)
		}

		if err := verifyArtifact(pub, o); err != nil {
			t.Errorf("%s: the launcher signature must verify: %v", key, err)
		}
	}
}

func TestWindowsAndOldFeedsFallBackToThePlatformsEntry(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	cases := map[string]map[string]string{
		"windows-amd64": {"windows-amd64": "launcher:windows-amd64"},
		"linux-amd64":   nil,
	}

	for key, launchers := range cases {
		feed := launcherFeed(t, priv, map[string]string{key: key}, launchers)

		o, err := offerFor(t, feed, pub, key)
		if err != nil || !strings.Contains(o.URL, "installer-") {
			t.Fatalf("%s: %v %+v", key, err, o)
		}

		if err := verifyArtifact(pub, o); err != nil {
			t.Errorf("%s: %v", key, err)
		}
	}
}

func TestSecurity_T35_InstallerSignatureIsNeverALauncherSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	// A launcher entry signed with the plain label, and an installer signed with the launcher label.
	feed := launcherFeed(t, priv, map[string]string{"linux-amd64": "launcher:linux-amd64"},
		map[string]string{"linux-amd64": "linux-amd64"})

	o, err := offerFor(t, feed, pub, "linux-amd64")
	if err != nil {
		t.Fatal(err)
	}

	if err := verifyArtifact(pub, o); !errors.Is(err, ErrBadArtifactSignature) {
		t.Errorf("launcher with the plain label: %v", err)
	}

	feed = launcherFeed(t, priv, map[string]string{"linux-amd64": "launcher:linux-amd64"}, nil)

	if o, err = offerFor(t, feed, pub, "linux-amd64"); err != nil {
		t.Fatal(err)
	}

	if err := verifyArtifact(pub, o); !errors.Is(err, ErrBadArtifactSignature) {
		t.Errorf("installer with the launcher label: %v", err)
	}
}
