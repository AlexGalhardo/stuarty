package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

func TestH264MFOmitsBFrames(t *testing.T) {
	opts := encoderOpts("h264_mf", exportcore.CodecH264, 720, 1280, timeline.Rational{Num: 30, Den: 1}, 0)
	for _, opt := range opts {
		if flag, _ := opt.Pair(); flag == "-bf" {
			t.Fatalf("h264_mf encoderOpts emitted -bf (%+v), want omitted for Media Foundation compatibility", opt)
		}
	}

	nvenc := encoderOpts("h264_nvenc", exportcore.CodecH264, 720, 1280, timeline.Rational{Num: 30, Den: 1}, 0)
	foundBF := false
	for _, opt := range nvenc {
		if flag, _ := opt.Pair(); flag == "-bf" {
			foundBF = true
		}
	}
	if !foundBF {
		t.Error("h264_nvenc should still emit -bf 2")
	}
}

func TestEncodeCandidatesRespectsForcedAndOrder(t *testing.T) {
	det := &ffmpeg.EncoderDetection{
		Chosen: "h264_nvenc",
		Candidates: []ffmpeg.Candidate{
			{Name: "h264_nvenc", Status: ffmpeg.EncoderOK},
			{Name: "h264_qsv", Status: ffmpeg.EncoderFailed},
			{Name: "h264_mf", Status: ffmpeg.EncoderOK},
			{Name: "libopenh264", Status: ffmpeg.EncoderOK},
		},
	}

	got := encodeCandidates(det.Candidates[0], det)
	var names []string
	for _, c := range got {
		names = append(names, c.Name)
	}
	want := []string{"h264_nvenc", "h264_mf", "libopenh264"}
	if !slices.Equal(names, want) {
		t.Fatalf("encodeCandidates() = %v, want %v", names, want)
	}

	forced := *det
	forced.Forced = true
	gotForced := encodeCandidates(forced.Candidates[0], &forced)
	if len(gotForced) != 1 || gotForced[0].Name != "h264_nvenc" {
		t.Fatalf("forced encodeCandidates() = %+v, want only primary encoder", gotForced)
	}
}

func TestAttachEncoderDetailsScrubsPaths(t *testing.T) {
	rawErr := exportErr(
		ReasonEncoder,
		"The video encoder failed.",
		errors.New("exit status 1"),
		"\n[h264_mf @ 000001] Failed to initialize C:\\Users\\secret\\video.mp4\nsecond line\n",
	)

	enriched := attachEncoderDetails(rawErr, "libopenh264", []string{"h264_mf", "libopenh264"})
	ffErr, ok := errors.AsType[*ffmpeg.Error](enriched)
	if !ok {
		t.Fatalf("expected *ffmpeg.Error, got %T", enriched)
	}

	if ffErr.Details["encoder"] != "libopenh264" {
		t.Errorf("details.encoder = %v, want libopenh264", ffErr.Details["encoder"])
	}

	tried, ok := ffErr.Details["tried_encoders"].([]string)
	if !ok || !slices.Equal(tried, []string{"h264_mf", "libopenh264"}) {
		t.Errorf("details.tried_encoders = %v, want [h264_mf libopenh264]", ffErr.Details["tried_encoders"])
	}

	reason, _ := ffErr.Details["encoder_reason"].(string)
	if reason == "" {
		t.Fatal("details.encoder_reason is empty")
	}
	if strings.Contains(reason, `C:\Users\secret`) || strings.Contains(reason, "\n") {
		t.Errorf("details.encoder_reason = %q, want single scrubbed line without local paths", reason)
	}
}

func TestRunEncoderCandidatesFallsBackAndRemovesPartial(t *testing.T) {
	partial := filepath.Join(t.TempDir(), "job.partial.mp4")
	candidates := []ffmpeg.Candidate{
		{Name: "h264_mf", Status: ffmpeg.EncoderOK},
		{Name: "libopenh264", Status: ffmpeg.EncoderOK},
	}

	var calls []string
	used, err := runEncoderCandidates(context.Background(), candidates, partial, func(c ffmpeg.Candidate) error {
		calls = append(calls, c.Name)
		if c.Name == "h264_mf" {
			_ = os.WriteFile(partial, []byte("corrupt"), 0o600)
			return exportErr(ReasonEncoder, "The video encoder failed.", errors.New("exit 1"), "mf error")
		}
		// Partial file from failed primary attempt must already be deleted before fallback runs.
		if _, statErr := os.Stat(partial); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("partial file still existed before fallback attempt: %v", statErr)
		}
		return os.WriteFile(partial, []byte("ok"), 0o600)
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if used.Name != "libopenh264" || !slices.Equal(calls, []string{"h264_mf", "libopenh264"}) {
		t.Fatalf("used=%q calls=%v, want libopenh264 after [h264_mf libopenh264]", used.Name, calls)
	}
}

func TestRunEncoderCandidatesDoesNotFallbackOnRenderError(t *testing.T) {
	candidates := []ffmpeg.Candidate{
		{Name: "h264_mf", Status: ffmpeg.EncoderOK},
		{Name: "libopenh264", Status: ffmpeg.EncoderOK},
	}

	calls := 0
	_, err := runEncoderCandidates(context.Background(), candidates, "", func(ffmpeg.Candidate) error {
		calls++
		return exportErr(ReasonRender, "A frame could not be composed.", errors.New("boom"), "")
	})
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v, want immediate failure on non-encoder error", calls, err)
	}
}

func TestRunEncoderCandidatesEnrichesErrorWhenAllFail(t *testing.T) {
	candidates := []ffmpeg.Candidate{
		{Name: "h264_nvenc", Status: ffmpeg.EncoderOK},
		{Name: "h264_mf", Status: ffmpeg.EncoderOK},
	}

	_, err := runEncoderCandidates(context.Background(), candidates, "", func(c ffmpeg.Candidate) error {
		return exportErr(ReasonEncoder, "The video encoder failed.", errors.New("exit 1"), c.Name+" failed to open")
	})
	ffErr, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok {
		t.Fatalf("expected *ffmpeg.Error, got %v", err)
	}
	if ffErr.Details["encoder"] != "h264_mf" {
		t.Errorf("encoder = %v, want h264_mf", ffErr.Details["encoder"])
	}
	tried, _ := ffErr.Details["tried_encoders"].([]string)
	if !slices.Equal(tried, []string{"h264_nvenc", "h264_mf"}) {
		t.Errorf("tried_encoders = %v, want [h264_nvenc h264_mf]", tried)
	}
	if reason, _ := ffErr.Details["encoder_reason"].(string); !strings.Contains(reason, "h264_mf failed to open") {
		t.Errorf("encoder_reason = %q", reason)
	}
}
