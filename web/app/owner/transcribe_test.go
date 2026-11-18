package owner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
)

// installModel records a model of the manifest as installed, with real bytes under dir.
func installModel(t *testing.T, dir string, m transcribe.Manifest, model string) {
	t.Helper()

	installModels(t, dir, m, model)
}

// installModels records several models of the manifest as installed.
func installModels(t *testing.T, dir string, m transcribe.Manifest, models ...string) {
	t.Helper()

	var tools []service.InstalledTool

	for _, model := range models {
		var entry transcribe.ManifestEntry

		for i := range m.Entries {
			if e := &m.Entries[i]; e.Name == "whisper" && e.Kind == transcribe.KindModel && e.Model == model {
				entry = *e
			}
		}

		body := []byte("model " + model)
		sum := sha256.Sum256(body)
		rel := "whisper/" + model + ".bin"

		if err := os.MkdirAll(filepath.Join(dir, "whisper"), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), body, 0o600); err != nil {
			t.Fatal(err)
		}

		tools = append(tools, service.InstalledTool{
			Name:    "whisper",
			Kind:    entry.Kind,
			Model:   model,
			Version: "t",
			SHA256:  hex.EncodeToString(sum[:]),
			Path:    rel,
		})
	}

	raw, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "installed.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// The model that `tools:install whisper --model base` installed is the one transcribe uses
// when VUKEM_VIDEO_WHISPER_MODEL is not set (VV-007).
func TestWhisperProviderUsesTheInstalledModel(t *testing.T) {
	m, err := transcribe.DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	entries, ok := m.For("whisper", "small", runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("no whisper helper for this platform")
	}

	dir := t.TempDir()
	p := &whisperProvider{dir: dir, model: "small", manifest: m, entries: entries}

	if got := p.Model(); got != "small" {
		t.Errorf("nothing installed: model = %q, want the default small", got)
	}

	installModel(t, dir, m, "base")

	if got := p.Model(); got != "base" {
		t.Errorf("base installed: model = %q, want base", got)
	}

	if got := p.current().ModelName; got != "base" || p.current().ModelPath == "" {
		t.Errorf("current: %q, path %q", got, p.current().ModelPath)
	}

	p.explicit = true
	if got := p.Model(); got != "small" {
		t.Errorf("explicit small: model = %q", got)
	}
}

// The model recorded by the install wins over the preference order; a recorded model that is no
// longer installed falls back to it (VV-007).
func TestWhisperProviderPrefersTheRecordedDefault(t *testing.T) {
	m, err := transcribe.DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	entries, ok := m.For("whisper", "small", runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("no whisper helper for this platform")
	}

	dir := t.TempDir()
	p := &whisperProvider{dir: dir, model: "small", manifest: m, entries: entries}

	installModels(t, dir, m, "small", "tiny")

	if got := p.Model(); got != "small" {
		t.Errorf("nothing recorded: model = %q, want small (preference order)", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "defaults.json"), []byte(`{"whisper":"tiny"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := p.Model(); got != "tiny" {
		t.Errorf("tiny recorded: model = %q, want tiny", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "defaults.json"), []byte(`{"whisper":"base"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := p.Model(); got != "small" {
		t.Errorf("recorded base is not installed: model = %q, want the preference small", got)
	}
}
