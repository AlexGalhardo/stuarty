package exporter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service"
)

func TestExportRefusesWhatItCannotDo(t *testing.T) {
	doc := &timeline.Document{Items: map[string]timeline.Item{"itm_a": {Start: 0, Duration: 30}}}

	cases := map[string]struct {
		cfg  Config
		in   service.ExportInput
		code string
	}{
		"no runner":   {Config{}, service.ExportInput{Doc: doc}, "E_INTERNAL"},
		"no document": {Config{Runner: &ffmpeg.Runner{}}, service.ExportInput{}, "E_INTERNAL"},
	}

	for name, c := range cases {
		_, err := New(c.cfg).Export(context.Background(), c.in)

		fe, ok := errors.AsType[*ffmpeg.Error](err)
		if !ok || fe.Code != c.code {
			t.Errorf("%s: err = %v, want %s", name, err, c.code)
		}
	}
}

// SDD-007 section 6.5: with no forced encoder, a detected software encoder keeps the address
// space limit; only a detected hardware encoder takes the runner without it.
func TestPickRunnerKeepsTheAddressSpaceLimitForSoftwareEncoders(t *testing.T) {
	software, hardware := &ffmpeg.Runner{}, &ffmpeg.Runner{}

	cases := map[string]struct {
		det  *ffmpeg.EncoderDetection
		err  error
		want *ffmpeg.Runner
	}{
		"libx264":           {&ffmpeg.EncoderDetection{Chosen: "libx264"}, nil, software},
		"libopenh264":       {&ffmpeg.EncoderDetection{Chosen: "libopenh264"}, nil, software},
		"unknown":           {&ffmpeg.EncoderDetection{Chosen: "mpeg4"}, nil, software},
		"none":              {&ffmpeg.EncoderDetection{}, nil, software},
		"nil detection":     {nil, nil, software},
		"failed":            {&ffmpeg.EncoderDetection{Chosen: "h264_nvenc"}, errors.New("boom"), software},
		"h264_nvenc":        {&ffmpeg.EncoderDetection{Chosen: "h264_nvenc"}, nil, hardware},
		"h264_vaapi":        {&ffmpeg.EncoderDetection{Chosen: "h264_vaapi"}, nil, hardware},
		"h264_videotoolbox": {&ffmpeg.EncoderDetection{Chosen: "h264_videotoolbox"}, nil, hardware},
	}

	for name, tc := range cases {
		if got := pickRunner(tc.det, tc.err, software, hardware); got != tc.want {
			t.Errorf("%s: picked the wrong runner", name)
		}
	}
}

func TestJobCarriesTheGPUHost(t *testing.T) {
	host := filepath.Join(t.TempDir(), "stuarty-render")
	a := &adapter{cfg: Config{PixelRenderer: export.PixelRendererGPU, GPUHost: host}}

	job := a.job(service.ExportInput{}, nil, nil)
	if job.GPUHost != host || job.PixelRenderer != export.PixelRendererGPU {
		t.Fatalf("job = %q %q, want %q gpu", job.GPUHost, job.PixelRenderer, host)
	}
}

func TestDescribeReportsTheVerificationChecksForEachOutput(t *testing.T) {
	t.Parallel()

	settings := timeline.Settings{Width: 960, Height: 540}
	for _, tc := range []struct {
		format       export.Format
		video, audio string
		check, part  string
	}{
		{export.FormatGIF, "gif", "", "dimensions", "480x270 (reduced"},
		{export.FormatWAV, "", "pcm_s16le", "streams", "48000 Hz stereo"},
		{export.FormatAAC, "", "aac", "faststart", "moov before mdat"},
		{export.FormatMP4, "h264", "aac", "pixel_format", "yuv420p"},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			t.Parallel()
			video, audio, checks := describe(&export.Result{
				Format: tc.format, Width: 480, Height: 270, DurationMS: 1000, Frames: 30,
			}, settings)
			if video != tc.video || audio != tc.audio {
				t.Fatalf("codecs = %q, %q", video, audio)
			}
			for _, check := range checks {
				if check.Name == tc.check && check.OK && strings.Contains(check.Detail, tc.part) {
					return
				}
			}
			t.Fatalf("missing %q check with %q in %+v", tc.check, tc.part, checks)
		})
	}
}

func TestOutputDigestReadsThePublishedBytes(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "export.mp4")
	if _, err := sha256File(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("digest before the file exists: %v", err)
	}
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if digest, err := sha256File(path); err != nil || digest != helloSHA256 {
		t.Fatalf("digest = %q, %v", digest, err)
	}
}

func TestAudioAdapterRequiresARunnerAndDocument(t *testing.T) {
	t.Parallel()

	for _, cfg := range []Config{{}, {Runner: &ffmpeg.Runner{}}} {
		a := &adapter{cfg: cfg}
		_, measureErr := a.MeasureAudio(t.Context(), service.AudioMeasureInput{})
		previewErr := a.RenderAudioPreview(t.Context(), service.AudioPreviewInput{})
		for _, err := range []error{measureErr, previewErr} {
			if ff, ok := errors.AsType[*ffmpeg.Error](err); !ok || ff.Code != "E_INTERNAL" {
				t.Fatalf("empty audio request returned %v, want E_INTERNAL", err)
			}
		}
	}
}

func TestAudioExportResolvesThePresetWithoutDetectingAVideoEncoder(t *testing.T) {
	t.Parallel()
	a := &adapter{}
	in := service.ExportInput{Format: string(export.FormatWAV), Preset: "missing"}
	if _, _, err := a.codecPlan(t.Context(), in, nil); err == nil {
		t.Fatal("an unknown preset was accepted")
	}
	in.Preset = "youtube-shorts"
	preset, plan, err := a.codecPlan(t.Context(), in, nil)
	if err != nil || preset == nil || preset.ID != in.Preset || plan.Codec != "" {
		t.Fatalf("audio codec plan = %+v, %+v, %v", preset, plan, err)
	}
}

func TestExporterMeasuresSilenceAndReportsCodecAvailability(t *testing.T) {
	ffmpegDir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")
	if ffmpegDir == "" {
		t.Skip("verify:env-skip STUARTY_TEST_FFMPEG_DIR is not set")
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	ctx := t.Context()
	installation, err := ffmpeg.Discover(ctx, ffmpeg.Config{
		FFmpegPath: filepath.Join(ffmpegDir, "ffmpeg"+suffix), CPUCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	runner, err := ffmpeg.NewRunner(installation, workspace)
	if err != nil {
		t.Fatal(err)
	}
	a := &adapter{cfg: Config{Runner: runner, Workspace: workspace}}
	codecs := a.Codecs(ctx)
	if len(codecs) != len(exportcore.Codecs()) {
		t.Fatalf("the encoder probe returned %d codecs", len(codecs))
	}
	for _, codec := range codecs {
		if codec.Available && codec.Encoder == "" || !codec.Available && codec.Reason == "" {
			t.Fatalf("incomplete codec availability: %+v", codec)
		}
	}
	doc := &timeline.Document{
		Settings: timeline.DefaultSettings(),
		Items:    map[string]timeline.Item{"itm_visual": {Duration: 30}},
	}
	if result, err := a.MeasureAudio(
		ctx,
		service.AudioMeasureInput{Doc: doc, JobID: "measure_silence"},
	); err != nil || result.Seconds != 1 || result.Before.IntegratedLUFS != nil ||
		result.After.IntegratedLUFS != nil {
		t.Fatalf("a second of silence measured as %+v, %v", result, err)
	}
	doc.Items["itm_visual"] = timeline.Item{Duration: 30 * (export.MaxMeasureSeconds + 1)}
	if _, err := a.MeasureAudio(
		ctx,
		service.AudioMeasureInput{Doc: doc, JobID: "measure_long"},
	); !errors.Is(
		err,
		service.ErrAudioRangeTooLong,
	) {
		t.Fatalf("measure beyond the limit = %v", err)
	}
	if err := a.RenderAudioPreview(
		ctx,
		service.AudioPreviewInput{Doc: doc, JobID: "preview_empty"},
	); err == nil ||
		!strings.Contains(err.Error(), "preview source is empty") {
		t.Fatalf("preview of an empty source = %v", err)
	}
}
