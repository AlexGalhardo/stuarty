// Command updatefeed makes and checks the signed feed of the desktop auto-update (SDD-009 section 10.5,
// P3-REL-004). The release workflow runs it; the client that reads the feed is desktop/update.
//
//	UPDATE_FEED_SIGNING_KEY=<64 hex> updatefeed sign -version 0.15.0 -base-url https://host/updates [-notes-file notes.md] -out feed.json <file>...
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
	"time"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

const (
	defaultKeyEnv    = "UPDATE_FEED_SIGNING_KEY"
	feedSchema       = 1
	maxArtifactBytes = 1 << 30
	filePrefix       = "stuarty-desktop_"
)

var (
	stableVersion = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	// A desktop installer: stuarty-desktop_<version>_<goos>_<goarch>_setup.exe, .dmg or .deb.
	installerName = regexp.MustCompile(
		`^stuarty-desktop_([0-9.]+)_(windows|darwin|linux)_(amd64|arm64)(?:_setup\.exe|\.dmg|\.deb)$`,
	)
	// A launcher artifact, the file the desktop launcher swaps for itself: an AppImage, a zipped macOS
	// application or a zipped Windows portable build (EPIC-55, D-039).
	launcherName = regexp.MustCompile(
		`^stuarty-desktop_([0-9.]+)_(windows|darwin|linux)_(amd64|arm64)(?:\.AppImage|\.app\.zip|_portable\.zip)$`,
	)
	// The download a person grabs: stuarty-desktop-0-28-0.exe, .AppImage, or -arm64.dmg.
	// The version uses dashes. .exe is Windows amd64, .AppImage is Linux amd64, .dmg names its arch.
	friendlyName = regexp.MustCompile(
		`^stuarty-desktop-([0-9]+-[0-9]+-[0-9]+)(?:-(amd64|arm64))?(\.exe|\.AppImage|\.dmg)$`,
	)
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// artifact is one file the feed lists: its platform key and whether it replaces the launcher.
type artifact struct {
	platform string
	launcher bool
}

// classify reads the platform of a desktop installer or launcher artifact of version.
// version is MAJOR.MINOR.PATCH with dots. The dashed download names count as that version.
func classify(name, version string) (artifact, bool) {
	if m := installerName.FindStringSubmatch(name); len(m) == 4 && m[1] == version {
		return artifact{platform: m[2] + "-" + m[3]}, true
	}

	if m := launcherName.FindStringSubmatch(name); len(m) == 4 && m[1] == version {
		return artifact{platform: m[2] + "-" + m[3], launcher: true}, true
	}

	m := friendlyName.FindStringSubmatch(name)
	if len(m) != 4 || strings.ReplaceAll(m[1], "-", ".") != version {
		return artifact{}, false
	}

	switch m[3] {
	case ".exe":
		if m[2] != "" {
			return artifact{}, false
		}

		return artifact{platform: "windows-amd64"}, true
	case ".AppImage":
		if m[2] != "" {
			return artifact{}, false
		}

		return artifact{platform: "linux-amd64", launcher: true}, true
	case ".dmg":
		if m[2] != "amd64" && m[2] != "arm64" {
			return artifact{}, false
		}

		return artifact{platform: "darwin-" + m[2]}, true
	default:
		return artifact{}, false
	}
}

// launcherPlatform prefixes the platform in the signed message of a launcher artifact: the same
// artifact message as an installer, with the platform "launcher:<goos>-<goarch>", so the signature of
// an installer is never valid for a launcher artifact and the other way round.
const launcherPlatform = "launcher:"

type envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type payload struct {
	Schema      int              `json:"schema"`
	Version     string           `json:"version"`
	PublishedAt string           `json:"published_at"`
	Expires     string           `json:"expires"`
	Platforms   map[string]entry `json:"platforms"`
	// Notes is a short plain text summary of the release, signed with the rest of the payload.
	Notes string `json:"notes,omitempty"`
	// Launchers are the artifacts the launcher replaces itself with, by platform; absent when the
	// release has none. Additive: a client that does not know the field ignores it.
	Launchers map[string]entry `json:"launchers,omitempty"`
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
	notesFile := fs.String("notes-file", "", "the release notes (Markdown) to summarize into the feed; none when empty")
	keyEnv := fs.String("key-env", defaultKeyEnv, "environment variable that holds the signing seed")
	validFor := fs.Duration("valid-for", update.DefaultFeedValidity, "how long the signed feed stays fresh")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if *validFor <= 0 || *validFor > update.MaxFeedValidity {
		return fmt.Errorf("-valid-for must be positive and at most %s", update.MaxFeedValidity)
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

	notes := ""

	if *notesFile != "" {
		raw, rerr := os.ReadFile(*notesFile)
		if rerr != nil {
			return rerr
		}

		notes = summarizeNotes(string(raw))
	}

	body, err := buildPayload(*version, base, notes, files, priv, time.Now().UTC(), *validFor)
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

	_, err = fmt.Fprintf(stdout, "updatefeed: %s lists %d file(s) for %s\n", *out, len(files), *version)

	return err
}

func buildPayload(
	version, base, notes string, files []string, priv ed25519.PrivateKey, now time.Time, validFor time.Duration,
) ([]byte, error) {
	p := payload{
		Schema: feedSchema, Version: version, Platforms: map[string]entry{}, Notes: notes,
		PublishedAt: now.Format(time.RFC3339), Expires: now.Add(validFor).Format(time.RFC3339),
	}

	for _, file := range files {
		name := filepath.Base(file)

		kind, ok := classify(name, version)
		if !ok {
			return nil, fmt.Errorf("%s is not a desktop installer or launcher artifact of version %s", name, version)
		}

		group, signed := p.Platforms, ""
		if kind.launcher {
			if p.Launchers == nil {
				p.Launchers = map[string]entry{}
			}

			group, signed = p.Launchers, launcherPlatform
		}

		platform := kind.platform
		if _, dup := group[platform]; dup {
			return nil, fmt.Errorf("two files for %s", signed+platform)
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

		sig := ed25519.Sign(priv, update.ArtifactMessage(version, signed+platform, sum))
		group[platform] = entry{
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
		len(p.Platforms)+len(p.Launchers),
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

	if p.PublishedAt == "" || p.Expires == "" {
		return nil, errors.New("the feed does not sign published_at and expires")
	}

	if p.Schema != feedSchema || !stableVersion.MatchString(p.Version) || len(p.Platforms) == 0 {
		return nil, errors.New("the feed has an unknown schema, a version that is not stable or no platform")
	}

	if len(p.Notes) > maxNotesBytes {
		return nil, fmt.Errorf("the notes of the feed are %d bytes, the limit is %d", len(p.Notes), maxNotesBytes)
	}

	if err := verifyEntries(&p, p.Platforms, "", pub, sums); err != nil {
		return nil, err
	}

	if err := verifyEntries(&p, p.Launchers, launcherPlatform, pub, sums); err != nil {
		return nil, err
	}

	return &p, nil
}

// verifyEntries checks every file of a group of the feed against checksums.txt and its own signature;
// signed is the prefix of the platform in the signed message ("" for an installer).
func verifyEntries(
	p *payload,
	group map[string]entry,
	signed string,
	pub ed25519.PublicKey,
	sums map[string]string,
) error {
	platforms := make([]string, 0, len(group))
	for k := range group {
		platforms = append(platforms, k)
	}

	slices.Sort(platforms)

	for _, platform := range platforms {
		e := group[platform]
		name := e.URL[strings.LastIndex(e.URL, "/")+1:]

		want, ok := sums[name]
		if !ok {
			return fmt.Errorf("%s%s: %s is not in checksums.txt", signed, platform, name)
		}

		if !hex64.MatchString(e.SHA256) || e.SHA256 != want {
			return fmt.Errorf(
				"%s%s: the feed digest %s differs from checksums.txt %s",
				signed,
				platform,
				e.SHA256,
				want,
			)
		}

		asig, err := base64.StdEncoding.DecodeString(e.Signature)
		if err != nil || !ed25519.Verify(pub, update.ArtifactMessage(p.Version, signed+platform, e.SHA256), asig) {
			return fmt.Errorf("%s%s: the artifact signature does not verify", signed, platform)
		}

		if !strings.HasPrefix(name, filePrefix+p.Version+"_") {
			return fmt.Errorf("%s%s: %s is not a file of %s", signed, platform, name, p.Version)
		}
	}

	return nil
}
