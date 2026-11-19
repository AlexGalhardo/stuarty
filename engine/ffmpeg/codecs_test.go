package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestEncoderAllowList is the allow list of RF-089: the HEVC and AV1 encoders are the vendor encoders and the
// permissively licensed software ones, no GPL encoder is on any list, and every name maps to its codec.
func TestEncoderAllowList(t *testing.T) {
	t.Parallel()

	wantHEVC := []string{"hevc_videotoolbox", "hevc_nvenc", "hevc_qsv", "hevc_amf", "hevc_mf", "hevc_vaapi"}
	wantAV1 := []string{"av1_nvenc", "av1_qsv", "av1_amf", "av1_vaapi", "libsvtav1", "libaom-av1"}

	if !slices.Equal(EncodersFor(CodecHEVC), wantHEVC) {
		t.Errorf("HEVC encoders %v", EncodersFor(CodecHEVC))
	}

	if !slices.Equal(EncodersFor(CodecAV1), wantAV1) {
		t.Errorf("AV1 encoders %v", EncodersFor(CodecAV1))
	}

	if !slices.Equal(EncodersFor(CodecH264), EncoderOrder) || EncodersFor("vp9") != nil {
		t.Error("the H.264 list is EncoderOrder, and a codec with no list has none")
	}

	for _, gpl := range []string{"libx265", "libx264rgb", "libxavs2", "libxvid", "libfdk_aac"} {
		if !IsRefusedEncoder(gpl) {
			t.Errorf("%s is not refused", gpl)
		}

		if CodecOf(gpl) != "" {
			t.Errorf("%s is on an allow list", gpl)
		}
	}

	for _, codec := range []string{CodecH264, CodecHEVC, CodecAV1} {
		for _, name := range EncodersFor(codec) {
			if IsRefusedEncoder(name) {
				t.Errorf("%s is allowed and refused", name)
			}

			if CodecOf(name) != codec {
				t.Errorf("%s writes %q, want %s", name, CodecOf(name), codec)
			}
		}
	}

	for _, soft := range []string{"libopenh264", "libx264", "libsvtav1", "libaom-av1"} {
		if IsHardwareEncoder(soft) {
			t.Errorf("%s is a software encoder", soft)
		}
	}

	for _, hard := range []string{"h264_nvenc", "hevc_qsv", "av1_amf", "hevc_mf", "h264_videotoolbox"} {
		if !IsHardwareEncoder(hard) {
			t.Errorf("%s is a hardware encoder", hard)
		}
	}

	if IsHardwareEncoder("libx265") || IsHardwareEncoder("nonsense") {
		t.Error("a name outside the lists is no hardware encoder")
	}
}

func codecFixture(t *testing.T, listed []string, works ...string) (*Runner, *Capabilities, string) {
	t.Helper()

	r, caps, dir := hwFixtureDir(t, works...)

	// The listing is the one the caller wants, whatever the pinned fake offers.
	caps.Encoders = map[string]bool{"libopenh264": true}
	for _, n := range listed {
		caps.Encoders[n] = true
	}

	return r, caps, dir
}

// TestCapabilityHEVCAndAV1Cases: the four cases of the epic: hardware only, permissive software, a GPL
// encoder only, and nothing.
func TestCapabilityHEVCAndAV1Cases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("hardware only", func(t *testing.T) {
		t.Parallel()

		r, caps, _ := codecFixture(
			t,
			[]string{"hevc_nvenc", "hevc_amf", "av1_nvenc"},
			"hevc_amf",
			"av1_nvenc",
			"libopenh264",
		)

		det, err := r.DetectCodec(ctx, caps, CodecHEVC, DetectOptions{})
		if err != nil || det.Chosen != "hevc_amf" {
			t.Fatalf("hevc: %v %+v", err, det)
		}

		// hevc_nvenc is listed and its driver does not open: failed, then the next one.
		if c, _ := statusOf(det, "hevc_nvenc"); c.Status != EncoderFailed ||
			!strings.Contains(c.StderrTail, "Error while opening encoder") {
			t.Errorf("hevc_nvenc: %+v", c)
		}

		if c, _ := statusOf(det, "hevc_qsv"); c.Status != EncoderNotListed {
			t.Errorf("hevc_qsv is not in the build: %+v", c)
		}

		av1, err := r.DetectCodec(ctx, caps, CodecAV1, DetectOptions{})
		if err != nil || av1.Chosen != "av1_nvenc" {
			t.Fatalf("av1: %v %+v", err, av1)
		}
	})

	t.Run("permissive software only", func(t *testing.T) {
		t.Parallel()

		r, caps, _ := codecFixture(t, []string{"libsvtav1", "libaom-av1"}, "libsvtav1", "libaom-av1")

		av1, err := r.DetectCodec(ctx, caps, CodecAV1, DetectOptions{})
		if err != nil || av1.Chosen != "libsvtav1" {
			t.Fatalf("av1: %v %+v", err, av1)
		}

		// There is no software HEVC encoder on the list: HEVC is unavailable.
		_, err = r.DetectCodec(ctx, caps, CodecHEVC, DetectOptions{})
		assertCodecUnavailable(t, err, CodecHEVC)
	})

	t.Run("a GPL encoder only is refused", func(t *testing.T) {
		t.Parallel()

		// The user's FFmpeg has libx265 and it works; the product never tries it.
		r, caps, _ := codecFixture(t, []string{"libx265"}, "libx265", "libopenh264")

		det, err := r.DetectCodec(ctx, caps, CodecHEVC, DetectOptions{})
		assertCodecUnavailable(t, err, CodecHEVC)

		for _, c := range det.Candidates {
			if c.Name == "libx265" {
				t.Errorf("libx265 was a candidate: %+v", c)
			}
		}

		// A forced libx265 is refused with its own reason, even for the H.264 detection.
		forced, ferr := r.DetectEncoder(ctx, caps, DetectOptions{Forced: "libx265", Refresh: true})
		if forced.Chosen != "" || ferr == nil {
			t.Fatalf("libx265 was chosen: %v %+v", ferr, forced)
		}

		if c := forced.Candidates[0]; c.Status != EncoderFailed || c.Reason != ReasonRefusedEncoder ||
			!strings.Contains(c.StderrTail, "GPL") {
			t.Errorf("forced libx265: %+v", c)
		}
	})

	t.Run("nothing", func(t *testing.T) {
		t.Parallel()

		r, caps, _ := codecFixture(t, nil, "libopenh264")

		for _, codec := range []string{CodecHEVC, CodecAV1} {
			_, err := r.DetectCodec(ctx, caps, codec, DetectOptions{})
			assertCodecUnavailable(t, err, codec)
		}
	})
}

func assertCodecUnavailable(t *testing.T, err error, codec string) {
	t.Helper()

	e, ok := errors.AsType[*Error](err)
	if !ok || e.Code != CodeNoEncoder || e.Reason != ReasonCodecUnavailable {
		t.Fatalf("%s: error = %v", codec, err)
	}

	if e.Details["codec"] != codec || e.Details["reason"] != ReasonCodecUnavailable || e.Hint == "" {
		t.Errorf("%s: details %v, hint %q", codec, e.Details, e.Hint)
	}

	if _, ok := e.Details["candidates"].([]map[string]any); !ok {
		t.Errorf("%s: no candidates in %v", codec, e.Details)
	}

	if strings.Contains(e.Message, "libx265") {
		t.Errorf("%s: the message names a refused encoder: %s", codec, e.Message)
	}
}

// TestCapabilityTestEncodeChecksTheCodec: an encoder that opens and writes the wrong codec is not offered, and
// the answer says what the file held.
func TestCapabilityTestEncodeChecksTheCodec(t *testing.T) {
	t.Parallel()

	r, caps, dir := codecFixture(t, []string{"hevc_nvenc", "hevc_qsv"}, "hevc_nvenc", "hevc_qsv")
	if err := os.WriteFile(filepath.Join(dir, "wrongcodec.txt"), []byte("hevc_nvenc"), 0o644); err != nil {
		t.Fatal(err)
	}

	det, err := r.DetectCodec(context.Background(), caps, CodecHEVC, DetectOptions{})
	if err != nil || det.Chosen != "hevc_qsv" {
		t.Fatalf("%v %+v", err, det)
	}

	c, _ := statusOf(det, "hevc_nvenc")
	if c.Status != EncoderFailed || !strings.Contains(c.StderrTail, "instead of 15 hevc packets") {
		t.Errorf("hevc_nvenc: %+v", c)
	}
}

// TestCapabilityIsCachedPerCodec: the answer of a codec lives for the life of the runner, a refresh runs it
// again, and H.264 and HEVC do not share an answer.
func TestCapabilityIsCachedPerCodec(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r, caps, dir := codecFixture(t, []string{"hevc_nvenc"}, "hevc_nvenc", "libopenh264")

	first, err := r.DetectCodec(ctx, caps, CodecHEVC, DetectOptions{})
	if err != nil || first.Chosen != "hevc_nvenc" {
		t.Fatalf("%v %+v", err, first)
	}

	// The driver goes away: the cached answer stays until a refresh.
	if err := os.WriteFile(filepath.Join(dir, "works.txt"), []byte("libopenh264"), 0o644); err != nil {
		t.Fatal(err)
	}

	again, err := r.DetectCodec(ctx, caps, CodecHEVC, DetectOptions{})
	if err != nil || again != first {
		t.Fatalf("not cached: %v %+v", err, again)
	}

	_, err = r.DetectCodec(ctx, caps, CodecHEVC, DetectOptions{Refresh: true})
	assertCodecUnavailable(t, err, CodecHEVC)

	h264, err := r.DetectCodec(ctx, caps, CodecH264, DetectOptions{})
	if err != nil || h264.Chosen != "libopenh264" {
		t.Fatalf("h264: %v %+v", err, h264)
	}
}

// TestCapabilityUnknownCodec: a codec the product does not write is the same stable error, not a crash.
func TestCapabilityUnknownCodec(t *testing.T) {
	t.Parallel()

	r, caps, _ := codecFixture(t, nil, "libopenh264")

	_, err := r.DetectCodec(context.Background(), caps, "vp9", DetectOptions{})

	e, ok := errors.AsType[*Error](err)
	if !ok || e.Code != CodeNoEncoder || e.Reason != ReasonCodecUnavailable || e.Details["codec"] != "vp9" ||
		e.Hint == "" {
		t.Fatalf("error = %v", err)
	}
}

// TestCapabilityEncoderForSizeWorksForHEVC: the size test of D-034 follows the encoder that detection chose
// for HEVC, with the HEVC packet count, and falls to the next HEVC candidate.
func TestCapabilityEncoderForSizeWorksForHEVC(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r, caps, dir := codecFixture(t, []string{"hevc_nvenc", "hevc_amf"}, "hevc_nvenc", "hevc_amf")

	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hevc_nvenc"), 0o644); err != nil {
		t.Fatal(err)
	}

	det, err := r.DetectCodec(ctx, caps, CodecHEVC, DetectOptions{})
	if err != nil || det.Chosen != "hevc_nvenc" {
		t.Fatalf("%v %+v", err, det)
	}

	got, err := r.EncoderForSize(ctx, det, SizeProbe{Width: 3840, Height: 2160, FPSNum: 30, FPSDen: 1})
	if err != nil || got.Name != "hevc_amf" {
		t.Fatalf("4K: %v %+v", err, got)
	}
}
