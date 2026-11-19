package export

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestExportPresetTable: the five placements of RF-087 are there, in the order of SDD-000 section 15, and
// each one carries what the epic asks for: a container, a codec order that ends in something the
// product can always write, targets, and the official page and the date its limits were read on.
func TestExportPresetTable(t *testing.T) {
	t.Parallel()

	want := []string{"youtube-shorts", "instagram-reels", "facebook-reels", "tiktok", "x-video"}
	if got := IDs(); !slices.Equal(got, want) {
		t.Fatalf("preset ids %v, want %v", got, want)
	}

	seen := map[string]bool{}

	for _, p := range Presets() {
		if seen[p.ID] {
			t.Errorf("%s: id used twice", p.ID)
		}

		seen[p.ID] = true

		if p.Network == "" || p.Placement == "" || p.Label == "" {
			t.Errorf("%s: network, placement and label are required", p.ID)
		}

		if p.Container != "mp4" {
			t.Errorf("%s: container %q, want mp4", p.ID, p.Container)
		}

		if len(p.Codecs) == 0 || !slices.Contains(p.Codecs, CodecH264) {
			t.Errorf("%s: codecs %v must end in something the product always writes (h264)", p.ID, p.Codecs)
		}

		for _, c := range p.Codecs {
			if !c.Valid() || c == "" {
				t.Errorf("%s: unknown codec %q", p.ID, c)
			}
		}

		tg := p.Targets
		if tg.Width <= 0 || tg.Height <= 0 || tg.VideoKbps <= 0 || tg.AudioKbps <= 0 || tg.SampleHz != 48000 ||
			tg.Channels != 2 {
			t.Errorf("%s: incomplete targets %+v", p.ID, tg)
		}

		if len(p.Sources) == 0 {
			t.Errorf("%s: a preset with no source is not shipped", p.ID)
		}

		for _, s := range p.Sources {
			u, err := url.Parse(s.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				t.Errorf("%s: source %q is not an https page", p.ID, s.URL)
			}

			d, err := time.Parse(time.DateOnly, s.Read)
			if err != nil || d.After(time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)) || d.Year() < 2026 {
				t.Errorf("%s: source %s has the read date %q", p.ID, s.URL, s.Read)
			}

			if strings.TrimSpace(s.Says) == "" {
				t.Errorf("%s: source %s says nothing", p.ID, s.URL)
			}
		}
	}
}

// TestExportPresetLimits pins the limits to what each official page said on the day it was read: a change
// of a limit is a change of this table, of SDD-004 section 8.2 and of the date.
func TestExportPresetLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id     string
		limits Limits
		codecs []Codec
	}{
		{"youtube-shorts", Limits{MaxDurationMS: 180_000, AspectMax: Ratio{1, 1}}, []Codec{CodecH264}},
		{"instagram-reels", Limits{
			MinDurationMS: 3_000, MaxDurationMS: 900_000, MinFPS: 23, MaxFPS: 60, MaxWidth: 1920,
			AspectMin: Ratio{1, 100}, AspectMax: Ratio{10, 1}, MaxBytes: 300_000_000, MaxVideoKbps: 25_000,
		}, []Codec{CodecHEVC, CodecH264}},
		{"facebook-reels", Limits{
			MinDurationMS: 3_000, MaxDurationMS: 90_000, MinFPS: 24, MaxFPS: 60, MinWidth: 540, MinHeight: 960,
			AspectMin: Ratio{9, 16}, AspectMax: Ratio{9, 16}, AspectTolerancePermille: 10,
		}, []Codec{CodecHEVC, CodecH264}},
		{"tiktok", Limits{
			MaxDurationMS: 600_000, MinFPS: 23, MaxFPS: 60, MinWidth: 360, MinHeight: 360, MaxWidth: 4096,
			MaxHeight: 4096, MaxBytes: 4_000_000_000,
		}, []Codec{CodecHEVC, CodecH264}},
		{"x-video", Limits{
			MinDurationMS: 500, MaxDurationMS: 1_200_000, MaxFPS: 60, AspectMin: Ratio{1, 3}, AspectMax: Ratio{3, 1},
			MaxBytes: 8_000_000_000,
		}, []Codec{CodecH264}},
	}

	for _, c := range cases {
		p, ok := Lookup(c.id)
		if !ok {
			t.Fatalf("%s is not a preset", c.id)
		}

		if p.Limits != c.limits {
			t.Errorf("%s: limits %+v, want %+v", c.id, p.Limits, c.limits)
		}

		if !slices.Equal(p.Codecs, c.codecs) {
			t.Errorf("%s: codecs %v, want %v", c.id, p.Codecs, c.codecs)
		}
	}

	if _, ok := Lookup("myspace"); ok {
		t.Error("an unknown preset was found")
	}
}

// TestExportPresetsAreCopies: changing what Presets returns does not change the table.
func TestExportPresetsAreCopies(t *testing.T) {
	t.Parallel()

	a := Presets()
	a[0].Codecs[0] = CodecAV1
	a[0].Sources[0].URL = "x"
	a[0].Label = "changed"

	b := Presets()
	if b[0].Codecs[0] != CodecH264 || b[0].Sources[0].URL == "x" || b[0].Label == "changed" {
		t.Fatalf("the table was changed through a copy: %+v", b[0])
	}
}

// TestExportPresetVideoKbps: the bit rate follows the pixel count, rises above 30 fps, is a share of
// it for HEVC and AV1, has a floor and respects the ceiling of the preset.
func TestExportPresetVideoKbps(t *testing.T) {
	t.Parallel()

	ig, _ := Lookup("instagram-reels")
	x, _ := Lookup("x-video")

	cases := []struct {
		name       string
		p          Preset
		w, h, n, d int
		codec      Codec
		want       int
	}{
		{"reference size", ig, 1080, 1920, 30, 1, CodecH264, 8000},
		{"default codec", ig, 1080, 1920, 30, 1, "", 8000},
		{"half the pixels", ig, 540, 960, 30, 1, CodecH264, 2000},
		{"above 30 fps", ig, 1080, 1920, 60, 1, CodecH264, 12000},
		{"exactly 30 fps", ig, 1080, 1920, 30000, 1001, CodecH264, 8000},
		{"hevc", ig, 1080, 1920, 30, 1, CodecHEVC, 4800},
		{"av1", ig, 1080, 1920, 30, 1, CodecAV1, 4000},
		{"floor", ig, 320, 240, 30, 1, CodecH264, 1000},
		{"ceiling", ig, 3840, 2160, 60, 1, CodecH264, 25000},
		{"x reference", x, 720, 1280, 30, 1, CodecH264, 5000},
	}

	for _, c := range cases {
		if got := c.p.VideoKbps(c.w, c.h, c.n, c.d, c.codec); got != c.want {
			t.Errorf("%s: %d kbit/s, want %d", c.name, got, c.want)
		}
	}
}

// TestExportPresetPlanCodec: the first codec of the list the machine can write wins, H.264 is always
// writable, and a fall back names what was wanted.
func TestExportPresetPlanCodec(t *testing.T) {
	t.Parallel()

	only := func(cs ...Codec) func(Codec) bool {
		return func(c Codec) bool { return slices.Contains(cs, c) }
	}

	cases := []struct {
		name      string
		preferred []Codec
		offered   func(Codec) bool
		want      Plan
	}{
		{"first is offered", []Codec{CodecHEVC, CodecH264}, only(CodecHEVC), Plan{Codec: CodecHEVC}},
		{"first is not offered", []Codec{CodecHEVC, CodecH264}, only(), Plan{Codec: CodecH264, Wanted: CodecHEVC}},
		{"h264 only", []Codec{CodecH264}, only(CodecAV1), Plan{Codec: CodecH264}},
		{
			"second is offered",
			[]Codec{CodecAV1, CodecHEVC, CodecH264},
			only(CodecHEVC),
			Plan{Codec: CodecHEVC, Wanted: CodecAV1},
		},
		{"nothing listed", nil, only(), Plan{Codec: CodecH264}},
		{"list without h264", []Codec{CodecAV1}, only(), Plan{Codec: CodecH264, Wanted: CodecAV1}},
	}

	for _, c := range cases {
		if got := PlanCodec(c.preferred, c.offered); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestExportPresetCodecNames: the codec names of SDD-000 section 15, and the zero value is H.264.
func TestExportPresetCodecNames(t *testing.T) {
	t.Parallel()

	if !slices.Equal(Codecs(), []Codec{"h264", "hevc", "av1"}) {
		t.Errorf("codecs %v", Codecs())
	}

	for _, c := range []Codec{"", "h264", "hevc", "av1"} {
		if !c.Valid() {
			t.Errorf("%q is a codec", c)
		}
	}

	for _, c := range []Codec{"libx265", "h265", "vp9", "H264"} {
		if c.Valid() {
			t.Errorf("%q is not a codec of an export", c)
		}
	}

	if Codec("").Normal() != CodecH264 {
		t.Error("the zero codec is h264")
	}
}
