// Package ffmpegpins holds the SHA-256 of the FFmpeg and ffprobe binaries the release bundles,
// per platform, generated from .config/ffmpeg.env. `doctor --verify` compares the bundled files
// with them (SDD-011 section 9).
package ffmpegpins

import "runtime"

//go:generate go run ../tools/ffmpegpins -env ../../.config/ffmpeg.env -out pins_gen.go

// For returns the pins of a platform, keyed "ffmpeg" and "ffprobe", or nil when the release
// bundles no FFmpeg for it. The map is a copy.
func For(goos, goarch string) map[string]string {
	t, ok := table[goos+"/"+goarch]
	if !ok {
		return nil
	}

	return map[string]string{"ffmpeg": t["ffmpeg"], "ffprobe": t["ffprobe"]}
}

// Current returns the pins of the platform this binary runs on.
func Current() map[string]string {
	return For(runtime.GOOS, runtime.GOARCH)
}
