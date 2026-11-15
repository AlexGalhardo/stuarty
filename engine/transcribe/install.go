package transcribe

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed" // the manifest is part of the binary
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

// Reasons of E_TRANSCRIBE_UNAVAILABLE (details.reason).
const (
	ReasonChecksumMismatch = "checksum_mismatch"
	ReasonSizeMismatch     = "size_mismatch"
	ReasonNotPublished     = "not_published"
	ReasonDownloadRefused  = "download_refused"
	ReasonDigestMismatch   = "digest_mismatch"
	ReasonBadArchive       = "bad_archive"
	ReasonNotInstalled     = "not_installed"
)

// Names of what is installed; the models are the only names `--model` and VUKEM_VIDEO_WHISPER_MODEL accept.
const (
	HelperName   = "whisper"
	DefaultModel = "small"

	maxMemberBytes  = 512 << 20
	maxArchiveBytes = 2 << 30
)

// Models are the model names of the manifest.
var Models = []string{"tiny", "base", "small", "large-v3-turbo"}

// SupportedPlatforms are the operating systems and architectures the helper is installed for; the
// manifest has an entry for each (SDD-007 section 6.8 item 7).
var SupportedPlatforms = [][2]string{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

//go:embed manifest.json
var manifestJSON []byte

// Manifest is the pinned list of helpers and models, embedded in the binary (SDD-011 section 10.2).
type Manifest struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Entry is one helper or model. A custom URL does not exist: the install takes only what is here.
type Entry struct {
	Kind    string `json:"kind"` // helper or model
	Name    string `json:"name"`
	Version string `json:"version"`
	OS      string `json:"os,omitempty"`
	Arch    string `json:"arch,omitempty"`
	URL     string `json:"url"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	// Published is false for an asset of this repository's helpers release that no CI run has built yet:
	// its size and digest are not known, and the install refuses it.
	Published bool     `json:"published"`
	Licence   string   `json:"licence"`
	Notice    string   `json:"notice"`
	Hosts     []string `json:"hosts"`

	// Helper only: the archive format, the files taken from it and the links made beside them.
	Format     string   `json:"format,omitempty"`
	Members    []Member `json:"members,omitempty"`
	Links      []Link   `json:"links,omitempty"`
	Executable string   `json:"executable,omitempty"`
	// File is the name of a model under tools/models.
	File string `json:"file,omitempty"`
}

// Member is a file taken from an archive by its exact name there and written under To, a plain file name.
type Member struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Executable bool   `json:"executable"`
}

// Link is a symbolic link made beside the members (Unix only), to a name that is a plain file name.
type Link struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

var (
	manifestOnce sync.Once
	manifest     *Manifest
	errManifest  error
)

// LoadManifest reads and validates the embedded manifest.
func LoadManifest() (*Manifest, error) {
	manifestOnce.Do(func() {
		manifest, errManifest = ParseManifest(manifestJSON)
	})

	return manifest, errManifest
}

// ParseManifest reads a manifest and checks every entry.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("transcribe: manifest: %w", err)
	}

	for i := range m.Entries {
		if err := m.Entries[i].validate(); err != nil {
			return nil, fmt.Errorf("transcribe: manifest entry %d (%s): %w", i, m.Entries[i].Name, err)
		}
	}

	return &m, nil
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
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\:\x00")
}

func (e *Entry) validate() error {
	u, err := url.Parse(e.URL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return errors.New("the URL is not an https URL")
	}

	if !slices.Contains(e.Hosts, u.Hostname()) {
		return fmt.Errorf("the host %s is not in hosts", u.Hostname())
	}

	switch e.Kind {
	case "helper":
		if e.OS == "" || e.Arch == "" || !plainName(e.Executable) {
			return errors.New("a helper needs os, arch and executable")
		}
	case "model":
		if !slices.Contains(Models, e.Name) || !plainName(e.File) {
			return errors.New("a model has one of the known names and a plain file name")
		}
	default:
		return fmt.Errorf("unknown kind %q", e.Kind)
	}

	if !e.Published {
		return nil
	}

	if e.Size <= 0 || !isHex64(e.SHA256) {
		return errors.New("a published entry needs its exact size and a SHA-256 of 64 lower-case hex characters")
	}

	if e.Kind == "helper" {
		return e.validateMembers()
	}

	return nil
}

func (e *Entry) validateMembers() error {
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

	if !slices.ContainsFunc(e.Members, func(m Member) bool { return m.To == e.Executable }) {
		return errors.New("the executable is not one of the members")
	}

	return nil
}

// Helper is the entry of the helper for a platform.
func (m *Manifest) Helper(goos, goarch string) (*Entry, error) {
	for i := range m.Entries {
		if e := &m.Entries[i]; e.Kind == "helper" && e.OS == goos && e.Arch == goarch {
			return e, nil
		}
	}

	return nil, fmt.Errorf("transcribe: no helper for %s/%s", goos, goarch)
}

// Model is the entry of a model; only the names of Models exist.
func (m *Manifest) Model(name string) (*Entry, error) {
	for i := range m.Entries {
		if e := &m.Entries[i]; e.Kind == "model" && e.Name == name {
			return e, nil
		}
	}

	return nil, fmt.Errorf("transcribe: %q is not a model of the manifest (%s)", name, strings.Join(Models, ", "))
}

// Announce is what the install says before it downloads: the size and the source (SDD-011 section 10.2).
func Announce(entries ...*Entry) string {
	var total int64

	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		total += e.Size
		parts = append(
			parts,
			fmt.Sprintf("%s %s (%.1f MB) from %s", e.Name, e.Version, float64(e.Size)/1e6, hostOf(e.URL)),
		)
	}

	return fmt.Sprintf("This downloads %s: %.1f MB in all.", strings.Join(parts, " and "), float64(total)/1e6)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "an unknown host"
	}

	return u.Hostname()
}

// Installer downloads what the manifest pins into the tools folder. It is run by an explicit step of a
// person or an operator, never by a job (SDD-007 section 6.8).
type Installer struct {
	// ToolsDir is <workspace>/tools.
	ToolsDir string
	// Manifest is the embedded one; tests give their own.
	Manifest *Manifest
	// OS and Arch default to the machine's.
	OS, Arch string
	// Progress receives the bytes downloaded and the size.
	Progress func(done, total int64)
	// Test trusts a local server; production code leaves it nil.
	Test *outbound.TestOptions
}

func unavailable(reason, msg string, cause error, details map[string]any) *ffmpeg.Error {
	d := map[string]any{"reason": reason}
	maps.Copy(d, details)

	return &ffmpeg.Error{Code: CodeUnavailable, Reason: reason, Message: msg, Details: d, Err: cause}
}

// InstallWhisper installs the helper of this platform and then the model.
func (in *Installer) InstallWhisper(ctx context.Context, model string) error {
	m := in.Manifest
	if m == nil {
		var err error
		if m, err = LoadManifest(); err != nil {
			return err
		}
	}

	goos, goarch := in.OS, in.Arch
	if goos == "" {
		goos, goarch = runtime.GOOS, runtime.GOARCH
	}

	h, err := m.Helper(goos, goarch)
	if err != nil {
		return err
	}

	md, err := m.Model(model)
	if err != nil {
		return err
	}

	if err := in.Install(ctx, h); err != nil {
		return err
	}

	return in.Install(ctx, md)
}

// Install downloads one entry, checks its size and digest, and only then puts it in place and records it.
// Any failure leaves nothing executable behind.
func (in *Installer) Install(ctx context.Context, e *Entry) error {
	if !e.Published {
		return unavailable(ReasonNotPublished, "This helper has not been published for this platform yet.", nil,
			map[string]any{"name": e.Name, "os": e.OS, "arch": e.Arch})
	}

	if err := e.validate(); err != nil {
		return unavailable(ReasonDownloadRefused, "The manifest entry is not valid.", err, nil)
	}

	dl := filepath.Join(in.ToolsDir, ".download")
	if err := os.MkdirAll(dl, 0o700); err != nil {
		return unavailable(ReasonDownloadRefused, "The download folder could not be made.", err, nil)
	}

	part := filepath.Join(dl, e.Name+".part")
	_ = os.Remove(part)

	defer func() { _ = os.Remove(part) }()

	if err := in.fetch(ctx, e, part); err != nil {
		return err
	}

	var (
		rec Installed
		err error
	)
	if e.Kind == "model" {
		rec, err = in.placeModel(e, part)
	} else {
		rec, err = in.placeHelper(e, part, dl)
	}

	if err != nil {
		return err
	}

	return in.record(rec)
}

// fetch streams the URL into path while hashing it, and checks size and digest.
func (in *Installer) fetch(ctx context.Context, e *Entry, path string) error {
	client := outbound.HelperDownload(outbound.HelperOptions{
		Hosts: e.Hosts, Timeout: 2 * time.Hour, MaxRedirects: 5, Test: in.Test,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, http.NoBody)
	if err != nil {
		return unavailable(ReasonDownloadRefused, "The download address is not valid.", err, nil)
	}

	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return unavailable(ReasonDownloadRefused, "The download was refused.", err, nil)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return unavailable(ReasonDownloadRefused, "The server did not answer with the file.", nil,
			map[string]any{"status": res.StatusCode})
	}

	if res.ContentLength >= 0 && res.ContentLength != e.Size {
		return unavailable(ReasonSizeMismatch, "The file is not the size the manifest pins.", nil,
			map[string]any{"expected": e.Size, "got": res.ContentLength})
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return unavailable(ReasonDownloadRefused, "The download file could not be made.", err, nil)
	}

	h := sha256.New()
	// One byte more than the manifest size, so a longer body is seen and refused.
	src := &progressReader{r: io.LimitReader(res.Body, e.Size+1), total: e.Size, on: in.Progress}
	n, copyErr := io.Copy(io.MultiWriter(f, h), src)

	if err := f.Close(); copyErr == nil {
		copyErr = err
	}

	if copyErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return unavailable(ReasonDownloadRefused, "The download was interrupted.", copyErr, nil)
	}

	if n != e.Size {
		return unavailable(ReasonSizeMismatch, "The file is not the size the manifest pins.", nil,
			map[string]any{"expected": e.Size, "got": n})
	}

	want, _ := hex.DecodeString(e.SHA256)
	if subtle.ConstantTimeCompare(h.Sum(nil), want) != 1 {
		return unavailable(ReasonChecksumMismatch, "The file does not match the checksum the manifest pins.", nil, nil)
	}

	return nil
}

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	on    func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)

	p.done += int64(n)
	if p.on != nil {
		p.on(min(p.done, p.total), p.total)
	}

	return n, err
}

func (in *Installer) placeModel(e *Entry, part string) (Installed, error) {
	dir := filepath.Join(in.ToolsDir, "models")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Installed{}, unavailable(ReasonDownloadRefused, "The models folder could not be made.", err, nil)
	}

	if err := os.Rename(part, filepath.Join(dir, e.File)); err != nil {
		return Installed{}, unavailable(ReasonDownloadRefused, "The model could not be put in place.", err, nil)
	}

	return Installed{
		Name: e.Name, Kind: "model", Version: e.Version, Path: "models/" + e.File, SHA256: e.SHA256, Size: e.Size,
	}, nil
}

// placeHelper unpacks the members the manifest lists, by the names the manifest gives and never by the
// names the archive carries, into a folder beside the destination, and renames it into place.
func (in *Installer) placeHelper(e *Entry, part, dl string) (Installed, error) {
	stage := filepath.Join(dl, e.Name+".partial")
	_ = os.RemoveAll(stage)

	if err := os.MkdirAll(stage, 0o700); err != nil {
		return Installed{}, unavailable(ReasonBadArchive, "The unpack folder could not be made.", err, nil)
	}

	defer func() { _ = os.RemoveAll(stage) }()

	var err error

	switch e.Format {
	case "zip":
		err = unzip(part, stage, e.Members)
	case "tar.gz":
		err = untar(part, stage, e.Members)
	default:
		err = fmt.Errorf("unknown archive format %q", e.Format)
	}

	if err != nil {
		return Installed{}, unavailable(
			ReasonBadArchive,
			"The archive does not hold what the manifest lists.",
			err,
			nil,
		)
	}

	if runtime.GOOS != "windows" {
		for _, l := range e.Links {
			if err := os.Symlink(l.Target, filepath.Join(stage, l.Name)); err != nil {
				return Installed{}, unavailable(ReasonBadArchive, "A link of the helper could not be made.", err, nil)
			}
		}
	}

	sum, err := fileSHA256(filepath.Join(stage, e.Executable))
	if err != nil {
		return Installed{}, unavailable(ReasonBadArchive, "The helper program is missing.", err, nil)
	}

	rel := filepath.ToSlash(filepath.Join(HelperName, e.Version, e.OS+"-"+e.Arch))
	dst := filepath.Join(in.ToolsDir, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return Installed{}, unavailable(ReasonBadArchive, "The tools folder could not be made.", err, nil)
	}

	if err := os.RemoveAll(dst); err != nil {
		return Installed{}, unavailable(ReasonBadArchive, "The old helper could not be removed.", err, nil)
	}

	if err := os.Rename(stage, dst); err != nil {
		return Installed{}, unavailable(ReasonBadArchive, "The helper could not be put in place.", err, nil)
	}

	return Installed{
		Name: e.Name, Kind: "helper", Version: e.Version, OS: e.OS, Arch: e.Arch,
		Path: rel + "/" + e.Executable, SHA256: sum,
	}, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// memberTable maps the name an archive entry must have to the member the manifest names for it.
func memberTable(members []Member) map[string]Member {
	t := make(map[string]Member, len(members))
	for _, m := range members {
		t[m.From] = m
	}

	return t
}

func writeMember(dir string, m Member, r io.Reader) error {
	mode := os.FileMode(0o600)
	if m.Executable {
		mode = 0o700
	}

	f, err := os.OpenFile(filepath.Join(dir, m.To), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}

	n, err := io.Copy(f, io.LimitReader(r, maxMemberBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}

	if err == nil && n > maxMemberBytes {
		err = fmt.Errorf("member %s is over %d bytes", m.To, maxMemberBytes)
	}

	return err
}

func unzip(archive, dir string, members []Member) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}

	defer func() { _ = zr.Close() }()

	want := memberTable(members)
	done := map[string]bool{}

	var total uint64

	for _, f := range zr.File {
		m, ok := want[f.Name]
		if !ok || !f.Mode().IsRegular() || done[f.Name] {
			continue
		}

		if total += f.UncompressedSize64; total > maxArchiveBytes {
			return errors.New("the archive unpacks to too much")
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		err = writeMember(dir, m, rc)
		_ = rc.Close()

		if err != nil {
			return err
		}

		done[f.Name] = true
	}

	return allDone(members, done)
}

func untar(archive, dir string, members []Member) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}

	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	want := memberTable(members)
	done := map[string]bool{}

	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		m, ok := want[h.Name]
		if !ok || h.Typeflag != tar.TypeReg || done[h.Name] {
			continue
		}

		if err := writeMember(dir, m, tr); err != nil {
			return err
		}

		done[h.Name] = true
	}

	return allDone(members, done)
}

func allDone(members []Member, done map[string]bool) error {
	for _, m := range members {
		if !done[m.From] {
			return fmt.Errorf("the member %q is not in the archive", m.From)
		}
	}

	return nil
}
