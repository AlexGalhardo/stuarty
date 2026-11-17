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
		"-c:v", "mpeg4", "-dct", "int", "-q:v", "4", "-c:a", "aac", "-f", "mp4")...)
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
		"-vf", "select='lt(mod(n,20),11)'", "-fps_mode", "vfr",
		"-c:v", "mpeg4", "-dct", "int", "-q:v", "4", "-f", "matroska")...)
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

// AlphaImage is a 200x100 RGBA PNG whose left half is fully transparent and whose right half is opaque red.
func (g Gen) AlphaImage(ctx context.Context) (string, error) {
	return g.ff(ctx, "alpha.png",
		"-f", "lavfi", "-i", "color=c=red:s=200x100:r=1:d=1",
		"-vf", "format=rgba,geq=r='r(X,Y)':g='g(X,Y)':b='b(X,Y)':a='if(gte(X,100),255,0)'",
		"-frames:v", "1", "-c:v", "png", "-pix_fmt", "rgba", "-f", "image2")
}

// Truncated is a clip with its index at the front whose data ends early: ffprobe reads the whole
// declared length and the decoder can produce only part of it.
func (g Gen) Truncated(ctx context.Context) (string, error) {
	full, err := g.ff(ctx, "truncated-full.mp4", append(append(src(320, 240, 30, 6), tone(6)...),
		"-c:v", "mpeg4", "-dct", "int", "-q:v", "6", "-c:a", "aac", "-movflags", "+faststart", "-f", "mp4")...)
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

// AudioFile is a 2 s tone at 44.1 kHz in the container and codec the arguments name, for example
// AudioFile(ctx, "tone.wav", "-c:a", "pcm_s16le", "-f", "wav").
func (g Gen) AudioFile(ctx context.Context, name string, codec ...string) (string, error) {
	return g.ff(ctx, name, append(tone(2), codec...)...)
}

// NoisyTone is a 2 s WAV of white noise (about -48 dBFS once normalized) with a 440 Hz tone joining it at 1 s (the first second is the
// noise alone, as a pause in speech is), for the tests of the noise reduction. The noise is seeded, so the file is the same bytes every time.
func (g Gen) NoisyTone(ctx context.Context, name string) (string, error) {
	return g.ff(ctx, name,
		"-f", "lavfi", "-i", "sine=f=440:r=48000:d=2",
		"-f", "lavfi", "-i", "anoisesrc=d=2:r=48000:a=0.004:c=white:seed=1",
		"-filter_complex", "[0:a]volume='if(gte(t,1),1,0)':eval=frame[t];[t][1:a]amix=inputs=2:normalize=0",
		"-c:a", "pcm_s16le", "-f", "wav")
}

// OneClick is a mono WAV of 2 s of silence with one 10 ms burst of a 1 kHz tone at 0.5 s, the fixture that shows
// where a speed curve or a reverse puts a sound (SDD-004 sections 3.11 and 3.12).
func (g Gen) OneClick(ctx context.Context, name string) (string, error) {
	return g.ff(ctx, name,
		"-f", "lavfi", "-i", "aevalsrc=exprs='0.8*sin(2*PI*1000*t)*between(t,0.5,0.51)':s=48000:d=2",
		"-ar", "48000", "-ac", "1", "-c:a", "pcm_s16le", "-f", "wav")
}

// Speech3Pauses is a WAV of four 1 s tones separated by pauses of 0.5 s, 1.0 s and 0.2 s of
// digital silence: the speech fixture of the silence tests.
func (g Gen) Speech3Pauses(ctx context.Context) (string, error) {
	return g.ff(ctx, "speech-3-pauses.wav",
		"-f", "lavfi", "-i", "sine=f=300:r=48000:d=1",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono:d=0.5",
		"-f", "lavfi", "-i", "sine=f=400:r=48000:d=1",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono:d=1",
		"-f", "lavfi", "-i", "sine=f=500:r=48000:d=1",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono:d=0.2",
		"-f", "lavfi", "-i", "sine=f=600:r=48000:d=1",
		"-filter_complex", "[0:a][1:a][2:a][3:a][4:a][5:a][6:a]concat=n=7:v=0:a=1[a]",
		"-map", "[a]", "-ar", "48000", "-ac", "1", "-c:a", "pcm_s16le", "-f", "wav")
}

// Click120 is a mono WAV of 20 s with a decaying 1 kHz click on every beat of a 120 BPM grid, the
// first at 0.25 s: the beat fixture of the beat grid tests.
func (g Gen) Click120(ctx context.Context) (string, error) {
	return g.ff(ctx, "click-120.wav",
		"-f", "lavfi", "-i", "aevalsrc=exprs='0.8*sin(2*PI*1000*t)*exp(-50*mod(t+0.25,0.5))':s=48000:d=20",
		"-ar", "48000", "-ac", "1", "-c:a", "pcm_s16le", "-f", "wav")
}

// Tone23LUFS is a mono 1 kHz sine of 5 s whose integrated loudness is -23 LUFS (the lavfi sine peaks at
// 0.125, so a volume of 0.8 gives a peak of 0.1, which is -23 LUFS for a 1 kHz sine).
func (g Gen) Tone23LUFS(ctx context.Context) (string, error) {
	return g.ff(ctx, "tone-23lufs.wav",
		"-f", "lavfi", "-i", "sine=f=1000:r=48000:d=5", "-af", "volume=0.8",
		"-ar", "48000", "-ac", "1", "-c:a", "pcm_s16le", "-f", "wav")
}

// Long is a 60 s 1080x1920 clip at 30 fps with sound: the NFR-005 fixture (SDD-011 11.2). The source
// codec is MPEG-4 Part 2, which the LGPL build can encode; the original of a phone is HEVC.
func (g Gen) Long(ctx context.Context) (string, error) {
	return g.ff(ctx, "ingest-60s-1080p.mp4", append(append(src(1080, 1920, 30, 60), tone(60)...),
		"-c:v", "mpeg4", "-dct", "int", "-q:v", "5", "-c:a", "aac", "-f", "mp4")...)
}

// AlphaClip is a ProRes 4444 clip with an alpha channel and no sound: the left half is fully
// transparent and the right half is opaque red.
func (g Gen) AlphaClip(ctx context.Context, name string, w, h, fps int, secs float64) (string, error) {
	return g.ff(
		ctx,
		name,
		"-f",
		"lavfi",
		"-i",
		fmt.Sprintf("color=c=red:s=%dx%d:r=%d:d=%g", w, h, fps, secs),
		"-vf",
		fmt.Sprintf(
			"format=yuva444p,geq=lum='lum(X,Y)':cb='cb(X,Y)':cr='cr(X,Y)':a='if(gte(X,%d),255,0)',format=yuva444p10le",
			w/2,
		),
		"-c:v",
		"prores_ks",
		"-profile:v",
		"4444",
		"-pix_fmt",
		"yuva444p10le",
		"-f",
		"mov",
	)
}

// AlphaClipWithSound is AlphaClip with a 440 Hz tone: the left half is transparent, the right half opaque
// red, and the sound is 16-bit PCM.
func (g Gen) AlphaClipWithSound(ctx context.Context, name string, w, h, fps int, secs float64) (string, error) {
	return g.ff(
		ctx,
		name,
		append(append([]string{
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=red:s=%dx%d:r=%d:d=%g", w, h, fps, secs),
		}, tone(secs)...),
			"-vf", fmt.Sprintf(
				"format=yuva444p,geq=lum='lum(X,Y)':cb='cb(X,Y)':cr='cr(X,Y)':a='if(gte(X,%d),255,0)',format=yuva444p10le",
				w/2,
			),
			"-c:v", "prores_ks", "-profile:v", "4444", "-pix_fmt", "yuva444p10le", "-c:a", "pcm_s16le", "-f", "mov")...,
	)
}

// AlphaWebM is a VP9 clip in WebM with an alpha plane and no sound: the left half is fully transparent
// and the right half is opaque red. VP9 keeps the alpha apart and ffprobe still says yuv420p.
func (g Gen) AlphaWebM(ctx context.Context, name string, w, h, fps int, secs float64) (string, error) {
	return g.ff(
		ctx,
		name,
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=red:s=%dx%d:r=%d:d=%g", w, h, fps, secs),
		"-vf", fmt.Sprintf(
			"format=yuva420p,geq=lum='lum(X,Y)':cb='cb(X,Y)':cr='cr(X,Y)':a='if(gte(X,%d),255,0)'", w/2),
		"-c:v", "libvpx-vp9", "-pix_fmt", "yuva420p", "-auto-alt-ref", "0", "-crf", "20", "-b:v", "0",
		"-f", "webm",
	)
}
