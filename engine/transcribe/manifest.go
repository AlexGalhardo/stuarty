package transcribe

import (
	_ "embed" // the manifest is part of the binary
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
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
	// File is the name the file has under tools/<name>/<version>/. For a helper that comes in an archive
	// it is the executable.
	File    string `json:"file"`
	License string `json:"license,omitempty"`
	// Notice is the licence text of the file, for the third-party notices (SDD-009).
	Notice string `json:"notice,omitempty"`
	// Published is false for an asset of this repository's helpers release that no CI run has built yet:
	// its size and digest are not known, and the install must refuse it (reason not_published).
	Published bool `json:"published"`

	// Helper only, when the download is an archive: its format ("zip" or "tar.gz"), the files taken from it
	// by their exact names there (never by the names an archive offers) and the links made beside them.
	// ExtractHelper does the unpacking after the digest of the archive has matched.
	Format  string   `json:"format,omitempty"`
	Members []Member `json:"members,omitempty"`
	Links   []Link   `json:"links,omitempty"`
}

// Member is a file taken from an archive by its exact name there and written under To, a plain file name.
type Member struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Executable bool   `json:"executable"`
}

// Link is a symbolic link made beside the members (not on Windows), to a plain file name.
type Link struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// Manifest lists everything the product may download.
type Manifest struct {
	// Hosts is the allowlist of every hop of a download, redirects included.
	Hosts   []string        `json:"hosts"`
	Entries []ManifestEntry `json:"entries"`
}

// DefaultManifest is the manifest embedded in the binary. It is validated: every URL is https on a listed
// host, every published entry has its exact size and a SHA-256, every file and member name is plain.
func DefaultManifest() (Manifest, error) {
	return ParseManifest(manifestJSON)
}

// ParseManifest reads and validates a manifest.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("transcribe: the embedded manifest is not valid: %w", err)
	}

	for i := range m.Entries {
		if err := m.Entries[i].validate(m.Hosts); err != nil {
			return Manifest{}, fmt.Errorf("transcribe: manifest entry %d (%s): %w", i, m.Entries[i].File, err)
		}
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

func isHex64(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}

	_, err := hex.DecodeString(s)

	return err == nil
}

// plainName is a file name with no separator and no traversal.
func plainName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/:") && !strings.Contains(s, "\\") &&
		strings.IndexByte(s, 0) < 0
}

func (e *ManifestEntry) validate(hosts []string) error {
	u, err := url.Parse(e.URL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return errors.New("the URL is not an https URL")
	}

	if !slices.Contains(hosts, u.Hostname()) {
		return fmt.Errorf("the host %s is not in hosts", u.Hostname())
	}

	if !plainName(e.File) {
		return errors.New("the file name is not a plain name")
	}

	switch e.Kind {
	case KindHelper:
		if e.OS == "" || e.Arch == "" {
			return errors.New("a helper needs os and arch")
		}
	case KindModel:
		if !slices.Contains(Models, e.Model) {
			return fmt.Errorf("%q is not one of the models %v", e.Model, Models)
		}
	default:
		return fmt.Errorf("unknown kind %q", e.Kind)
	}

	if !e.Published {
		return nil
	}

	if e.Bytes <= 0 || !isHex64(e.SHA256) {
		return errors.New("a published entry needs its exact size and a SHA-256 of 64 lower-case hex characters")
	}

	return e.validateMembers()
}

func (e *ManifestEntry) validateMembers() error {
	if e.Format == "" {
		if len(e.Members) > 0 || len(e.Links) > 0 {
			return errors.New("members and links need an archive format")
		}

		return nil
	}

	if e.Format != "zip" && e.Format != "tar.gz" {
		return fmt.Errorf("unknown archive format %q", e.Format)
	}

	for _, m := range e.Members {
		if m.From == "" || !plainName(m.To) {
			return fmt.Errorf("member %q is not a plain name", m.To)
		}
	}

	for _, l := range e.Links {
		if !plainName(l.Name) || !plainName(l.Target) {
			return fmt.Errorf("link %q is not a plain name", l.Name)
		}
	}

	if !slices.ContainsFunc(e.Members, func(m Member) bool { return m.To == e.File }) {
		return errors.New("the executable (file) is not one of the members")
	}

	return nil
}
