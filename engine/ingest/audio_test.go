package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

func TestMinSilence(t *testing.T) {
	t.Parallel()

	r := []SilenceRange{{0, 7200}, {10000, 58000}, {60000, 69600}}
	if got := MinSilence(r, 19200); len(got) != 1 || got[0].Start != 10000 {
		t.Errorf("min 400 ms: %v", got)
	}

	if got := MinSilence(r, 0); len(got) != 3 {
		t.Errorf("no minimum: %v", got)
	}
}

// probedSamples is the duration of the audio stream of the probe artifact, in samples at 48 kHz.
func probedSamples(t *testing.T, e *env, hash string, res *Result) int64 {
	t.Helper()

	var doc struct {
		FFprobe struct {
			Streams []struct {
				CodecType  string `json:"codec_type"`
				DurationTS int64  `json:"duration_ts"`
				TimeBase   string `json:"time_base"`
			} `json:"streams"`
		} `json:"ffprobe"`
	}

	if err := json.Unmarshal(readJSONFile(t, e.publishedPath(hash, res.find("probe", ""))), &doc); err != nil {
		t.Fatal(err)
	}

	for _, s := range doc.FFprobe.Streams {
		if s.CodecType != "audio" {
			continue
		}

		var num, den int64
		if _, err := fmt.Sscanf(s.TimeBase, "%d/%d", &num, &den); err != nil {
			t.Fatal(err)
		}

		return int64(math.Round(float64(s.DurationTS) * float64(num) / float64(den) * SampleRate))
	}

	t.Fatal("no audio stream in the probe")

	return 0
}

// TestAudioFacts: an MP3, a WAV, an AAC file and the audio of an MP4 each produce the normalized
// and peaks artifacts, and sample_count is the duration of the source at 48 kHz.
func TestAudioFacts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		codec []string
	}{
		{"tone.mp3", []string{"-c:a", "libmp3lame", "-f", "mp3"}},
		{"tone.wav", []string{"-c:a", "pcm_s16le", "-f", "wav"}},
		{"tone.aac", []string{"-c:a", "aac", "-f", "adts"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := e.gen.AudioFile(ctx, c.name, c.codec...)
			hash, res := e.mustIngest(mustGen(t, p, err))
			checkAudioFacts(t, e, hash, res)
		})
	}

	t.Run("audio of an mp4", func(t *testing.T) {
		p, err := e.gen.Clip(ctx, "with-audio.mp4", 160, 120, 30, 2)
		hash, res := e.mustIngest(mustGen(t, p, err))
		checkAudioFacts(t, e, hash, res)

		if !res.Facts.HasAudio {
			t.Error("has_audio is false")
		}
	})
}

func checkAudioFacts(t *testing.T, e *env, hash string, res *Result) {
	t.Helper()

	if res.find("normalized", "") == nil || res.find("peaks", "") == nil {
		t.Fatalf("artifacts %v", res.names())
	}

	want := probedSamples(t, e, hash, res)
	t.Logf("sample_count %d, probe %d", res.Facts.SampleCount, want)

	// The decoder of a lossy format pads or trims a few frames at the ends.
	if d := res.Facts.SampleCount - want; d < -2400 || d > 2400 {
		t.Errorf("sample_count %d, probe says %d", res.Facts.SampleCount, want)
	}

	if res.Facts.SampleCount <= 0 {
		t.Error("sample_count is not set")
	}
}

// A text payload with an .mp3 name is not media: the sniff refuses it before FFmpeg starts.
func TestTextPayloadWithAnMP3NameIsRefused(t *testing.T) {
	e := newEnv(t)
	path := filepath.Join(t.TempDir(), "note.mp3")

	body := []byte("root:x:0:0:root:/root:/bin/bash\nthis is not audio at all, only text\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := e.ingest(context.Background(), path, nil)

	var fe *ffmpeg.Error
	if !errors.As(err, &fe) || fe.Code != ffmpeg.CodeUnsupportedMedia {
		t.Fatalf("error = %v, want E_UNSUPPORTED_MEDIA", err)
	}
}

func readLoudness(t *testing.T, e *env, hash string, res *Result) Loudness {
	t.Helper()

	var l Loudness
	if err := json.Unmarshal(readJSONFile(t, e.publishedPath(hash, res.find("loudness", ""))), &l); err != nil {
		t.Fatal(err)
	}

	return l
}

// The fixture has pauses of 0.5 s, 1.0 s and 0.2 s: two of them last 0.4 s or more, one lasts 0.8 s or more.
func TestSilencesOnSpeechFixture(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Speech3Pauses(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))

	var s Silence
	if err := json.Unmarshal(readJSONFile(t, e.publishedPath(hash, res.find("silence", ""))), &s); err != nil {
		t.Fatal(err)
	}

	if len(s.Sets) != 4 || s.Sets[1].ThresholdDB != -35 {
		t.Fatalf("sets %+v", s.Sets)
	}

	set := s.Sets[1].Ranges
	if len(set) != 3 {
		t.Fatalf("ranges at -35 dB: %v", set)
	}

	if got := MinSilence(set, 400*SampleRate/1000); len(got) != 2 {
		t.Errorf("min_silence 0.4 s: %v, want 2 ranges", got)
	}

	if got := MinSilence(set, 800*SampleRate/1000); len(got) != 1 {
		t.Errorf("min_silence 0.8 s: %v, want 1 range", got)
	}

	for i, want := range []int64{24000, 48000, 9600} {
		if d := set[i].End - set[i].Start - want; d < -512 || d > 512 {
			t.Errorf("range %d lasts %d samples, want %d", i, set[i].End-set[i].Start, want)
		}
	}

	if kept := MinSilence(set, 19200); len(kept) != 2 {
		t.Errorf("min_silence 0.4 s kept %v, want 2 ranges", kept)
	}
}

// A -23 LUFS reference tone measures -23 LUFS within 0.5 LU.
func TestLoudnessFixture(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Tone23LUFS(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))

	l := readLoudness(t, e, hash, res)
	if l.Silent || l.IntegratedLUFS == nil {
		t.Fatalf("loudness %+v", l)
	}

	t.Logf("%.2f LUFS", *l.IntegratedLUFS)

	if math.Abs(*l.IntegratedLUFS+23) > 0.5 {
		t.Errorf("%.2f LUFS, want -23 within 0.5 LU", *l.IntegratedLUFS)
	}
}

func TestSilentFileIsMarkedSilent(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AudioFile(context.Background(), "quiet.wav", "-af", "volume=0", "-c:a", "pcm_s16le", "-f", "wav")
	hash, res := e.mustIngest(mustGen(t, p, err))

	if l := readLoudness(t, e, hash, res); !l.Silent || l.IntegratedLUFS != nil {
		t.Errorf("loudness %+v, want silent", l)
	}
}
