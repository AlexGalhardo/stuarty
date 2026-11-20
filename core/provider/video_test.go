package provider

import (
	"math"
	"strings"
	"testing"
)

func TestNormalizeVideo(t *testing.T) {
	t.Parallel()

	got := NormalizeVideo(VideoRequest{Model: " m ", Aspect: " 9:16 ", FirstFrame: " ast_1 "})
	if got.Model != "m" || got.Aspect != "9:16" || got.FirstFrame != "ast_1" || got.Seconds != DefaultVideoSeconds {
		t.Fatalf("got %+v", got)
	}

	got = NormalizeVideo(VideoRequest{Seconds: 8})
	if got.Aspect != DefaultVideoAspect || got.Seconds != 8 {
		t.Fatalf("the defaults: %+v", got)
	}
}

func TestValidateVideo(t *testing.T) {
	t.Parallel()

	ok := VideoRequest{Model: "vid-1", Prompt: "a fox in snow", Seconds: 5, Aspect: "16:9"}

	cases := []struct {
		name   string
		mutate func(r *VideoRequest)
		limits VideoLimits
		path   string
	}{
		{"valid", func(*VideoRequest) {}, VideoLimits{}, ""},
		{"valid in the documented limits", func(*VideoRequest) {}, VideoLimits{
			MinSeconds: 4, MaxSeconds: 8, Aspects: []string{"16:9", "9:16"},
		}, ""},
		{"bad model", func(r *VideoRequest) { r.Model = "a b" }, VideoLimits{}, "model"},
		{"empty prompt", func(r *VideoRequest) { r.Prompt = "" }, VideoLimits{}, "prompt"},
		{"prompt at the limit", func(r *VideoRequest) {
			r.Prompt = strings.Repeat("é", MaxVideoPrompt)
		}, VideoLimits{}, ""},
		{"prompt too long", func(r *VideoRequest) {
			r.Prompt = strings.Repeat("é", MaxVideoPrompt+1)
		}, VideoLimits{}, "prompt"},
		{"prompt with NUL", func(r *VideoRequest) { r.Prompt = "a\x00b" }, VideoLimits{}, "prompt"},
		{"zero seconds", func(r *VideoRequest) { r.Seconds = 0 }, VideoLimits{}, "seconds"},
		{"negative seconds", func(r *VideoRequest) { r.Seconds = -3 }, VideoLimits{}, "seconds"},
		{"the longest", func(r *VideoRequest) { r.Seconds = MaxVideoSeconds }, VideoLimits{}, ""},
		{"too many seconds", func(r *VideoRequest) { r.Seconds = MaxVideoSeconds + 1 }, VideoLimits{}, "seconds"},
		{"below the model", func(r *VideoRequest) { r.Seconds = 3 }, VideoLimits{MinSeconds: 4}, "seconds"},
		{"above the model", func(r *VideoRequest) { r.Seconds = 9 }, VideoLimits{MaxSeconds: 8}, "seconds"},
		{"a model limit above the product's does not widen it", func(r *VideoRequest) { r.Seconds = 61 }, VideoLimits{
			MaxSeconds: 120,
		}, "seconds"},
		{"unknown aspect", func(r *VideoRequest) { r.Aspect = "21:9" }, VideoLimits{}, "aspect"},
		{
			"aspect the model lacks",
			func(r *VideoRequest) { r.Aspect = "1:1" },
			VideoLimits{Aspects: []string{"16:9"}},
			"aspect",
		},
		{"a first frame", func(r *VideoRequest) { r.FirstFrame = "ast_01HXYZ" }, VideoLimits{}, ""},
		{
			"a first frame that is not an id",
			func(r *VideoRequest) { r.FirstFrame = "../x" },
			VideoLimits{},
			"first_frame",
		},
		{
			"a first frame too long",
			func(r *VideoRequest) { r.FirstFrame = strings.Repeat("a", 65) },
			VideoLimits{},
			"first_frame",
		},
		{
			"a first frame the model refuses",
			func(r *VideoRequest) { r.FirstFrame = "ast_1" },
			VideoLimits{NoFirstFrame: true},
			"first_frame",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := ok
			tc.mutate(&r)

			got := ValidateVideo(r, tc.limits)
			switch {
			case tc.path == "" && got != nil:
				t.Fatalf("want valid, got %v", got)
			case tc.path != "" && (got == nil || got.Path != tc.path):
				t.Fatalf("want an error on %s, got %v", tc.path, got)
			}
		})
	}
}

func TestVideoCost(t *testing.T) {
	t.Parallel()

	p := Price{Currency: "USD", Unit: UnitSecond, UnitMicros: 400_000}

	cases := []struct {
		name    string
		seconds int
		price   Price
		want    int64
	}{
		{"five seconds", 5, p, 2_000_000},
		{"one second", 1, p, 400_000},
		{"zero seconds", 0, p, 0},
		{"negative seconds", -1, p, 0},
		{"a free model", 5, Price{Currency: "USD", Unit: UnitSecond}, 0},
		{"saturates", 60, Price{Currency: "USD", Unit: UnitSecond, UnitMicros: math.MaxInt64 / 59}, math.MaxInt64},
		{
			"just under the saturation",
			2,
			Price{Currency: "USD", Unit: UnitSecond, UnitMicros: math.MaxInt64 / 2},
			math.MaxInt64 - 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := VideoCost(tc.seconds, tc.price)
			if got.Micros != tc.want || got.Currency != "USD" {
				t.Fatalf("got %+v, want %d USD", got, tc.want)
			}
		})
	}

	r := NormalizeVideo(VideoRequest{Seconds: 8})
	if got := EstimateVideo(r, p); got.Micros != 3_200_000 {
		t.Fatalf("EstimateVideo got %+v", got)
	}
}

func TestNormalizePriceSecond(t *testing.T) {
	t.Parallel()

	if _, f := NormalizePrice(Price{Currency: "USD", Unit: UnitSecond, UnitMicros: 100}); f != nil {
		t.Fatalf("a price per second: %v", f)
	}

	if _, f := NormalizePrice(
		Price{Currency: "USD", Unit: UnitSecond, UnitMicros: 100, InputMicrosPerMTok: 1},
	); f == nil {
		t.Fatal("a price per second with a token amount must be refused")
	}

	if _, f := NormalizePrice(Price{Currency: "USD", Unit: UnitSecond, UnitMicros: -1}); f == nil {
		t.Fatal("a negative price per second must be refused")
	}
}

func TestCheckVideo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []byte
		want string
		ok   bool
	}{
		{"mp4", []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00"), "mp4", true},
		{"quicktime", []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00"), "mov", true},
		{"webm", []byte{0x1A, 0x45, 0xDF, 0xA3, 0x01, 0x00}, "webm", true},
		{"a png", []byte("\x89PNG\r\n\x1a\n"), "", false},
		{"a short file", []byte("ftyp"), "", false},
		{"empty", nil, "", false},
		{"text", []byte("<html>not a video</html>"), "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := CheckVideo(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got %q %v, want %q %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestValidTaskID(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"abc", "operations/abc-123", "models/veo-3.1/operations/9f2c", "vid_01=="} {
		if !ValidTaskID(s) {
			t.Errorf("%q should be valid", s)
		}
	}

	for _, s := range []string{"", "/abc", "a b", "a\nb", "a\"b", strings.Repeat("a", MaxTaskID+1), "a;rm"} {
		if ValidTaskID(s) {
			t.Errorf("%q should be refused", s)
		}
	}
}
