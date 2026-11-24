//go:build windows

package servicetest

import (
	"context"
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service"
)

// The scripted end to end run of epic 61, task 10, with the real helper: a person records the first display, stops,
// adds the recording to the project, places it on the timeline, and the clip lasts what the file lasts. The frames
// go to the workspace of the test, which is removed with it, and nothing reads them.
//
// verify:env-skip needs a Windows desktop session, the built stuarty-capture.exe (scripts/build-capture.sh, named by
// STUARTY_TEST_CAPTURE_HELPER) and the pinned FFmpeg (STUARTY_TEST_FFMPEG_DIR); with either missing the test says why
// and passes without capturing. STUARTY_TEST_RECORD_SECONDS sets the length, 6 by default (60 for criterion 1).
func TestRecordingEndToEndWithTheRealHelper(t *testing.T) {
	helper := os.Getenv("STUARTY_TEST_CAPTURE_HELPER")
	if helper == "" {
		t.Skip("verify:env-skip STUARTY_TEST_CAPTURE_HELPER names no built stuarty-capture.exe")
	}

	seconds := 6

	if v := os.Getenv("STUARTY_TEST_RECORD_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("STUARTY_TEST_RECORD_SECONDS=%q", v)
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
	}})

	project := m.newProject("e2e")
	ctx := personCtx()

	sources, err := m.svc.RecordingSources(ctx, service.RecordingSourcesRequest{})
	if err != nil {
		t.Fatalf("RecordingSources: %v", err)
	}

	if len(sources.Displays) == 0 {
		t.Skip("verify:env-skip this session has no display to capture")
	}

	rec, err := m.svc.RecordingStart(ctx, service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: sources.Displays[0].ID}, FPS: 30,
	})
	if err != nil {
		t.Fatalf("RecordingStart: %v", err)
	}

	began := time.Now()

	// the recorded duration is what is under test, so the test waits for exactly that long
	<-time.After(time.Duration(seconds) * time.Second)

	if _, err := m.svc.RecordingStop(m.as(agent), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatalf("RecordingStop: %v", err)
	}

	wall := time.Since(began).Seconds()
	ready := waitRecording(t, m, rec.ID, "ready")

	got := float64(ready.DurationMS) / 1000
	if math.Abs(got-wall) > 0.5 {
		t.Errorf("ffprobe reports %.2f s for a recording of %.2f s (the criterion allows 0.5 s)", got, wall)
	}

	t.Logf("recorded %.2f s of wall time, the file lasts %.2f s, %d frames, %d repeated, encoder %s",
		wall, got, ready.Frames, ready.Dropped, ready.Encoder)

	if _, err := m.svc.RecordingConfirm(ctx, service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatalf("RecordingConfirm: %v", err)
	}

	var placed *service.EditResult

	pollUntil(30*time.Second, func() bool {
		placed, err = m.svc.RecordingPlace(m.as(agent), service.RecordingPlaceRequest{ID: rec.ID})

		return err == nil || code(err) != "E_ASSET_NOT_READY"
	})

	if err != nil {
		t.Fatalf("RecordingPlace: %v", err)
	}

	pg, err := m.svc.ProjectGet(m.as(agent), service.ProjectGetRequest{ProjectID: project, Detail: "document"})
	if err != nil {
		t.Fatal(err)
	}

	if len(pg.Document.Items) != 1 {
		t.Fatalf(
			"the timeline has %d items after placing, want one clip (revision %d)",
			len(pg.Document.Items), placed.Revision,
		)
	}

	fps := float64(pg.Document.Settings.FPS.Num) / float64(pg.Document.Settings.FPS.Den)

	for _, item := range pg.Document.Items {
		clip := float64(item.Duration) / fps
		if math.Abs(clip-got) > 0.5 {
			t.Errorf("the clip lasts %.2f s, the recording %.2f s", clip, got)
		}
	}
}

func waitRecording(t *testing.T, m *mediaEnv, id, want string) *service.Recording {
	t.Helper()

	var rec *service.Recording

	pollUntil(60*time.Second, func() bool {
		var err error

		rec, err = m.svc.RecordingGet(personCtx(), service.RecordingRef{ID: id})
		if err != nil {
			t.Fatal(err)
		}

		return rec.State == want || rec.State == "failed" || rec.State == "cancelled"
	})

	if rec.State != want {
		t.Fatalf("recording %s is %s, want %s: %+v", id, rec.State, want, rec.Error)
	}

	return rec
}
