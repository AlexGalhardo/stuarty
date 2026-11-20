package provider

import (
	"math"
	"strings"
	"testing"
)

func TestNormalizeImage(t *testing.T) {
	t.Parallel()

	got := NormalizeImage(ImageRequest{Model: " m ", Size: " 1024X1024 "})
	if got.Model != "m" || got.Size != "1024x1024" || got.Count != 1 {
		t.Fatalf("got %+v", got)
	}

	got = NormalizeImage(ImageRequest{Count: 3})
	if got.Size != DefaultImageSize || got.Count != 3 {
		t.Fatalf("the defaults: %+v", got)
	}
}

func TestValidateImage(t *testing.T) {
	t.Parallel()

	ok := ImageRequest{Model: "img-1", Prompt: "a red fox", Size: "1024x1024", Count: 1}

	cases := []struct {
		name   string
		mutate func(r *ImageRequest)
		limits ImageLimits
		path   string
	}{
		{"valid", func(*ImageRequest) {}, ImageLimits{}, ""},
		{"valid in the documented sizes", func(*ImageRequest) {}, ImageLimits{Sizes: []string{"1024x1024"}}, ""},
		{"bad model", func(r *ImageRequest) { r.Model = "a b" }, ImageLimits{}, "model"},
		{"empty prompt", func(r *ImageRequest) { r.Prompt = "" }, ImageLimits{}, "prompt"},
		{
			"prompt at the limit",
			func(r *ImageRequest) { r.Prompt = strings.Repeat("é", MaxImagePrompt) },
			ImageLimits{},
			"",
		},
		{
			"prompt over the limit",
			func(r *ImageRequest) { r.Prompt = strings.Repeat("é", MaxImagePrompt+1) },
			ImageLimits{},
			"prompt",
		},
		{"prompt with a NUL", func(r *ImageRequest) { r.Prompt = "a\x00" }, ImageLimits{}, "prompt"},
		{"prompt not utf-8", func(r *ImageRequest) { r.Prompt = "a\xff" }, ImageLimits{}, "prompt"},
		{"size not a size", func(r *ImageRequest) { r.Size = "big" }, ImageLimits{}, "size"},
		{"size with three parts", func(r *ImageRequest) { r.Size = "1x2x3" }, ImageLimits{}, "size"},
		{"size too small", func(r *ImageRequest) { r.Size = "32x32" }, ImageLimits{}, "size"},
		{"size too large", func(r *ImageRequest) { r.Size = "4097x1024" }, ImageLimits{}, "size"},
		{"size at the limits", func(r *ImageRequest) { r.Size = "4096x64" }, ImageLimits{}, ""},
		{
			"size not documented",
			func(r *ImageRequest) { r.Size = "512x512" },
			ImageLimits{Sizes: []string{"1024x1024"}},
			"size",
		},
		{"count zero", func(r *ImageRequest) { r.Count = 0 }, ImageLimits{}, "count"},
		{"count negative", func(r *ImageRequest) { r.Count = -1 }, ImageLimits{}, "count"},
		{"count at the limit", func(r *ImageRequest) { r.Count = MaxImageCount }, ImageLimits{}, ""},
		{"count over the limit", func(r *ImageRequest) { r.Count = MaxImageCount + 1 }, ImageLimits{}, "count"},
		{"count over the model", func(r *ImageRequest) { r.Count = 2 }, ImageLimits{MaxCount: 1}, "count"},
		{
			"count over the limit with a larger model limit",
			func(r *ImageRequest) { r.Count = 11 },
			ImageLimits{MaxCount: 20},
			"count",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := ok
			tc.mutate(&r)

			f := ValidateImage(r, tc.limits)
			switch {
			case tc.path == "" && f != nil:
				t.Fatalf("want valid, got %v", f)
			case tc.path != "" && (f == nil || f.Path != tc.path):
				t.Fatalf("want a refusal at %q, got %v", tc.path, f)
			}
		})
	}
}

func TestNormalizeVoice(t *testing.T) {
	t.Parallel()

	got := NormalizeVoice(VoiceRequest{Model: " m ", Voice: " alloy ", Language: " pt-BR ", Format: " MP3 "})
	if got.Model != "m" || got.Voice != "alloy" || got.Language != "pt-BR" || got.Format != "mp3" {
		t.Fatalf("got %+v", got)
	}

	if got := NormalizeVoice(VoiceRequest{}); got.Format != DefaultVoiceFmt {
		t.Fatalf("the default format: %+v", got)
	}
}

func TestValidateVoice(t *testing.T) {
	t.Parallel()

	ok := VoiceRequest{Model: "tts-1", Text: "Hello there.", Voice: "alloy", Format: "mp3"}

	cases := []struct {
		name   string
		mutate func(r *VoiceRequest)
		limits VoiceLimits
		path   string
	}{
		{"valid", func(*VoiceRequest) {}, VoiceLimits{}, ""},
		{"valid with a language", func(r *VoiceRequest) { r.Language = "pt-BR" }, VoiceLimits{}, ""},
		{"bad model", func(r *VoiceRequest) { r.Model = "" }, VoiceLimits{}, "model"},
		{"empty text", func(r *VoiceRequest) { r.Text = "" }, VoiceLimits{}, "text"},
		{
			"text at the default limit",
			func(r *VoiceRequest) { r.Text = strings.Repeat("é", DefaultVoiceChars) },
			VoiceLimits{},
			"",
		},
		{
			"text over the default limit",
			func(r *VoiceRequest) { r.Text = strings.Repeat("é", DefaultVoiceChars+1) },
			VoiceLimits{},
			"text",
		},
		{"text over the model limit", func(r *VoiceRequest) { r.Text = "12345" }, VoiceLimits{MaxChars: 4}, "text"},
		{"text with a NUL", func(r *VoiceRequest) { r.Text = "a\x00" }, VoiceLimits{}, "text"},
		{"text not utf-8", func(r *VoiceRequest) { r.Text = "a\xff" }, VoiceLimits{}, "text"},
		{"empty voice", func(r *VoiceRequest) { r.Voice = "" }, VoiceLimits{}, "voice"},
		{"voice with a space", func(r *VoiceRequest) { r.Voice = "a b" }, VoiceLimits{}, "voice"},
		{
			"voice too long",
			func(r *VoiceRequest) { r.Voice = strings.Repeat("a", MaxVoiceID+1) },
			VoiceLimits{},
			"voice",
		},
		{"language with a space", func(r *VoiceRequest) { r.Language = "pt BR" }, VoiceLimits{}, "language"},
		{"language of one letter", func(r *VoiceRequest) { r.Language = "p" }, VoiceLimits{}, "language"},
		{
			"language too long",
			func(r *VoiceRequest) { r.Language = "pt-" + strings.Repeat("a", 40) },
			VoiceLimits{},
			"language",
		},
		{"unknown format", func(r *VoiceRequest) { r.Format = "ogg" }, VoiceLimits{}, "format"},
		{"empty format", func(r *VoiceRequest) { r.Format = "" }, VoiceLimits{}, "format"},
		{
			"format not written by the model",
			func(r *VoiceRequest) { r.Format = "wav" },
			VoiceLimits{Formats: []string{"mp3"}},
			"format",
		},
		{"every format", func(r *VoiceRequest) { r.Format = "opus" }, VoiceLimits{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := ok
			tc.mutate(&r)

			f := ValidateVoice(r, tc.limits)
			switch {
			case tc.path == "" && f != nil:
				t.Fatalf("want valid, got %v", f)
			case tc.path != "" && (f == nil || f.Path != tc.path):
				t.Fatalf("want a refusal at %q, got %v", tc.path, f)
			}
		})
	}
}

func TestNormalizePriceUnits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   Price
		path string
	}{
		{"image", Price{Currency: "usd", Unit: " Image ", UnitMicros: 40_000}, ""},
		{"image at zero is a free model the person chose", Price{Currency: "USD", Unit: UnitImage}, ""},
		{"mchar", Price{Currency: "USD", Unit: UnitMChar, UnitMicros: 15_000_000}, ""},
		{
			"image with token amounts",
			Price{Currency: "USD", Unit: UnitImage, UnitMicros: 1, InputMicrosPerMTok: 1},
			"input_micros_per_mtok",
		},
		{
			"image with output amounts",
			Price{Currency: "USD", Unit: UnitImage, OutputMicrosPerMTok: 1},
			"input_micros_per_mtok",
		},
		{"image negative", Price{Currency: "USD", Unit: UnitImage, UnitMicros: -1}, "unit_micros"},
		{"image too large", Price{Currency: "USD", Unit: UnitImage, UnitMicros: MaxPriceMicros + 1}, "unit_micros"},
		{"tokens with unit micros", Price{Currency: "USD", UnitMicros: 5}, "unit_micros"},
		{"unknown unit", Price{Currency: "USD", Unit: "minute"}, "unit"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, f := NormalizePrice(tc.in)

			switch {
			case tc.path == "" && f != nil:
				t.Fatalf("want valid, got %v", f)
			case tc.path != "" && (f == nil || f.Path != tc.path):
				t.Fatalf("want a refusal at %q, got %v", tc.path, f)
			case tc.path == "" && (p.Currency != "USD" || p.Unit != tc.in.Unit && p.Unit != strings.ToLower(strings.TrimSpace(tc.in.Unit))):
				t.Fatalf("not normalized: %+v", p)
			}
		})
	}
}

func TestImageCost(t *testing.T) {
	t.Parallel()

	p := Price{Currency: "USD", Unit: UnitImage, UnitMicros: 40_000}

	cases := []struct {
		name  string
		count int
		price Price
		want  int64
	}{
		{"one", 1, p, 40_000},
		{"four", 4, p, 160_000},
		{"none", 0, p, 0},
		{"negative", -3, p, 0},
		{"a free image", 2, Price{Currency: "USD", Unit: UnitImage}, 0},
		{
			"overflow saturates",
			10,
			Price{Currency: "USD", Unit: UnitImage, UnitMicros: math.MaxInt64 / 5},
			math.MaxInt64,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ImageCost(tc.count, tc.price)
			if got.Micros != tc.want || got.Currency != "USD" {
				t.Fatalf("got %+v, want %d", got, tc.want)
			}
		})
	}

	if got := EstimateImage(ImageRequest{Count: 3}, p); got.Micros != 120_000 {
		t.Fatalf("estimate %+v", got)
	}
}

func TestVoiceCost(t *testing.T) {
	t.Parallel()

	p := Price{Currency: "USD", Unit: UnitMChar, UnitMicros: 15_000_000}

	cases := []struct {
		name  string
		chars int
		want  int64
	}{
		{"none", 0, 0},
		{"negative", -1, 0},
		{"a thousand", 1000, 15_000},
		{"one character is never rounded to nothing", 1, 15},
		{"a million", 1_000_000, 15_000_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := VoiceCost(tc.chars, p); got.Micros != tc.want || got.Currency != "USD" {
				t.Fatalf("got %+v, want %d", got, tc.want)
			}
		})
	}

	if got := VoiceCost(1, Price{Currency: "USD", Unit: UnitMChar, UnitMicros: 1}); got.Micros != 1 {
		t.Fatalf("a price of one micro per million characters still rounds up to one: %+v", got)
	}

	// Characters, not bytes: a text of é is one character each.
	if got := EstimateVoice(VoiceRequest{Text: strings.Repeat("é", 1000)}, p); got.Micros != 15_000 {
		t.Fatalf("estimate %+v", got)
	}

	if got := VoiceCost(
		math.MaxInt32,
		Price{Currency: "USD", Unit: UnitMChar, UnitMicros: MaxPriceMicros},
	); got.Micros <= 0 {
		t.Fatalf("a huge cost never wraps to a small number: %+v", got)
	}
}

func TestCheckImage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n....."), "png"},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0}, "jpeg"},
		{"webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8 "), "webp"},
		{"wav is not an image", []byte("RIFF\x10\x00\x00\x00WAVEfmt "), ""},
		{"html", []byte("<html><body>"), ""},
		{"empty", nil, ""},
		{"a png signature cut short", []byte("\x89PNG"), ""},
		{"gif is not offered", []byte("GIF89a"), ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := CheckImage(tc.in)
			if got != tc.want || ok != (tc.want != "") {
				t.Fatalf("got %q, %v", got, ok)
			}
		})
	}
}

func TestCheckAudio(t *testing.T) {
	t.Parallel()

	mp3Frame := []byte{0xFF, 0xFB, 0x90, 0x00}
	adts := []byte{0xFF, 0xF1, 0x50, 0x80}
	wav := []byte("RIFF\x10\x00\x00\x00WAVEfmt ")

	cases := []struct {
		name   string
		in     []byte
		format string
		want   bool
	}{
		{"mp3 with an id3 tag", []byte("ID3\x04\x00"), "mp3", true},
		{"mp3 frame sync", mp3Frame, "mp3", true},
		{"adts is not mp3", adts, "mp3", false},
		{"wav", wav, "wav", true},
		{"webp is not wav", []byte("RIFF\x10\x00\x00\x00WEBPVP8 "), "wav", false},
		{"flac", []byte("fLaC\x00"), "flac", true},
		{"opus in an ogg page", []byte("OggS\x00\x02"), "opus", true},
		{"aac in adts", adts, "aac", true},
		{"mp3 frame is not aac", mp3Frame, "aac", false},
		{"a wav asked as mp3", wav, "mp3", false},
		{"an mp3 asked as wav", mp3Frame, "wav", false},
		{"html asked as anything", []byte("<html>"), "mp3", false},
		{"empty", nil, "mp3", false},
		{"one byte", []byte{0xFF}, "aac", false},
		{"unknown format", mp3Frame, "ogg", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := CheckAudio(tc.in, tc.format); got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestExtensions(t *testing.T) {
	t.Parallel()

	if AudioExt("opus") != "ogg" || AudioExt("mp3") != "mp3" || ImageExt("jpeg") != "jpg" || ImageExt("png") != "png" {
		t.Fatal("extensions")
	}
}
