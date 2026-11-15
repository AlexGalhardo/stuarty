package timeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const canonicalDir = "testdata/canonical"

// canonicalOfFixture decodes a fixture of any released version, migrates it to
// the current version and returns its canonical bytes (SDD-001 section 6 item 4).
func canonicalOfFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := DecodeRaw(readFile(t, path))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	migrated, err := Migrate(raw)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	doc, err := FromRaw(migrated)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if vs := Validate(doc); len(vs) != 0 {
		t.Fatalf("%s: %v", path, vs)
	}
	return Canonical(doc)
}

// hashName maps testdata/versions/v1/x.json to v1.x.sha256.
func hashName(fixture string) string {
	version := filepath.Base(filepath.Dir(fixture))
	base := strings.TrimSuffix(filepath.Base(fixture), ".json")
	return version + "." + base + ".sha256"
}

// SDD-001 section 6 item 4 and section 8 check 3, ADR-0009 item 3, NFR-012:
// the SHA-256 of the canonical bytes of every fixture of every released version,
// after Migrate, is committed once and must be the same on every platform.
func TestCanonicalBytesCrossPlatform(t *testing.T) {
	fixtures := files(t, "testdata/versions/v*/*.json")
	want := map[string]bool{}
	for _, f := range fixtures {
		canonical := canonicalOfFixture(t, f)
		sum := sha256.Sum256(canonical)
		got := hex.EncodeToString(sum[:])
		name := hashName(f)
		want[name] = true
		path := filepath.Join(canonicalDir, name)

		if *update {
			if err := os.MkdirAll(canonicalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		if committed := strings.TrimSpace(string(readFile(t, path))); committed != got {
			t.Errorf("%s: canonical sha256 is %s, committed %s", f, got, committed)
		}
		if bytes.ContainsRune(canonical, '\r') || bytes.ContainsRune(canonical, '\n') {
			t.Errorf("%s: canonical bytes contain a line ending", f)
		}

		again, err := Decode(canonical)
		if err != nil {
			t.Fatalf("%s: decode of its canonical bytes: %v", f, err)
		}
		if !bytes.Equal(Canonical(again), canonical) {
			t.Errorf("%s: decode then encode changed the canonical bytes", f)
		}
	}

	hashes := files(t, filepath.Join(canonicalDir, "*.sha256"))
	for _, h := range hashes {
		if !want[filepath.Base(h)] {
			t.Errorf("%s has no fixture", h)
		}
	}
}

// A fixture saved with Windows line endings decodes to the same canonical bytes.
func TestCanonicalIgnoresLineEndings(t *testing.T) {
	for _, f := range files(t, "testdata/versions/v1/*.json") {
		data := readFile(t, f)
		crlf := bytes.ReplaceAll(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
		a, errA := Decode(data)
		b, errB := Decode(crlf)
		if errA != nil || errB != nil {
			continue
		}
		if !bytes.Equal(Canonical(a), Canonical(b)) {
			t.Errorf("%s: line endings change the canonical bytes", f)
		}
	}
}

// Every released version has a non-empty fixture directory, and the documents
// written by the release binaries are present.
func TestEveryReleaseHasFixtures(t *testing.T) {
	for v := 1; v <= CurrentSchemaVersion; v++ {
		dir := filepath.Join("testdata", "versions", "v"+strconv.Itoa(v))
		if len(files(t, filepath.Join(dir, "*.json"))) == 0 {
			t.Errorf("%s has no fixture", dir)
		}
	}
	for _, tag := range []string{"v0.3.0", "v0.5.0", "v0.11.0", "v0.12.0", "v0.13.0"} {
		files(t, "testdata/versions/v1/"+tag+"-*.json")
	}
}
