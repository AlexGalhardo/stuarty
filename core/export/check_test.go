package export

import (
	"slices"
	"strings"
	"testing"
)

func codes(fs []Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Code
	}

	return out
}

func in1080(durMS int64) Input {
	return Input{Width: 1080, Height: 1920, FPSNum: 30, FPSDen: 1, DurationMS: durMS}
}

func preset(t *testing.T, id string) Preset {
	t.Helper()

	p, ok := Lookup(id)
	if !ok {
		t.Fatalf("no preset %s", id)
	}

	return p
}

// TestExportCheckDuration: every boundary of the duration of every preset that states one.
func TestExportCheckDuration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id   string
		ms   int64
		want []string
	}{
		{"youtube-shorts", 180_000, nil},
		{"youtube-shorts", 180_001, []string{CodeDurationTooLong}},
		{"youtube-shorts", 1, nil},
		{"instagram-reels", 2_999, []string{CodeDurationTooShort}},
		{"instagram-reels", 3_000, nil},
		{"instagram-reels", 900_000, nil},
		{"instagram-reels", 900_001, []string{CodeDurationTooLong}},
		{"facebook-reels", 2_999, []string{CodeDurationTooShort}},
		{"facebook-reels", 3_000, nil},
		{"facebook-reels", 90_000, nil},
		{"facebook-reels", 90_001, []string{CodeDurationTooLong}},
		{"tiktok", 600_000, nil},
		{"tiktok", 600_001, []string{CodeDurationTooLong}},
		{"x-video", 499, []string{CodeDurationTooShort}},
		{"x-video", 500, nil},
		{"x-video", 1_200_000, nil},
		{"x-video", 1_200_001, []string{CodeDurationTooLong}},
	}

	for _, c := range cases {
		got := codes(preset(t, c.id).Check(in1080(c.ms)))
		// The size of a long 1080p estimate may also trip a size limit; this test is about the duration.
		got = slices.DeleteFunc(got, func(s string) bool { return s == CodeSizeTooLarge })
		if !slices.Equal(got, c.want) {
			t.Errorf("%s at %d ms: %v, want %v", c.id, c.ms, got, c.want)
		}
	}
}

// TestExportCheckTooLongNamesTheExactLimit is acceptance criterion 2: a timeline that is too long is
// reported with the exact limit, from the numbers alone (nothing is encoded).
func TestExportCheckTooLongNamesTheExactLimit(t *testing.T) {
	t.Parallel()

	fs := preset(t, "facebook-reels").Check(in1080(95_500))
	if len(fs) != 1 {
		t.Fatalf("findings %+v", fs)
	}

	f := fs[0]
	if f.Code != CodeDurationTooLong || f.Severity != SeverityError || f.Limit != "90 s" || f.Got != "95.5 s" {
		t.Errorf("finding %+v", f)
	}

	if !strings.Contains(f.Message, "95.5 s") || !strings.Contains(f.Message, "at most 90 s") ||
		!strings.Contains(f.Message, "Facebook Reels") {
		t.Errorf("message %q", f.Message)
	}

	if f.CheckName() != "preset_duration_too_long" {
		t.Errorf("check name %q", f.CheckName())
	}
}

// TestExportCheckFPS: the boundaries of the frame rate, including the NTSC rates.
func TestExportCheckFPS(t *testing.T) {
	t.Parallel()

	ig := preset(t, "instagram-reels")
	fb := preset(t, "facebook-reels")
	x := preset(t, "x-video")

	cases := []struct {
		name string
		p    Preset
		n, d int
		want []string
	}{
		{"ig 23.976", ig, 24000, 1001, nil},
		{"ig 23", ig, 23, 1, nil},
		{"ig 22", ig, 22, 1, []string{CodeFPSTooLow}},
		{"ig 60", ig, 60, 1, nil},
		{"ig 60.06", ig, 60000, 999, []string{CodeFPSTooHigh}},
		{"ig 61", ig, 61, 1, []string{CodeFPSTooHigh}},
		{"fb 23.976 is under 24", fb, 24000, 1001, []string{CodeFPSTooLow}},
		{"fb 24", fb, 24, 1, nil},
		{"x 60", x, 60, 1, nil},
		{"x 120", x, 120, 1, []string{CodeFPSTooHigh}},
		{"x 5 has no minimum", x, 5, 1, nil},
	}

	for _, c := range cases {
		in := Input{Width: 1080, Height: 1920, FPSNum: c.n, FPSDen: c.d, DurationMS: 10_000}
		if got := codes(c.p.Check(in)); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// TestExportCheckResolution: the minimum and the maximum of each preset that states one.
func TestExportCheckResolution(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id   string
		w, h int
		want []string
	}{
		{"facebook-reels", 540, 960, nil},
		{"facebook-reels", 539, 960, []string{CodeResolutionTooSmall}},
		{"facebook-reels", 540, 959, []string{CodeResolutionTooSmall}},
		{"tiktok", 360, 360, nil},
		{"tiktok", 359, 640, []string{CodeResolutionTooSmall}},
		{"tiktok", 4096, 2160, nil},
		{"tiktok", 4097, 2160, []string{CodeResolutionTooLarge}},
		{"tiktok", 2160, 4097, []string{CodeResolutionTooLarge}},
		{"instagram-reels", 1920, 1080, nil},
		{"instagram-reels", 1921, 1080, []string{CodeResolutionTooLarge}},
		{"instagram-reels", 4000, 8000, []string{CodeResolutionTooLarge}},
		{"youtube-shorts", 1080, 1920, nil},
		{"x-video", 4096, 2160, nil}, // the page contradicts itself: not enforced
	}

	for _, c := range cases {
		in := Input{Width: c.w, Height: c.h, FPSNum: 30, FPSDen: 1, DurationMS: 10_000}
		got := slices.DeleteFunc(codes(preset(t, c.id).Check(in)), func(s string) bool { return s == CodeSizeTooLarge })

		if !slices.Equal(got, c.want) {
			t.Errorf("%s at %dx%d: %v, want %v", c.id, c.w, c.h, got, c.want)
		}
	}
}

// TestExportCheckAspectRatio: each range, its tolerance and its two ends.
func TestExportCheckAspectRatio(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id   string
		w, h int
		bad  bool
	}{
		{"youtube-shorts", 1080, 1920, false},
		{"youtube-shorts", 1080, 1080, false}, // square
		{"youtube-shorts", 1081, 1080, true},
		{"youtube-shorts", 1920, 1080, true},
		{"facebook-reels", 1080, 1920, false},
		{"facebook-reels", 1088, 1920, false}, // inside 1%
		{"facebook-reels", 1100, 1920, true},
		{"facebook-reels", 1080, 1080, true},
		{"x-video", 1000, 3000, false}, // 1:3
		{"x-video", 1000, 3001, true},
		{"x-video", 3000, 1000, false}, // 3:1
		{"x-video", 3001, 1000, true},
		{"instagram-reels", 1920, 192, false}, // 10:1
		{"instagram-reels", 1921, 192, true},
		{"instagram-reels", 192, 19200, false}, // 0.01:1
		{"instagram-reels", 190, 19200, true},
		{"tiktok", 400, 3000, false}, // no aspect limit
	}

	for _, c := range cases {
		in := Input{Width: c.w, Height: c.h, FPSNum: 30, FPSDen: 1, DurationMS: 10_000}
		got := slices.Contains(codes(preset(t, c.id).Check(in)), CodeAspectRatio)

		if got != c.bad {
			t.Errorf("%s at %dx%d: aspect finding %v, want %v", c.id, c.w, c.h, got, c.bad)
		}
	}
}

// TestExportCheckSize: the estimate before the encode, and the real size after it, at the boundary.
func TestExportCheckSize(t *testing.T) {
	t.Parallel()

	ig := preset(t, "instagram-reels")

	// 8000 + 128 kbit/s is 1,016,000 bytes a second: 300 s is over 300 MB, 295 s is under.
	if got := codes(ig.Check(in1080(295_000))); len(got) != 0 {
		t.Errorf("295 s: %v", got)
	}

	fs := ig.Check(in1080(296_000))
	if !slices.Equal(codes(fs), []string{CodeSizeTooLarge}) {
		t.Fatalf("296 s: %+v", fs)
	}

	if !strings.Contains(fs[0].Message, "estimated") || fs[0].Limit != "300.0 MB" {
		t.Errorf("estimate message %+v", fs[0])
	}

	// With HEVC the same minutes are smaller.
	in := in1080(400_000)
	in.Codec = CodecHEVC
	if got := codes(ig.Check(in)); len(got) != 0 {
		t.Errorf("hevc 400 s: %v", got)
	}

	// A real file is judged by its bytes, whatever the estimate said.
	file := in1080(10_000)
	file.Bytes = 300_000_000
	if got := codes(ig.Check(file)); len(got) != 0 {
		t.Errorf("a file of exactly the limit: %v", got)
	}

	file.Bytes = 300_000_001
	fs = ig.Check(file)
	if !slices.Equal(codes(fs), []string{CodeSizeTooLarge}) || strings.Contains(fs[0].Message, "estimated") {
		t.Errorf("a file over the limit: %+v", fs)
	}

	// A preset with no size limit never reports one.
	if got := codes(preset(t, "youtube-shorts").Check(in1080(180_000))); len(got) != 0 {
		t.Errorf("shorts: %v", got)
	}
}

// TestExportCheckFitsEveryPresetAtItsRecommendedSize: the recommended picture of a preset fits it.
func TestExportCheckFitsEveryPresetAtItsRecommendedSize(t *testing.T) {
	t.Parallel()

	for _, p := range Presets() {
		in := Input{Width: p.Targets.Width, Height: p.Targets.Height, FPSNum: 30, FPSDen: 1, DurationMS: 30_000}
		if fs := p.Check(in); len(fs) != 0 {
			t.Errorf("%s: its own recommended size does not fit: %+v", p.ID, fs)
		}
	}
}

// TestExportCheckReportsEverythingInOrder: several problems at once, in the documented order.
func TestExportCheckReportsEverythingInOrder(t *testing.T) {
	t.Parallel()

	in := Input{Width: 5000, Height: 400, FPSNum: 120, FPSDen: 1, DurationMS: 950_000}
	got := codes(preset(t, "instagram-reels").Check(in))
	want := []string{CodeDurationTooLong, CodeFPSTooHigh, CodeResolutionTooLarge, CodeAspectRatio, CodeSizeTooLarge}

	if !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

// TestExportCheckIgnoresWhatItDoesNotKnow: an input with no picture or no rate is not a finding.
func TestExportCheckIgnoresWhatItDoesNotKnow(t *testing.T) {
	t.Parallel()

	if fs := preset(t, "facebook-reels").Check(Input{DurationMS: 10_000}); len(fs) != 0 {
		t.Errorf("%+v", fs)
	}
}
