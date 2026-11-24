package writing

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReadingMinutes(t *testing.T) {
	t.Parallel()

	for words, want := range map[int]int{
		-3: 0, 0: 0, 1: 1, 199: 1, 200: 1, 201: 2, 400: 2, 401: 3, 842: 5, 1000: 5,
	} {
		if got := ReadingMinutes(words); got != want {
			t.Errorf("ReadingMinutes(%d) = %d, want %d", words, got, want)
		}
	}
}

// A post of 842 words is listed with a reading time of 5 minutes (epic 78 criterion 5).
func TestSummarizeAPostOf842Words(t *testing.T) {
	t.Parallel()

	body := "---\ntitle: The long one\ntags: [a, b]\n---\n" + strings.Repeat("word ", 842)

	s := Summarize("post", body)
	if s.Words != 842 || s.ReadingMinutes != 5 {
		t.Fatalf("words, minutes = %d, %d; want 842, 5", s.Words, s.ReadingMinutes)
	}

	if strings.Contains(s.Excerpt, "title") || !strings.HasPrefix(s.Excerpt, "word word") {
		t.Fatalf("the excerpt carries the front matter: %q", s.Excerpt)
	}
}

func TestExcerpt(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 159) + "é" + strings.Repeat("b", 50)

	tests := []struct {
		name, kind, body, want string
	}{
		{"empty", "copy", "", ""},
		{"short", "copy", "  Hello there.\r\n\r\n", "Hello there."},
		{
			"a copy keeps a leading rule as text",
			"copy",
			"---\nnot front matter\n---\nx",
			"---\nnot front matter\n---\nx",
		},
		{"a post drops its front matter", "post", "---\ntitle: T\n---\n\nThe body.", "The body."},
		{"a post with no front matter", "post", "Just text", "Just text"},
		{"a post whose front matter never ends is the whole text", "post", "---\ntitle: T", "---\ntitle: T"},
		{"160 characters, not bytes", "copy", long, strings.Repeat("a", 159) + "é"},
		{"the cut trims the space it ends on", "copy", strings.Repeat("a", 159) + " tail", strings.Repeat("a", 159)},
	}

	for _, tc := range tests {
		if got := Excerpt(tc.kind, tc.body); got != tc.want {
			t.Errorf("%s: Excerpt = %q, want %q", tc.name, got, tc.want)
		}
	}

	if got := Excerpt("copy", strings.Repeat("é", 500)); utf8.RuneCountInString(got) != ExcerptRunes {
		t.Errorf("the excerpt has %d characters", utf8.RuneCountInString(got))
	}
}

func TestSummaryText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []Candidate
		want int
	}{
		{"none", nil, -1},
		{"a caption is never summarised", []Candidate{{"caption", "words here"}}, -1},
		{"a text with no words is skipped", []Candidate{{"copy", " - "}, {"script", "real words"}}, 1},
		{"a post beats a copy and a script", []Candidate{{"script", "a b"}, {"copy", "c d"}, {"post", "e f"}}, 2},
		{"a copy beats a script", []Candidate{{"script", "a b"}, {"copy", "c d"}}, 1},
		{"an empty kind is a copy", []Candidate{{"script", "a b"}, {"", "c d"}}, 1},
		{"the first of a kind wins", []Candidate{{"copy", "a b"}, {"copy", "c d"}}, 0},
		{"an unknown kind is not summarised", []Candidate{{"other", "a b"}}, -1},
	}

	for _, tc := range tests {
		got, ok := SummaryText(tc.in)
		if tc.want < 0 && ok || tc.want >= 0 && (!ok || got != tc.want) {
			t.Errorf("%s: %d, %v; want %d", tc.name, got, ok, tc.want)
		}
	}
}
