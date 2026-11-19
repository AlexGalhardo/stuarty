package ffmpeg

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

const encodersListing = `Encoders:
 V..... = Video
 A..... = Audio
 ------
 V....D libopenh264          OpenH264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 V....D h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 A....D aac                  AAC (Advanced Audio Coding)
`

const filtersListing = `Filters:
  T.. = Timeline support
  .S. = Slice threading
  ..C = Command support
  A = Audio input/output
 ------
 ... aresample         A->A       Resample audio data.
 T.C volume            A->A       Change input volume.
 ... scale             V->V       Scale the input video size and/or convert the image format.
 ... split             V->N       Pass on the input to N video outputs.
`

const muxersListing = `File formats:
 D. = Demuxing supported
 .E = Muxing supported
 --
  E mp4             MP4 (MPEG-4 Part 14)
  E mov             QuickTime / MOV
 DE wav             WAV / WAVE (Waveform Audio)
`

func TestParseListing(t *testing.T) {
	enc := parseListing(encodersListing)
	if !enc["libopenh264"] || !enc["aac"] || !enc["h264_nvenc"] || enc["="] || len(enc) != 3 {
		t.Errorf("encoders = %v", enc)
	}
	f := parseListing(filtersListing)
	if !f["scale"] || !f["volume"] || !f["aresample"] || !f["split"] || len(f) != 4 {
		t.Errorf("filters = %v", f)
	}
	if m := parseListing(muxersListing); !m["mp4"] || !m["mov"] || !m["wav"] || len(m) != 3 {
		t.Errorf("muxers = %v", m)
	}
}

func TestMissingRequired(t *testing.T) {
	full := &Capabilities{Encoders: map[string]bool{}, Filters: map[string]bool{}, Muxers: map[string]bool{}}
	for _, n := range requiredEncoders {
		full.Encoders[n] = true
	}
	for _, n := range requiredFilters {
		full.Filters[n] = true
	}
	for _, n := range requiredMuxers {
		full.Muxers[n] = true
	}
	if m := missingRequired(full); len(m) != 0 {
		t.Fatalf("missing = %v", m)
	}
	delete(full.Encoders, "libvpx-vp9")
	delete(full.Filters, "ebur128")
	delete(full.Muxers, "image2")
	m := missingRequired(full)
	e := missingErr(m)
	if e.Code != CodeFFmpegMissing || len(e.Details["missing"].([]string)) != 3 ||
		!strings.Contains(e.Message, "encoder:libvpx-vp9") {
		t.Errorf("%+v", e)
	}
}

func TestReadCapabilitiesOnThePinnedBuild(t *testing.T) {
	r, _ := pinnedRunner(t)
	c, err := r.ReadCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"libopenh264", "aac", "mpeg4", "libvpx-vp9", "mjpeg", "png", "pcm_s16le"} {
		if !c.Encoders[n] {
			t.Errorf("encoder %s is missing", n)
		}
	}
	for _, n := range []string{"eq", "boxblur"} {
		if c.Filters[n] {
			t.Errorf("GPL-only filter %s is present in the pinned build", n)
		}
	}
	if !c.Zscale || !c.Tonemap || !c.Bwdif || !c.Yadif || !c.HDROK {
		t.Errorf("optional capabilities: %+v", c)
	}
	if c.License.Name != "lgpl" {
		t.Errorf("license = %+v", c.License)
	}
	if c.Encoders["libx264"] || c.Encoders["libx265"] {
		t.Error("the pinned build has a GPL encoder")
	}
}

func TestDetectEncoderOnThePinnedBuild(t *testing.T) {
	r, _ := pinnedRunner(t)
	ctx := context.Background()
	caps, err := r.ReadCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}

	det, err := r.DetectEncoder(ctx, caps, DetectOptions{})
	if err != nil {
		t.Fatalf("no encoder on this machine: %v", err)
	}
	if det.Chosen == "" || det.Forced {
		t.Fatalf("detection = %+v", det)
	}
	var seen []string
	for _, c := range det.Candidates {
		t.Logf("%s: %s %d ms %q", c.Name, c.Status, c.MS, c.StderrTail)
		seen = append(seen, c.Name)
		if c.Name == "libx264" && c.Status != EncoderNotListed {
			t.Errorf("libx264 status %s on a bundled-style build", c.Status)
		}
		if len(c.StderrTail) > 300 || strings.Contains(c.StderrTail, r.Workspace) ||
			strings.Contains(c.StderrTail, filepath.ToSlash(r.Workspace)) {
			t.Errorf("stderr tail of %s holds a path or is too long: %q", c.Name, c.StderrTail)
		}
	}
	if strings.Join(seen, ",") != strings.Join(EncoderOrder, ",") {
		t.Errorf("candidates in order %v, want %v", seen, EncoderOrder)
	}
	chosen, ok := det.ChosenCandidate()
	if !ok || chosen.Status != EncoderOK || chosen.Input.Filter == "" {
		t.Errorf("chosen = %+v", chosen)
	}
	// The first working candidate in ADR-0011 order is the chosen one.
	for _, c := range det.Candidates {
		if c.Status == EncoderOK {
			if c.Name != det.Chosen {
				t.Errorf("chosen %s but %s works earlier in the order", det.Chosen, c.Name)
			}
			break
		}
	}
	for _, c := range det.Candidates {
		if c.Name == "libopenh264" && c.Status != EncoderOK {
			t.Errorf("libopenh264 of the pinned build: %+v", c)
		}
	}

	// Cached for the life of the runner: a second call starts nothing.
	n := r.started.Load()
	again, err := r.DetectEncoder(ctx, caps, DetectOptions{})
	if err != nil || again != det || r.started.Load() != n {
		t.Errorf("not cached: %v, started %d -> %d", err, n, r.started.Load())
	}
	fresh, err := r.DetectEncoder(ctx, caps, DetectOptions{Refresh: true})
	if err != nil || fresh == det || r.started.Load() == n {
		t.Errorf("refresh did not re-run: %v", err)
	}

	forced, err := r.DetectEncoder(ctx, caps, DetectOptions{Forced: "libopenh264", Refresh: true})
	if err != nil || forced.Chosen != "libopenh264" || !forced.Forced || len(forced.Candidates) != 1 {
		t.Errorf("forced libopenh264: %v %+v", err, forced)
	}
}

func TestNoEncoderNamesTheFix(t *testing.T) {
	r, _ := pinnedRunner(t)
	ctx := context.Background()
	caps, err := r.ReadCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}

	check := func(name string, err error, wantTried string) {
		t.Helper()
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeNoEncoder {
			t.Fatalf("%s: error = %v, want E_NO_ENCODER", name, err)
		}
		if !strings.Contains(e.Hint, "Use the FFmpeg that ships with Stuarty") ||
			!strings.Contains(e.Hint, "STUARTY_ALLOW_USER_X264=true") ||
			!strings.Contains(e.Hint, "STUARTY_FFMPEG") {
			t.Errorf("%s: the hint does not name the fix: %q", name, e.Hint)
		}
		if !strings.HasPrefix(e.Message, "No H.264 encoder works on this machine. Tried: ") ||
			!strings.Contains(e.Message, wantTried) {
			t.Errorf("%s: message = %q", name, e.Message)
		}
		if strings.Contains(e.Message, "tmp") {
			t.Errorf("%s: the message names a path: %q", name, e.Message)
		}
	}

	// An empty allowlist: no candidate at all.
	_, err = r.DetectEncoder(ctx, caps, DetectOptions{Candidates: []string{}, Refresh: true})
	check("empty allowlist", err, "nothing")

	// libx264 is not offered for a build that is not the user's own with the opt-in.
	_, err = r.DetectEncoder(ctx, caps, DetectOptions{Forced: "libx264", Refresh: true})
	check("forced libx264", err, "libx264 (")

	// A forced encoder is never silently replaced, even when others work.
	_, err = r.DetectEncoder(ctx, caps, DetectOptions{Forced: "x264-or-else", Refresh: true})
	check("forced unknown", err, "x264-or-else (failed")

	// A listed-but-failing encoder reports its failure and the others are not tried.
	_, err = r.DetectEncoder(ctx, caps, DetectOptions{Candidates: []string{"h264_qsv", "h264_amf"}, Refresh: true})
	if _, ok := errors.AsType[*Error](err); ok {
		check("no hardware", err, "h264_qsv (")
	} else if err != nil {
		t.Fatal(err)
	}
}
