package ffmpeg

import (
	"crypto/sha256"
	_ "embed" // the manifest is part of the binary
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

//go:embed manifest.json
var openH264ManifestJSON []byte

// The Cisco OpenH264 library of the own FFmpeg build (D-017, ADR-0011). The build does not carry it: it is
// downloaded by an explicit install step, never by an export, a transcription or an agent call, and the
// build loads it at run time from the directory the runner names in EnvOpenH264Dir.
const (
	// OpenH264Tool is the name of the install step: "vukem-video tools:install openh264".
	OpenH264Tool = "openh264"
	// EnvOpenH264Dir is the variable that tells the own build where the verified library is. It is set
	// only for the tools of an installation whose library passed verification, and only the own build reads it.
	EnvOpenH264Dir = "VUKEM_VIDEO_OPENH264_DIR"
	// OpenH264InstallCommand is what an error says to run.
	OpenH264InstallCommand = "vukem-video tools:install " + OpenH264Tool
)

// Reasons of the errors of the OpenH264 library (details.reason).
const (
	ReasonOpenH264NotInstalled = "openh264_not_installed"
	ReasonOpenH264Changed      = "openh264_changed"
	ReasonOpenH264Platform     = "openh264_unsupported_platform"
	ReasonChecksumMismatch     = "checksum_mismatch"
	ReasonSizeMismatch         = "size_mismatch"
	ReasonNotPublished         = "not_published"
	ReasonDownloadFailed       = "download_failed"
	ReasonBadArchive           = "bad_archive"
)

// OpenH264Entry pins the download of the Cisco library for one platform: the compressed file (size and
// SHA-256 of what the host serves) and the library inside it (size and SHA-256 of what is run).
type OpenH264Entry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	URL     string `json:"url"`
	Bytes   int64  `json:"bytes"`
	SHA256  string `json:"sha256"`
	// File is the name of the library under tools/openh264/<version>/, the name the own build loads.
	File          string `json:"file"`
	LibraryBytes  int64  `json:"library_bytes"`
	LibrarySHA256 string `json:"library_sha256"`
	License       string `json:"license"`
	// Published is false for an entry whose size and digests are not known yet: the install refuses it.
	Published bool `json:"published"`
}

// OpenH264Manifest lists what the install step may download for the own build (SDD-007 6.8).
type OpenH264Manifest struct {
	// Hosts is the allowlist of every hop of the download, redirects included.
	Hosts   []string        `json:"hosts"`
	Entries []OpenH264Entry `json:"entries"`
}

// DefaultOpenH264Manifest is the embedded manifest, validated.
func DefaultOpenH264Manifest() (OpenH264Manifest, error) {
	return ParseOpenH264Manifest(openH264ManifestJSON)
}

// ParseOpenH264Manifest reads and validates a manifest: https URLs on listed hosts only, plain file
// names, and for a published entry the exact sizes and the digests in lower-case hex.
func ParseOpenH264Manifest(data []byte) (OpenH264Manifest, error) {
	var m OpenH264Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return OpenH264Manifest{}, fmt.Errorf("ffmpeg: the embedded OpenH264 manifest is not valid: %w", err)
	}
	for i := range m.Entries {
		if err := m.Entries[i].validate(m.Hosts); err != nil {
			return OpenH264Manifest{}, fmt.Errorf("ffmpeg: OpenH264 manifest entry %d: %w", i, err)
		}
	}
	return m, nil
}

// For returns the entry of a platform.
func (m OpenH264Manifest) For(goos, goarch string) (OpenH264Entry, bool) {
	for i := range m.Entries {
		if e := m.Entries[i]; e.OS == goos && e.Arch == goarch {
			return e, true
		}
	}
	return OpenH264Entry{}, false
}

func isHex64(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func plainName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\:") && !strings.Contains(s, "\x00")
}

func (e *OpenH264Entry) validate(hosts []string) error {
	u, err := url.Parse(e.URL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return errors.New("the URL is not an https URL")
	}
	if !slices.Contains(hosts, u.Hostname()) {
		return fmt.Errorf("the host %s is not in hosts", u.Hostname())
	}
	if e.Name != OpenH264Tool || e.OS == "" || e.Arch == "" || !plainName(e.File) || !plainName(e.Version) {
		return errors.New("the entry needs the name openh264, an os, an arch, a plain version and a plain file name")
	}
	if !e.Published {
		return nil
	}
	if e.Bytes <= 0 || e.LibraryBytes <= 0 || !isHex64(e.SHA256) || !isHex64(e.LibrarySHA256) {
		return errors.New("a published entry needs both exact sizes and both SHA-256 digests in lower-case hex")
	}
	return nil
}

// OpenH264Path is where the verified library of an entry lives under the tools directory.
func OpenH264Path(toolsDir string, e OpenH264Entry) string {
	return filepath.Join(toolsDir, OpenH264Tool, e.Version, e.File)
}

// OpenH264State is the answer of CheckOpenH264.
type OpenH264State struct {
	// Dir is the directory that holds the library; empty unless Installed.
	Dir       string
	Installed bool
	// Reason is empty when Installed, else ReasonOpenH264NotInstalled, ReasonOpenH264Changed or
	// ReasonOpenH264Platform.
	Reason string
	Entry  OpenH264Entry
}

// CheckOpenH264 says whether the library of this platform is installed and unchanged: the file must
// exist with the exact size of the manifest and its SHA-256 must match, which is checked at each start
// (SDD-007 6.8 item 6). It reads files only and never starts a download.
func CheckOpenH264(toolsDir string, m OpenH264Manifest, goos, goarch string) OpenH264State {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	e, ok := m.For(goos, goarch)
	if !ok || !e.Published {
		return OpenH264State{Reason: ReasonOpenH264Platform, Entry: e}
	}
	st := OpenH264State{Entry: e, Reason: ReasonOpenH264NotInstalled}
	if toolsDir == "" {
		return st
	}
	path := OpenH264Path(toolsDir, e)
	info, err := os.Lstat(path)
	if err != nil {
		return st
	}
	// Anything at the path that is not the regular file the install wrote (a directory, a link) was put there.
	st.Reason = ReasonOpenH264Changed
	if !info.Mode().IsRegular() || info.Size() != e.LibraryBytes {
		return st
	}
	f, err := os.Open(path)
	if err != nil {
		return st
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, e.LibraryBytes+1)); err != nil ||
		hex.EncodeToString(h.Sum(nil)) != e.LibrarySHA256 {
		return st
	}
	return OpenH264State{Dir: filepath.Dir(path), Installed: true, Entry: e}
}

// openH264Error is E_NO_ENCODER for a library that is missing or changed: what to run, what it costs,
// and the keys of SDD-007 6.8 item 2 (missing, download_bytes, install_command, tool).
func openH264Error(st OpenH264State) *Error {
	e := &Error{Code: CodeNoEncoder, Reason: st.Reason}
	switch st.Reason {
	case ReasonOpenH264Platform:
		e.Message = "The OpenH264 library that this FFmpeg build loads is not available for " + runtime.GOOS + "/" +
			runtime.GOARCH + "."
		e.Hint = "Use the hardware or operating system encoder of this machine, or point " + EnvFFmpeg +
			" at an FFmpeg that has libx264 and set VUKEM_VIDEO_ALLOW_USER_X264=true."
		e.Details = map[string]any{"reason": st.Reason, "tool": OpenH264Tool}
		return e
	case ReasonOpenH264Changed:
		e.Message = "The OpenH264 library on disk is not the file the manifest pins, so it is not used."
	default:
		e.Message = "This FFmpeg build encodes H.264 with Cisco OpenH264, a library that is not installed."
	}
	e.Hint = fmt.Sprintf("Run %q (it downloads %.1f MB from %s once; nothing downloads it automatically).",
		OpenH264InstallCommand, float64(st.Entry.Bytes)/1e6, hostOfURL(st.Entry.URL))
	e.Details = map[string]any{
		"reason": st.Reason, "missing": []string{OpenH264Tool}, "download_bytes": st.Entry.Bytes,
		"install_command": OpenH264InstallCommand, "tool": OpenH264Tool,
	}
	return e
}

func hostOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "an unknown host"
	}
	return u.Hostname()
}

// openH264Reason is the reason of a library that is not usable; a state nobody checked is not installed.
func openH264Reason(st OpenH264State) string {
	if st.Reason == "" {
		return ReasonOpenH264NotInstalled
	}
	return st.Reason
}
