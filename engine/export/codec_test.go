package export

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

func optsText(name string, codec exportcore.Codec, kbps int) string {
	var parts []string

	for _, o := range encoderOpts(name, codec, 1080, 1920, timeline.Rational{Num: 30, Den: 1}, kbps) {
		n, v := o.Pair()
		parts = append(parts, n+" "+v)
	}

	return strings.Join(parts, " ")
}

// TestHEVCAndAV1EncoderOptions: the options of each new encoder (SDD-004 section 8.3): the bit rate of the
// codec, the tag HEVC needs in MP4, no profile and no B-frame option, and the rate of a preset when there is one.
func TestHEVCAndAV1EncoderOptions(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"hevc_nvenc":        "-c:v hevc_nvenc -b:v 4800k -tag:v hvc1 -g 15",
		"hevc_vaapi":        "-c:v hevc_vaapi -b:v 4800k -tag:v hvc1 -g 15",
		"hevc_videotoolbox": "-c:v hevc_videotoolbox -b:v 4800k -tag:v hvc1 -g 15",
		"av1_nvenc":         "-c:v av1_nvenc -b:v 4000k -g 15",
		"av1_qsv":           "-c:v av1_qsv -b:v 4000k -g 15",
		"libsvtav1":         "-c:v libsvtav1 -b:v 4000k -preset 8 -g 15",
		"libaom-av1":        "-c:v libaom-av1 -b:v 4000k -cpu-used 8 -row-mt 1 -g 15",
	}

	for name, w := range want {
		codec := exportcore.Codec(ffmpeg.CodecOf(name))
		if got := optsText(name, codec, 0); got != w {
			t.Errorf("%s: %q, want %q", name, got, w)
		}
	}

	if got := optsText("hevc_amf", exportcore.CodecHEVC, 3300); got != "-c:v hevc_amf -b:v 3300k -tag:v hvc1 -g 15" {
		t.Errorf("a preset rate: %q", got)
	}

	// The H.264 encoders keep their options, and a preset rate replaces only the bit rate.
	if got := optsText("h264_nvenc", "", 5000); got != "-c:v h264_nvenc -b:v 5000k -profile:v high -g 15 -bf 2" {
		t.Errorf("h264 with a preset rate: %q", got)
	}
}

// TestHEVCAndAV1EncodeArgvIsInsideTheGrammar: the whole encoder command of every allowed encoder, with and
// without a preset, passes the argument checker (a builder that cannot make a refused argv).
func TestHEVCAndAV1EncodeArgvIsInsideTheGrammar(t *testing.T) {
	setup(t)

	doc := loadScene(t, "contain-text")
	ig, _ := exportcore.Lookup("instagram-reels")

	job := newJob(t, doc, "argv-grammar", "argv.mp4")
	s := testState(t, job, "argv-grammar")

	// The mix and the output are inside the run directory, as in a real export.
	mix, _ := s.dir.Path("mix.wav")
	out, _ := s.dir.Path("out.mp4")

	for _, enc := range slices.Concat(ffmpeg.HEVCEncoders, ffmpeg.AV1Encoders) {
		for _, p := range []*exportcore.Preset{nil, &ig} {
			s.job.Preset = p

			cand := ffmpeg.Candidate{Name: enc, Status: ffmpeg.EncoderOK}
			if strings.HasSuffix(enc, "_vaapi") {
				cand.Input.VaapiDevice = "/dev/dri/renderD128"
				cand.Input.Filter = "format=nv12,hwupload"
			} else {
				cand.Input.Filter = "format=yuv420p"
			}

			argv, err := s.encodeArgv(cand, exportcore.Codec(ffmpeg.CodecOf(enc)), mix, out)
			if err != nil {
				t.Errorf("%s: %v", enc, err)

				continue
			}

			if i := slices.Index(argv, "-c:v"); i < 0 || argv[i+1] != enc {
				t.Errorf("%s: %v", enc, argv)
			}
		}
	}
}

// allowedOrSkip finds an encoder of the codec that passes its test encode on this machine, or skips with the
// reason of every candidate.
func allowedOrSkip(t *testing.T, codec exportcore.Codec) (*ffmpeg.Runner, string) {
	t.Helper()
	setup(t)

	r, err := ffmpeg.NewRunner(shared.runner.In, shared.dir)
	if err != nil {
		t.Fatal(err)
	}

	det, err := r.DetectCodec(context.Background(), shared.caps, string(codec), ffmpeg.DetectOptions{})
	if err != nil {
		var why []string
		if det != nil {
			for _, c := range det.Candidates {
				why = append(why, c.Name+"="+string(c.Status))
			}
		}

		t.Skipf( // verify:env-skip the machine has no allowed encoder of the codec
			"no allowed %s encoder passes its test encode on this machine (%s)", codec, strings.Join(why, " "),
		)
	}

	return r, det.Chosen
}

func exportWith(t *testing.T, r *ffmpeg.Runner, id string, codec exportcore.Codec, preset *exportcore.Preset) *Result {
	t.Helper()

	doc := loadScene(t, "contain-text")
	job := newJob(t, doc, id, filepath.Join(id, "out.mp4"))
	job.Runner, job.Encoder = r, ffmpeg.DetectOptions{}
	job.Codec, job.Preset = codec, preset

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}

	return res
}

// TestHEVCExportPassesVerification is acceptance criterion 3 for HEVC: where an allowed HEVC encoder works,
// the export uses it, is tagged hvc1 and passes the verification (which requires an hevc stream and its
// packet count); where none does, it skips and says why. The export is the one of a preset that prefers HEVC,
// so the same file shows the preset taking the first codec the machine offers.
func TestHEVCExportPassesVerification(t *testing.T) {
	r, enc := allowedOrSkip(t, exportcore.CodecHEVC)

	tiktok, _ := exportcore.Lookup("tiktok")
	doc := loadScene(t, "contain-text")

	plan, err := PlanCodec(context.Background(), r, shared.caps, ffmpeg.DetectOptions{}, "", &tiktok)
	if err != nil || plan.Codec != exportcore.CodecHEVC || plan.Wanted != "" || len(plan.Notes) != 0 {
		t.Fatalf("a preset that prefers hevc, on a machine that offers it: %+v %v", plan, err)
	}

	res := exportWith(t, r, "hevc-export", exportcore.CodecHEVC, nil)
	if res.Codec != exportcore.CodecHEVC || res.Encoder != enc || res.CodecWanted != "" {
		t.Errorf("result %+v, want hevc by %s", res, enc)
	}

	t.Logf("hevc by %s: %d bytes (%dx%d)", res.Encoder, res.Size, doc.Settings.Width, doc.Settings.Height)
}

// TestAV1ExportPassesVerification is the same for AV1.
func TestAV1ExportPassesVerification(t *testing.T) {
	r, enc := allowedOrSkip(t, exportcore.CodecAV1)

	res := exportWith(t, r, "av1-export", exportcore.CodecAV1, nil)
	if res.Codec != exportcore.CodecAV1 || res.Encoder != enc {
		t.Errorf("result %+v, want av1 by %s", res, enc)
	}

	t.Logf("av1 by %s: %d bytes", res.Encoder, res.Size)
}

// noEncoderRunner is a runner of the same FFmpeg and a listing without the encoders of the codec: a machine
// that has none.
func noEncoderRunner(t *testing.T, drop ...string) (*ffmpeg.Runner, *ffmpeg.Capabilities) {
	t.Helper()
	setup(t)

	r, err := ffmpeg.NewRunner(shared.runner.In, shared.dir)
	if err != nil {
		t.Fatal(err)
	}

	caps := *shared.caps
	caps.Encoders = maps.Clone(shared.caps.Encoders)

	for _, name := range drop {
		delete(caps.Encoders, name)
	}

	return r, &caps
}

// TestHEVCWithNoEncoderFallsBackToH264 is acceptance criterion 3 for a machine without an allowed encoder:
// the export of a preset that prefers HEVC is written as H.264, says so in the result and does not fail.
func TestHEVCWithNoEncoderFallsBackToH264(t *testing.T) {
	r, caps := noEncoderRunner(t, slices.Concat(ffmpeg.HEVCEncoders, ffmpeg.AV1Encoders)...)

	tiktok, _ := exportcore.Lookup("tiktok")
	doc := loadScene(t, "contain-text")

	job := newJob(t, doc, "hevc-fallback", filepath.Join("hevc-fallback", "out.mp4"))
	job.Runner, job.Caps, job.Encoder = r, caps, ffmpeg.DetectOptions{Forced: "libopenh264"}
	job.Preset = &tiktok

	// The scene is 1080x1920 at 30 fps and 3 s, inside every limit of tiktok: a failed verification is a defect.
	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}

	if res.Codec != exportcore.CodecH264 || res.CodecWanted != exportcore.CodecHEVC || len(res.Notes) != 1 ||
		!strings.Contains(res.Notes[0], "HEVC") || !strings.Contains(res.Notes[0], "H.264") {
		t.Errorf("result %+v", res)
	}
}

// TestHEVCNamedAndUnavailableIsAStableError: a codec that is asked for by name and has no encoder is
// E_NO_ENCODER with the reason codec_unavailable, never a crash and never another codec.
func TestHEVCNamedAndUnavailableIsAStableError(t *testing.T) {
	r, caps := noEncoderRunner(t, slices.Concat(ffmpeg.HEVCEncoders, ffmpeg.AV1Encoders)...)

	for _, codec := range []exportcore.Codec{exportcore.CodecHEVC, exportcore.CodecAV1} {
		doc := loadScene(t, "contain-text")
		job := newJob(t, doc, "named-"+string(codec), filepath.Join("named", string(codec), "out.mp4"))
		job.Runner, job.Caps, job.Codec = r, caps, codec

		_, err := Run(context.Background(), job)

		e, ok := errors.AsType[*ffmpeg.Error](err)
		if !ok || e.Code != ffmpeg.CodeNoEncoder || e.Reason != ffmpeg.ReasonCodecUnavailable ||
			e.Details["codec"] != string(codec) {
			t.Errorf("%s: %v", codec, err)
		}

		if _, serr := os.Stat(job.OutPath); serr == nil {
			t.Errorf("%s: a file was left behind", codec)
		}
	}
}

// TestHEVCNeverUsesAGPLEncoder is acceptance criterion 4: a libx265 that is in the user's FFmpeg is never used,
// not for HEVC asked by name and not for a preset that prefers HEVC, whatever else the machine has.
func TestHEVCNeverUsesAGPLEncoder(t *testing.T) {
	r, caps := noEncoderRunner(t, ffmpeg.HEVCEncoders...)
	caps.Encoders["libx265"] = true

	doc := loadScene(t, "contain-text")
	job := newJob(t, doc, "gpl-named", filepath.Join("gpl", "named.mp4"))
	job.Runner, job.Caps, job.Codec = r, caps, exportcore.CodecHEVC

	_, err := Run(context.Background(), job)
	if e, ok := errors.AsType[*ffmpeg.Error](err); !ok || e.Code != ffmpeg.CodeNoEncoder ||
		strings.Contains(e.Message, "libx265") {
		t.Fatalf("hevc by name with only libx265: %v", err)
	}

	det, _ := r.DetectCodec(context.Background(), caps, ffmpeg.CodecHEVC, ffmpeg.DetectOptions{})
	for _, c := range det.Candidates {
		if c.Name == "libx265" {
			t.Fatalf("libx265 was tried: %+v", c)
		}
	}

	plan, perr := PlanCodec(context.Background(), r, caps, ffmpeg.DetectOptions{}, "", presetOf(t, "instagram-reels"))
	if perr != nil || plan.Codec != exportcore.CodecH264 || plan.Wanted != exportcore.CodecHEVC {
		t.Fatalf("plan %+v %v", plan, perr)
	}
}

func presetOf(t *testing.T, id string) *exportcore.Preset {
	t.Helper()

	p, ok := exportcore.Lookup(id)
	if !ok {
		t.Fatalf("no preset %s", id)
	}

	return &p
}

// TestHEVCPlanCodecCases: the plan of an export in each case, over a runner whose machine offers what the
// test lists.
func TestHEVCPlanCodecCases(t *testing.T) {
	setup(t)

	ctx := context.Background()
	noHW := slices.Concat(ffmpeg.HEVCEncoders, ffmpeg.AV1Encoders)

	t.Run("no preset and no codec is h264", func(t *testing.T) {
		r, caps := noEncoderRunner(t, noHW...)

		plan, err := PlanCodec(ctx, r, caps, ffmpeg.DetectOptions{}, "", nil)
		if err != nil || plan.Codec != exportcore.CodecH264 || plan.Wanted != "" || len(plan.Notes) != 0 {
			t.Errorf("%+v %v", plan, err)
		}
	})

	t.Run("h264 named is h264 and runs no detection of another codec", func(t *testing.T) {
		r, caps := noEncoderRunner(t, noHW...)

		plan, err := PlanCodec(ctx, r, caps, ffmpeg.DetectOptions{}, exportcore.CodecH264, presetOf(t, "tiktok"))
		if err != nil || plan.Codec != exportcore.CodecH264 || plan.Wanted != "" {
			t.Errorf("%+v %v", plan, err)
		}
	})

	t.Run("a preset that takes only h264 never asks for another codec", func(t *testing.T) {
		r, caps := noEncoderRunner(t, noHW...)

		plan, err := PlanCodec(ctx, r, caps, ffmpeg.DetectOptions{}, "", presetOf(t, "youtube-shorts"))
		if err != nil || plan.Codec != exportcore.CodecH264 || plan.Wanted != "" {
			t.Errorf("%+v %v", plan, err)
		}
	})

	t.Run("a cancelled context is returned", func(t *testing.T) {
		r, caps := noEncoderRunner(t, noHW...)
		cctx, cancel := context.WithCancel(ctx)
		cancel()

		if _, err := PlanCodec(
			cctx,
			r,
			caps,
			ffmpeg.DetectOptions{},
			"",
			presetOf(t, "tiktok"),
		); !errors.Is(
			err,
			context.Canceled,
		) {
			t.Errorf("error = %v", err)
		}
	})
}

// TestHEVCPresetLimitsAreVerifiedOnTheFile: the limits of a preset are checked on the file that was written. A
// preset whose size limit is smaller than any file ends E_VERIFY_FAILED with the check preset_size_too_large and
// leaves nothing behind.
func TestHEVCPresetLimitsAreVerifiedOnTheFile(t *testing.T) {
	setup(t)

	tiny := exportcore.Preset{
		ID:        "tiny",
		Label:     "Tiny",
		Container: "mp4",
		Codecs:    []exportcore.Codec{exportcore.CodecH264},
		Limits:    exportcore.Limits{MaxBytes: 100},
		Targets: exportcore.Targets{
			Width:     1080,
			Height:    1920,
			VideoKbps: 2000,
			AudioKbps: 128,
			SampleHz:  48000,
			Channels:  2,
		},
	}

	doc := loadScene(t, "contain-text")
	job := newJob(t, doc, "preset-size", filepath.Join("preset-size", "out.mp4"))
	job.Preset = &tiny

	_, err := Run(context.Background(), job)

	e, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok || e.Code != CodeVerifyFailed {
		t.Fatalf("error = %v", err)
	}

	checks, _ := e.Details["checks"].([]Check)
	if len(checks) != 1 || checks[0].Name != "preset_size_too_large" || checks[0].Expected != "0.0 MB" {
		t.Errorf("checks %+v", checks)
	}

	if _, serr := os.Stat(job.OutPath); serr == nil {
		t.Error("a file that failed its preset was published")
	}
}

// TestHEVCPresetWithAFormatThatHasNoCodecIsRefused: a codec or a preset is for mp4.
func TestHEVCPresetWithAFormatThatHasNoCodecIsRefused(t *testing.T) {
	setup(t)

	doc := loadScene(t, "contain-text")

	for _, f := range []Format{FormatGIF, FormatWAV, FormatAAC} {
		job := newJob(t, doc, "fmt-"+string(f), filepath.Join("fmt", string(f)))
		job.Format, job.Codec = f, exportcore.CodecHEVC

		_, err := Run(context.Background(), job)
		if e, ok := errors.AsType[*ffmpeg.Error](
			err,
		); !ok || e.Code != CodeExportFailed ||
			e.Reason != ReasonBadFormat {
			t.Errorf("%s: %v", f, err)
		}
	}

	job := newJob(t, doc, "bad-codec", "bad-codec.mp4")
	job.Codec = "libx265"

	if _, err := Run(context.Background(), job); err == nil || !strings.Contains(err.Error(), "codec") {
		t.Errorf("an unknown codec: %v", err)
	}
}
