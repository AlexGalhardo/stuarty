package aihost

import (
	_ "embed" // the catalogue is part of the binary
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/engine/transcribe"
)

//go:embed manifest.json
var manifestJSON []byte

// The install set of the face tracking feature and the three files in it (SDD-024 section 3).
const (
	// SetName is the name `tools:install` and the install route take for the set.
	SetName = "face"
	// ToolHelper, ToolRuntime and ToolModel are the names the files have in tools/installed.json.
	ToolHelper  = "aihelper"
	ToolRuntime = "onnxruntime"
	ToolModel   = "yunet"
	// ModelVersion is the model entry that the helper reads.
	ModelVersion = "2023mar"
)

// AllowedLicenses are the licences a catalogue entry may carry: the allowlist of ADR-0050 item 4 and of
// internal/tools/notices. Non-commercial, GPL, AGPL, gated or unstated licences are refused.
var AllowedLicenses = []string{"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "Zlib", "0BSD"}

// Entry is one pinned file of the catalogue: the entry of the whisper manifest, and for a model the
// dataset the weights were trained on, which the install prompt shows (ADR-0050 section 7).
type Entry struct {
	transcribe.ManifestEntry

	Dataset string `json:"dataset,omitempty"`
}

// Manifest is the catalogue of the local AI helper: what may be downloaded, from which hosts.
type Manifest struct {
	// Hosts is the allowlist of every hop of a download, redirects included.
	Hosts   []string `json:"hosts"`
	Entries []Entry  `json:"entries"`
}

// DefaultManifest is the catalogue embedded in the binary, validated.
func DefaultManifest() (Manifest, error) { return ParseManifest(manifestJSON) }

// ParseManifest reads and validates a catalogue.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("aihost: the embedded manifest is not valid: %w", err)
	}

	for i := range m.Entries {
		if err := m.Entries[i].validate(m.Hosts); err != nil {
			return Manifest{}, fmt.Errorf("aihost: manifest entry %d (%s): %w", i, m.Entries[i].File, err)
		}
	}

	return m, nil
}

func isHex64(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}

	_, err := hex.DecodeString(s)

	return err == nil
}

func plainName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/:\\") && !strings.Contains(s, "\x00")
}

func (e *Entry) validate(hosts []string) error {
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

	if !slices.Contains(AllowedLicenses, e.License) {
		return fmt.Errorf("the licence %q is not one the catalogue allows (ADR-0050 item 4)", e.License)
	}

	if strings.TrimSpace(e.Notice) == "" {
		return errors.New("the licence notice is missing")
	}

	switch {
	case e.Name == ToolHelper && e.Kind == transcribe.KindHelper,
		e.Name == ToolRuntime && e.Kind == transcribe.KindHelper:
		if e.OS == "" || e.Arch == "" {
			return errors.New("a helper needs os and arch")
		}
	case e.Name == ToolModel && e.Kind == transcribe.KindModel:
		if e.Model != ModelVersion || e.Dataset == "" {
			return fmt.Errorf("the model must be %s and name its dataset", ModelVersion)
		}
	default:
		return fmt.Errorf("%s of kind %s is not part of the face set", e.Name, e.Kind)
	}

	if !e.Published {
		return nil
	}

	if e.Bytes <= 0 || !isHex64(e.SHA256) {
		return errors.New("a published entry needs its exact size and a SHA-256 of 64 lower-case hex characters")
	}

	return e.validateMembers()
}

func (e *Entry) validateMembers() error {
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

	if !slices.ContainsFunc(e.Members, func(m transcribe.Member) bool { return m.To == e.File }) {
		return errors.New("the program (file) is not one of the members")
	}

	return nil
}

// Set returns the three entries of the install set for a platform, as the entries the install step of
// the service downloads: the helper, the runtime library and the model, in that order. ok is false when
// the catalogue has no helper or no runtime for the platform, or no model.
func (m Manifest) Set(goos, goarch string) (entries []transcribe.ManifestEntry, ok bool) {
	var helper, rt, model *Entry

	for i := range m.Entries {
		e := &m.Entries[i]

		switch {
		case e.Name == ToolHelper && e.OS == goos && e.Arch == goarch:
			helper = e
		case e.Name == ToolRuntime && e.OS == goos && e.Arch == goarch:
			rt = e
		case e.Name == ToolModel:
			model = e
		}
	}

	if helper == nil || rt == nil || model == nil {
		return nil, false
	}

	return []transcribe.ManifestEntry{helper.ManifestEntry, rt.ManifestEntry, model.ManifestEntry}, true
}

// Dataset is the dataset the model of the set was trained on, which the install prompt shows.
func (m Manifest) Dataset() string {
	for i := range m.Entries {
		if m.Entries[i].Name == ToolModel {
			return m.Entries[i].Dataset
		}
	}

	return ""
}
