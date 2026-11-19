package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

const appImage = "stuarty-desktop_0.15.0_linux_amd64.AppImage"

const releaseNotes = "## Added\n\n- **Launcher** with [updates](https://x.example/u)\n- `a` thing\n\n## Fixed\n\n- a bug\n"

func TestTheFeedCarriesTheNotesAndTheLauncherArtifacts(t *testing.T) {
	pub := useTestKey(t)
	dir, files := writeInstallers(t)

	launcher := filepath.Join(dir, appImage)
	if err := os.WriteFile(launcher, []byte(strings.Repeat("appimage ", 50)), 0o600); err != nil {
		t.Fatal(err)
	}

	files = append(files, launcher)

	notes := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(notes, []byte(releaseNotes), 0o600); err != nil {
		t.Fatal(err)
	}

	feed := filepath.Join(t.TempDir(), "feed.json")

	var out, errb bytes.Buffer

	args := append([]string{
		"sign", "-version", "0.15.0", "-base-url", "https://host.example/updates", "-notes-file", notes, "-out", feed,
	}, files...)
	if code := run(args, &out, &errb); code != 0 {
		t.Fatalf("sign exit %d: %s", code, errb.String())
	}

	raw, err := os.ReadFile(feed)
	if err != nil {
		t.Fatal(err)
	}

	key, err := hex.DecodeString(pub)
	if err != nil {
		t.Fatal(err)
	}

	sums, err := readChecksums(checksumsOf(t, files))
	if err != nil {
		t.Fatal(err)
	}

	p, err := verifyFeed(raw, key, sums)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if want := "- Launcher with updates\n- a thing\n- a bug"; p.Notes != want {
		t.Errorf("notes = %q, want %q", p.Notes, want)
	}

	got, ok := p.Launchers["linux-amd64"]
	if !ok || !strings.HasSuffix(got.URL, "/"+appImage) || len(p.Platforms) != 2 {
		t.Fatalf("launchers = %+v, platforms = %d", p.Launchers, len(p.Platforms))
	}

	// The signature of a launcher artifact is bound to its group: it does not verify as the signature
	// of an installer of the same platform and digest.
	sig, err := base64.StdEncoding.DecodeString(got.Signature)
	if err != nil {
		t.Fatal(err)
	}

	if ed25519.Verify(key, update.ArtifactMessage("0.15.0", "linux-amd64", got.SHA256), sig) {
		t.Error("a launcher signature verifies as an installer signature")
	}

	// The notes are signed with the payload: a changed note breaks the feed.
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}

	body, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		t.Fatal(err)
	}

	env.Payload = base64.StdEncoding.EncodeToString([]byte(strings.Replace(string(body), "a bug", "a b0g", 1)))
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := verifyFeed(
		tampered,
		key,
		sums,
	); err == nil ||
		!strings.Contains(err.Error(), "signature of the feed") {
		t.Errorf("a changed note must break the signature, got %v", err)
	}
}

func TestAFeedWithoutNotesOrLaunchersIsUnchanged(t *testing.T) {
	useTestKey(t)
	_, files := writeInstallers(t)
	feed := filepath.Join(t.TempDir(), "feed.json")

	var out, errb bytes.Buffer

	args := append([]string{"sign", "-version", "0.15.0", "-base-url", "https://h.example/u", "-out", feed}, files...)
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
	if strings.Contains(string(body), "notes") || strings.Contains(string(body), "launchers") {
		t.Errorf("payload = %s", body)
	}
}

func TestSummarizeNotesIsShortAndPlain(t *testing.T) {
	if got := summarizeNotes("# v1\n\nintro text\n"); got != "" {
		t.Errorf("no bullets, got %q", got)
	}

	if got := summarizeNotes(strings.Repeat("- item\n", 30)); strings.Count(got, "\n") != maxNoteLines-1 {
		t.Errorf("lines = %d", strings.Count(got, "\n")+1)
	}

	got := summarizeNotes("- " + strings.Repeat("é", 2000))
	if len(got) > maxNotesBytes || !utf8.ValidString(got) {
		t.Errorf("len %d, valid %v", len(got), utf8.ValidString(got))
	}
}
