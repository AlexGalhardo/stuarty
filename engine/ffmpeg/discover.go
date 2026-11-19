package ffmpeg

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// Source is where an FFmpeg came from (SDD-011 8.1).
type Source string

// The sources of an FFmpeg, in the order they are tried.
const (
	SourceConfigured Source = "configured"
	SourceBundled    Source = "bundled"
	SourcePath       Source = "path"
)

// EnvFFmpeg and EnvFFprobe name the variables of the configured source (SDD-000 section 10).
// The caller reads them; this package never touches the environment of the server.
const (
	EnvFFmpeg  = "STUARTY_FFMPEG"
	EnvFFprobe = "STUARTY_FFPROBE"
)

// ffmpegMaxOutput is the stderr tail kept for ffmpeg: silence analysis prints one line
// per silence (SDD-011 1.3).
const ffmpegMaxOutput = 4 << 20

// Config says where to look and how runs are bounded.
type Config struct {
	// FFmpegPath and FFprobePath are STUARTY_FFMPEG and STUARTY_FFPROBE. A set
	// FFmpegPath that is unusable is an error, never a silent fallback.
	FFmpegPath, FFprobePath string
	// ExecDir is the directory of the stuarty executable, for the bundled copy.
	// Empty skips the bundled source.
	ExecDir string
	// CPUCount is the CPU budget of a run (vukem cpu.Count, read by the caller). Zero means the
	// machine's logical CPU count.
	CPUCount int
	// ToolsDir is <workspace>/tools, where the explicit install step puts the Cisco OpenH264 library the
	// own FFmpeg build loads (D-017). Empty means none is installed.
	ToolsDir string
	// OpenH264Manifest pins that library; the embedded manifest when it has no entries. Tests replace it.
	OpenH264Manifest OpenH264Manifest
	// Limits bound every run through prlimit(1). Linux only: the caller leaves it zero elsewhere.
	Limits engine.Limits
	// LookPath resolves ffmpeg on the PATH; exec.LookPath when nil. Tests replace it.
	LookPath func(string) (string, error)
	Log      *slog.Logger
}

// Installation is a usable FFmpeg and ffprobe pair.
type Installation struct {
	FFmpeg, FFprobe *engine.Tool
	Source          Source
	Info            VersionInfo // of ffmpeg
	ProbeInfo       VersionInfo // of ffprobe
	CPUCount        int
	// UserBuild is true when the pair is the user's own: configured, or found on the PATH.
	// A bundled build is never a user build (libx264 is not offered for it, SDD-011 8.4).
	UserBuild bool
	// OpenH264 is the state of the Cisco library, checked once at discovery and meaningful only for the own
	// build (Flavour). Installed means the file passed its size and SHA-256 check.
	OpenH264 OpenH264State

	// limits are the limits the tools were opened with; the sandbox wrapper applies them itself.
	limits engine.Limits
}

// Warnings lists the version warnings of SDD-011 8.2.
func (in *Installation) Warnings() []string { return in.Info.Warnings }

// ThreadLimit is the value to put in -threads and -filter_threads, or zero when the CPU
// budget equals the machine's and FFmpeg's own count is right (SDD-011 1.3).
func (in *Installation) ThreadLimit() int {
	if in.CPUCount > 0 && in.CPUCount < runtime.NumCPU() {
		return in.CPUCount
	}
	return 0
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

type candidate struct {
	source Source
	ffmpeg string
	probe  string // empty: next to ffmpeg
}

// Discover finds FFmpeg in the order of ADR-0011 and SDD-011 8.1: the configured path,
// the copy next to the executable, then the PATH. The first usable pair wins.
func Discover(ctx context.Context, cfg Config) (*Installation, error) {
	look := cfg.LookPath
	if look == nil {
		look = exec.LookPath
	}
	if cfg.CPUCount <= 0 {
		cfg.CPUCount = runtime.NumCPU()
	}

	if cfg.FFmpegPath != "" {
		in, err := tryPair(ctx, cfg, candidate{SourceConfigured, cfg.FFmpegPath, cfg.FFprobePath})
		if err != nil {
			return nil, missing([]Source{SourceConfigured}, true, err)
		}
		return in, nil
	}

	var cands []candidate
	if cfg.ExecDir != "" {
		for _, p := range []string{
			filepath.Join(cfg.ExecDir, exeName("ffmpeg")),
			filepath.Join(cfg.ExecDir, "ffmpeg", exeName("ffmpeg")),
			filepath.Join(cfg.ExecDir, "..", "libexec", "stuarty", exeName("ffmpeg")),
		} {
			cands = append(cands, candidate{SourceBundled, p, cfg.FFprobePath})
		}
	}
	if p, err := look("ffmpeg"); err == nil {
		// A relative result is refused: exec.LookPath already refuses ".", and a PATH entry
		// that is relative would make the answer depend on the working directory.
		if filepath.IsAbs(p) {
			cands = append(cands, candidate{SourcePath, p, cfg.FFprobePath})
		}
	}

	var last error
	for _, c := range cands {
		if _, err := os.Stat(c.ffmpeg); err != nil && c.source == SourceBundled {
			continue
		}
		in, err := tryPair(ctx, cfg, c)
		if err == nil {
			return in, nil
		}
		last = err
	}
	return nil, missing([]Source{SourceConfigured, SourceBundled, SourcePath}, false, last)
}

func missing(tried []Source, configured bool, cause error) *Error {
	names := make([]string, len(tried))
	for i, t := range tried {
		names[i] = string(t)
	}
	e := &Error{
		Code:    CodeFFmpegMissing,
		Reason:  "ffmpeg_missing",
		Message: "No usable FFmpeg was found (looked at: the configured path, next to the program, the PATH).",
		Hint:    `Install FFmpeg 7 or newer, or set ` + EnvFFmpeg + ` to its path. Run "stuarty doctor" for details.`,
		Details: map[string]any{"tried": names, "configured": configured},
		Err:     cause,
	}
	if configured {
		e.Message = "The FFmpeg set in " + EnvFFmpeg + " is not usable."
		e.Hint = "Fix the path in " + EnvFFmpeg + " or unset it to let the program look for FFmpeg itself."
	}
	return e
}

func tryPair(ctx context.Context, cfg Config, c candidate) (*Installation, error) {
	if !filepath.IsAbs(c.ffmpeg) {
		return nil, errors.New("path is not absolute")
	}
	probePath := c.probe
	if probePath == "" {
		probePath = filepath.Join(filepath.Dir(c.ffmpeg), exeName("ffprobe"))
	} else if !filepath.IsAbs(probePath) {
		return nil, errors.New("ffprobe path is not absolute")
	}

	ff, info, err := openTool(ctx, cfg, "ffmpeg", c.ffmpeg, ffmpegMaxOutput, nil)
	if err != nil {
		return nil, err
	}
	lib, ff, err := attachOpenH264(ctx, cfg, c.ffmpeg, info, ff)
	if err != nil {
		return nil, err
	}
	fp, pinfo, err := openTool(ctx, cfg, "ffprobe", probePath, 0, nil)
	if err != nil {
		return nil, err
	}
	// A mixed pair of versions is refused unless the user chose the ffprobe.
	if c.probe == "" && pinfo.Version != info.Version {
		return nil, errors.New("ffmpeg and ffprobe next to it are different versions")
	}
	in := &Installation{
		FFmpeg: ff, FFprobe: fp, Source: c.source, Info: info, ProbeInfo: pinfo, CPUCount: cfg.CPUCount,
		UserBuild: c.source != SourceBundled, OpenH264: lib, limits: cfg.Limits,
	}
	if cfg.Log != nil {
		cfg.Log.InfoContext(
			ctx,
			"ffmpeg resolved",
			"source",
			c.source,
			"version",
			info.Version,
			"license",
			info.License.Name,
		)
	}
	return in, nil
}

// openTool resolves one binary through engine.Find and requires it to print "<name> version".
func openTool(
	ctx context.Context, cfg Config, name, path string, maxOutput int, env map[string]string,
) (*engine.Tool, VersionInfo, error) {
	t, err := engine.Find(ctx, engine.Config{
		Name: name, Path: path, VersionArgs: []string{"-hide_banner", "-version"},
		Limits: cfg.Limits, CPUCount: cfg.CPUCount, MaxOutput: maxOutput, Log: cfg.Log, Env: env,
	})
	if err != nil {
		return nil, VersionInfo{}, err
	}
	vctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "stuarty-ffver-")
	if err != nil {
		return nil, VersionInfo{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	res, err := t.Run(vctx, dir, "-hide_banner", "-version")
	if err != nil {
		return nil, VersionInfo{}, err
	}
	info, ok := ParseVersion(name, res.Stdout)
	if !ok {
		return nil, VersionInfo{}, errors.New(strings.TrimSpace(name) + " did not print its version")
	}
	return t, info, nil
}

// attachOpenH264 checks the Cisco library for the own build and, when it is installed and unchanged,
// reopens ffmpeg with EnvOpenH264Dir set so the build can load it. Any other build is left as it is:
// it never sees the variable.
func attachOpenH264(
	ctx context.Context, cfg Config, path string, info VersionInfo, ff *engine.Tool,
) (OpenH264State, *engine.Tool, error) {
	if info.Flavour().Name != FlavourOwn {
		return OpenH264State{}, ff, nil
	}
	m := cfg.OpenH264Manifest
	if len(m.Entries) == 0 {
		var err error
		if m, err = DefaultOpenH264Manifest(); err != nil {
			return OpenH264State{}, nil, err
		}
	}
	st := CheckOpenH264(cfg.ToolsDir, m, "", "")
	if !st.Installed {
		return st, ff, nil
	}
	withLib, _, err := openTool(ctx, cfg, "ffmpeg", path, ffmpegMaxOutput, map[string]string{EnvOpenH264Dir: st.Dir})
	if err != nil {
		return OpenH264State{}, nil, err
	}
	return st, withLib, nil
}
