package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWaivers(t *testing.T, body string) string {
	t.Helper()

	file := filepath.Join(t.TempDir(), "waivers.json")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return file
}

func TestAuditExprsWaivers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	put(t, root, "licenses/go-modules/lib@v1/LICENSE", "BSD text")

	sum := sha256.Sum256([]byte("BSD text"))
	good := hex.EncodeToString(sum[:])

	body := func(sha string) string {
		return `{"waivers":[{"package":"lib","claimed":"GPL-2.0-only","shipped":"BSD-3-Clause",` +
			`"path":"licenses/go-modules/lib@v1/LICENSE","sha256":"` + sha + `","reason":"header comments"}]}`
	}

	in := "lib\tGPL-2.0-only\nother\tGPL-2.0-only\nlib\tAGPL-3.0-only\n"

	t.Run("matching checksum waives only the named package and id", func(t *testing.T) {
		t.Parallel()

		w, err := loadWaivers(writeWaivers(t, body(good)), root)
		if err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer

		n, err := auditExprs(strings.NewReader(in), &out, w)
		if err != nil || n != 2 {
			t.Fatalf("n = %d, err = %v, output %q", n, err, out.String())
		}

		for _, want := range []string{
			"waived\tlib\tGPL-2.0-only claimed, BSD-3-Clause shipped: header comments\n",
			"deny\tother\tGPL-2.0-only\n",
			"deny\tlib\tAGPL-3.0-only\n",
		} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("missing %q in %q", want, out.String())
			}
		}
	})

	t.Run("a changed text expires the waiver", func(t *testing.T) {
		t.Parallel()

		w, err := loadWaivers(writeWaivers(t, body(strings.Repeat("0", 64))), root)
		if err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer

		n, _ := auditExprs(strings.NewReader("lib\tGPL-2.0-only\n"), &out, w)
		if n != 1 || !strings.HasPrefix(out.String(), "expired\tlib\t") {
			t.Errorf("n = %d, output %q", n, out.String())
		}
	})

	t.Run("a missing text expires the waiver", func(t *testing.T) {
		t.Parallel()

		w, err := loadWaivers(writeWaivers(t, body(good)), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer

		n, _ := auditExprs(strings.NewReader("lib\tGPL-2.0-only\n"), &out, w)
		if n != 1 || !strings.HasPrefix(out.String(), "expired\tlib\t") {
			t.Errorf("n = %d, output %q", n, out.String())
		}
	})
}

func TestLoadWaiversRejectsIncompleteEntries(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`{"waivers":[{"package":"a","claimed":"X","shipped":"MIT","path":"p","sha256":"s"}]}`,
		`{"waivers":[],"extra":1}`,
		`not json`,
	} {
		if _, err := loadWaivers(writeWaivers(t, body), ""); err == nil {
			t.Errorf("loadWaivers accepted %q", body)
		}
	}
}
