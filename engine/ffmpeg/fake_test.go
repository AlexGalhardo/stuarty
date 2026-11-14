package ffmpeg

import (
	"fmt"
	"os"
	"path/filepath"
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
	for _, a := range os.Args[1:] {
		if a == "-version" {
			fmt.Printf("%s version %s Copyright (c) the FFmpeg developers\nconfiguration: --enable-version3 --disable-libx264\n", base, version)
			return true
		}
	}
	os.Exit(1)
	return true
}
