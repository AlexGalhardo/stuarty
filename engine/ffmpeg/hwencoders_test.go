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

// hwFixture is the own build (its fake version line and its -encoders listing) with a copy of the test
// binary as ffmpeg and ffprobe. works names the encoders whose test encode succeeds; every other one fails
// to open, as a driver that is not there does (D-034).
func hwFixture(t *testing.T, works ...string) (*Runner, *Capabilities) {
	t.Helper()
	r, caps, _ := hwFixtureDir(t, works...)
	return r, caps
}

// hwFixtureDir is hwFixture that also returns the folder of the fake tools, where a test can add small.txt.
func hwFixtureDir(t *testing.T, works ...string) (*Runner, *Capabilities, string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "builds", "own", "version.txt"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := install(t, dir, string(src))
	if err := os.WriteFile(filepath.Join(dir, "works.txt"), []byte(strings.Join(works, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _, _ := fakeLibrary(t)
	in, err := Discover(context.Background(), Config{
		FFmpegPath: bin, CPUCount: 1, ToolsDir: t.TempDir(), OpenH264Manifest: m,
	})
	if err != nil {
		t.Fatal(err)
	}
	in.OpenH264 = OpenH264State{Installed: true}
	r, err := NewRunner(in, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	listing, err := os.ReadFile(filepath.Join("testdata", "builds", "own", "encoders.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return r, &Capabilities{Encoders: parseListing(string(listing))}, dir
}

func statusOf(det *EncoderDetection, name string) (Candidate, bool) {
	for _, c := range det.Candidates {
		if c.Name == name {
			return c, true
		}
	}
	return Candidate{}, false
}

// TestOwnBuildFakeHasTheHardwareEncoders keeps the fake of the own build in line with D-034: the three
// flags on the configure line (and still no GPL one) and the encoders they add in the listing.
func TestOwnBuildFakeHasTheHardwareEncoders(t *testing.T) {
	v := fakeBuild(t, "own")
	for _, flag := range []string{"--enable-ffnvcodec", "--enable-libvpl", "--enable-amf"} {
		if !slices.Contains(v.Flags, flag) {
			t.Errorf("the fake of the own build lacks %s", flag)
		}
	}
	if f := v.Flavour(); f.Name != FlavourOwn || !f.LGPLOnly() || len(f.Problems()) != 0 {
		t.Errorf("the own build with hardware flags: %+v", f)
	}
	_, caps := hwFixture(t)
	for _, enc := range []string{"h264_nvenc", "h264_qsv", "h264_amf", "libopenh264"} {
		if !caps.Encoders[enc] {
			t.Errorf("%s is not in the listing of the own build", enc)
		}
	}
}

// TestHardwareEncoderRanking is the ranking of SDD-004 section 8 and SDD-011 8.4: NVENC, then Quick Sync,
// then AMF, then the software encoder; a vendor whose runtime is missing fails to open and the next one
// is chosen, down to libopenh264, and the default order is the one the product ships.
func TestHardwareEncoderRanking(t *testing.T) {
	cases := []struct {
		name   string
		works  []string
		chosen string
	}{
		{"all three vendors", []string{"h264_nvenc", "h264_qsv", "h264_amf", "libopenh264"}, "h264_nvenc"},
		{"no NVIDIA runtime", []string{"h264_qsv", "h264_amf", "libopenh264"}, "h264_qsv"},
		{"AMD only", []string{"h264_amf", "libopenh264"}, "h264_amf"},
		{"Intel only", []string{"h264_qsv", "libopenh264"}, "h264_qsv"},
		{"no GPU", []string{"libopenh264"}, "libopenh264"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, caps := hwFixture(t, c.works...)
			det, err := r.DetectEncoder(context.Background(), caps, DetectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if det.Chosen != c.chosen {
				t.Fatalf("chosen %s, want %s: %+v", det.Chosen, c.chosen, det.Candidates)
			}
			for _, name := range []string{"h264_nvenc", "h264_qsv", "h264_amf"} {
				cand, ok := statusOf(det, name)
				if !ok {
					t.Fatalf("%s was not a candidate", name)
				}
				want := EncoderFailed
				if slices.Contains(c.works, name) {
					want = EncoderOK
				}
				if cand.Status != want {
					t.Errorf("%s: %s, want %s", name, cand.Status, want)
				}
				if want == EncoderFailed && (!strings.Contains(cand.StderrTail, "Error while opening encoder") ||
					strings.Contains(cand.StderrTail, "second line")) {
					t.Errorf("%s: the failure line is %q", name, cand.StderrTail)
				}
			}
			chosen, _ := det.ChosenCandidate()
			if chosen.Input.Filter != "format=yuv420p" || chosen.Input.VaapiDevice != "" {
				t.Errorf("input of %s: %+v", chosen.Name, chosen.Input)
			}
		})
	}
}

// TestHardwareEncoderMissingFromTheBuild: a build without a vendor's encoder lists it as not_listed and
// never starts a test encode for it, even when the machine could run it.
func TestHardwareEncoderMissingFromTheBuild(t *testing.T) {
	r, caps := hwFixture(t, "h264_nvenc", "h264_qsv", "h264_amf", "libopenh264")
	delete(caps.Encoders, "h264_nvenc")
	delete(caps.Encoders, "h264_qsv")
	det, err := r.DetectEncoder(context.Background(), caps, DetectOptions{})
	if err != nil || det.Chosen != "h264_amf" {
		t.Fatalf("%v: %+v", err, det)
	}
	for _, name := range []string{"h264_nvenc", "h264_qsv"} {
		if c, _ := statusOf(det, name); c.Status != EncoderNotListed || c.StderrTail != "" {
			t.Errorf("%s: %+v", name, c)
		}
	}
}

// TestNoHardwareAndNoSoftware: when every hardware encoder fails and libopenh264 is not installed, the
// error names the three vendors and the install command.
func TestNoHardwareAndNoSoftware(t *testing.T) {
	r, caps := hwFixture(t)
	r.In.OpenH264 = OpenH264State{Reason: ReasonOpenH264NotInstalled}
	_, err := r.DetectEncoder(context.Background(), caps, DetectOptions{})
	e, ok := errors.AsType[*Error](err)
	if !ok || e.Code != CodeNoEncoder {
		t.Fatalf("error = %v", err)
	}
	for _, want := range []string{"h264_nvenc (failed", "h264_qsv (failed", "h264_amf (failed", "libopenh264 (failed"} {
		if !strings.Contains(e.Message, want) {
			t.Errorf("message lacks %q: %s", want, e.Message)
		}
	}
	if e.Details["install_command"] != OpenH264InstallCommand {
		t.Errorf("details: %v", e.Details)
	}
}

// TestForcedHardwareEncoderIsNeverReplaced: VUKEM_VIDEO_ENCODER=h264_amf on a machine where only NVENC
// works is E_NO_ENCODER with the AMF failure, not a silent switch to NVENC.
func TestForcedHardwareEncoderIsNeverReplaced(t *testing.T) {
	r, caps := hwFixture(t, "h264_nvenc", "libopenh264")
	det, err := r.DetectEncoder(context.Background(), caps, DetectOptions{Forced: "h264_amf"})
	e, ok := errors.AsType[*Error](err)
	if !ok || e.Code != CodeNoEncoder || det.Chosen != "" || len(det.Candidates) != 1 ||
		!strings.Contains(e.Message, "h264_amf (failed") {
		t.Fatalf("%v %+v", err, det)
	}

	det, err = r.DetectEncoder(context.Background(), caps, DetectOptions{Forced: "h264_nvenc", Refresh: true})
	if err != nil || det.Chosen != "h264_nvenc" || !det.Forced {
		t.Fatalf("forced NVENC: %v %+v", err, det)
	}
}
