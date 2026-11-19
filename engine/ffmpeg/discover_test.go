package ffmpeg

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// install copies the test binary as ffmpeg and ffprobe into dir, with the given version.
func install(t *testing.T, dir, version string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"ffmpeg", "ffprobe"} {
		src, err := os.Open(os.Args[0])
		if err != nil {
			t.Fatal(err)
		}
		dst, err := os.OpenFile(filepath.Join(dir, exeName(n)), os.O_CREATE|os.O_WRONLY, 0o755)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			t.Fatal(err)
		}
		_ = src.Close()
		_ = dst.Close()
	}
	if err := os.WriteFile(filepath.Join(dir, "version.txt"), []byte(version), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, exeName("ffmpeg"))
}

func noPath(string) (string, error) { return "", errors.New("not found") }

func TestDiscoverOrder(t *testing.T) {
	root := t.TempDir()
	configured := install(t, filepath.Join(root, "configured"), "n9.0.2-configured")
	bundled := install(t, filepath.Join(root, "exe"), "n9.0.2-bundled")
	onPath := install(t, filepath.Join(root, "path"), "n9.0.2-path")
	look := func(string) (string, error) { return onPath, nil }
	ctx := context.Background()

	in, err := Discover(ctx, Config{FFmpegPath: configured, ExecDir: filepath.Dir(bundled), LookPath: look})
	if err != nil || in.Source != SourceConfigured || in.Info.Version != "n9.0.2-configured" {
		t.Fatalf("configured: %v %+v", err, in)
	}
	if filepath.Dir(in.FFprobe.Path()) != filepath.Dir(in.FFmpeg.Path()) {
		t.Error("ffprobe is not next to ffmpeg")
	}
	if !in.UserBuild {
		t.Error("a configured build belongs to the user")
	}

	in, err = Discover(ctx, Config{ExecDir: filepath.Dir(bundled), LookPath: look})
	if err != nil || in.Source != SourceBundled || in.Info.Version != "n9.0.2-bundled" || in.UserBuild {
		t.Fatalf("bundled: %v %+v", err, in)
	}

	in, err = Discover(ctx, Config{ExecDir: filepath.Join(root, "nothing"), LookPath: look})
	if err != nil || in.Source != SourcePath || in.Info.Version != "n9.0.2-path" || !in.UserBuild {
		t.Fatalf("path: %v %+v", err, in)
	}
	if in.Info.License.Name != "lgpl" || !in.Info.License.Version3 {
		t.Errorf("license = %+v", in.Info.License)
	}
}

func TestDiscoverBundledLayouts(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "bin")
	install(t, filepath.Join(exe, "ffmpeg"), "n9.0.2-sub")
	in, err := Discover(context.Background(), Config{ExecDir: exe, LookPath: noPath})
	if err != nil || in.Info.Version != "n9.0.2-sub" {
		t.Fatalf("<exe>/ffmpeg/ffmpeg: %v", err)
	}

	root = t.TempDir()
	exe = filepath.Join(root, "bin")
	if err := os.MkdirAll(exe, 0o755); err != nil {
		t.Fatal(err)
	}
	install(t, filepath.Join(root, "libexec", "stuarty"), "n9.0.2-libexec")
	in, err = Discover(context.Background(), Config{ExecDir: exe, LookPath: noPath})
	if err != nil || in.Info.Version != "n9.0.2-libexec" {
		t.Fatalf("../libexec/stuarty: %v", err)
	}
}

func TestDiscoverConfiguredUnusableIsNotReplaced(t *testing.T) {
	root := t.TempDir()
	onPath := install(t, filepath.Join(root, "path"), "n9.0.2-path")
	bad := filepath.Join(root, "missing", exeName("ffmpeg"))
	_, err := Discover(
		context.Background(),
		Config{FFmpegPath: bad, LookPath: func(string) (string, error) { return onPath, nil }},
	)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeFFmpegMissing {
		t.Fatalf("error = %v, want E_FFMPEG_MISSING", err)
	}
	if e.Details["configured"] != true {
		t.Errorf("details = %v", e.Details)
	}
	if strings.Contains(e.Message+e.Hint, root) {
		t.Error("the message names a path")
	}
}

func TestDiscoverNothingFound(t *testing.T) {
	_, err := Discover(context.Background(), Config{ExecDir: t.TempDir(), LookPath: noPath})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeFFmpegMissing {
		t.Fatalf("error = %v", err)
	}
	want := "No usable FFmpeg was found (looked at: the configured path, next to the program, the PATH)."
	if e.Message != want {
		t.Errorf("message = %q", e.Message)
	}
	if !strings.Contains(e.Hint, "STUARTY_FFMPEG") || !strings.Contains(e.Hint, "doctor") {
		t.Errorf("hint = %q", e.Hint)
	}
	if got := e.Details["tried"].([]string); len(got) != 3 {
		t.Errorf("tried = %v", got)
	}
}

func TestDiscoverMixedPairAndRelativePath(t *testing.T) {
	root := t.TempDir()
	a := install(t, filepath.Join(root, "a"), "n9.0.2-a")
	b := install(t, filepath.Join(root, "b"), "n9.0.2-b")
	probeB := filepath.Join(filepath.Dir(b), exeName("ffprobe"))
	_ = b

	// An ffprobe the user chose is accepted whatever its version.
	in, err := Discover(context.Background(), Config{FFmpegPath: a, FFprobePath: probeB, LookPath: noPath})
	if err != nil || in.ProbeInfo.Version != "n9.0.2-b" {
		t.Fatalf("chosen ffprobe: %v", err)
	}

	// Next to ffmpeg, an ffprobe of another version is refused.
	if err := os.WriteFile(
		filepath.Join(filepath.Dir(a), "ffprobe.version"),
		[]byte("n9.0.2-other"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = Discover(context.Background(), Config{FFmpegPath: a, LookPath: noPath}); err == nil {
		t.Fatal("a mixed pair was accepted when only ffmpeg was configured")
	}
	if _, err = Discover(
		context.Background(),
		Config{LookPath: func(string) (string, error) { return a, nil }, ExecDir: t.TempDir()},
	); err == nil {
		t.Error("a mixed pair found on the PATH was accepted")
	}

	_, err = Discover(
		context.Background(),
		Config{LookPath: func(string) (string, error) { return "relative/ffmpeg", nil }, ExecDir: t.TempDir()},
	)
	if err == nil {
		t.Error("a relative PATH result was accepted")
	}
}

func TestParseVersion(t *testing.T) {
	lgpl := "ffmpeg version n9.0.2-39-g7adfbe40dc-20261031 Copyright (c) 2000-2026 the FFmpeg developers\r\nbuilt with gcc 16.2.0\r\n" +
		"configuration: --prefix=/ffbuild/prefix --enable-version3 --disable-libx264 --disable-libx265 --enable-libopenh264\r\nlibavutil 60. 8.100\r\n"
	v, ok := ParseVersion("ffmpeg", lgpl)
	if !ok || v.Version != "n9.0.2-39-g7adfbe40dc-20261031" || v.License != (License{"lgpl", true}) || v.Major != 9 ||
		len(v.Warnings) != 0 {
		t.Errorf("pinned build: %v %+v", ok, v)
	}

	cases := []struct {
		name, out string
		license   string
		warnings  []string
	}{
		{
			"gpl",
			"ffmpeg version 9.0.2-full_build-www.gyan.dev Copyright\nconfiguration: --enable-gpl --enable-version3 --enable-libx264",
			"gpl",
			nil,
		},
		{
			"nonfree",
			"ffmpeg version 7.1 Copyright\nconfiguration: --enable-gpl --enable-nonfree",
			"nonfree",
			[]string{"untested_ffmpeg_version"},
		},
		{
			"old",
			"ffmpeg version 6.1.1 Copyright\nconfiguration: --enable-gpl",
			"gpl",
			[]string{"untested_ffmpeg_version", "old_ffmpeg_version"},
		},
		{
			"master",
			"ffmpeg version N-128227-g4811625b5f Copyright\nconfiguration: --disable-gpl",
			"lgpl",
			[]string{"untested_ffmpeg_version"},
		},
		{"no configuration line", "ffmpeg version n9.0.2 Copyright", "lgpl", nil},
	}
	for _, c := range cases {
		v, ok := ParseVersion("ffmpeg", c.out)
		if !ok || v.License.Name != c.license || strings.Join(v.Warnings, ",") != strings.Join(c.warnings, ",") {
			t.Errorf("%s: %v %+v", c.name, ok, v)
		}
	}
	if _, ok := ParseVersion("ffmpeg", "ffprobe version 9.0"); ok {
		t.Error("an ffprobe banner passed as ffmpeg")
	}
	if _, ok := ParseVersion("ffmpeg", "hello"); ok {
		t.Error("garbage parsed")
	}
}
