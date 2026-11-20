package publish

import (
	"slices"
	"strings"
	"testing"
)

// A short vertical video that fits X and the draft of TikTok.
func clip() Post {
	return Post{
		Output: File{Bytes: 5_000_000, Format: "mp4", Media: &Media{
			Width: 720, Height: 1280, FPSNum: 30, FPSDen: 1, DurationMS: 30_000, VideoCodec: "h264", AudioCodec: "aac",
		}},
		Title: "A short clip", Visibility: Public,
	}
}

func TestValidateX(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(p *Post)
		want []string
	}{
		{"a valid post", func(*Post) {}, nil},
		{"a title of 280 characters", func(p *Post) { p.Title = strings.Repeat("a", 280) }, nil},
		{
			"a title of 281 characters", func(p *Post) { p.Title = strings.Repeat("a", 281) },
			[]string{"description:too_long"},
		},
		{
			// 139, the blank line (2) and 139 make 280.
			"title and description at 280 weighted characters",
			func(p *Post) { p.Title, p.Description = strings.Repeat("a", 139), strings.Repeat("b", 139) },
			nil,
		},
		{
			"title and description at 281 weighted characters",
			func(p *Post) { p.Title, p.Description = strings.Repeat("a", 140), strings.Repeat("b", 139) },
			[]string{"description:too_long"},
		},
		{
			// 23 for the link, 1 for the space and 256 make 280.
			"a link weighs 23 whatever its length",
			func(p *Post) {
				p.Title = "https://example.com/" + strings.Repeat("path/", 40) + " " + strings.Repeat("a", 256)
			},
			nil,
		},
		{
			"a link weighs 23 and one more character is over",
			func(p *Post) {
				p.Title = "https://example.com/" + strings.Repeat("path/", 40) + " " + strings.Repeat("a", 257)
			},
			[]string{"description:too_long"},
		},
		{
			"an emoji weighs 2: 140 of them fit",
			func(p *Post) { p.Title = strings.Repeat("\U0001F600", 140) },
			nil,
		},
		{
			"an emoji weighs 2: 141 of them do not",
			func(p *Post) { p.Title = strings.Repeat("\U0001F600", 141) },
			[]string{"description:too_long"},
		},
		{
			"the tags count as hashtags in the text",
			func(p *Post) { p.Title, p.Tags = strings.Repeat("a", 270), []string{"clip", "video"} },
			[]string{"description:too_long"},
		},
		{"private is not offered", func(p *Post) { p.Visibility = Private }, []string{"visibility:not_allowed"}},
		{"unlisted is not offered", func(p *Post) { p.Visibility = Unlisted }, []string{"visibility:not_allowed"}},
		{
			"a thumbnail is refused",
			func(p *Post) { p.Thumbnail = &File{Bytes: 10, Format: "png"} },
			[]string{"thumbnail:not_allowed"},
		},
		{"20 minutes is the longest", func(p *Post) { p.Output.Media.DurationMS = 1_200_000 }, nil},
		{
			"over 20 minutes", func(p *Post) { p.Output.Media.DurationMS = 1_200_001 },
			[]string{"output:duration_too_long"},
		},
		{"half a second is the shortest", func(p *Post) { p.Output.Media.DurationMS = 500 }, nil},
		{
			"under half a second", func(p *Post) { p.Output.Media.DurationMS = 499 },
			[]string{"output:duration_too_short"},
		},
		{"60 fps", func(p *Post) { p.Output.Media.FPSNum = 60 }, nil},
		{"61 fps", func(p *Post) { p.Output.Media.FPSNum = 61 }, []string{"output:fps_too_high"}},
		{
			"a 3:1 picture", func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 1800, 600 }, nil,
		},
		{
			"a 4:1 picture", func(p *Post) { p.Output.Media.Width, p.Output.Media.Height = 2000, 500 },
			[]string{"output:aspect_ratio"},
		},
		{"8 GB", func(p *Post) { p.Output.Bytes = 8_000_000_000 }, nil},
		{"one byte over 8 GB", func(p *Post) { p.Output.Bytes = 8_000_000_001 }, []string{"output:too_large"}},
		{
			"an hevc video", func(p *Post) { p.Output.Media.VideoCodec = "hevc" },
			[]string{"output:unsupported_format"},
		},
		{
			"a video that is a mov file", func(p *Post) { p.Output.Format = "mov" },
			[]string{"output:unsupported_format"},
		},
		{"no kids declaration is asked", func(p *Post) { p.MadeForKids = nil }, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := clip()
			tc.edit(&p)

			got := paths(Validate(p, XLimits()))
			if !slices.Equal(got, tc.want) {
				t.Errorf("issues = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestXRefusesALongTextWithTheExactCount(t *testing.T) {
	t.Parallel()

	p := clip()
	p.Title = strings.Repeat("a", 285)

	issues := Validate(p, XLimits())
	if len(issues) != 1 || issues[0].Path != "description" {
		t.Fatalf("issues = %v", issues)
	}

	caption := issues[0]

	if !strings.Contains(caption.Message, "280 weighted characters") || !strings.Contains(caption.Message, "285") {
		t.Errorf("the message does not state the limit and the count: %q", caption.Message)
	}
}

func TestValidateTikTokDraft(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(p *Post)
		want []string
	}{
		{"a valid draft", func(p *Post) { p.Visibility = "" }, nil},
		{"private is the only visibility", func(p *Post) { p.Visibility = Private }, nil},
		{"public is not offered", func(p *Post) { p.Visibility = Public }, []string{"visibility:not_allowed"}},
		{"unlisted is not offered", func(p *Post) { p.Visibility = Unlisted }, []string{"visibility:not_allowed"}},
		{"a title is only a name", func(p *Post) { p.Visibility, p.Title = "", strings.Repeat("a", 100) }, nil},
		{
			"a title over 100", func(p *Post) { p.Visibility, p.Title = "", strings.Repeat("a", 101) },
			[]string{"title:too_long"},
		},
		{"no title", func(p *Post) { p.Visibility, p.Title = "", " " }, []string{"title:required"}},
		{
			"the description and the tags are not sent, so no limit is checked on them",
			func(p *Post) {
				p.Visibility = ""
				p.Description = strings.Repeat("é", 20_000) + string(rune(0x200b))
				p.Tags = []string{strings.Repeat("a", 900), " "}
			},
			nil,
		},
		{
			"a thumbnail is refused",
			func(p *Post) { p.Visibility, p.Thumbnail = "", &File{Bytes: 10, Format: "jpg"} },
			[]string{"thumbnail:not_allowed"},
		},
		{"10 minutes is the longest", func(p *Post) { p.Visibility, p.Output.Media.DurationMS = "", 600_000 }, nil},
		{
			"over 10 minutes", func(p *Post) { p.Visibility, p.Output.Media.DurationMS = "", 600_001 },
			[]string{"output:duration_too_long"},
		},
		{"23 fps", func(p *Post) { p.Visibility, p.Output.Media.FPSNum = "", 23 }, nil},
		{"22 fps", func(p *Post) { p.Visibility, p.Output.Media.FPSNum = "", 22 }, []string{"output:fps_too_low"}},
		{"60 fps", func(p *Post) { p.Visibility, p.Output.Media.FPSNum = "", 60 }, nil},
		{"61 fps", func(p *Post) { p.Visibility, p.Output.Media.FPSNum = "", 61 }, []string{"output:fps_too_high"}},
		{
			"360 on the short side",
			func(p *Post) { p.Visibility, p.Output.Media.Width, p.Output.Media.Height = "", 360, 640 },
			nil,
		},
		{
			"359 on the short side",
			func(p *Post) { p.Visibility, p.Output.Media.Width, p.Output.Media.Height = "", 359, 640 },
			[]string{"output:resolution_too_small"},
		},
		{
			"4096 on the long side",
			func(p *Post) { p.Visibility, p.Output.Media.Width, p.Output.Media.Height = "", 2304, 4096 },
			nil,
		},
		{
			"4097 on the long side",
			func(p *Post) { p.Visibility, p.Output.Media.Width, p.Output.Media.Height = "", 2304, 4097 },
			[]string{"output:resolution_too_large"},
		},
		{"4 GB", func(p *Post) { p.Visibility, p.Output.Bytes = "", 4_000_000_000 }, nil},
		{
			"one byte over 4 GB", func(p *Post) { p.Visibility, p.Output.Bytes = "", 4_000_000_001 },
			[]string{"output:too_large"},
		},
		{"an hevc video", func(p *Post) { p.Visibility, p.Output.Media.VideoCodec = "", "hevc" }, nil},
		{
			"a vp9 video", func(p *Post) { p.Visibility, p.Output.Media.VideoCodec = "", "vp9" },
			[]string{"output:unsupported_format"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := clip()
			tc.edit(&p)

			got := paths(Validate(p, TikTokLimits()))
			if !slices.Equal(got, tc.want) {
				t.Errorf("issues = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTheNetworksOfEpic22CarryTheirSourceAndDate(t *testing.T) {
	t.Parallel()

	for name, l := range map[string]Limits{"x": XLimits(), "tiktok": TikTokLimits()} {
		if !strings.HasPrefix(l.Source, "https://") || l.Read != "2026-11-20" {
			t.Errorf("%s: source %q read %q", name, l.Source, l.Read)
		}
	}

	if tl := TikTokLimits(); !tl.NoCaption || len(tl.Visibilities) != 1 || tl.Visibilities[0] != Private {
		t.Errorf("TikTok must offer no caption and only private: %+v", tl)
	}
}

func TestPriceOfAPost(t *testing.T) {
	t.Parallel()

	x := XPrice()

	for _, tc := range []struct {
		name string
		text string
		want int64
		link bool
	}{
		{"plain text", "A new clip is out", 15_000, false},
		{"empty", "", 15_000, false},
		{"an https link", "Watch https://example.com/clip now", 200_000, true},
		{"an http link", "http://example.com", 200_000, true},
		{"a bare domain", "Watch it on example.com tonight", 200_000, true},
		{"a bare domain with a path", "see example.com/watch?v=1", 200_000, true},
		{"a domain at the end of a sentence", "Read more at stuarty.dev.", 200_000, true},
		{"a hashtag is not a link", "New #clip out", 15_000, false},
		{"a mention is not a link", "thanks @someone.cool for this", 15_000, false},
		{"an e-mail is not a link", "write to me@example.com", 15_000, false},
		{"a version number is not a link", "version 1.2 is out", 15_000, false},
		{"abbreviations are not links", "e.g. this, i.e. that", 15_000, false},
		{"a decimal is not a link", "it costs 0.5 now", 15_000, false},
	} {
		got, link := x.For(tc.text)
		if got != tc.want || link != tc.link {
			t.Errorf("%s: For(%q) = %d, %v; want %d, %v", tc.name, tc.text, got, link, tc.want, tc.link)
		}
	}

	if x.Currency != "USD" || x.Source == "" || x.Read != "2026-11-20" {
		t.Errorf("the price carries no source or date: %+v", x)
	}
}

func TestFormatMicros(t *testing.T) {
	t.Parallel()

	for micros, want := range map[int64]string{
		0: "0.00", 15_000: "0.015", 200_000: "0.20", 10_000: "0.01", 1: "0.000001", 1_000_000: "1.00",
		12_500_000: "12.50", MaxPriceMicros: "100.00",
	} {
		if got := FormatMicros(micros); got != want {
			t.Errorf("FormatMicros(%d) = %q, want %q", micros, got, want)
		}
	}
}

func TestPriceBounds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		post, link int64
		ok         bool
	}{
		{"the prices that were read", 15_000, 200_000, true},
		{"the highest", MaxPriceMicros, MaxPriceMicros, true},
		{"over the highest", MaxPriceMicros + 1, 200_000, false},
		{"a free post", 0, 200_000, false},
		{"a negative price", 15_000, -1, false},
	} {
		p := XPrice()
		p.PostMicros, p.LinkMicros = tc.post, tc.link

		if got := p.Valid() == nil; got != tc.ok {
			t.Errorf("%s: Valid = %v", tc.name, got)
		}
	}
}

func TestStateSentAsDraft(t *testing.T) {
	t.Parallel()

	for _, from := range []State{StateUploading, StateProcessing} {
		if !Can(from, StateSentAsDraft) {
			t.Errorf("a %s upload may end as a draft", from)
		}
	}

	if Can(StatePrepared, StateSentAsDraft) || Can(StateWaitingApproval, StateSentAsDraft) {
		t.Error("nothing is a draft before it is sent")
	}

	for _, to := range []State{StateUploading, StateProcessing, StatePublished, StateFailed} {
		if Can(StateSentAsDraft, to) {
			t.Errorf("a draft is final but moves to %s", to)
		}
	}

	if !StateSentAsDraft.Final() || StateSentAsDraft == StatePublished {
		t.Error("a draft is a final state of its own")
	}
}
