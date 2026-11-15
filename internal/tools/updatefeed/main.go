// Command updatefeed makes and checks the signed feed of the desktop auto-update (SDD-009 section 10.5,
// P3-REL-004). The release workflow runs it; the client that reads the feed is desktop/update.
//
//	UPDATE_FEED_SIGNING_KEY=<64 hex> updatefeed sign -version 0.15.0 -base-url https://host/updates -out feed.json <installer>...
//	UPDATE_FEED_SIGNING_KEY=<64 hex> updatefeed pubkey
//	updatefeed verify -pub <64 hex> -checksums checksums.txt feed.json
//
// The signing key is the 32 byte Ed25519 seed in hex. It is read from the environment variable named by
// -key-env (default UPDATE_FEED_SIGNING_KEY), never from a file or an argument, and never printed.
package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/desktop/update"
)

const (
	defaultKeyEnv    = "UPDATE_FEED_SIGNING_KEY"
	feedSchema       = 1
	maxArtifactBytes = 1 << 30
	filePrefix       = "vukem-video-desktop_"
)

var (
	stableVersion = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	// A desktop installer: vukem-video-desktop_<version>_<goos>_<goarch>_setup.exe, .dmg or .deb.
	installerName = regexp.MustCompile(
		`^vukem-video-desktop_([0-9.]+)_(windows|darwin|linux)_(amd64|arm64)(?:_setup\.exe|\.dmg|\.deb)$`,
	)
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "usage: updatefeed sign|pubkey|verify ...")

		return 2
	}

	var err error

	switch argv[0] {
	case "sign":
		err = runSign(argv[1:], stdout)
	case "pubkey":
		err = runPubkey(argv[1:], stdout)
	case "verify":
		err = runVerify(argv[1:], stdout)
	default:
		fmt.Fprintf(stderr, "updatefeed: unknown command %q\n", argv[0])

		return 2
	}

	if err != nil {
		fmt.Fprintf(stderr, "updatefeed: %v\n", err)

		return 1
	}

	return 0
}

func seedFromEnv(name string) (ed25519.PrivateKey, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("the secret %s is not set", name)
	}

	seed, err := hex.DecodeString(raw)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("the secret %s must be %d hex characters (an Ed25519 seed)", name, ed25519.SeedSize*2)
	}

	return ed25519.NewKeyFromSeed(seed), nil
}

func runPubkey(argv []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("pubkey", flag.ContinueOnError)
	keyEnv := fs.String("key-env", defaultKeyEnv, "environment variable that holds the signing seed")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	priv, err := seedFromEnv(*keyEnv)
	if err != nil {
		return err
	}

	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return errors.New("not an Ed25519 key")
	}

	_, err = fmt.Fprintln(stdout, hex.EncodeToString(pub))

	return err
}

func runSign(argv []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	version := fs.String("version", "", "the stable version, MAJOR.MINOR.PATCH")
	baseURL := fs.String("base-url", "", "https URL of the directory that serves the installers")
	out := fs.String("out", "feed.json", "the feed file to write")
	keyEnv := fs.String("key-env", defaultKeyEnv, "environment variable that holds the signing seed")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if !stableVersion.MatchString(*version) {
		return fmt.Errorf("version %q is not a stable MAJOR.MINOR.PATCH: a pre-release is never in the feed", *version)
	}

	base := strings.TrimRight(*baseURL, "/")
	if !strings.HasPrefix(base, "https://") {
		return fmt.Errorf("base URL %q must be https", *baseURL)
	}

	files := fs.Args()
	if len(files) == 0 {
		return errors.New("no installer given")
	}

	priv, err := seedFromEnv(*keyEnv)
	if err != nil {
		return err
	}

	body, err := buildPayload(*version, base, files, priv)
	if err != nil {
		return err
	}

	sig := ed25519.Sign(priv, update.FeedMessage(body))

	doc, err := json.MarshalIndent(envelope{
		Payload:   base64.StdEncoding.EncodeToString(body),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(*out, append(doc, '\n'), 0o644); err != nil { //nolint:gosec // the feed is public
		return err
	}

	_, err = fmt.Fprintf(stdout, "updatefeed: %s lists %d installer(s) for %s\n", *out, len(files), *version)

	return err
}

func buildPayload(version, base string, files []string, priv ed25519.PrivateKey) ([]byte, error) {
	p := payload{Schema: feedSchema, Version: version, Platforms: map[string]entry{}}

	for _, file := range files {
		name := filepath.Base(file)

		m := installerName.FindStringSubmatch(name)
		if len(m) != 4 || m[1] != version {
			return nil, fmt.Errorf("%s is not a desktop installer of version %s", name, version)
		}

		platform := m[2] + "-" + m[3]
		if _, dup := p.Platforms[platform]; dup {
			return nil, fmt.Errorf("two installers for %s", platform)
		}

		sum, size, err := digest(file)
		if err != nil {
			return nil, err
		}

		if size <= 0 || size > maxArtifactBytes {
			return nil, fmt.Errorf(
				"%s has size %d, the client takes 1 to %d bytes",
				name,
				size,
				int64(maxArtifactBytes),
			)
		}

		sig := ed25519.Sign(priv, update.ArtifactMessage(version, platform, sum))
		p.Platforms[platform] = entry{
			URL: base + "/" + name, SHA256: sum, Size: size, Signature: base64.StdEncoding.EncodeToString(sig),
		}
	}

	return json.Marshal(p)
}

func digest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()

	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func runVerify(argv []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	pubHex := fs.String("pub", "", "the public key, 64 hex characters")
	checksums := fs.String("checksums", "", "checksums.txt of the release (sha256sum format)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("usage: updatefeed verify -pub <hex> -checksums <checksums.txt> feed.json")
	}

	pub, err := hex.DecodeString(*pubHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("-pub must be 64 hex characters")
	}

	raw, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}

	sums, err := readChecksums(*checksums)
	if err != nil {
		return err
	}

	p, err := verifyFeed(raw, pub, sums)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(
		stdout,
		"updatefeed: the feed of %s is signed and its %d digest(s) equal checksums.txt\n",
		p.Version,
		len(p.Platforms),
	)

	return err
}

// readChecksums maps a file name to its SHA-256 from a sha256sum listing.
func readChecksums(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	sums := map[string]string{}
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		sum, name, ok := strings.Cut(strings.TrimRight(sc.Text(), "\r"), " ")
		if !ok {
			continue
		}

		sums[strings.TrimLeft(name, " *")] = sum
	}

	return sums, sc.Err()
}

func verifyFeed(raw []byte, pub ed25519.PublicKey, sums map[string]string) (*payload, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("the feed is not JSON: %w", err)
	}

	body, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, errors.New("the payload is not base64")
	}

	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil || !ed25519.Verify(pub, update.FeedMessage(body), sig) {
		return nil, errors.New("the signature of the feed does not verify")
	}

	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}

	if p.Schema != feedSchema || !stableVersion.MatchString(p.Version) || len(p.Platforms) == 0 {
		return nil, errors.New("the feed has an unknown schema, a version that is not stable or no platform")
	}

	platforms := make([]string, 0, len(p.Platforms))
	for k := range p.Platforms {
		platforms = append(platforms, k)
	}

	slices.Sort(platforms)

	for _, platform := range platforms {
		e := p.Platforms[platform]
		name := e.URL[strings.LastIndex(e.URL, "/")+1:]

		want, ok := sums[name]
		if !ok {
			return nil, fmt.Errorf("%s: %s is not in checksums.txt", platform, name)
		}

		if !hex64.MatchString(e.SHA256) || e.SHA256 != want {
			return nil, fmt.Errorf("%s: the feed digest %s differs from checksums.txt %s", platform, e.SHA256, want)
		}

		asig, err := base64.StdEncoding.DecodeString(e.Signature)
		if err != nil || !ed25519.Verify(pub, update.ArtifactMessage(p.Version, platform, e.SHA256), asig) {
			return nil, fmt.Errorf("%s: the artifact signature does not verify", platform)
		}

		if !strings.HasPrefix(name, filePrefix+p.Version+"_") {
			return nil, fmt.Errorf("%s: %s is not an installer of %s", platform, name, p.Version)
		}
	}

	return &p, nil
}
