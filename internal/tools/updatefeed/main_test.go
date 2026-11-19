package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/desktop/update"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

const (
	seedFile     = "../../../desktop/update/testdata/test-key.seed"
	pubFile      = "../../../desktop/update/testdata/test-key.pub"
	winInstaller = "stuarty-desktop_0.15.0_windows_amd64_setup.exe"
	debInstaller = "stuarty-desktop_0.15.0_linux_amd64.deb"
)

func useTestKey(t *testing.T) string {
	t.Helper()

	seed, err := os.ReadFile(seedFile)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(defaultKeyEnv, strings.TrimSpace(string(seed)))

	pub, err := os.ReadFile(pubFile)
	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSpace(string(pub))
}

func writeInstallers(t *testing.T) (dir string, files []string) {
	t.Helper()

	dir = t.TempDir()

	for name, body := range map[string]string{
		winInstaller: strings.Repeat("windows installer ", 50),
		debInstaller: strings.Repeat("debian package ", 50),
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}

		files = append(files, p)
	}

	return dir, files
}

func checksumsOf(t *testing.T, files []string) string {
	t.Helper()

	var b strings.Builder

	for _, f := range files {
		sum, _, err := digest(f)
		if err != nil {
			t.Fatal(err)
		}

		b.WriteString(sum + "  " + filepath.Base(f) + "\n")
	}

	p := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

func TestClassifyFriendlyDownloadNames(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		launcher bool
	}{
		{"stuarty-desktop-0-28-0.exe", "windows-amd64", false},
		{"stuarty-desktop-0-28-0.AppImage", "linux-amd64", true},
		{"stuarty-desktop-0-28-0-arm64.dmg", "darwin-arm64", false},
		{"stuarty-desktop-0-28-0-amd64.dmg", "darwin-amd64", false},
		{winInstaller, "windows-amd64", false},
		{"stuarty-desktop_0.15.0_linux_amd64.AppImage", "linux-amd64", true},
	}
	for _, c := range cases {
		version := "0.28.0"
		if strings.Contains(c.name, "0.15.0") {
			version = "0.15.0"
		}

		got, ok := classify(c.name, version)
		if !ok || got.platform != c.platform || got.launcher != c.launcher {
			t.Errorf("%s: got %+v ok=%v", c.name, got, ok)
		}
	}

	if _, ok := classify("stuarty-desktop-0-28-0.exe", "0.27.0"); ok {
		t.Error("a file of another version was accepted")
	}

	if _, ok := classify("stuarty-desktop-0-28-0-arm64.exe", "0.28.0"); ok {
		t.Error("an exe must not carry an architecture suffix")
	}
}

func TestPubkeyMatchesTheTestFixture(t *testing.T) {
	want := useTestKey(t)

	var out, errb bytes.Buffer
	if code := run([]string{"pubkey"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}

	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("pubkey = %q, want %q", out.String(), want)
	}
}

func TestSignThenVerifyAgainstChecksums(t *testing.T) {
	pub := useTestKey(t)
	_, files := writeInstallers(t)
	feed := filepath.Join(t.TempDir(), "feed.json")

	var out, errb bytes.Buffer

	args := append(
		[]string{"sign", "-version", "0.15.0", "-base-url", "https://host.example/updates/", "-out", feed},
		files...)
	if code := run(args, &out, &errb); code != 0 {
		t.Fatalf("sign exit %d: %s", code, errb.String())
	}

	sums := checksumsOf(t, files)
	if code := run([]string{"verify", "-pub", pub, "-checksums", sums, feed}, &out, &errb); code != 0 {
		t.Fatalf("verify exit %d: %s", code, errb.String())
	}

	// A digest that differs from checksums.txt is refused.
	bad := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(
		bad,
		[]byte(strings.Repeat("0", 64)+"  "+winInstaller+"\n"+strings.Repeat("1", 64)+"  "+debInstaller+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	errb.Reset()

	if code := run(
		[]string{"verify", "-pub", pub, "-checksums", bad, feed},
		&out,
		&errb,
	); code != 1 ||
		!strings.Contains(errb.String(), "differs from checksums.txt") {
		t.Fatalf("a wrong digest must fail: exit %d, %q", code, errb.String())
	}

	// Another key does not verify the feed.
	other := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	errb.Reset()

	if code := run([]string{"verify", "-pub", other, "-checksums", sums, feed}, &out, &errb); code != 1 {
		t.Fatalf("a wrong key must fail, exit %d", code)
	}
}

func TestSignedFeedCarriesAFreshnessWindowAndVerifyRequiresIt(t *testing.T) {
	pub := useTestKey(t)
	_, files := writeInstallers(t)
	feed := filepath.Join(t.TempDir(), "feed.json")

	var out, errb bytes.Buffer

	args := append(
		[]string{
			"sign", "-version", "0.15.0", "-base-url", "https://host.example/u", "-valid-for", "48h", "-out", feed,
		},
		files...)
	if code := run(args, &out, &errb); code != 0 {
		t.Fatalf("sign exit %d: %s", code, errb.String())
	}

	raw, err := os.ReadFile(feed)
	if err != nil {
		t.Fatal(err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}

	body, _ := base64.StdEncoding.DecodeString(env.Payload)

	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}

	published, err1 := time.Parse(time.RFC3339, p.PublishedAt)

	expires, err2 := time.Parse(time.RFC3339, p.Expires)
	if err1 != nil || err2 != nil || expires.Sub(published) != 48*time.Hour {
		t.Fatalf("published_at %q, expires %q: want a 48h window", p.PublishedAt, p.Expires)
	}

	// A feed signed without the fields (an old tool) does not verify.
	p.PublishedAt, p.Expires = "", ""
	legacy, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}

	priv := ed25519.NewKeyFromSeed(mustSeed(t))
	legacyDoc, err := json.Marshal(envelope{
		Payload:   base64.StdEncoding.EncodeToString(legacy),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, update.FeedMessage(legacy))),
	})
	if err != nil {
		t.Fatal(err)
	}

	legacyFile := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(legacyFile, legacyDoc, 0o600); err != nil {
		t.Fatal(err)
	}

	errb.Reset()

	if code := run(
		[]string{"verify", "-pub", pub, "-checksums", checksumsOf(t, files), legacyFile}, &out, &errb,
	); code != 1 || !strings.Contains(errb.String(), "published_at and expires") {
		t.Fatalf("a feed without the window must fail: exit %d, %q", code, errb.String())
	}

	for _, bad := range []string{"0s", "-1h", "20000h"} {
		errb.Reset()

		if code := run(
			append(
				[]string{"sign", "-valid-for", bad, "-version", "0.15.0", "-base-url", "https://h.example/u"},
				files...),
			&out,
			&errb,
		); code != 1 {
			t.Errorf("-valid-for %s must fail, exit %d", bad, code)
		}
	}
}

func mustSeed(t *testing.T) []byte {
	t.Helper()

	raw, err := os.ReadFile(seedFile)
	if err != nil {
		t.Fatal(err)
	}

	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}

	return seed
}

func TestSignRefusesWhatTheFeedMustNeverHold(t *testing.T) {
	useTestKey(t)
	_, files := writeInstallers(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"pre-release", []string{"-version", "0.15.0-rc.1", "-base-url", "https://h.example/u"}, "never in the feed"},
		{"http", []string{"-version", "0.15.0", "-base-url", "http://h.example/u"}, "must be https"},
		{
			"another version",
			[]string{"-version", "0.16.0", "-base-url", "https://h.example/u"},
			"not a desktop installer or launcher artifact of version 0.16.0",
		},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer

		args := append(append([]string{"sign", "-out", filepath.Join(t.TempDir(), "f.json")}, c.args...), files...)
		if code := run(args, &out, &errb); code != 1 || !strings.Contains(errb.String(), c.want) {
			t.Errorf("%s: exit %d, %q", c.name, code, errb.String())
		}
	}

	t.Setenv(defaultKeyEnv, "")

	var out, errb bytes.Buffer

	args := append([]string{"sign", "-version", "0.15.0", "-base-url", "https://h.example/u"}, files...)
	if code := run(args, &out, &errb); code != 1 || !strings.Contains(errb.String(), defaultKeyEnv) {
		t.Errorf("a missing key must name the secret: exit %d, %q", code, errb.String())
	}
}

// TestTheClientAcceptsTheFeed proves the format against desktop/update itself: the client checks the
// feed this tool signed, downloads the installer from the same host and verifies both signatures.
func TestTheClientAcceptsTheFeed(t *testing.T) {
	pub := useTestKey(t)
	dir, files := writeInstallers(t)
	feed := filepath.Join(t.TempDir(), "feed.json")

	var out, errb bytes.Buffer
	var doc []byte

	// The client allows one host, so the installers are served by the feed server itself.
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	mux.HandleFunc("/feed.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(doc) })

	one := httptest.NewTLSServer(mux)
	t.Cleanup(one.Close)

	args := append([]string{"sign", "-version", "0.15.0", "-base-url", one.URL, "-out", feed}, files...)
	if code := run(args, &out, &errb); code != 0 {
		t.Fatalf("sign exit %d: %s", code, errb.String())
	}

	var err error

	doc, err = os.ReadFile(feed)
	if err != nil {
		t.Fatal(err)
	}

	key, err := hex.DecodeString(pub)
	if err != nil {
		t.Fatal(err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(one.Certificate())

	installed := &fakeInstaller{}

	client, err := update.New(update.Config{
		FeedURL: one.URL + "/feed.json", PublicKey: key, CurrentVersion: "0.14.0", Platform: "windows-amd64",
		StagingDir: filepath.Join(t.TempDir(), "staging"), AutoCheck: true, Installer: installed,
		Test: &outbound.TestOptions{RootCAs: pool},
	})
	if err != nil {
		t.Fatal(err)
	}

	offer, err := client.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("the client refused the feed: %v", err)
	}

	if offer.Version != "0.15.0" {
		t.Fatalf("offer version = %q", offer.Version)
	}

	if err := client.Apply(context.Background(), offer); err != nil {
		t.Fatalf("the client refused the installer: %v", err)
	}

	if len(installed.calls) != 1 {
		t.Fatalf("the installer was called %d times", len(installed.calls))
	}
}

type fakeInstaller struct{ calls []update.Artifact }

func (f *fakeInstaller) Install(_ context.Context, a update.Artifact) error {
	f.calls = append(f.calls, a)

	return nil
}
