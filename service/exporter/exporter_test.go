package exporter

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/export"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/service"
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
	host := filepath.Join(t.TempDir(), "vukem-render")
	a := &adapter{cfg: Config{PixelRenderer: export.PixelRendererGPU, GPUHost: host}}

	job := a.job(service.ExportInput{}, nil, nil)
	if job.GPUHost != host || job.PixelRenderer != export.PixelRendererGPU {
		t.Fatalf("job = %q %q, want %q gpu", job.GPUHost, job.PixelRenderer, host)
	}
}
