package publish

import (
	"slices"
	"strings"
	"testing"
)

func yt() Limits {
	return Limits{
		TitleMax:             100,
		DescriptionMaxBytes:  5000,
		TagsMaxChars:         500,
		FileMaxBytes:         256 << 30,
		ThumbnailMaxBytes:    50 << 20,
		Formats:              []string{"mp4"},
		ThumbnailFormats:     []string{"png", "jpg"},
		Visibilities:         []Visibility{Private, Unlisted, Public},
		NeedsKidsDeclaration: true,
		Forbidden:            "<>",
	}
}

func good() Post {
	return Post{Output: File{Bytes: 1000, Format: "mp4"}, Title: "My video", MadeForKids: new(false)}
}

func paths(issues []Issue) []string {
	out := make([]string, len(issues))
	for i, x := range issues {
		out[i] = x.Path + ":" + x.Code
	}

	return out
}

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(p *Post)
		want []string
	}{
		{"a valid post", func(*Post) {}, nil},
		{"no title", func(p *Post) { p.Title = "  " }, []string{"title:required"}},
		{"title at the limit", func(p *Post) { p.Title = strings.Repeat("é", 100) }, nil},
		{"title over the limit", func(p *Post) { p.Title = strings.Repeat("é", 101) }, []string{"title:too_long"}},
		{"angle bracket in the title", func(p *Post) { p.Title = "a <b>" }, []string{"title:forbidden_character"}},
		{"control character in the title", func(p *Post) { p.Title = "a\x00b" }, []string{"title:invalid"}},
		{"description at the limit in bytes", func(p *Post) { p.Description = strings.Repeat("a", 5000) }, nil},
		{
			"description over the limit in bytes",
			func(p *Post) { p.Description = strings.Repeat("é", 2501) },
			[]string{"description:too_long"},
		},
		{"newline in the description is fine", func(p *Post) { p.Description = "a\nb" }, nil},
		{
			"description with a bracket",
			func(p *Post) { p.Description = "x > y" },
			[]string{"description:forbidden_character"},
		},
		{"tags at the limit", func(p *Post) { p.Tags = []string{strings.Repeat("a", 500)} }, nil},
		{
			"tags over the limit with commas",
			func(p *Post) { p.Tags = []string{strings.Repeat("a", 250), strings.Repeat("b", 250)} },
			[]string{"tags:too_long"},
		},
		{
			"a tag with a space counts quotes",
			func(p *Post) { p.Tags = []string{strings.Repeat("a", 498) + " b"} },
			[]string{"tags:too_long"},
		},
		{"an empty tag", func(p *Post) { p.Tags = []string{"a", " "} }, []string{"tags[1]:invalid"}},
		{"visibility defaults to private", func(p *Post) { p.Visibility = "" }, nil},
		{"unknown visibility", func(p *Post) { p.Visibility = "friends" }, []string{"visibility:not_allowed"}},
		{"kids not declared", func(p *Post) { p.MadeForKids = nil }, []string{"made_for_kids:required"}},
		{"wrong video format", func(p *Post) { p.Output.Format = "avi" }, []string{"output:unsupported_format"}},
		{"an empty file", func(p *Post) { p.Output.Bytes = 0 }, []string{"output:required"}},
		{"a file too large", func(p *Post) { p.Output.Bytes = 257 << 30 }, []string{"output:too_large"}},
		{"thumbnail fine", func(p *Post) { p.Thumbnail = &File{Bytes: 10, Format: "PNG"} }, nil},
		{
			"thumbnail wrong format",
			func(p *Post) { p.Thumbnail = &File{Bytes: 10, Format: "gif"} },
			[]string{"thumbnail:unsupported_format"},
		},
		{
			"thumbnail too large",
			func(p *Post) { p.Thumbnail = &File{Bytes: 51 << 20, Format: "png"} },
			[]string{"thumbnail:too_large"},
		},
		{
			"every problem at once",
			func(p *Post) { p.Title = ""; p.MadeForKids = nil; p.Output.Format = "x" },
			[]string{"title:required", "made_for_kids:required", "output:unsupported_format"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := good()
			tc.edit(&p)

			got := paths(Validate(p, yt()))
			if !slices.Equal(got, tc.want) {
				t.Errorf("issues = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTagChars(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		tags []string
		want int
	}{
		{nil, 0},
		{[]string{"Foo-Baz"}, 7},
		{[]string{"Foo Baz"}, 9},
		{[]string{"a", "b"}, 3},
		{[]string{"a b", "c"}, 7},
	} {
		if got := TagChars(tc.tags); got != tc.want {
			t.Errorf("TagChars(%v) = %d, want %d", tc.tags, got, tc.want)
		}
	}
}

func TestStateMachine(t *testing.T) {
	t.Parallel()

	all := []State{StatePrepared, StateWaitingApproval, StateUploading, StateProcessing, StatePublished, StateFailed}
	allowed := map[[2]State]bool{
		{StatePrepared, StateWaitingApproval}:  true,
		{StateWaitingApproval, StateUploading}: true,
		{StateWaitingApproval, StateFailed}:    true,
		{StateUploading, StateProcessing}:      true,
		{StateUploading, StatePublished}:       true,
		{StateUploading, StateFailed}:          true,
		{StateProcessing, StatePublished}:      true,
		{StateProcessing, StateFailed}:         true,
	}

	for _, from := range all {
		for _, to := range all {
			if got := Can(from, to); got != allowed[[2]State{from, to}] {
				t.Errorf("Can(%s, %s) = %v", from, to, got)
			}
		}
	}

	if !StatePublished.Final() || !StateFailed.Final() || StateProcessing.Final() {
		t.Error("Final is wrong")
	}
}
