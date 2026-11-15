package transcribe

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ingest/mediagen"
)

// pinnedFFmpeg is the runner of the pinned FFmpeg, a folder of artifacts holding a 2 s WAV, and a run
// directory. It skips when no pinned FFmpeg is installed (VUKEM_VIDEO_TEST_FFMPEG_DIR,
// scripts/install-ffmpeg.sh).
func pinnedFFmpeg(t *testing.T) (r *ffmpeg.Runner, artifacts, audio string, run *ffmpeg.RunDir) {
	t.Helper()

	dir := os.Getenv("VUKEM_VIDEO_TEST_FFMPEG_DIR")
	if dir == "" {
		t.Skip("no pinned FFmpeg: set VUKEM_VIDEO_TEST_FFMPEG_DIR")
	}

	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	in, err := ffmpeg.Discover(
		context.Background(),
		ffmpeg.Config{FFmpegPath: filepath.Join(dir, name), CPUCount: runtime.NumCPU()},
	)
	if err != nil || in.Info.License.Name != "lgpl" {
		t.Skip("the FFmpeg in VUKEM_VIDEO_TEST_FFMPEG_DIR is not the pinned LGPL build")
	}

	ws := t.TempDir()

	if r, err = ffmpeg.NewRunner(in, ws); err != nil {
		t.Fatal(err)
	}

	artifacts = filepath.Join(ws, "artifacts", "aa")
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}

	gen := mediagen.Gen{
		Dir: artifacts,
		Run: func(ctx context.Context, dir string, args ...string) (string, error) {
			res, err := in.FFmpeg.Run(ctx, dir, args...)

			return res.Stderr, err
		},
	}

	if audio, err = gen.AudioFile(
		context.Background(),
		"normalized.wav",
		"-c:a",
		"pcm_s16le",
		"-f",
		"wav",
	); err != nil {
		t.Fatal(err)
	}

	if run, err = r.NewRunDir("transcribe-test"); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = run.Remove() })

	return r, artifacts, audio, run
}

// The whole path with the pinned FFmpeg and the fake helper: the WAV is written in the run directory,
// the helper reads it, the words come back.
func TestTranscribeWithTheFakeHelper(t *testing.T) {
	r, artifacts, audio, run := pinnedFFmpeg(t)

	canned, err := filepath.Abs("testdata/pause.json")
	if err != nil {
		t.Fatal(err)
	}
	w := &Whisper{
		FFmpeg: r, Helper: fakeWhisper(t), HelperVersion: "fake", ModelName: "tiny", ModelPath: fakeWhisper(t),
		Env: map[string]string{"FAKEWHISPER_MODE": "ok", "FAKEWHISPER_CANNED": canned},
	}
	var last time.Duration
	raw, err := w.Transcribe(context.Background(), Input{
		Audio: audio, InputDirs: []string{artifacts}, Duration: 4 * time.Second, Language: "pt", WorkDir: run,
	}, func(done, _ time.Duration) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Words) != 5 || raw.Info["model"] != "tiny" || last != 3500*time.Millisecond {
		t.Errorf("words %d, info %v, progress %s", len(raw.Words), raw.Info, last)
	}
	st, err := os.Stat(filepath.Join(run.Dir(), "in.wav"))
	if err != nil || st.Size() < 1000 {
		t.Errorf("the 16 kHz WAV was not written: %v", err)
	}
}

// The whole path of the API provider: the audio is cut by FFmpeg, sent under the fixed name, the words
// come back shifted by the start of the part, and the progress and the message name the host.
func TestAPITranscribeSendsOnlyTheAudio(t *testing.T) {
	r, artifacts, audio, run := pinnedFFmpeg(t)

	var body []byte

	srv, test := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ = io.ReadAll(req.Body)
		_, _ = w.Write([]byte(goodJSON))
	}))

	a, err := NewAPI(APIConfig{URL: srv.URL, Key: canaryKey, FFmpeg: r, Test: test})
	if err != nil {
		t.Fatal(err)
	}

	var last time.Duration

	raw, err := a.Transcribe(context.Background(), Input{
		Audio: audio, InputDirs: []string{artifacts}, Duration: 2 * time.Second, Language: "pt", WorkDir: run,
	}, func(done, _ time.Duration) { last = done })
	if err != nil {
		t.Fatal(err)
	}

	if len(raw.Words) != 2 || raw.Language != "pt" || last != 2*time.Second {
		t.Errorf("words %d, language %q, progress %s", len(raw.Words), raw.Language, last)
	}

	for _, secret := range []string{canaryKey, run.Dir(), artifacts, "normalized.wav", "transcribe-test"} {
		if bytes.Contains(body, []byte(secret)) {
			t.Errorf("the body carries %q", secret)
		}
	}

	if !bytes.Contains(body, []byte(`filename="audio.wav"`)) || !bytes.Contains(body, []byte("RIFF")) {
		t.Error("the body is not the audio under the fixed name")
	}
}
