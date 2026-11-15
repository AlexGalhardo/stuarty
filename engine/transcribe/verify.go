package transcribe

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

const installedFile = "installed.json"

// Installed is one thing the install step put under tools/.
type Installed struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Version string `json:"version"`
	OS      string `json:"os,omitempty"`
	Arch    string `json:"arch,omitempty"`
	// Path is relative to the tools folder, with forward slashes.
	Path string `json:"path"`
	// SHA256 is the digest of the file at Path: the helper program, or the model.
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,omitempty"`
}

// installedList is tools/installed.json.
type installedList struct {
	Version int         `json:"version"`
	Items   []Installed `json:"items"`
}

// ReadInstalled reads tools/installed.json; a missing file is an empty list.
func ReadInstalled(toolsDir string) ([]Installed, error) {
	data, err := os.ReadFile(filepath.Join(toolsDir, installedFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var l installedList
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("transcribe: %s: %w", installedFile, err)
	}

	return l.Items, nil
}

// record adds or replaces the entry of what was installed.
func (in *Installer) record(rec Installed) error {
	items, err := ReadInstalled(in.ToolsDir)
	if err != nil {
		items = nil // a broken record is rewritten from what is installed now
	}

	items = slices.DeleteFunc(items, func(x Installed) bool {
		return x.Kind == rec.Kind && x.Name == rec.Name && x.OS == rec.OS && x.Arch == rec.Arch
	})
	items = append(items, rec)

	data, err := json.MarshalIndent(installedList{Version: 1, Items: items}, "", "  ")
	if err != nil {
		return err
	}

	tmp := filepath.Join(in.ToolsDir, installedFile+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return unavailable(ReasonDownloadRefused, "The record of installed tools could not be written.", err, nil)
	}

	return os.Rename(tmp, filepath.Join(in.ToolsDir, installedFile))
}

// Status is what doctor prints for the transcription helper.
type Status struct {
	HelperPath string
	ModelPath  string
	// Problem says why something is unavailable when it is installed but not usable, for example a
	// digest that no longer matches tools/installed.json; empty when it is absent or fine.
	Problem string
}

// ResolveWhisper makes the provider from what is installed. Nothing is downloaded: a helper or a model
// that is absent leaves the path empty, so Check reports it (E_TRANSCRIBE_UNAVAILABLE with the install
// command). The digest of the helper program is checked against tools/installed.json; a mismatch makes
// the helper unavailable and Status.Problem says why (SDD-007 section 6.8 item 6).
func ResolveWhisper(toolsDir string, m *Manifest, model string) (*Whisper, Status, error) {
	if model == "" {
		model = DefaultModel
	}

	h, err := m.Helper(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, Status{}, err
	}

	md, err := m.Model(model)
	if err != nil {
		return nil, Status{}, err
	}

	w := &Whisper{
		HelperVersion: h.Version, ModelName: md.Name, ModelSHA256: md.SHA256,
		HelperItem: Item{Name: HelperName, URLHost: hostOf(h.URL), Bytes: h.Size},
		ModelItem:  Item{Name: md.Name, URLHost: hostOf(md.URL), Bytes: md.Size},
	}

	items, err := ReadInstalled(toolsDir)
	if err != nil {
		return w, Status{Problem: err.Error()}, nil //nolint:nilerr // an unreadable record means nothing is installed
	}

	var st Status

	for _, it := range items {
		path := filepath.Join(toolsDir, filepath.FromSlash(it.Path))

		switch {
		case it.Kind == "helper" && it.OS == runtime.GOOS && it.Arch == runtime.GOARCH:
			if problem := verifyFile(path, it.SHA256); problem != "" {
				st.Problem = "the helper " + problem

				continue
			}

			w.Helper, st.HelperPath = path, path
			if runtime.GOOS == "linux" {
				w.Env = map[string]string{"LD_LIBRARY_PATH": filepath.Dir(path)}
			}
		case it.Kind == "model" && it.Name == md.Name:
			if fi, err := os.Stat(path); err != nil || fi.Size() != md.Size || it.SHA256 != md.SHA256 {
				st.Problem = "the model " + md.Name + " does not match the manifest"

				continue
			}

			w.ModelPath, st.ModelPath = path, path
		}
	}

	return w, st, nil
}

// verifyFile compares the digest of a file with the recorded one; the empty string means it matches.
func verifyFile(path, want string) string {
	got, err := fileSHA256(path)

	switch {
	case errors.Is(err, os.ErrNotExist):
		return "is recorded as installed but its file is missing"
	case err != nil:
		return "cannot be read: " + err.Error()
	case subtle.ConstantTimeCompare([]byte(got), []byte(strings.ToLower(want))) != 1:
		return "program does not match its recorded SHA-256 (" + ReasonDigestMismatch + ")"
	}

	return ""
}

// UnavailableError is the E_TRANSCRIBE_UNAVAILABLE of a missing helper or model: what is missing, its
// download size and the command that installs it (SDD-007 section 6.8 item 2).
func UnavailableError(m *MissingError, model string) *ffmpeg.Error {
	names := make([]string, 0, len(m.Items))
	needModel := false

	for _, it := range m.Items {
		names = append(names, it.Name)
		needModel = needModel || it.Name != HelperName
	}

	cmd := "vukem-video tools:install whisper"
	if needModel {
		cmd += " --model " + model
	}

	e := unavailable(ReasonNotInstalled,
		"Local transcription needs "+strings.Join(names, " and ")+", which is not installed.", m,
		map[string]any{"missing": names, "download_bytes": m.DownloadBytes, "install_command": cmd})
	e.Hint = fmt.Sprintf(
		"Run %q, choose a smaller model with VUKEM_VIDEO_WHISPER_MODEL, or configure an API provider.",
		cmd,
	)

	return e
}
