package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeBundle(t *testing.T, entries map[string]string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "x.vvbundle")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)

	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestHashes(t *testing.T) {
	t.Parallel()

	doc := `{"schema":1}`
	sum := sha256.Sum256([]byte(doc))
	docHash := hex.EncodeToString(sum[:])
	manifestFor := func(hash string) string {
		return `{"format":"stuarty-bundle","document":{"sha256":"` + hash + `"},"media":[{"hash":"bbb"},{"hash":"aaa"}]}`
	}

	t.Run("document and sorted media", func(t *testing.T) {
		t.Parallel()

		got, err := hashes(
			writeBundle(t, map[string]string{"manifest.json": manifestFor(docHash), "document.json": doc}),
		)
		if err != nil {
			t.Fatal(err)
		}

		if want := []string{"document " + docHash, "media aaa", "media bbb"}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("manifest hash that does not match the document", func(t *testing.T) {
		t.Parallel()

		_, err := hashes(writeBundle(t, map[string]string{"manifest.json": manifestFor("00"), "document.json": doc}))
		if err == nil || !strings.Contains(err.Error(), "hashes to") {
			t.Fatalf("got %v, want a hash mismatch", err)
		}
	})

	t.Run("another format and a missing document", func(t *testing.T) {
		t.Parallel()

		if _, err := hashes(writeBundle(t, map[string]string{"manifest.json": `{"format":"zip"}`})); err == nil {
			t.Fatal("want an error for another format")
		}

		if _, err := hashes(writeBundle(t, map[string]string{"manifest.json": manifestFor(docHash)})); err == nil {
			t.Fatal("want an error for a missing document")
		}
	})
}
