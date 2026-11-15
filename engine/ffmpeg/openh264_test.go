package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ownRunner is a runner over a fake of the own build whose Cisco library has the given state.
func ownRunner(t *testing.T, st OpenH264State) *Runner {
	t.Helper()
	return &Runner{In: &Installation{Info: fakeBuild(t, "own"), OpenH264: st}}
}

func TestEncoderGateForTheOwnBuild(t *testing.T) {
	m, _, _ := fakeLibrary(t)
	listed := &Capabilities{Encoders: map[string]bool{"libopenh264": true, "h264_nvenc": true}}
	missing := OpenH264State{Reason: ReasonOpenH264NotInstalled, Entry: m.Entries[0]}

	// Library absent: libopenh264 fails with the install command, a hardware encoder is still tried.
	r := ownRunner(t, missing)
	g := r.encoderGate("libopenh264", listed, DetectOptions{})
	if !g.skip || g.status != EncoderFailed || g.reason != ReasonOpenH264NotInstalled ||
		!strings.Contains(g.why, OpenH264InstallCommand) {
		t.Errorf("absent: %+v", g)
	}
	if g := r.encoderGate("h264_nvenc", listed, DetectOptions{}); g.skip {
		t.Errorf("a hardware encoder was gated: %+v", g)
	}
	// Not in the build at all: not_listed, not an install problem.
	if g := r.encoderGate(
		"libopenh264",
		&Capabilities{Encoders: map[string]bool{}},
		DetectOptions{},
	); g.status != EncoderNotListed ||
		g.reason != "" {
		t.Errorf("unlisted: %+v", g)
	}
	// A library that changed on disk is gated with its own reason.
	if g := ownRunner(
		t,
		OpenH264State{Reason: ReasonOpenH264Changed},
	).encoderGate("libopenh264", listed, DetectOptions{}); g.reason != ReasonOpenH264Changed {
		t.Errorf("changed: %+v", g)
	}
	// Installed: the encoder is tried (a real test encode decides).
	if g := ownRunner(t, OpenH264State{Installed: true}).encoderGate("libopenh264", listed, DetectOptions{}); g.skip {
		t.Errorf("installed: %+v", g)
	}
	// The BtbN build has OpenH264 inside and needs no library.
	btbn := &Runner{In: &Installation{Info: fakeBuild(t, "btbn")}}
	if g := btbn.encoderGate("libopenh264", listed, DetectOptions{}); g.skip {
		t.Errorf("btbn: %+v", g)
	}
	// libx264 stays for a user's build only.
	if g := r.encoderGate(
		"libx264",
		&Capabilities{Encoders: map[string]bool{"libx264": true}},
		DetectOptions{AllowUserX264: true},
	); !g.skip {
		t.Errorf("libx264 on a bundled build: %+v", g)
	}
}

// TestNoEncoderNamesTheOpenH264Install is the actionable error of the task: with the library absent and
// nothing else working, export fails with the install command in the hint and in the details.
func TestNoEncoderNamesTheOpenH264Install(t *testing.T) {
	det := &EncoderDetection{Candidates: []Candidate{
		{Name: "h264_nvenc", Status: EncoderNotListed},
		{
			Name: "libopenh264", Status: EncoderFailed, Reason: ReasonOpenH264NotInstalled,
			StderrTail: "the Cisco OpenH264 library is not installed; run " + OpenH264InstallCommand,
		},
	}}
	e := noEncoderError(det)
	if e.Code != CodeNoEncoder || !strings.Contains(e.Hint, OpenH264InstallCommand) ||
		e.Details["install_command"] != OpenH264InstallCommand || e.Details["reason"] != ReasonOpenH264NotInstalled ||
		e.Details["tool"] != OpenH264Tool || !strings.Contains(e.Message, "libopenh264") {
		t.Errorf("%+v", e)
	}
	// Without the library problem the older hint stays.
	plain := noEncoderError(&EncoderDetection{Candidates: []Candidate{{Name: "libopenh264", Status: EncoderNotListed}}})
	if strings.Contains(plain.Hint, "tools:install") || plain.Details["install_command"] != nil {
		t.Errorf("%+v", plain)
	}
}

func TestOpenH264ErrorKeys(t *testing.T) {
	m, _, _ := fakeLibrary(t)
	e := openH264Error(OpenH264State{Reason: ReasonOpenH264NotInstalled, Entry: m.Entries[0]})
	if e.Details["install_command"] != OpenH264InstallCommand || e.Details["download_bytes"] != m.Entries[0].Bytes ||
		!strings.Contains(
			e.Hint,
			"ciscobinary.openh264.org",
		) || strings.Contains(e.Message+e.Hint, string(filepath.Separator)+"tools") {
		t.Errorf("%+v", e)
	}
}

// TestDiscoverOwnBuildOpenH264 runs discovery over a fake of the own build: the library is reported
// absent, then installed and verified, and only then does the tool see EnvOpenH264Dir.
func TestDiscoverOwnBuildOpenH264(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "builds", "own", "version.txt"))
	if err != nil {
		t.Fatal(err)
	}
	bin := install(t, t.TempDir(), string(src))
	m, archive, _ := fakeLibrary(t)
	tools := t.TempDir()
	cfg := Config{FFmpegPath: bin, CPUCount: 1, ToolsDir: tools, OpenH264Manifest: m}

	in, err := Discover(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if in.Flavour().Name != FlavourOwn || in.OpenH264.Installed || in.OpenH264.Reason != ReasonOpenH264NotInstalled {
		t.Fatalf("before: %v %+v", in.Flavour(), in.OpenH264)
	}
	if out := runVersion(t, in); strings.Contains(out, "openh264dir: "+tools) {
		t.Errorf("the tool saw the library directory before the install: %s", out)
	}

	if _, err := InstallOpenH264(
		context.Background(),
		InstallOpenH264Options{ToolsDir: tools, Manifest: m, Client: &countingClient{body: archive}},
	); err != nil {
		t.Fatal(err)
	}
	in, err = Discover(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !in.OpenH264.Installed {
		t.Fatalf("after: %+v", in.OpenH264)
	}
	if out := runVersion(t, in); !strings.Contains(out, "openh264dir: "+in.OpenH264.Dir) {
		t.Errorf("the tool did not see the library directory: %s", out)
	}

	// Another FFmpeg never gets the variable or a library state.
	other := install(t, t.TempDir(), "n9.0.2-fake")
	oin, err := Discover(
		context.Background(),
		Config{FFmpegPath: other, CPUCount: 1, ToolsDir: tools, OpenH264Manifest: m},
	)
	if err != nil || oin.OpenH264 != (OpenH264State{}) {
		t.Errorf("other build: %v %+v", err, oin.OpenH264)
	}
}

func runVersion(t *testing.T, in *Installation) string {
	t.Helper()
	res, err := in.FFmpeg.Run(context.Background(), t.TempDir(), "-hide_banner", "-version")
	if err != nil {
		t.Fatal(err)
	}
	return res.Stdout
}

// The error after a missing library names the command and is built from the state alone.
func TestOpenH264GateNamesTheInstall(t *testing.T) {
	m, _, _ := fakeLibrary(t)
	r := ownRunner(t, CheckOpenH264(t.TempDir(), m, "", ""))
	g := r.encoderGate("libopenh264", &Capabilities{Encoders: map[string]bool{"libopenh264": true}}, DetectOptions{})
	if g.status != EncoderFailed || openH264Error(r.In.OpenH264).Details["install_command"] != OpenH264InstallCommand {
		t.Errorf("gate: %+v", g)
	}
}
