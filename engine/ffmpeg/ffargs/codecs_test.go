package ffargs

import (
	"slices"
	"testing"
)

var (
	hevcEncoders = []string{
		"hevc_videotoolbox", "hevc_nvenc", "hevc_qsv", "hevc_amf", "hevc_mf", "hevc_vaapi",
	}
	av1Encoders = []string{"av1_nvenc", "av1_qsv", "av1_amf", "av1_vaapi", "libsvtav1", "libaom-av1"}
)

// TestEncoderAllowListGrammar: the grammar takes the HEVC and AV1 encoders of RF-089 as -c:v and no
// other: libx265 and every GPL or unlisted encoder is refused whatever the argument list says (FF-15).
func TestEncoderAllowListGrammar(t *testing.T) {
	t.Parallel()

	pol := testPolicy()

	for _, enc := range slices.Concat(hevcEncoders, av1Encoders) {
		di := DetectInput{Filter: "format=yuv420p"}
		if enc == "hevc_vaapi" || enc == "av1_vaapi" {
			di = DetectInput{VaapiDevice: "/dev/dri/renderD128", Filter: "format=nv12,hwupload"}
		}

		argv, err := EncoderTest(pol, enc, di, run("probe-"+enc+".mp4"))
		if err != nil {
			t.Errorf("%s: %v", enc, err)

			continue
		}

		if err := Check(FFmpeg, argv, pol); err != nil {
			t.Errorf("%s: %v", enc, err)
		}
	}

	for _, enc := range []string{
		"libx265", "libx264rgb", "libxavs2", "libxvid", "libkvazaar", "librav1e", "libvvenc",
		"hevc_d3d12va", "hevc_vulkan", "av1_vulkan", "av1_d3d12va", "av1_mf", "libx265 ", "HEVC_NVENC",
	} {
		if _, err := EncoderTest(pol, enc, DetectInput{Filter: "format=yuv420p"}, run("probe.mp4")); err == nil {
			t.Errorf("%s was accepted as an encoder", enc)
		}
	}
}

// TestEncoderOptionsOfTheNewCodecs: the test encode of each new encoder carries only the documented options,
// and the options a vendor or a codec could ask for stay refused until a capability test proves one is needed.
func TestEncoderOptionsOfTheNewCodecs(t *testing.T) {
	t.Parallel()

	pol := testPolicy()

	argv, err := EncoderTest(pol, "libsvtav1", DetectInput{Filter: "format=yuv420p"}, run("a.mp4"))
	if err != nil || !slices.Contains(argv, "8") {
		t.Fatalf("libsvtav1: %v %v", err, argv)
	}

	argv, err = EncoderTest(pol, "libaom-av1", DetectInput{Filter: "format=yuv420p"}, run("b.mp4"))
	if err != nil || !slices.Contains(argv, "-cpu-used") || !slices.Contains(argv, "-row-mt") {
		t.Fatalf("libaom-av1: %v %v", err, argv)
	}

	for _, enc := range slices.Concat(hevcEncoders, av1Encoders) {
		argv, err := EncoderTest(pol, enc, DetectInput{Filter: "format=yuv420p"}, run("c.mp4"))
		if err != nil {
			continue // vaapi needs a device, checked above
		}

		for _, extra := range [][]string{
			{"-preset", "p7"},
			{"-preset", "veryslow"},
			{"-preset", "9"},
			{"-tag:v", "hev1"},
			{"-tag:v", "avc1"},
			{"-x265-params", "crf=20"},
			{"-svtav1-params", "tune=0"},
			{"-rc", "vbr"},
			{"-tier", "high"},
			{"-level", "5.1"},
			{"-profile:v", "main10"},
			{"-b:v", "8G"},
			{"-init_hw_device", "cuda=hw"},
		} {
			if err := Check(FFmpeg, insertBeforeLast(argv, extra[0], extra[1]), pol); err == nil {
				t.Errorf("%s: %v was accepted", enc, extra)
			}
		}
	}

	// The two values that are allowed, and only on the output.
	good := insertBeforeLast(mustTest(t, pol, "hevc_nvenc"), "-tag:v", "hvc1")
	if err := Check(FFmpeg, good, pol); err != nil {
		t.Errorf("-tag:v hvc1: %v", err)
	}

	good = insertBeforeLast(mustTest(t, pol, "hevc_nvenc"), "-preset", "8")
	if err := Check(FFmpeg, good, pol); err != nil {
		t.Errorf("-preset 8: %v", err)
	}
}

func mustTest(t *testing.T, pol Policy, enc string) []string {
	t.Helper()

	argv, err := EncoderTest(pol, enc, DetectInput{Filter: "format=yuv420p"}, run("d.mp4"))
	if err != nil {
		t.Fatalf("%s: %v", enc, err)
	}

	return argv
}
