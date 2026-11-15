package transcribe

import (
	_ "embed" // the manifest is part of the binary
	"encoding/json"
	"fmt"
	"slices"
)

//go:embed manifest.json
var manifestJSON []byte

// Entry kinds of the manifest.
const (
	KindHelper = "helper"
	KindModel  = "model"
)

// ManifestEntry pins one downloadable file (SDD-007 section 6.8 item 3): where
// it comes from, its exact size and its SHA-256. Nothing is run or used before
// the digest of the downloaded bytes matches.
type ManifestEntry struct {
	// Name is the tool the entry belongs to ("whisper").
	Name string `json:"name"`
	// Kind is KindHelper or KindModel.
	Kind string `json:"kind"`
	// Model is the model name of a KindModel entry ("small").
	Model   string `json:"model,omitempty"`
	Version string `json:"version"`
	// OS and Arch are GOOS and GOARCH for a helper; empty for a model.
	OS     string `json:"os,omitempty"`
	Arch   string `json:"arch,omitempty"`
	URL    string `json:"url"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	// File is the name the file has under tools/<name>/<version>/.
	File    string `json:"file"`
	License string `json:"license,omitempty"`
}

// Manifest lists everything the product may download.
type Manifest struct {
	// Hosts is the allowlist of every hop of a download, redirects included.
	Hosts   []string        `json:"hosts"`
	Entries []ManifestEntry `json:"entries"`
}

// DefaultManifest is the manifest embedded in the binary.
func DefaultManifest() (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return Manifest{}, fmt.Errorf("transcribe: the embedded manifest is not valid: %w", err)
	}

	return m, nil
}

// Models lists the model names the manifest pins for a tool.
func (m Manifest) Models(name string) []string {
	var out []string

	for i := range m.Entries {
		if e := &m.Entries[i]; e.Name == name && e.Kind == KindModel && !slices.Contains(out, e.Model) {
			out = append(out, e.Model)
		}
	}

	return out
}

// For returns the entries to install for a tool on a platform: its helper and
// the model asked for. ok is false when the manifest has no helper for the
// platform or no such model.
func (m Manifest) For(name, model, goos, goarch string) (entries []ManifestEntry, ok bool) {
	var helper, mod *ManifestEntry

	for i := range m.Entries {
		e := &m.Entries[i]
		if e.Name != name {
			continue
		}

		switch {
		case e.Kind == KindHelper && e.OS == goos && e.Arch == goarch:
			helper = e
		case e.Kind == KindModel && e.Model == model:
			mod = e
		}
	}

	if helper == nil || mod == nil {
		return nil, false
	}

	return []ManifestEntry{*helper, *mod}, true
}
