package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
)

// Downloader does one HTTP request; in the product it is the client of
// outbound.HelperDownload, which is the only one that may dial.
type Downloader interface {
	Do(req *http.Request) (*http.Response, error)
}

// Tools is what the install step needs: the manifest that pins what may be
// downloaded, the client that does it, and the directory it installs into. The
// install step is the only code path that downloads (SDD-007 section 6.8); no
// job, no edit and no read starts it.
type Tools struct {
	// Dir is <workspace>/tools.
	Dir string
	// Manifest pins every file, its size and its SHA-256.
	Manifest transcribe.Manifest
	// Client downloads; in the product it is outbound.HelperDownload.
	Client Downloader
	// GOOS and GOARCH select the helper; the runtime's when empty.
	GOOS, GOARCH string
	// OpenH264 pins the Cisco library of the own FFmpeg build; the embedded manifest when it has no
	// entries. Its hosts are not those of Manifest: the client of the install is built over both.
	OpenH264 ffmpeg.OpenH264Manifest
}

const installedFile = "installed.json"

// ToolsInstallRequest names what to install.
type ToolsInstallRequest struct {
	Name  string `json:"name"`
	Model string `json:"model,omitempty"`
	// Force downloads again what is installed and verified.
	Force bool `json:"force,omitempty"`
}

// ToolFile is one file of an install, with what it costs.
type ToolFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Host  string `json:"host"`
}

// ToolsInstallResult is the answer of the install call: it announces the size
// and the source before the first byte arrives.
type ToolsInstallResult struct {
	State         string     `json:"state"` // "running", or "installed" when nothing had to be downloaded
	Files         []ToolFile `json:"files"`
	DownloadBytes int64      `json:"download_bytes"`
	Text          string     `json:"text"`
}

// InstalledTool is a line of tools/installed.json.
type InstalledTool struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Model   string `json:"model,omitempty"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Path    string `json:"path"` // slash path under tools/
}

// ToolsStatusRequest has no members.
type ToolsStatusRequest struct{}

// ToolsStatusResult is what is installed and how a running install is going.
type ToolsStatusResult struct {
	Installed []InstalledTool `json:"installed"`
	State     string          `json:"state"` // "idle", "running" or "failed"
	Done      int64           `json:"done_bytes"`
	Total     int64           `json:"total_bytes"`
	Error     *Error          `json:"error,omitempty"`
	Text      string          `json:"text"`
}

// toolsState is the progress of the one install that may run at a time.
type toolsState struct {
	mu      sync.Mutex
	running bool
	done    int64
	total   int64
	err     *Error
}

func (t *Tools) platform() (goos, goarch string) {
	goos, goarch = t.GOOS, t.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}

	if goarch == "" {
		goarch = runtime.GOARCH
	}

	return goos, goarch
}

// humanForTools is the caller class that may install: an admin at the browser (the local
// editor holds every scope; a network member does not, SDD-007 section 5.2), an admin key,
// or the operator at the terminal of the owner machine.
func humanForTools(c Caller) bool {
	admin := slices.Contains(c.Scopes, ScopeAdmin)

	return c.Browser && admin || c.Operator || strings.HasPrefix(c.Actor, "key:") && admin
}

func installDenied() *Error {
	return newError(CodeForbidden, "only a person can install the transcription helper.").
		withHint("the local token and the agent surfaces cannot start a download; ask the person to run " +
			"stuarty tools:install whisper in a terminal of this machine (it works while the server " +
			"runs) or to use the install button of the editor")
}

// ToolsInstaller is the install step alone, over a directory of tools and no store. The CLI uses it
// to install while a server owns the workspace: the process that runs the command is the person at
// the terminal, and the server finds the files on its next read of tools/installed.json (SDD-007
// section 6.8). The server itself still refuses the local token and the agent surfaces.
type ToolsInstaller interface {
	ToolsInstall(ctx context.Context, req ToolsInstallRequest) (*ToolsInstallResult, error)
	ToolsStatus(ctx context.Context, req ToolsStatusRequest) (*ToolsStatusResult, error)
}

// NewToolsInstaller returns the install step over t.
func NewToolsInstaller(t *Tools) ToolsInstaller {
	return &svc{tools: t, now: time.Now}
}

// auditInstall records the start of an install; the installer without a store records nothing.
func (s *svc) auditInstall(ctx context.Context, actor, name string) {
	if s.store == nil {
		return
	}

	_ = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, s.nowMs(), actor, "tools.install", "", name)
	})
}

// ToolsInstall announces what will be downloaded and starts the download in the
// background. It refuses every caller that is not a person (browser session,
// admin key, or the operator at the terminal).
func (s *svc) ToolsInstall(ctx context.Context, req ToolsInstallRequest) (*ToolsInstallResult, error) {
	c, err := authorizeScope(ctx, ScopeAdmin, "")
	if err != nil {
		return nil, err
	}

	if c.ProjectID != "" || !humanForTools(c) {
		return nil, installDenied()
	}

	if s.tools == nil || s.tools.Client == nil {
		return nil, newError(codeTranscribeOff, "installing helpers is not available in this build.")
	}

	if req.Name == ffmpeg.OpenH264Tool {
		return s.installOpenH264(ctx, c)
	}

	model := req.Model
	if model == "" {
		model = "small"
	}

	goos, goarch := s.tools.platform()

	entries, ok := s.tools.Manifest.For(req.Name, model, goos, goarch)
	if !ok {
		return nil, newError(
			command.CodeValidation,
			"there is no %q helper with model %q for %s/%s.",
			req.Name,
			model,
			goos,
			goarch,
		).
			withPath("name").
			withHint("the models this build can install: %s",
				strings.Join(s.tools.Manifest.Models(req.Name), ", "))
	}

	res := &ToolsInstallResult{State: "running"}

	for i := range entries {
		u, perr := url.Parse(entries[i].URL)
		if perr != nil || u.Scheme != "https" || !slices.Contains(s.tools.Manifest.Hosts, u.Hostname()) {
			return nil, internalError()
		}

		res.Files = append(res.Files, ToolFile{Name: entries[i].File, Bytes: entries[i].Bytes, Host: u.Hostname()})
		res.DownloadBytes += entries[i].Bytes
	}

	if !req.Force && s.allInstalled(entries) {
		return &ToolsInstallResult{
			State: "installed", Files: res.Files,
			Text: fmt.Sprintf("%s (%s) is already installed and its files verify; pass force to install it again",
				req.Name, model),
		}, nil
	}

	res.Text = fmt.Sprintf("installing %s (%s): %d files, %s from %s",
		req.Name, model, len(res.Files), sizesText(res.DownloadBytes), res.Files[0].Host)

	if err := s.beginInstall(res.DownloadBytes); err != nil {
		return nil, err
	}

	bg := context.WithoutCancel(ctx)

	go func() {
		ierr := s.installEntries(bg, entries)
		if ierr == nil {
			ierr = s.recordDefaultModel(req.Name, entries)
		}

		s.endInstall(ierr)
	}()

	s.auditInstall(ctx, c.Actor, req.Name)

	return res, nil
}

// allInstalled is true when every entry is recorded at its version and its file still has the recorded digest.
func (s *svc) allInstalled(entries []transcribe.ManifestEntry) bool {
	installed := readInstalled(s.tools.Dir)

	for i := range entries {
		e := &entries[i]

		recorded := slices.ContainsFunc(installed, func(t InstalledTool) bool {
			return t.Name == e.Name && t.Kind == e.Kind && t.Model == e.Model && t.Version == e.Version
		})
		if _, ok := VerifiedTool(s.tools.Dir, e.Name, e.Kind, e.Model); !recorded || !ok {
			return false
		}
	}

	return true
}

// ToolsStatus lists what is installed and the progress of a running install.
// It only reads, so any caller that may read may ask: an agent needs to know whether the helper is
// installed before it asks for a transcript (VV-080). Only the install is for a person.
func (s *svc) ToolsStatus(ctx context.Context, _ ToolsStatusRequest) (*ToolsStatusResult, error) {
	if _, err := authorize(ctx, "ToolsStatus", ""); err != nil {
		return nil, err
	}

	res := &ToolsStatusResult{State: "idle", Installed: []InstalledTool{}}

	// The list is read under the lock: the install records each file before it
	// clears running under the same lock, so an idle answer lists every file.
	s.toolsState.mu.Lock()
	defer s.toolsState.mu.Unlock()

	if s.tools != nil {
		res.Installed = readInstalled(s.tools.Dir)
	}

	res.Done, res.Total = s.toolsState.done, s.toolsState.total

	switch {
	case s.toolsState.running:
		res.State = "running"
	case s.toolsState.err != nil:
		res.State, res.Error = "failed", s.toolsState.err
	}

	res.Text = fmt.Sprintf("tools: %d installed, install %s", len(res.Installed), res.State)

	return res, nil
}

// installEntries downloads, verifies and moves each entry into place, then
// records it. A file that is not the exact size with the exact digest is
// deleted and nothing of it is kept.
func (s *svc) installEntries(ctx context.Context, entries []transcribe.ManifestEntry) *Error {
	if err := os.MkdirAll(filepath.Join(s.tools.Dir, ".download"), 0o700); err != nil {
		return internalError()
	}

	for i := range entries {
		if err := s.installOne(ctx, &entries[i]); err != nil {
			return err
		}
	}

	return nil
}

func (s *svc) installOne(ctx context.Context, e *transcribe.ManifestEntry) *Error {
	part := filepath.Join(s.tools.Dir, ".download", e.File+".part")
	defer func() { _ = os.Remove(part) }()

	got, derr := s.download(ctx, e, part)
	if derr != nil {
		return derr
	}

	// Compared in constant time, and only now is the file allowed anywhere it
	// could be used.
	if len(got) != len(e.SHA256) || subtle.ConstantTimeCompare([]byte(got), []byte(strings.ToLower(e.SHA256))) != 1 {
		return checksumMismatch(e)
	}

	dir := filepath.Join(s.tools.Dir, e.Name, e.Version)

	if e.Format != "" {
		return s.installArchive(e, part, dir)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return internalError()
	}

	final := filepath.Join(dir, e.File)
	if err := os.Rename(part, final); err != nil {
		return internalError()
	}

	if e.Kind == transcribe.KindHelper {
		if err := os.Chmod(final, 0o700); err != nil { //nolint:gosec // a helper must be executable, by its owner only
			return internalError()
		}
	}

	return s.recordInstalled(InstalledTool{
		Name: e.Name, Kind: e.Kind, Model: e.Model, Version: e.Version, SHA256: strings.ToLower(e.SHA256),
		Path: filepath.ToSlash(filepath.Join(e.Name, e.Version, e.File)),
	})
}

// installArchive unpacks a helper archive whose digest has already matched.
// It is unpacked into a new folder beside the download and moved into place
// only whole, so a failed unpack leaves nothing in tools/<name>/<version>/.
func (s *svc) installArchive(e *transcribe.ManifestEntry, archive, dir string) *Error {
	staging := filepath.Join(s.tools.Dir, ".download", e.Name+"-"+e.Version+".unpack")

	_ = os.RemoveAll(staging)
	defer func() { _ = os.RemoveAll(staging) }()

	if err := os.MkdirAll(staging, 0o700); err != nil {
		return internalError()
	}

	program, err := transcribe.ExtractHelper(archive, staging, e)
	if err != nil {
		return newError(codeTranscribeOff, "the downloaded archive %s does not hold what the manifest lists.", e.File).
			withDetail("reason", "bad_archive").
			withHint("try the install again; if it keeps failing the mirror serves a different file")
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return internalError()
	}

	if err := os.RemoveAll(dir); err != nil {
		return internalError()
	}

	if err := os.Rename(staging, dir); err != nil {
		return internalError()
	}

	return s.recordInstalled(InstalledTool{
		Name: e.Name, Kind: e.Kind, Model: e.Model, Version: e.Version, SHA256: program,
		Path: filepath.ToSlash(filepath.Join(e.Name, e.Version, e.File)),
	})
}

func checksumMismatch(e *transcribe.ManifestEntry) *Error {
	return newError(
		codeTranscribeOff,
		"the downloaded file %s is not the file the manifest pins and was deleted.",
		e.File,
	).
		withDetail("reason", "checksum_mismatch").
		withHint("try the install again; if it keeps failing the mirror serves a different file")
}

// download streams the body to the part file while it hashes it. The response
// is capped at the size of the manifest, so a longer body is cut and refused.
func (s *svc) download(ctx context.Context, e *transcribe.ManifestEntry, part string) (string, *Error) {
	u, err := url.Parse(e.URL)
	if err != nil || u.Scheme != "https" {
		return "", internalError()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, http.NoBody)
	if err != nil {
		return "", internalError()
	}

	resp, err := s.tools.Client.Do(req)
	if err != nil {
		return "", newError(codeTranscribeOff, "the download of %s failed.", e.File).
			withDetail("reason", "download_failed").withHint("check the connection and try again")
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", newError(codeTranscribeOff, "the download of %s answered %d.", e.File, resp.StatusCode).
			withDetail("reason", "download_failed")
	}

	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", internalError()
	}

	sum := sha256.New()

	n, cerr := io.Copy(io.MultiWriter(f, sum), &progressReader{r: io.LimitReader(resp.Body, e.Bytes+1), s: s})
	if err := f.Close(); err != nil && cerr == nil {
		cerr = err
	}

	if cerr != nil || n != e.Bytes {
		return "", checksumMismatch(e)
	}

	return hex.EncodeToString(sum.Sum(nil)), nil
}

type progressReader struct {
	r io.Reader
	s *svc
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)

	p.s.toolsState.mu.Lock()
	p.s.toolsState.done += int64(n)
	p.s.toolsState.mu.Unlock()

	return n, err
}

func readInstalled(dir string) []InstalledTool {
	raw, err := os.ReadFile(filepath.Join(dir, installedFile))
	if err != nil {
		return []InstalledTool{}
	}

	var out []InstalledTool
	if json.Unmarshal(raw, &out) != nil {
		return []InstalledTool{}
	}

	return out
}

func (s *svc) recordInstalled(t InstalledTool) *Error {
	all := readInstalled(s.tools.Dir)
	all = slices.DeleteFunc(
		all,
		func(o InstalledTool) bool { return o.Name == t.Name && o.Kind == t.Kind && o.Model == t.Model },
	)
	all = append(all, t)

	body, err := json.Marshal(all)
	if err != nil {
		return internalError()
	}

	if err := writeFileAtomic(filepath.Join(s.tools.Dir, installedFile), body); err != nil {
		return internalError()
	}

	return nil
}

const defaultsFile = "defaults.json"

// recordDefaultModel makes the model of a finished install the workspace default of that tool, so
// a job that names no model uses it (VV-007). An install without a model file records nothing.
func (s *svc) recordDefaultModel(name string, entries []transcribe.ManifestEntry) *Error {
	model := ""

	for i := range entries {
		if entries[i].Kind == transcribe.KindModel {
			model = entries[i].Model
		}
	}

	if model == "" {
		return nil
	}

	s.toolsState.mu.Lock()
	defer s.toolsState.mu.Unlock()

	all := readDefaults(s.tools.Dir)
	all[name] = model

	body, err := json.Marshal(all)
	if err != nil || writeFileAtomic(filepath.Join(s.tools.Dir, defaultsFile), body) != nil {
		return internalError()
	}

	return nil
}

func readDefaults(dir string) map[string]string {
	out := map[string]string{}

	if raw, err := os.ReadFile(filepath.Join(dir, defaultsFile)); err == nil {
		if json.Unmarshal(raw, &out) != nil || out == nil {
			out = map[string]string{}
		}
	}

	return out
}

// DefaultModel is the model the last install of the tool recorded as the workspace default. ok is
// false when none is recorded or the recorded model is no longer installed and verified.
func DefaultModel(dir, name string) (model string, ok bool) {
	model = readDefaults(dir)[name]
	if model == "" {
		return "", false
	}

	if _, found := VerifiedTool(dir, name, transcribe.KindModel, model); !found {
		return "", false
	}

	return model, true
}

// VerifiedTool returns the absolute path of an installed file after checking
// its SHA-256 against tools/installed.json: a helper is never run, and a model
// never read, before this succeeds (SDD-007 section 6.8 item 6). ok is false
// when it is not installed or the bytes changed.
func VerifiedTool(dir, name, kind, model string) (path string, ok bool) {
	for _, t := range readInstalled(dir) {
		if t.Name != name || t.Kind != kind || t.Model != model {
			continue
		}

		abs := filepath.Join(dir, filepath.FromSlash(t.Path))

		sum, err := fileSHA256(abs)
		if err != nil || len(sum) != len(t.SHA256) || subtle.ConstantTimeCompare([]byte(sum), []byte(t.SHA256)) != 1 {
			return "", false
		}

		return abs, true
	}

	return "", false
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
