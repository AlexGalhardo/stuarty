package publish

import (
	"slices"
	"strings"
	"testing"
)

// The limits of an Instagram Reel and of a Facebook Page Reel as the connectors declare them (SDD-020
// section 18); the numbers of length, frame rate, picture and size come from core/export, read on 2026-11-19.
func igLimits() Limits {
	return Limits{
		TitleMax: 2200, DescriptionMaxBytes: 8800, TagsMaxChars: 2200, FileMaxBytes: 300_000_000,
		Formats: []string{"mp4", "mov"}, Visibilities: []Visibility{Public},
		Preset: "instagram-reels", VideoCodecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac"},
		CaptionMax: 2200, HashtagsMax: 30, DailyPosts: 100,
	}
}

func fbLimits() Limits {
	return Limits{
		TitleMax: 2200, DescriptionMaxBytes: 8800, TagsMaxChars: 2200, FileMaxBytes: 1 << 30,
		Formats: []string{"mp4"}, Visibilities: []Visibility{Public},
		Preset: "facebook-reels", VideoCodecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac"},
		CaptionMax: 2200, HashtagsMax: 30, DailyPosts: 30,
	}
}

func reel() Post {
	return Post{
		Output: File{Bytes: 5_000_000, Format: "mp4", Media: &Media{
			Width: 1080, Height: 1920, FPSNum: 30, FPSDen: 1, DurationMS: 30_000, VideoCodec: "h264", AudioCodec: "aac",
		}},
		Title: "A reel", Visibility: Public,
	}
}

func TestValidateReelLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		limits func() Limits
		edit   func(p *Post)
		want   []string
	}{
		{"instagram: a valid reel", igLimits, func(*Post) {}, nil},
		{"facebook: a valid reel", fbLimits, func(*Post) {}, nil},
		{"instagram: 3 seconds is the shortest", igLimits, func(p *Post) { p.Output.Media.DurationMS = 3000 }, nil},
		{
			"instagram: 2.999 seconds", igLimits, func(p *Post) { p.Output.Media.DurationMS = 2999 },
			[]string{"output:duration_too_short"},
		},
		{"instagram: 15 minutes is the longest", igLimits, func(p *Post) { p.Output.Media.DurationMS = 900_000 }, nil},
		{
			"instagram: over 15 minutes", igLimits, func(p *Post) { p.Output.Media.DurationMS = 900_001 },
			[]string{"output:duration_too_long"},
		},
		{"facebook: 90 seconds is the longest", fbLimits, func(p *Post) { p.Output.Media.DurationMS = 90_000 }, nil},
		{
			"facebook: 90.001 seconds", fbLimits, func(p *Post) { p.Output.Media.DurationMS = 90_001 },
			[]string{"output:duration_too_long"},
		},
		{"instagram: 23 fps", igLimits, func(p *Post) { p.Output.Media.FPSNum = 23 }, nil},
		{"instagram: 22 fps", igLimits, func(p *Post) { p.Output.Media.FPSNum = 22 }, []string{"output:fps_too_low"}},
		{"instagram: 60 fps", igLimits, func(p *Post) { p.Output.Media.FPSNum = 60 }, nil},
		{"instagram: 61 fps", igLimits, func(p *Post) { p.Output.Media.FPSNum = 61 }, []string{"output:fps_too_high"}},
		{"facebook: 23 fps", fbLimits, func(p *Post) { p.Output.Media.FPSNum = 23 }, []string{"output:fps_too_low"}},
		{"facebook: 24 fps", fbLimits, func(p *Post) { p.Output.Media.FPSNum = 24 }, nil},
		{
			"instagram: 1920 wide is the widest", igLimits,
			func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 1920, 1080 }, nil,
		},
		{
			"instagram: 1921 wide", igLimits,
			func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 1921, 1080 },
			[]string{"output:resolution_too_large"},
		},
		{
			"facebook: 540x960 is the smallest", fbLimits,
			func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 540, 960 }, nil,
		},
		{
			"facebook: 539x960", fbLimits,
			func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 539, 960 },
			[]string{"output:resolution_too_small"},
		},
		{
			"facebook: a landscape video is not 9:16", fbLimits,
			func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 1920, 1080 },
			[]string{"output:aspect_ratio"},
		},
		{
			"instagram: a landscape video is allowed", igLimits,
			func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 1920, 1080 }, nil,
		},
		{
			"instagram: a 12:1 picture", igLimits,
			func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 1200, 100 },
			[]string{"output:aspect_ratio"},
		},
		{"instagram: 300,000,000 bytes", igLimits, func(p *Post) { p.Output.Bytes = 300_000_000 }, nil},
		{
			"instagram: one byte over", igLimits, func(p *Post) { p.Output.Bytes = 300_000_001 },
			[]string{"output:too_large"},
		},
		{"facebook: 1 GiB", fbLimits, func(p *Post) { p.Output.Bytes = 1 << 30 }, nil},
		{"facebook: over 1 GiB", fbLimits, func(p *Post) { p.Output.Bytes = 1<<30 + 1 }, []string{"output:too_large"}},
		{
			"instagram: a vp9 video", igLimits, func(p *Post) { p.Output.Media.VideoCodec = "vp9" },
			[]string{"output:unsupported_format"},
		},
		{"instagram: hevc", igLimits, func(p *Post) { p.Output.Media.VideoCodec = "HEVC" }, nil},
		{
			"instagram: an opus track", igLimits, func(p *Post) { p.Output.Media.AudioCodec = "opus" },
			[]string{"output:unsupported_format"},
		},
		{"instagram: no audio track", igLimits, func(p *Post) { p.Output.Media.AudioCodec = "" }, nil},
		{"instagram: a mov file", igLimits, func(p *Post) { p.Output.Format = "mov" }, nil},
		{
			"facebook: a mov file",
			fbLimits,
			func(p *Post) { p.Output.Format = "mov" },
			[]string{"output:unsupported_format"},
		},
		{
			"no properties of the video", igLimits, func(p *Post) { p.Output.Media = nil },
			[]string{"output:required"},
		},
		{
			"a video with no length", igLimits, func(p *Post) { p.Output.Media.DurationMS = 0 },
			[]string{"output:required"},
		},
		{
			"private is not allowed", igLimits, func(p *Post) { p.Visibility = Private },
			[]string{"visibility:not_allowed"},
		},
		{
			"no visibility is private", fbLimits, func(p *Post) { p.Visibility = "" },
			[]string{"visibility:not_allowed"},
		},
		{
			"a thumbnail is not taken", igLimits, func(p *Post) { p.Thumbnail = &File{Bytes: 10, Format: "jpg"} },
			[]string{"thumbnail:not_allowed"},
		},
		{
			"every problem at once", igLimits,
			func(p *Post) { p.Title = ""; p.Visibility = Unlisted; p.Output.Media.DurationMS = 1 },
			[]string{"title:required", "visibility:not_allowed", "output:duration_too_short"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := reel()
			tc.edit(&p)

			if got := paths(Validate(p, tc.limits())); !slices.Equal(got, tc.want) {
				t.Errorf("issues = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateReelNamesTheExactLimit(t *testing.T) {
	t.Parallel()

	p := reel()
	p.Output.Media.DurationMS = 95_000

	issues := Validate(p, fbLimits())
	if len(issues) != 1 {
		t.Fatalf("issues = %+v", issues)
	}

	for _, want := range []string{"length", "Facebook Reels", "90 s", "95 s"} {
		if !strings.Contains(issues[0].Message, want) {
			t.Errorf("message %q does not say %q", issues[0].Message, want)
		}
	}
}

func TestValidateCaptionAndHashtags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(p *Post)
		want []string
	}{
		{"a caption of exactly 2200 characters", func(p *Post) { p.Title = strings.Repeat("é", 2200) }, nil},
		{
			"2201 characters",
			func(p *Post) { p.Title = strings.Repeat("é", 2201) },
			[]string{"title:too_long", "description:too_long"},
		},
		{
			"the title, the blank line and the description add up",
			func(p *Post) { p.Title = strings.Repeat("a", 1000); p.Description = strings.Repeat("b", 1198) },
			nil,
		},
		{
			"one more",
			func(p *Post) { p.Title = strings.Repeat("a", 1000); p.Description = strings.Repeat("b", 1199) },
			[]string{"description:too_long"},
		},
		{"30 hashtags from the tags", func(p *Post) { p.Tags = manyTags(30) }, nil},
		{"31 hashtags from the tags", func(p *Post) { p.Tags = manyTags(31) }, []string{"tags:too_long"}},
		{
			"hashtags of the description count too",
			func(p *Post) { p.Description = strings.Repeat("#a ", 25); p.Tags = manyTags(6) },
			[]string{"tags:too_long"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := reel()
			tc.edit(&p)

			if got := paths(Validate(p, igLimits())); !slices.Equal(got, tc.want) {
				t.Errorf("issues = %v, want %v", got, tc.want)
			}
		})
	}
}

func manyTags(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "t" + strings.Repeat("x", i%3)
	}

	return out
}

func TestCaption(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		p    Post
		want string
	}{
		{"title only", Post{Title: " Hello "}, "Hello"},
		{
			"all three",
			Post{Title: "T", Description: "D", Tags: []string{"one", "two words", "#three", " "}},
			"T\n\nD\n\n#one #twowords #three",
		},
		{"no description", Post{Title: "T", Tags: []string{"a"}}, "T\n\n#a"},
	} {
		if got := Caption(&tc.p); got != tc.want {
			t.Errorf("%s: Caption = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestHashtags(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]int{
		"": 0, "no tags": 0, "#a": 1, "#a #b": 2, "# alone": 0, "end #": 0, "##a": 1, "#_x #1": 2, "é #é": 1,
	} {
		if got := Hashtags(text); got != want {
			t.Errorf("Hashtags(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestParseFPS(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in       string
		num, den int
	}{
		{"30", 30, 1},
		{"30000/1001", 30000, 1001},
		{" 24 ", 24, 1},
		{"", 0, 0},
		{"x", 0, 0},
		{"30/0", 0, 0},
		{"-1", 0, 0},
		{"30/x", 0, 0},
	} {
		if n, d := ParseFPS(tc.in); n != tc.num || d != tc.den {
			t.Errorf("ParseFPS(%q) = %d/%d, want %d/%d", tc.in, n, d, tc.num, tc.den)
		}
	}
}

func TestYouTubeIsUnchangedByTheReelFields(t *testing.T) {
	t.Parallel()

	// A network with no preset and no codecs asks nothing of the media: the YouTube limits still pass a post
	// whose file carries none.
	if got := Validate(good(), yt()); got != nil {
		t.Fatalf("issues = %v", got)
	}
}
