package owner

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service"
)

// installTimeout bounds one download of the install step.
const installTimeout = 30 * time.Minute

// tools builds the install step: the manifest embedded in the binary, the
// directory under the workspace and the one client that may download. A build
// whose manifest pins no helper for this platform gets no install step.
func (o *Owner) tools(model string, explicit bool) (*service.Tools, transcribe.Provider, error) {
	t, err := InstallTools(string(o.Dir))
	if err != nil {
		return nil, nil, err
	}

	manifest, dir := t.Manifest, t.Dir

	entries, ok := manifest.For("whisper", model, runtime.GOOS, runtime.GOARCH)
	if !ok {
		return t, nil, nil
	}

	return t, &whisperProvider{dir: dir, model: model, explicit: explicit, manifest: manifest, entries: entries}, nil
}

// InstallTools is the install step of the workspace directory workspaceDir. The CLI builds it to
// install while a server owns the workspace.
func InstallTools(workspaceDir string) (*service.Tools, error) {
	manifest, err := transcribe.DefaultManifest()
	if err != nil {
		return nil, err
	}

	openh264, err := ffmpeg.DefaultOpenH264Manifest()
	if err != nil {
		return nil, err
	}

	// One client downloads for both manifests, over the union of their hosts; each manifest may
	// name only its own (SDD-007 section 6.8).
	hosts := slices.Concat(manifest.Hosts, openh264.Hosts)

	return &service.Tools{
		Dir: filepath.Join(workspaceDir, "tools"), Manifest: manifest, OpenH264: openh264,
		Client: outbound.HelperDownload(hosts, installTimeout),
	}, nil
}

// modelPreference is the order in which an installed model is taken when
// STUARTY_WHISPER_MODEL is not set: the default first, then the smaller ones.
var modelPreference = []string{"small", "base", "tiny", "large-v3-turbo"}

// whisperProvider is the whisper.cpp provider over what is installed now. It
// looks the installed files up again on every call, so an install made while
// the server runs is picked up, and it verifies the SHA-256 of the helper and
// of the model before anything is run or read: a changed file makes the helper
// unavailable (SDD-007 section 6.8 item 6).
type whisperProvider struct {
	dir string
	// model is the name STUARTY_WHISPER_MODEL asked for, or the default when it was not set.
	model string
	// explicit is true when the variable was set: then model is used as it is, installed or not.
	explicit bool
	manifest transcribe.Manifest
	entries  []transcribe.ManifestEntry
	ffmpeg   *ffmpeg.Runner
}

// resolve is the model and manifest entries in force now. Without the variable it is the
// model the last install recorded as the workspace default, else the first installed one in
// modelPreference, found again on every call like the files themselves, and the built-in
// default only when none is installed (VV-007).
func (p *whisperProvider) resolve() (string, []transcribe.ManifestEntry) {
	if p.explicit {
		return p.model, p.entries
	}

	order := modelPreference
	if def, ok := service.DefaultModel(p.dir, "whisper"); ok {
		order = slices.Concat([]string{def}, modelPreference)
	}

	for _, m := range order {
		entries, ok := p.manifest.For("whisper", m, runtime.GOOS, runtime.GOARCH)
		if !ok {
			continue
		}

		for i := range entries {
			if entries[i].Kind != transcribe.KindModel {
				continue
			}

			if _, found := service.VerifiedTool(p.dir, entries[i].Name, entries[i].Kind, entries[i].Model); found {
				return m, entries
			}
		}
	}

	return p.model, p.entries
}

// InstalledModels are the models whose file is installed and verified now (service.ModelPicker).
func (p *whisperProvider) InstalledModels() []string {
	var out []string

	for _, m := range transcribe.Models {
		entries, ok := p.manifest.For("whisper", m, runtime.GOOS, runtime.GOARCH)
		if !ok {
			continue
		}

		for i := range entries {
			if entries[i].Kind != transcribe.KindModel {
				continue
			}

			if _, found := service.VerifiedTool(p.dir, entries[i].Name, entries[i].Kind, entries[i].Model); found {
				out = append(out, m)
			}
		}
	}

	return out
}

// WithModel is this provider bound to one model by name, as if STUARTY_WHISPER_MODEL named it.
func (p *whisperProvider) WithModel(model string) transcribe.Provider {
	entries, _ := p.manifest.For("whisper", model, runtime.GOOS, runtime.GOARCH)
	bound := *p
	bound.model, bound.explicit, bound.entries = model, true, entries

	return &bound
}

func (p *whisperProvider) Name() string  { return transcribe.ProviderWhisper }
func (p *whisperProvider) Model() string { m, _ := p.resolve(); return m }

func (p *whisperProvider) current() *transcribe.Whisper {
	model, entries := p.resolve()
	w := &transcribe.Whisper{FFmpeg: p.ffmpeg, ModelName: model}

	for i := range entries {
		e := &entries[i]
		item := transcribe.Item{Name: e.File, URLHost: hostOf(e.URL), Bytes: e.Bytes}

		path, _ := service.VerifiedTool(p.dir, e.Name, e.Kind, e.Model)

		switch e.Kind {
		case transcribe.KindHelper:
			w.Helper, w.HelperItem, w.HelperVersion = path, item, e.Version

			// The shared libraries of the helper sit beside it.
			if path != "" && runtime.GOOS == "linux" {
				w.Env = map[string]string{"LD_LIBRARY_PATH": filepath.Dir(path)}
			}
		case transcribe.KindModel:
			w.ModelPath, w.ModelItem, w.ModelSHA256 = path, item, e.SHA256
			w.Limits = ffmpeg.LimitsForWhisper(e.Bytes, runtime.NumCPU())
		}
	}

	return w
}

func (p *whisperProvider) Check(ctx context.Context) error { return p.current().Check(ctx) }

func (p *whisperProvider) Transcribe(
	ctx context.Context,
	in transcribe.Input,
	progress func(done, total time.Duration),
) (transcribe.Raw, error) {
	w := p.current()
	// Verified again at the moment of use, not only when the job was accepted.
	if err := w.Check(ctx); err != nil {
		return transcribe.Raw{}, err
	}

	return w.Transcribe(ctx, in, progress)
}

func hostOf(rawURL string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if len(rawURL) > len(prefix) && rawURL[:len(prefix)] == prefix {
			rest := rawURL[len(prefix):]
			for i := range rest {
				if rest[i] == '/' {
					return rest[:i]
				}
			}

			return rest
		}
	}

	return ""
}
