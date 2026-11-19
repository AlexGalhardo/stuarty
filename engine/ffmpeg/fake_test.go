package ffmpeg

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fakeTool lets a copy of the test binary named ffmpeg or ffprobe answer -version, so
// discovery is tested without a real FFmpeg. The version line comes from version.txt next
// to the copy (<name>.version first), so a pair with different versions can be built.
func fakeTool() bool {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	if base != "ffmpeg" && base != "ffprobe" {
		return false
	}
	version := "n9.0.2-fake"
	for _, name := range []string{base + ".version", "version.txt"} {
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), name)); err == nil {
			version = strings.TrimSpace(string(b))
			break
		}
	}
	if slices.Contains(os.Args[1:], "-version") {
		// A full version output (testdata/builds) is printed as it is, with the library directory the
		// runner passed, so a test can see the environment of the tool.
		if strings.HasPrefix(version, "ffmpeg version ") {
			fmt.Println(strings.Replace(version, "ffmpeg version ", base+" version ", 1))
			fmt.Println("openh264dir:", os.Getenv("VUKEM_VIDEO_OPENH264_DIR"))
			return true
		}
		fmt.Printf(
			"%s version %s Copyright (c) the FFmpeg developers\nconfiguration: --enable-version3 --disable-libx264\n",
			base,
			version,
		)
		return true
	}
	if fakeEncode(base) {
		return true
	}
	os.Exit(1)
	return true
}

// fakeEncode answers the two commands of the encoder test (SDD-011 8.4) when a file named
// works.txt sits next to the copy: ffmpeg writes its output for an encoder listed in it (one name per
// line) and fails like a driver that cannot open otherwise; ffprobe reports 15 packets of the codec
// of the encoder that wrote the file (the file holds the codec), or of another codec when
// wrongcodec.txt lists the encoder, like a driver that writes something else.
func fakeEncode(base string) bool {
	b, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "works.txt"))
	if err != nil {
		return false
	}
	if base == "ffprobe" {
		codec := "h264"
		if got, err := os.ReadFile(strings.TrimPrefix(os.Args[len(os.Args)-1], "file:")); err == nil &&
			strings.HasPrefix(string(got), "fake:") {
			codec = strings.TrimPrefix(string(got), "fake:")
		}
		fmt.Println(codec + ",15")
		return true
	}
	args := os.Args[1:]
	i := slices.Index(args, "-c:v")
	if i < 0 || i+1 >= len(args) {
		return false
	}
	enc := args[i+1]
	if !slices.Contains(strings.Fields(string(b)), enc) {
		fmt.Fprintf(os.Stderr, "[%s @ 0000] Error while opening encoder: no device for %s\nsecond line\n", enc, enc)
		os.Exit(1)
	}
	// small.txt names the encoders that cannot do a 4K picture, like a GPU of an older generation.
	if s, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "small.txt")); err == nil &&
		slices.Contains(strings.Fields(string(s)), enc) &&
		slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "3840x2160") }) {
		fmt.Fprintf(os.Stderr, "[%s @ 0000] Error while opening encoder: frame size too large\n", enc)
		os.Exit(1)
	}
	codec := "h264"
	switch {
	case strings.HasPrefix(enc, "hevc_"):
		codec = "hevc"
	case strings.HasPrefix(enc, "av1_"), enc == "libsvtav1", enc == "libaom-av1":
		codec = "av1"
	}
	if w, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "wrongcodec.txt")); err == nil &&
		slices.Contains(strings.Fields(string(w)), enc) {
		codec = "mpeg4"
	}
	if err := os.WriteFile(strings.TrimPrefix(args[len(args)-1], "file:"), []byte("fake:"+codec), 0o644); err != nil {
		os.Exit(2)
	}
	return true
}
