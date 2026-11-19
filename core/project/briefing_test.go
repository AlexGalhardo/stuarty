package project_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
)

func validAnswers() map[string]string {
	return map[string]string{
		"audience": "indie developers", "goal": "sign up for the newsletter",
		"platform": "youtube-shorts", "format": "vertical-9x16", "duration": "30",
	}
}

func TestBriefingQuestionsAreDataAndACopy(t *testing.T) {
	t.Parallel()

	qs := project.BriefingQuestions()
	if len(qs) != 10 {
		t.Fatalf("%d questions", len(qs))
	}

	want := []string{
		"audience",
		"goal",
		"platform",
		"format",
		"duration",
		"tone",
		"references",
		"must_include",
		"must_avoid",
		"deadline",
	}
	for i, id := range want {
		if qs[i].ID != id || qs[i].Heading == "" || qs[i].Text == "" {
			t.Fatalf("question %d is %+v", i, qs[i])
		}
	}

	qs[2].Options[0] = "changed"

	if project.BriefingQuestions()[2].Options[0] != "youtube-shorts" {
		t.Fatal("the result shares the options with the table")
	}
}

func TestBriefingCheckAnswers(t *testing.T) {
	t.Parallel()

	with := func(k, v string) map[string]string {
		m := validAnswers()
		m[k] = v

		return m
	}

	without := func(k string) map[string]string {
		m := validAnswers()
		delete(m, k)

		return m
	}

	cases := map[string]struct {
		in   map[string]string
		path string
	}{
		"missing required":   {without("goal"), "answers.goal"},
		"blank required":     {with("audience", "  \n "), "answers.audience"},
		"unknown id":         {with("colour", "red"), "answers.colour"},
		"bad choice":         {with("platform", "myspace"), "answers.platform"},
		"seconds not number": {with("duration", "half a minute"), "answers.duration"},
		"seconds zero":       {with("duration", "0"), "answers.duration"},
		"seconds too long":   {with("duration", "181"), "answers.duration"},
		"seconds padded":     {with("duration", "030"), "answers.duration"},
		"bad date":           {with("deadline", "next friday"), "answers.deadline"},
		"impossible date":    {with("deadline", "2026-02-30"), "answers.deadline"},
		"too long":           {with("tone", strings.Repeat("a", 201)), "answers.tone"},
		"control":            {with("tone", "a\x00b"), "answers.tone"},
		"bidi":               {with("tone", "a"+string(rune(0x202e))+"b"), "answers.tone"},
	}

	for name, c := range cases {
		_, v := project.CheckAnswers(c.in)
		if v == nil || v.Code != "E_VALIDATION" || v.Path != c.path {
			t.Errorf("%s: %+v", name, v)
		}
	}

	got, v := project.CheckAnswers(with("deadline", " 2026-12-01 "))
	if v != nil || got["deadline"] != "2026-12-01" || len(got) != 6 {
		t.Fatalf("%v %v", got, v)
	}

	if got, v = project.CheckAnswers(with("tone", "")); v != nil || len(got) != 5 {
		t.Fatalf("an empty optional answer is left out: %v %v", got, v)
	}
}

func TestBriefingMarkdownRoundTrips(t *testing.T) {
	t.Parallel()

	in := validAnswers()
	in["references"] = "# a heading\n## Goal\n\\# kept\n\nsecond paragraph"
	in["deadline"] = "2026-12-01"

	checked, v := project.CheckAnswers(in)
	if v != nil {
		t.Fatal(v)
	}

	md := project.BriefingMarkdown(checked)
	if !strings.HasPrefix(md, "# Brief\n\n## Audience\n\nindie developers\n") {
		t.Fatalf("%q", md)
	}

	if strings.Count(md, "\n## Goal\n") != 1 {
		t.Fatalf("an answer made a heading: %q", md)
	}

	got, ok := project.ParseBriefing(md)
	if !ok || !maps.Equal(got, checked) {
		t.Fatalf("%v %v\n%q", ok, got, md)
	}

	// Writing what was read is the identity.
	again, _ := project.CheckAnswers(got)
	if project.BriefingMarkdown(again) != md {
		t.Fatal("the brief is not stable")
	}
}

func TestBriefingParseRefusesWhatItDidNotWrite(t *testing.T) {
	t.Parallel()

	for name, text := range map[string]string{
		"empty":           "",
		"free text":       "Make a funny video about cats.",
		"no title":        "## Audience\n\nme\n",
		"unknown heading": "# Brief\n\n## Mood\n\nhappy\n",
		"text first":      "# Brief\n\nhello\n\n## Audience\n\nme\n",
		"section twice":   "# Brief\n\n## Audience\n\nme\n\n## Audience\n\nyou\n",
	} {
		if _, ok := project.ParseBriefing(text); ok {
			t.Errorf("%s was taken for a briefing", name)
		}
	}

	// The headings of the briefing written by hand are not enough: the answers must be ones the briefing takes,
	// and the text the one it writes.
	full := project.BriefingMarkdown(validAnswers())
	for name, text := range map[string]string{
		"a required answer missing": "# Brief\n\n## Goal\n\nsell\n",
		"an answer too long":        strings.Replace(full, "indie developers", strings.Repeat("a", 401), 1),
		"a bad choice":              strings.Replace(full, "youtube-shorts", "myspace", 1),
		"a heading indented":        strings.Replace(full, "## Goal", "##  Goal", 1),
		"text before the title":     "notes\n" + full,
	} {
		if _, ok := project.ParseBriefing(text); ok {
			t.Errorf("%s was taken for a briefing", name)
		}
	}

	got, ok := project.ParseBriefing(strings.ReplaceAll(full, "\n", "\r\n"))
	if !ok || got["goal"] != "sign up for the newsletter" {
		t.Fatalf("%v %v", ok, got)
	}
}

func TestBriefingKeepsABriefWrittenByHand(t *testing.T) {
	t.Parallel()

	guided := project.BriefingMarkdown(validAnswers())
	byHand := "# Brief\n\n## Goal\n\n" + strings.Repeat("A long plan written by hand. ", 30) + "\n"

	for name, c := range map[string]struct {
		current string
		replace bool
		refused bool
	}{
		"empty":               {"", false, false},
		"blank":               {" \n", false, false},
		"guided":              {guided, false, false},
		"by hand":             {"My own plan.", false, true},
		"by hand, headings":   {byHand, false, true},
		"by hand, replace":    {"My own plan.", true, false},
		"guided, then edited": {guided + "\nOne more line.\n", false, true},
	} {
		v := project.CheckBriefingReplace(c.current, c.replace)
		if (v != nil) != c.refused || (v != nil && (v.Code != "E_INVALID_STATE" || v.Path != "replace")) {
			t.Errorf("%s: %+v", name, v)
		}
	}
}
