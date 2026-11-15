// Package mediagen synthesizes the media fixtures of the ingest tests with the pinned FFmpeg
// (SDD-011 12): no media file is committed. The commands are test input, not product commands,
// so they are run straight through the function the caller gives, not through ffargs.
package mediagen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Run runs ffmpeg in dir with the arguments and returns its stderr. The caller wires it to the pinned
// build, for example inst.FFmpeg.Run.
type Run func(ctx context.Context, dir string, args ...string) (string, error)

// Gen writes fixtures into Dir.
type Gen struct {
	Dir string
	Run Run
}

func (g Gen) ff(ctx context.Context, out string, args ...string) (string, error) {
	full := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-y", "-filter_threads", "1"}, args...)
	// One filter and one encoder thread: the slices of swscale and of mpeg4 follow the thread
	// count, and the fixtures must be the same bytes on every runner.
	full = append(full, "-threads", "1", out)
	if stderr, err := g.Run(ctx, g.Dir, full...); err != nil {
		return "", fmt.Errorf("mediagen %s: %w\n%s", out, err, stderr)
	}
	return filepath.Join(g.Dir, out), nil
}

func src(w, h, fps int, secs float64) []string {
	return []string{"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=s=%dx%d:r=%d:d=%g", w, h, fps, secs)}
}

func tone(secs float64) []string {
	return []string{"-f", "lavfi", "-i", fmt.Sprintf("sine=f=440:r=44100:d=%g", secs)}
}

// Clip is a video with sound: size, frame rate and duration in seconds.
func (g Gen) Clip(ctx context.Context, name string, w, h, fps int, secs float64) (string, error) {
	return g.ff(ctx, name, append(append(src(w, h, fps, secs), tone(secs)...),
		"-c:v", "mpeg4", "-q:v", "4", "-c:a", "aac", "-f", "mp4")...)
}

// Portrait is a 360x640 clip at 30 fps, 3 s, with sound.
func (g Gen) Portrait(ctx context.Context) (string, error) {
	return g.Clip(ctx, "portrait.mp4", 360, 640, 30, 3)
}

// Landscape is a 640x360 clip at 30 fps, 3 s, with sound.
func (g Gen) Landscape(ctx context.Context) (string, error) {
	return g.Clip(ctx, "landscape.mp4", 640, 360, 30, 3)
}

// Rotated is a 640x360 coded clip whose display matrix turns it a quarter turn, so it shows 360x640.
func (g Gen) Rotated(ctx context.Context) (string, error) {
	base, err := g.Clip(ctx, "rotated-base.mp4", 640, 360, 30, 3)
	if err != nil {
		return "", err
	}
	_ = base
	return g.ff(ctx, "rotated.mp4", "-display_rotation", "90", "-i", "rotated-base.mp4", "-c", "copy", "-f", "mp4")
}

// VFR is a 4 s video of a 60 fps capture with frames dropped in bursts, in Matroska, with no sound.
func (g Gen) VFR(ctx context.Context) (string, error) {
	return g.ff(ctx, "vfr.mkv", append(src(320, 240, 60, 4),
		"-vf", "select='lt(mod(n,20),11)'", "-fps_mode", "vfr", "-c:v", "mpeg4", "-q:v", "4", "-f", "matroska")...)
}

// Audio is a 3 s MP3 of a tone, a second of silence and a tone again.
func (g Gen) Audio(ctx context.Context) (string, error) {
	return g.ff(ctx, "audio.mp3",
		"-f", "lavfi", "-i", "sine=f=440:r=44100:d=1",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono:d=1",
		"-f", "lavfi", "-i", "sine=f=660:r=44100:d=1",
		"-filter_complex", "[0:a][1:a][2:a]concat=n=3:v=0:a=1[a]", "-map", "[a]",
		"-c:a", "libmp3lame", "-f", "mp3")
}

// Image is a 200x100 PNG.
func (g Gen) Image(ctx context.Context) (string, error) {
	return g.ff(ctx, "image.png", append(src(200, 100, 1, 1), "-frames:v", "1", "-c:v", "png", "-f", "image2")...)
}

// Truncated is a clip with its index at the front whose data ends early: ffprobe reads the whole
// declared length and the decoder can produce only part of it.
func (g Gen) Truncated(ctx context.Context) (string, error) {
	full, err := g.ff(ctx, "truncated-full.mp4", append(append(src(320, 240, 30, 6), tone(6)...),
		"-c:v", "mpeg4", "-q:v", "6", "-c:a", "aac", "-movflags", "+faststart", "-f", "mp4")...)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	out := filepath.Join(g.Dir, "truncated.mp4")
	err = os.WriteFile(out, b[:len(b)*2/5], 0o600) //nolint:gosec // G703: a fixture in the caller's own temp directory
	return out, err
}

// Long is a 60 s 1080x1920 clip at 30 fps with sound: the NFR-005 fixture (SDD-011 11.2). The source
// codec is MPEG-4 Part 2, which the LGPL build can encode; the original of a phone is HEVC.
func (g Gen) Long(ctx context.Context) (string, error) {
	return g.ff(ctx, "ingest-60s-1080p.mp4", append(append(src(1080, 1920, 30, 60), tone(60)...),
		"-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac", "-f", "mp4")...)
}
