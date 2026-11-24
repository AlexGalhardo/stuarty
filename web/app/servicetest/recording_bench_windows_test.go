//go:build windows

package servicetest

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service"
)

// The measure of NFR-067 (PRD-004): at most 1% of the frames dropped when a window of 1920 by 1080 is recorded at
// 60 fps for 10 minutes, through the real helper, the real host and the real FFmpeg. A dropped frame is a slot of
// the 60 fps grid with no new frame from the system. It is run by scripts/measure-capture.ps1, which opens the test
// window and prints the load of the machine; the number counts only on an idle machine.
//
// verify:env-skip needs STUARTY_BENCH_WINDOW (the title of the test window), STUARTY_TEST_CAPTURE_HELPER and
// STUARTY_TEST_FFMPEG_DIR; without them the test says why and passes. STUARTY_BENCH_SECONDS is the length (600).
func TestCaptureNFR067(t *testing.T) {
	title := os.Getenv("STUARTY_BENCH_WINDOW")
	helper := os.Getenv("STUARTY_TEST_CAPTURE_HELPER")

	if title == "" || helper == "" {
		t.Skip("verify:env-skip run scripts/measure-capture.ps1: it opens the test window and sets the variables")
	}

	seconds := 600

	if v := os.Getenv("STUARTY_BENCH_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("STUARTY_BENCH_SECONDS=%q", v)
		}

		seconds = n
	}

	in := pinnedFFmpeg(t)

	m := newMediaEnv(t, mediaCfg{realIngest: true, realExport: true, with: func(_ *env, o *service.Options) {
		run := o.Media.Runner

		o.Capture = service.NewLiveCapturer(&capturehost.Live{
			Runner: run, HelperPath: helper, FFmpegPath: in.FFmpeg.Path(),
			Detect: func(ctx context.Context) (*ffmpeg.EncoderDetection, error) {
				caps, err := run.ReadCapabilities(ctx)
				if err != nil {
					return nil, err
				}

				return run.DetectEncoder(ctx, caps, ffmpeg.DetectOptions{})
			},
		})
		o.Limits.MinFreeBytes = 1 << 20
		o.RecordingMaxSeconds = seconds + 60
	}})

	sources, err := m.svc.RecordingSources(personCtx(), service.RecordingSourcesRequest{})
	if err != nil {
		t.Fatal(err)
	}

	id := ""

	for _, w := range sources.Windows {
		if strings.Contains(w.Name, title) {
			id = w.ID

			break
		}
	}

	if id == "" {
		t.Fatalf("the window %q is not among the sources: %+v", title, sources.Windows)
	}

	rec, err := m.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		ProjectID: m.newProject("bench"), Source: service.RecordingSource{Kind: "window", ID: id, Name: title}, FPS: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("recording %s: %dx%d at %d fps with %s", rec.ID, rec.Width, rec.Height, rec.FPS, rec.Encoder)

	// the recorded duration is what is under test, so the test waits for exactly that long
	<-time.After(time.Duration(seconds) * time.Second)

	if _, err := m.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	done := waitRecording(t, m, rec.ID, "ready")

	rate := 100 * float64(done.Dropped) / float64(max(done.Frames, 1))

	t.Logf("NFR-067: %d of %d frames dropped = %.2f%%, the file lasts %.2f s for %d s recorded",
		done.Dropped, done.Frames, rate, float64(done.DurationMS)/1000, seconds)

	// A recording that ends early is not a rate to judge: the file must last what was recorded.
	if got := float64(done.DurationMS) / 1000; got < float64(seconds)-2 || got > float64(seconds)+2 {
		t.Errorf("the file lasts %.2f s for %d s recorded", got, seconds)
	}

	if done.Error != nil || done.StopReason != "stopped" {
		t.Logf("the recording ended with stop reason %q and error %+v", done.StopReason, done.Error)
	}

	if rate > 1 {
		t.Errorf("%.2f%% of the frames dropped, NFR-067 allows 1%% (valid only on an idle machine)", rate)
	}
}
