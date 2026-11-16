package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

func parsedBuild(t *testing.T, configuration string) VersionInfo {
	t.Helper()
	v, ok := ParseVersion("ffmpeg", "ffmpeg version n9.0.2-vukem1 Copyright\nconfiguration: "+configuration+"\n")
	if !ok {
		t.Fatalf("not parsed: %s", configuration)
	}
	return v
}

// TestSecurity_T27_OwnBuildConfigureLineRefused is NFR-043 over many spellings: a bundled build (the own one
// or the BtbN one) whose configure line has a GPL, nonfree or compiled-in OpenH264 flag is reported with
// that flag, whatever the position, and the clean lines report nothing. A user's FFmpeg is not judged.
func TestSecurity_T27_OwnBuildConfigureLineRefused(t *testing.T) {
	const own = "--extra-version=vukem1"
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"clean", "--enable-version3 --disable-gpl --disable-nonfree --enable-libvpx " + own, nil},
		{"gpl", "--enable-gpl " + own, []string{"--enable-gpl"}},
		{"gpl after the marker", own + " --enable-version3 --enable-gpl", []string{"--enable-gpl"}},
		{"nonfree", "--enable-nonfree " + own, []string{"--enable-nonfree"}},
		{"both", "--enable-nonfree --enable-gpl " + own, []string{"--enable-gpl", "--enable-nonfree"}},
		{"openh264 compiled in", "--enable-libopenh264 " + own, []string{"--enable-libopenh264"}},
		{
			"all three", "--enable-libopenh264 --enable-nonfree --enable-gpl " + own,
			[]string{"--enable-gpl", "--enable-nonfree", "--enable-libopenh264"},
		},
		{"a later disable does not clear an enable", "--enable-gpl --disable-gpl " + own, []string{"--enable-gpl"}},
		{"btbn with gpl", "--prefix=/ffbuild/prefix --enable-gpl", []string{"--enable-gpl"}},
		{"btbn with nonfree", "--prefix=/ffbuild/prefix --enable-nonfree", []string{"--enable-nonfree"}},
		{"btbn keeps its own openh264", "--prefix=/ffbuild/prefix --enable-libopenh264", nil},
		{"a user build", "--enable-gpl --enable-nonfree --enable-libx264", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parsedBuild(t, c.line).Flavour().Problems()
			if !slices.Equal(got, c.want) {
				t.Errorf("problems = %v, want %v", got, c.want)
			}
		})
	}

	// A copy that does not print the marker is never taken for the own build, so it cannot pass as one.
	if n := parsedBuild(t, "--enable-version3 --extra-version=other1").Flavour().Name; n != FlavourOther {
		t.Errorf("a build with another extra version is %s", n)
	}
}

// ownWithFake installs a copy of the test binary as the own build (version text of testdata/builds/own).
func ownWithFake(t *testing.T, build string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "builds", build, "version.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return install(t, t.TempDir(), string(src))
}

// TestSecurity_T26_OpenH264DirOnlyForTheOwnBuild: a build that is not the own one never receives
// VUKEM_VIDEO_OPENH264_DIR, not even from the environment of the process, and the own build gets it only
// from a library that verified.
func TestSecurity_T26_OpenH264DirOnlyForTheOwnBuild(t *testing.T) {
	ctx := context.Background()
	m, archive, _ := fakeLibrary(t)
	tools := t.TempDir()
	if _, err := InstallOpenH264(ctx, InstallOpenH264Options{
		ToolsDir: tools, Manifest: m, Client: &countingClient{body: archive},
	}); err != nil {
		t.Fatal(err)
	}
	ambient := t.TempDir()
	t.Setenv(EnvOpenH264Dir, ambient)

	for _, build := range []string{"btbn"} { // the BtbN build has OpenH264 inside
		bin := ownWithFake(t, build)
		in, err := Discover(ctx, Config{FFmpegPath: bin, CPUCount: 1, ToolsDir: tools, OpenH264Manifest: m})
		if err != nil {
			t.Fatal(err)
		}
		if in.Flavour().Name != FlavourBtbN || in.OpenH264 != (OpenH264State{}) {
			t.Errorf("%s: flavour %v, library state %+v", build, in.Flavour().Name, in.OpenH264)
		}
		if out := runVersion(t, in); strings.Contains(out, "openh264dir: "+tools) || strings.Contains(out, ambient) {
			t.Errorf("%s received the library directory: %s", build, out)
		}
	}

	other := install(t, t.TempDir(), "n9.0.2-fake")
	in, err := Discover(ctx, Config{FFmpegPath: other, CPUCount: 1, ToolsDir: tools, OpenH264Manifest: m})
	if err != nil {
		t.Fatal(err)
	}
	if out := runVersion(t, in); strings.Contains(out, tools) || strings.Contains(out, ambient) {
		t.Errorf("a user's FFmpeg received the library directory: %s", out)
	}

	// The own build with no library, or with a changed one, sees the variable empty: the ambient value
	// of the process never reaches it.
	empty := t.TempDir()
	in, err = Discover(
		ctx,
		Config{FFmpegPath: ownWithFake(t, "own"), CPUCount: 1, ToolsDir: empty, OpenH264Manifest: m},
	)
	if err != nil {
		t.Fatal(err)
	}
	if out := runVersion(t, in); strings.Contains(out, ambient) || strings.Contains(out, "openh264dir: "+empty) {
		t.Errorf("the own build without a library saw a directory: %s", out)
	}
}

// TestSecurity_T26_OpenH264SwapAfterInstall: a library replaced after the install, in any way, is not
// trusted at the next start (openh264_changed), the build is not handed its directory, and a new
// install puts the pinned file back.
func TestSecurity_T26_OpenH264SwapAfterInstall(t *testing.T) {
	ctx := context.Background()
	m, archive, library := fakeLibrary(t)
	lib := func(dir string) string { return OpenH264Path(dir, m.Entries[0]) }

	swaps := map[string]func(t *testing.T, p string){
		"same size, other bytes": func(t *testing.T, p string) { t.Helper(); writeFile(t, p, flipByte(library)) },
		"shorter":                func(t *testing.T, p string) { t.Helper(); writeFile(t, p, library[:len(library)-1]) },
		"longer":                 func(t *testing.T, p string) { t.Helper(); writeFile(t, p, append(slices.Clone(library), 0)) },
		"empty":                  func(t *testing.T, p string) { t.Helper(); writeFile(t, p, nil) },
		"a directory": func(t *testing.T, p string) {
			t.Helper()
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"a symlink to the right bytes": func(t *testing.T, p string) {
			t.Helper()
			elsewhere := filepath.Join(t.TempDir(), "copy")
			writeFile(t, elsewhere, library)
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, p); err != nil {
				// skip-env: symlink-privilege
				t.Skipf(
					"%s cannot create a symlink here (needs the symlink privilege or developer mode): %v",
					runtime.GOOS,
					err,
				)
			}
		},
	}
	for name, swap := range swaps {
		t.Run(name, func(t *testing.T) {
			tools := t.TempDir()
			opt := InstallOpenH264Options{ToolsDir: tools, Manifest: m, Client: &countingClient{body: archive}}
			if _, err := InstallOpenH264(ctx, opt); err != nil {
				t.Fatal(err)
			}
			swap(t, lib(tools))

			if st := CheckOpenH264(
				tools,
				m,
				"",
				"",
			); st.Installed || st.Dir != "" ||
				st.Reason != ReasonOpenH264Changed {
				t.Fatalf("after the swap: %+v", st)
			}
			in, err := Discover(ctx, Config{
				FFmpegPath: ownWithFake(t, "own"), CPUCount: 1, ToolsDir: tools, OpenH264Manifest: m,
			})
			if err != nil {
				t.Fatal(err)
			}
			if in.OpenH264.Installed || in.OpenH264.Reason != ReasonOpenH264Changed {
				t.Errorf("discovery: %+v", in.OpenH264)
			}
			if out := runVersion(t, in); strings.Contains(out, "openh264dir: "+tools) {
				t.Errorf("the build was handed a swapped library: %s", out)
			}
			if e := openH264Error(in.OpenH264); e.Reason != ReasonOpenH264Changed ||
				e.Details["install_command"] != OpenH264InstallCommand {
				t.Errorf("error: %+v", e)
			}

			// The explicit install repairs it.
			if name == "a directory" {
				if err := os.Remove(lib(tools)); err != nil {
					t.Fatal(err)
				}
			}
			opt.Client = &countingClient{body: archive}
			if st, err := InstallOpenH264(ctx, opt); err != nil || !st.Installed {
				t.Errorf("reinstall: %+v %v", st, err)
			}
		})
	}
}

func writeFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// demuxerNames reads the names in the demuxer column of "ffmpeg -demuxers" (a line can list several).
func demuxerNames(out string) map[string]bool {
	names := map[string]bool{}
	for n := range parseListing(out) {
		for part := range strings.SplitSeq(n, ",") {
			names[part] = true
		}
	}
	return names
}

// TestSecurity_T14_InputWhitelistMatchesTheBuild re-derives the input allowlist from "ffmpeg -demuxers" of
// the build under test (the own build when VUKEM_VIDEO_TEST_FFMPEG_DIR names it): every allowed name is a
// demuxer of the build, and none of the dangerous demuxers is allowed or in the way of the allowlist.
func TestSecurity_T14_InputWhitelistMatchesTheBuild(t *testing.T) {
	forbidden := []string{
		"hls", "applehttp", "dash", "concat", "ffconcat", "tee", "sdp", "rtsp", "rtp", "lavfi", "srt", "ass",
		"webvtt", "subviewer", "microdvd", "image2pipe", "data", "tty", "ivf_pipe",
		// Text and metadata demuxers of the pinned build that no feature reads (re-derived from -demuxers).
		"mpegtsraw", "ffmetadata", "lrc", "jacosub", "svg_pipe", "pls", "sbg", "mpl2", "sami", "stl", "realtext",
		"pjs", "vplayer", "subrip", "aqtitle", "mpsub", "dvbsub", "dvbtxt", "ttml", "smush", "bin", "libmodplug",
	}
	allowed := strings.Split(ffargs.InputFormats, ",")
	for _, f := range forbidden {
		if slices.Contains(allowed, f) {
			t.Errorf("%s is in the input allowlist", f)
		}
	}

	r, dir := pinnedRunner(t)
	argv, err := ffargs.Info("-demuxers")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if _, err := r.RunFFmpeg(context.Background(), Request{
		Dir: dir, Argv: argv, Deadline: 30 * time.Second, Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}
	have := demuxerNames(out.String())
	for _, n := range allowed {
		if !have[n] {
			t.Errorf("the allowed input format %s is not a demuxer of %s", n, r.In.Info.Version)
		}
		// Every other picture-in-a-pipe demuxer (svg, dpx, pam, ...) stays out: only PNG, JPEG and WebP are images.
		if strings.HasSuffix(n, "_pipe") && n != "png_pipe" && n != "jpeg_pipe" && n != "webp_pipe" {
			t.Errorf("%s is in the input allowlist", n)
		}
	}
	for _, f := range forbidden {
		if have[f] && slices.Contains(allowed, f) {
			t.Errorf("%s is a demuxer of the build and is allowed", f)
		}
	}
	if t.Failed() {
		return
	}
	t.Logf(
		"%d allowed names found in %d demuxer names of %s (%s)",
		len(allowed),
		len(have),
		r.In.Info.Version,
		r.In.Flavour().Name,
	)
}
