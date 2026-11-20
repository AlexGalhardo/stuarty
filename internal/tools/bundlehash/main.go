// Command bundlehash prints the hashes that the bundle-matrix job of ci.yml compares (SDD-009 section 9.4):
// the SHA-256 of the canonical document of a .vvbundle, recomputed from the entry and checked against the
// manifest, and the content hash of every media file. The output is the same on every operating system for
// the same project.
//
//	go run ./internal/tools/bundlehash <file.vvbundle>
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

const maxManifest = 16 << 20

type manifest struct {
	Format   string `json:"format"`
	Document struct {
		SHA256 string `json:"sha256"`
	} `json:"document"`
	Media []struct {
		Hash string `json:"hash"`
	} `json:"media"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: bundlehash <file.vvbundle>")
		os.Exit(2)
	}

	lines, err := hashes(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bundlehash: %v\n", err)
		os.Exit(1)
	}

	for _, l := range lines {
		fmt.Println(l)
	}
}

// hashes returns "document <sha256>" followed by one "media <hash>" per media file, sorted.
func hashes(path string) ([]string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var m manifest

	raw, err := readEntry(zr, "manifest.json", maxManifest)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}

	if m.Format != "stuarty-bundle" && m.Format != "vukem-video-bundle" {
		return nil, fmt.Errorf("manifest.json: format %q is not a stuarty bundle", m.Format)
	}

	doc, err := readEntry(zr, "document.json", maxManifest)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(doc)
	got := hex.EncodeToString(sum[:])

	if got != m.Document.SHA256 {
		return nil, fmt.Errorf("document.json hashes to %s, the manifest says %s", got, m.Document.SHA256)
	}

	media := make([]string, 0, len(m.Media))
	for _, f := range m.Media {
		media = append(media, "media "+f.Hash)
	}

	slices.Sort(media)

	return append([]string{"document " + got}, media...), nil
}

func readEntry(zr *zip.ReadCloser, name string, limit int64) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}

		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(r, limit+1))
		_ = r.Close() // a read-only zip entry: its Close reports nothing the read did not
		if err != nil {
			return nil, err
		}

		if int64(len(b)) > limit {
			return nil, fmt.Errorf("%s is larger than %d bytes", name, limit)
		}

		return b, nil
	}

	return nil, errors.New("no entry " + name)
}
