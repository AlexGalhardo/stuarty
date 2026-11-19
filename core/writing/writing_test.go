package writing

import (
	"slices"
	"strings"
	"testing"
)

func codes(r Report) []string {
	out := make([]string, len(r.Problems))
	for i := range r.Problems {
		out[i] = r.Problems[i].Code
	}

	return out
}

// Every length limit of the table, one below, at and one above it, in the unit of its network.
func TestCaptionLengthLimitsAtTheirBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		network string
		unit    Unit
		limit   int
		unitStr string // what one unit is made of
	}{
		{"youtube", UnitBytes, 5000, "a"},
		{"instagram", UnitCharacters, 2200, "a"},
		{"facebook", UnitCharacters, 63206, "a"},
		{"x", UnitWeighted, 280, "a"},
		{"tiktok", UnitUTF16, 2200, "a"},
	}

	for _, tc := range cases {
		t.Run(tc.network, func(t *testing.T) {
			t.Parallel()

			n, ok := Lookup(tc.network)
			if !ok || n.MaxLength != tc.limit || n.Unit != tc.unit {
				t.Fatalf("the table row of %s is %+v; want %d %s", tc.network, n, tc.limit, tc.unit)
			}

			for _, delta := range []int{-1, 0, 1} {
				r := ValidateCaption(strings.Repeat(tc.unitStr, tc.limit+delta), tc.network)
				over := slices.Contains(codes(r), ProblemTooLong)

				if over != (delta > 0) {
					t.Fatalf("%d units: too_long=%v, want %v (%v)", tc.limit+delta, over, delta > 0, r.Problems)
				}

				if r.Counts.Length != tc.limit+delta {
					t.Fatalf("length %d, want %d", r.Counts.Length, tc.limit+delta)
				}

				if delta > 0 {
					p := r.Problems[0]
					if p.Limit != tc.limit || p.Actual != tc.limit+delta || p.Unit != tc.unit {
						t.Fatalf("problem %+v does not say the limit, the actual and the unit", p)
					}

					if !strings.Contains(p.Message, "2") && !strings.Contains(p.Message, "5") &&
						!strings.Contains(p.Message, "6") {
						t.Fatalf("the sentence %q names no figure", p.Message)
					}
				}
			}
		})
	}
}

func TestCaptionUnitsDiffer(t *testing.T) {
	t.Parallel()

	// An emoji outside the BMP is 4 bytes, 1 character, 2 UTF-16 units and weighs 2 on X.
	const e = "\U0001F600"

	want := map[string]int{"youtube": 4, "instagram": 1, "facebook": 1, "tiktok": 2, "x": 2}
	for network, length := range want {
		if got := ValidateCaption(e, network).Counts.Length; got != length {
			t.Errorf("%s counts the emoji as %d, want %d", network, got, length)
		}
	}
}

func TestCaptionWeightedCountOfX(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want int
	}{
		{"latin", "hello", 5},
		{"a link weighs 23", "see https://example.com/a/very/long/path/that/goes/on?q=1 now", 4 + 23 + 4},
		{"two links", "https://a.example http://b.example", 23 + 1 + 23},
		{"cjk weighs 2", "你好", 4},
		{"the dot after a link is text", "go to https://example.com.", 6 + 23 + 1},
		{"nfc composes", "é", 1}, // e + combining acute is one character after NFC
		{"punctuation in the first range", "a—b", 3},
	}

	for _, tc := range cases {
		if got := ValidateCaption(tc.text, "x").Counts.Length; got != tc.want {
			t.Errorf("%s: %q weighs %d, want %d", tc.name, tc.text, got, tc.want)
		}
	}
}

func TestCaptionHashtagsAndMentionsAtTheirBoundary(t *testing.T) {
	t.Parallel()

	tags := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteString("#tag ")
		}

		return b.String()
	}
	mentions := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteString("@someone ")
		}

		return b.String()
	}

	for _, tc := range []struct {
		network string
		text    func(int) string
		limit   int
		code    string
	}{
		{"instagram", tags, 30, ProblemTooManyTags},
		{"youtube", tags, 60, ProblemTooManyTags},
		{"instagram", mentions, 20, ProblemTooManyMen},
	} {
		for _, delta := range []int{-1, 0, 1} {
			r := ValidateCaption(tc.text(tc.limit+delta), tc.network)
			if got := slices.Contains(codes(r), tc.code); got != (delta > 0) {
				t.Errorf("%s with %d: %s=%v, want %v", tc.network, tc.limit+delta, tc.code, got, delta > 0)
			}
		}
	}

	// A network that publishes no limit reports none, however many there are.
	for _, network := range []string{"x", "tiktok", "facebook"} {
		if r := ValidateCaption(tags(300)+mentions(300), network); slices.ContainsFunc(codes(r), func(c string) bool {
			return c == ProblemTooManyTags || c == ProblemTooManyMen
		}) {
			t.Errorf("%s reported a limit it does not publish: %v", network, r.Problems)
		}
	}
}

func TestCaptionCounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                            string
		text                            string
		hashtags, mentions, links, want int
	}{
		{"plain hashtags", "a #one and #two_2, #three!", 3, 0, 0, 0},
		{"a number alone is not a hashtag", "#2024 #a2024", 1, 0, 0, 0},
		{"inside a word is not one", "c#sharp a#b", 0, 0, 0, 0},
		{"html entity is not one", "&#39; &#x27;", 0, 0, 0, 0},
		{"unicode letters", "#café #日本語", 2, 0, 0, 0},
		{"mentions", "hi @ana, @bob.smith and @_x.", 0, 3, 0, 0},
		{"an email is not a mention", "mail me a@b.com", 0, 0, 0, 0},
		{"a link hides its fragment and user", "see https://example.com/#top and https://x.com/@ana", 0, 0, 2, 0},
		{"a link at the start of a word only", "xhttps://nope.example", 0, 0, 0, 0},
		{"a trailing dot is not the link", "https://example.com/a.", 0, 0, 1, 0},
		{"a mention name stops at 30", "@" + strings.Repeat("a", 40), 0, 1, 0, 0},
	}

	for _, tc := range cases {
		c := ValidateCaption(tc.text, "instagram").Counts
		if c.Hashtags != tc.hashtags || c.Mentions != tc.mentions || c.Links != tc.links {
			t.Errorf("%s: %q counted %+v, want hashtags %d mentions %d links %d", tc.name, tc.text, c,
				tc.hashtags, tc.mentions, tc.links)
		}
	}
}

func TestCaptionForbiddenEmptyAndUnknown(t *testing.T) {
	t.Parallel()

	r := ValidateCaption("a <b> c", "youtube")
	if !slices.Equal(codes(r), []string{ProblemForbidden}) || r.Problems[0].Actual != 2 {
		t.Fatalf("youtube refuses < and >: %+v", r.Problems)
	}

	if !strings.Contains(r.Problems[0].Message, "< >") {
		t.Fatalf("the sentence does not name the characters: %q", r.Problems[0].Message)
	}

	if r := ValidateCaption("a <b> c", "instagram"); !r.OK {
		t.Fatalf("instagram does not refuse them: %+v", r.Problems)
	}

	if r := ValidateCaption(
		" \n\t"+string(rune(0x200b))+" ",
		"instagram",
	); !slices.Equal(codes(r), []string{ProblemEmpty}) ||
		r.OK {
		t.Fatalf("a blank variant: %+v", r)
	}

	if r := ValidateCaption("hi", "myspace"); !slices.Equal(codes(r), []string{ProblemUnknownNet}) || r.OK {
		t.Fatalf("an unknown network: %+v", r)
	}

	if r := ValidateCaption("hi", ""); !r.OK || r.MaxLength != 0 || r.Counts.Length != 2 {
		t.Fatalf("the common variant is counted and has no limit: %+v", r)
	}
}

func TestEveryRowOfTheTableHasItsDateAndSource(t *testing.T) {
	t.Parallel()

	got := Networks()
	if len(got) != 5 {
		t.Fatalf("the table has %d rows", len(got))
	}

	for i := range got {
		n := &got[i]
		if len(n.Source) == 0 || len(n.Checked) != 10 || n.MaxLength <= 0 || n.Unit == "" {
			t.Errorf("%s: incomplete row %+v", n.ID, n)
		}

		for _, s := range n.Source {
			if !strings.HasPrefix(s, "https://") {
				t.Errorf("%s: source %q is not a page", n.ID, s)
			}
		}

		if _, ok := Lookup(n.ID); !ok || n.ID != strings.ToLower(n.ID) {
			t.Errorf("%s is not found by its id", n.ID)
		}
	}

	got[0].MaxLength = 1
	got[0].Source[0] = "x"

	if again, _ := Lookup(got[0].ID); again.MaxLength == 1 || again.Source[0] == "x" {
		t.Fatal("the table was changed through a copy")
	}
}

func TestParseCaption(t *testing.T) {
	t.Parallel()

	body := "Common words.\r\n\r\n## YouTube\r\nfor youtube\n\n## instagram\n#a\n\n## linkedin\nnot a network\n"
	got := ParseCaption(body)

	want := []Variant{
		{"", "Common words."}, {"youtube", "for youtube"}, {"instagram", "#a\n\n## linkedin\nnot a network"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	if got := ParseCaption("## x\nonly x"); len(got) != 1 || got[0].Network != "x" {
		t.Fatalf("a blank common variant is left out when a network exists: %+v", got)
	}

	if got := ParseCaption(""); len(got) != 1 || got[0].Network != "" || got[0].Text != "" {
		t.Fatalf("an empty body is one empty common variant: %+v", got)
	}

	if got := ParseCaption("no headings"); len(got) != 1 || got[0].Text != "no headings" {
		t.Fatalf("%+v", got)
	}
}

func TestParseScriptStructure(t *testing.T) {
	t.Parallel()

	body := "Intro idea for the editor.\n\n# Hook\nVO: Did you know bread rises?\nIt is alive.\n\nON SCREEN: BREAD\n\n" +
		"direction: slow zoom\n\n## Steps\n- vo: Mix flour and water.\n\ntext: 1. mix\n"
	s := ParseScript(body)

	if s.Plain || len(s.Scenes) != 3 {
		t.Fatalf("plain=%v scenes=%d: %+v", s.Plain, len(s.Scenes), s.Scenes)
	}

	if s.Scenes[0].Title != "" || !slices.Equal(s.Scenes[0].Notes, []string{"Intro idea for the editor."}) {
		t.Fatalf("scene 0: %+v", s.Scenes[0])
	}

	hook := s.Scenes[1]
	if hook.Title != "Hook" || !slices.Equal(hook.Narration, []string{"Did you know bread rises?\nIt is alive."}) ||
		!slices.Equal(hook.OnScreen, []string{"BREAD"}) || !slices.Equal(hook.Notes, []string{"direction: slow zoom"}) {
		t.Fatalf("scene 1: %+v", hook)
	}

	steps := s.Scenes[2]
	if !slices.Equal(steps.Narration, []string{"Mix flour and water."}) ||
		!slices.Equal(steps.OnScreen, []string{"1. mix"}) {
		t.Fatalf("scene 2: %+v", steps)
	}
}

func TestParseScriptPlainMode(t *testing.T) {
	t.Parallel()

	s := ParseScript("# One\nWords of the first scene here.\n\n#### not a scene heading\n\n## Two\nMore words.\n")
	if !s.Plain || len(s.Scenes) != 2 {
		t.Fatalf("%+v", s)
	}

	if len(s.Scenes[0].Narration) != 2 || s.Scenes[1].Narration[0] != "More words." || len(s.Scenes[0].Notes) != 0 {
		t.Fatalf("every paragraph is narration in plain mode: %+v", s.Scenes)
	}

	if s := ParseScript(""); len(s.Scenes) != 0 || !s.Plain {
		t.Fatalf("an empty script: %+v", s)
	}
}

func TestEstimateDurationChangesWithThePace(t *testing.T) {
	t.Parallel()

	words := strings.TrimSpace(strings.Repeat("word ", 150))
	s := ParseScript("# A\nVO: " + words + "\n\nON SCREEN: one two three\n\n# B\nVO: " + words[:len(words)/2] + "\n")

	// 150 words at 150 wpm is 60 s; 75 more words at 150 wpm is 30 s.
	e, err := EstimateDuration(s, 150)
	if err != nil || e.DurationMillis != 90000 || e.Words != 225 || e.OnScreenWords != 3 || e.WordsPerMinute != 150 {
		t.Fatalf("%+v, %v", e, err)
	}

	if e.Scenes[0].DurationMillis != 60000 || e.Scenes[1].DurationMillis != 30000 || e.Scenes[0].OnScreenWords != 3 {
		t.Fatalf("per scene: %+v", e.Scenes)
	}

	fast, _ := EstimateDuration(s, 300)
	slow, _ := EstimateDuration(s, 100)
	def, _ := EstimateDuration(s, 0)

	if fast.DurationMillis != 45000 || slow.DurationMillis != 135000 || def.DurationMillis != 90000 {
		t.Fatalf("fast %d slow %d default %d", fast.DurationMillis, slow.DurationMillis, def.DurationMillis)
	}

	// 7 words at 130 wpm is 3,230.77 ms: rounded to the nearest millisecond.
	odd, _ := EstimateDuration(ParseScript("VO: a b c d e f g"), 130)
	if odd.DurationMillis != 3231 {
		t.Fatalf("rounded to %d", odd.DurationMillis)
	}

	if none, _ := EstimateDuration(ParseScript("ON SCREEN: hi\n"), 150); none.DurationMillis != 0 {
		t.Fatalf("no narration takes no time: %+v", none)
	}
}

func TestWordsPerMinuteBounds(t *testing.T) {
	t.Parallel()

	for _, wpm := range []int{59, 301, -1} {
		if _, err := EstimateDuration(
			Script{},
			wpm,
		); err == nil || err.Code != "E_VALIDATION" ||
			err.Path != "words_per_minute" {
			t.Errorf("%d words per minute is accepted: %v", wpm, err)
		}
	}

	for _, wpm := range []int{0, 60, 300} {
		if _, err := EstimateDuration(Script{}, wpm); err != nil {
			t.Errorf("%d words per minute is refused: %v", wpm, err)
		}
	}
}

func TestCountWords(t *testing.T) {
	t.Parallel()

	for s, want := range map[string]int{"": 0, "  ": 0, "a b": 2, "-- ! ...": 0, "it's 3 o'clock — ok": 4, "a\nb\tc": 3} {
		if got := CountWords(s); got != want {
			t.Errorf("%q: %d words, want %d", s, got, want)
		}
	}
}

func TestRevisionRules(t *testing.T) {
	t.Parallel()

	three, four := 3, 4

	if err := CheckBase(3, nil); err != nil {
		t.Fatal(err)
	}

	if err := CheckBase(3, &three); err != nil {
		t.Fatal(err)
	}

	if err := CheckBase(3, &four); err == nil || err.Code != "E_REVISION_CONFLICT" || err.Path != "base_revision" {
		t.Fatalf("%v", err)
	}

	if NextRevision(3) != 4 || !Unchanged("a", "a") || Unchanged("a", "a ") {
		t.Fatal("next or unchanged")
	}

	// The 500th revision drops nothing; the 501st drops the first.
	if PruneBefore(500) != 1 || PruneBefore(501) != 2 || PruneBefore(1) != 0 {
		t.Fatalf("%d %d %d", PruneBefore(500), PruneBefore(501), PruneBefore(1))
	}

	if CheckRevision(0) == nil || CheckRevision(1) != nil {
		t.Fatal("revision numbers start at 1")
	}

	if _, err := CheckKind("poem"); err == nil || err.Path != "text_kind" {
		t.Fatal("an unknown kind is accepted")
	}

	if k, err := CheckKind(""); err != nil || k != KindCopy {
		t.Fatalf("%v %v", k, err)
	}

	for _, k := range Kinds() {
		if got, err := CheckKind(string(k)); err != nil || got != k {
			t.Errorf("%s: %v %v", k, got, err)
		}
	}

	for _, s := range []Source{SourceCreated, SourceWrite, SourceGenerated, SourceRevert} {
		if !s.Valid() {
			t.Errorf("%s", s)
		}
	}

	if Source("x").Valid() {
		t.Fatal("x")
	}
}

func TestDiffAtParagraphLevel(t *testing.T) {
	t.Parallel()

	a := "one\n\ntwo\n\nthree\n\nfour"
	b := "one\n\n2\n\nthree\r\n\r\n\r\nfour\n\nfive"

	got := Diff(a, b)
	want := []Hunk{
		{OpEqual, "one"}, {OpRemoved, "two"}, {OpAdded, "2"}, {OpEqual, "three"}, {OpEqual, "four"}, {OpAdded, "five"},
	}

	if got.Coarse || !slices.Equal(got.Hunks, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	if d := Diff("same", "same"); len(d.Hunks) != 1 || d.Hunks[0].Op != OpEqual {
		t.Fatalf("%+v", d)
	}

	if d := Diff("", "x"); len(d.Hunks) != 1 || d.Hunks[0] != (Hunk{OpAdded, "x"}) {
		t.Fatalf("%+v", d)
	}

	if d := Diff("", ""); len(d.Hunks) != 0 || d.Hunks == nil {
		t.Fatalf("%+v", d)
	}

	// A moved paragraph is a removal and an addition, never lost.
	d := Diff("a\n\nb", "b\n\na")
	if len(d.Hunks) != 3 {
		t.Fatalf("%+v", d)
	}
}

func TestDiffTooLargeIsCoarse(t *testing.T) {
	t.Parallel()

	big := func(prefix string) string {
		var b strings.Builder
		for i := range 2100 {
			b.WriteString(prefix)
			b.WriteString(strings.Repeat("x", i%7))
			b.WriteString("\n\n")
		}

		return b.String()
	}

	d := Diff(big("a"), big("b"))
	if !d.Coarse || len(d.Hunks) != 2 || d.Hunks[0].Op != OpRemoved || d.Hunks[1].Op != OpAdded {
		t.Fatalf("coarse=%v hunks=%d", d.Coarse, len(d.Hunks))
	}
}
