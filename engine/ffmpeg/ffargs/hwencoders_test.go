package ffargs

import "testing"

// TestHardwareEncodersUseOnlyTheDocumentedOptions is the grammar side of D-034: the test encode of NVENC,
// Quick Sync and AMF passes the checker with `-b:v 8M -profile:v high` (SDD-011 8.4) and nothing else,
// and the options a vendor could ask for (a device, a hardware decoder, a preset, a rate control mode)
// stay refused until a capability test proves one is needed.
func TestHardwareEncodersUseOnlyTheDocumentedOptions(t *testing.T) {
	pol := testPolicy()
	for _, enc := range []string{"h264_nvenc", "h264_qsv", "h264_amf"} {
		argv, err := EncoderTest(pol, enc, DetectInput{Filter: "format=yuv420p"}, run("probe-"+enc+".mp4"))
		if err != nil {
			t.Fatalf("%s: %v", enc, err)
		}
		if err := Check(FFmpeg, argv, pol); err != nil {
			t.Fatalf("%s: %v", enc, err)
		}

		for _, extra := range [][]string{
			{"-init_hw_device", "qsv=hw"},
			{"-filter_hw_device", "hw"},
			{"-hwaccel", "qsv"},
			{"-preset", "p5"},
			{"-rc", "vbr"},
			{"-gpu", "1"},
			{"-profile:v", "high444"},
			{"-b:v", "8G"},
			{"-extra_hw_frames", "8"},
		} {
			bad := insertBeforeLast(argv, extra[0], extra[1])
			if err := Check(FFmpeg, bad, pol); err == nil {
				t.Errorf("%s: %v was accepted", enc, extra)
			}
		}
	}
}
