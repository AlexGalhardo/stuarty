package ffmpeg

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestParseScoopShim(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "apps", "ffmpeg", "current", "bin", exeName("ffmpeg"))

	cases := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{"unquoted", "path = " + abs + "\r\nargs = \r\n", abs, true},
		{"double quoted", "path = \"" + abs + "\"\n", abs, true},
		{"single quoted", "PATH = '" + abs + "'\n", abs, true},
		{"relative refused", "path = bin/ffmpeg.exe\n", "", false},
		{"traversal refused", "path = " + filepath.Dir(abs) + "/../ffmpeg.exe\n", "", false},
		{"null byte refused", "path = " + abs + "\x00extra\n", "", false},
		{"empty path refused", "path = \n", "", false},
		{"no path key", "args = -version\n", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseScoopShim([]byte(tc.body))
			if ok != tc.ok || got != tc.want {
				t.Errorf("parseScoopShim(%q) = (%q, %v), want (%q, %v)", tc.body, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestDiscoverResolvesScoopShimAndStreamsRawVideo(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "apps", "ffmpeg", "current", "bin")
	realFFmpeg := install(t, realDir, "n9.0.2-scoop")
	realFFprobe := filepath.Join(realDir, exeName("ffprobe"))

	shimDir := filepath.Join(root, "shims")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Put dummy stub executables in shims/ with sibling .shim files pointing at realDir.
	shimFFmpeg := filepath.Join(shimDir, exeName("ffmpeg"))
	shimFFprobe := filepath.Join(shimDir, exeName("ffprobe"))
	if err := os.WriteFile(shimFFmpeg, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shimFFprobe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	ffmpegShim := filepath.Join(shimDir, "ffmpeg.shim")
	if err := os.WriteFile(ffmpegShim, []byte("path = "+realFFmpeg+"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffprobeShim := filepath.Join(shimDir, "ffprobe.shim")
	if err := os.WriteFile(ffprobeShim, []byte("path = "+realFFprobe+"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// On non-Windows where exeName has no .exe, non-regular or missing target is still rejected.
	missingExe := filepath.Join(shimDir, exeName("missing"))
	if got, ok := resolveScoopShim(missingExe); ok || got != missingExe {
		t.Fatalf("missing shim resolved to (%q, %v)", got, ok)
	}

	if runtime.GOOS == "windows" {
		badShim := filepath.Join(shimDir, "bad.exe")
		_ = os.WriteFile(badShim, []byte("stub"), 0o755)
		badBody := []byte("path = " + filepath.Join(realDir, "version.txt") + "\r\n")
		_ = os.WriteFile(filepath.Join(shimDir, "bad.shim"), badBody, 0o644)
		if _, ok := resolveScoopShim(badShim); ok {
			t.Fatal("non-.exe target in Windows shim was accepted")
		}
	}

	in, err := Discover(context.Background(), Config{
		ExecDir:  filepath.Join(root, "none"),
		LookPath: func(string) (string, error) { return shimFFmpeg, nil },
	})
	if err != nil {
		t.Fatalf("Discover through shim failed: %v", err)
	}

	if in.FFmpeg.Path() != realFFmpeg {
		t.Errorf("FFmpeg path = %q, want resolved target %q", in.FFmpeg.Path(), realFFmpeg)
	}
	if in.FFprobe.Path() != realFFprobe {
		t.Errorf("FFprobe path = %q, want resolved target %q", in.FFprobe.Path(), realFFprobe)
	}

	// Confirm the resolved installation runs cleanly through Runner.RunFFmpeg with stdout piping.
	ws := filepath.Join(root, "ws")
	r, err := NewRunner(in, ws)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := r.NewRunDir("shim-pipe")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Remove() }()

	var out bytes.Buffer
	res, err := r.RunFFmpeg(context.Background(), Request{
		Dir:      dir,
		Argv:     []string{"-hide_banner", "-version"},
		Deadline: 15 * time.Second,
		Stdout:   &out,
	})
	if err != nil {
		t.Fatalf("RunFFmpeg after shim resolution failed: %v (stderr: %s)", err, res.Stderr)
	}
	if !bytes.Contains(out.Bytes(), []byte("n9.0.2-scoop")) {
		t.Errorf("stdout = %q, want version n9.0.2-scoop", out.String())
	}
}
