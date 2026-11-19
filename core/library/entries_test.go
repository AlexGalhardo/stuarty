package library_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/library"
)

func TestValidName(t *testing.T) {
	t.Parallel()

	good := []string{"a", "weekly-recap", "a1-b2-c3", strings.Repeat("a", 64), "com10", "console"}
	bad := []string{
		"", "A", "a_b", "a b", "-a", "a-", "a--b", "a.b", "a/b", "..", ".", `a\b`, "c:", strings.Repeat("a", 65),
		"con", "nul", "com1", "lpt9", "é", "a\x00",
	}

	for _, n := range good {
		if err := library.ValidName(n); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}

	for _, n := range bad {
		if err := library.ValidName(n); err == nil {
			t.Errorf("%q: want an error", n)
		}
	}
}

func TestSlug(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"Weekly Recap!":           "weekly-recap",
		"  --Hello,   World-- ":   "hello-world",
		"CON":                     "con-note",
		"???":                     "",
		strings.Repeat("ab ", 40): strings.TrimRight(strings.Repeat("ab-", 40)[:64], "-"),
	} {
		if got := library.Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}

		if want != "" {
			if err := library.ValidName(want); err != nil {
				t.Errorf("Slug(%q) = %q is not a valid name: %v", in, want, err)
			}
		}
	}
}

func TestValidScopeAndTags(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"workspace", "prj_0123456789"} {
		if err := library.ValidScope(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}

	for _, s := range []string{"", "global", "prj_", "prj_ABCDEFGHIJ", "prj_0123456789x", "../x"} {
		if err := library.ValidScope(s); err == nil {
			t.Errorf("%q: want an error", s)
		}
	}

	if err := library.ValidTags([]string{"a", "b"}); err != nil {
		t.Error(err)
	}

	if library.ValidTags([]string{"a", "a"}) == nil || library.ValidTags([]string{"A"}) == nil ||
		library.ValidTags(strings.Split("a b c d e f g h i j k", " ")) == nil {
		t.Error("a repeated, an invalid and too many tags must fail")
	}
}

func TestMemoryRoundTrip(t *testing.T) {
	t.Parallel()

	m := library.Memory{
		Name: "hook-first", Description: "Open with the result: it keeps people", Scope: "workspace",
		Tags: []string{"shorts", "hooks"}, Actor: "local:cli", Kept: false,
		Created: "2026-11-19T10:00:00Z", Updated: "2026-11-19T10:00:00Z", Body: "The first three seconds matter.\n",
	}

	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	text, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	back, err := library.ParseMemory(text)
	if err != nil {
		t.Fatal(err)
	}

	if err := back.Validate(); err != nil {
		t.Fatal(err)
	}

	if back.Name != m.Name || back.Description != m.Description || back.Scope != m.Scope || back.Actor != m.Actor ||
		back.Kept != m.Kept || back.Body != m.Body || !reflect.DeepEqual(back.Tags, m.Tags) {
		t.Fatalf("got %+v", back)
	}

	again, err := back.Marshal()
	if err != nil || again != text {
		t.Fatalf("a rewrite differs:\n%s\n%s", text, again)
	}
}

func TestMemoryKeepsUnknownKeys(t *testing.T) {
	t.Parallel()

	m, err := library.ParseMemory("---\nname: a\ndescription: d\nmood: calm\n---\nx\n")
	if err != nil {
		t.Fatal(err)
	}

	m.Kept = true

	text, err := m.Marshal()
	if err != nil || !strings.Contains(text, "mood: calm") || !strings.Contains(text, "kept: true") {
		t.Fatalf("%v\n%s", err, text)
	}
}

func TestMemoryValidate(t *testing.T) {
	t.Parallel()

	ok := library.Memory{Name: "a", Description: "d", Scope: "workspace", Body: "x"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(m *library.Memory){
		"name":          func(m *library.Memory) { m.Name = "A b" },
		"description":   func(m *library.Memory) { m.Description = "" },
		"description2":  func(m *library.Memory) { m.Description = "a\nb" },
		"description3":  func(m *library.Memory) { m.Description = strings.Repeat("a", 201) },
		"scope":         func(m *library.Memory) { m.Scope = "everywhere" },
		"tags":          func(m *library.Memory) { m.Tags = []string{"Bad Tag"} },
		"body":          func(m *library.Memory) { m.Body = strings.Repeat("a", library.MaxMemoryBody+1) },
		"time":          func(m *library.Memory) { m.Created = "yesterday" },
		"control chars": func(m *library.Memory) { m.Description = "a\x01" },
	}

	for name, mutate := range cases {
		m := ok
		mutate(&m)

		var fe *library.FieldError
		if err := m.Validate(); !errors.As(err, &fe) {
			t.Errorf("%s: want a FieldError, got %v", name, err)
		}
	}
}

func TestPromptVariablesAndRender(t *testing.T) {
	t.Parallel()

	p := library.Prompt{
		Name: "intro", Description: "d", Defaults: map[string]string{"tone": "warm"},
		Body: "Write a {{ tone }} intro about {{topic}} for {{topic}} fans. {{ not valid }} {{Bad}}\n",
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}

	if got, want := library.Variables(p.Body), []string{"tone", "topic"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("variables %v", got)
	}

	out, err := library.Render(&p, map[string]string{"topic": "cats"})
	if err != nil {
		t.Fatal(err)
	}

	if want := "Write a warm intro about cats for cats fans. {{ not valid }} {{Bad}}\n"; out != want {
		t.Fatalf("got %q", out)
	}
}

func TestPromptRenderNeverRescansAValue(t *testing.T) {
	t.Parallel()

	p := library.Prompt{Name: "p", Description: "d", Body: "{{a}} and {{b}}"}

	out, err := library.Render(&p, map[string]string{"a": "{{b}}", "b": "B"})
	if err != nil || out != "{{b}} and B" {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestPromptRenderErrors(t *testing.T) {
	t.Parallel()

	p := library.Prompt{Name: "p", Description: "d", Body: "{{a}} {{b}}"}

	_, err := library.Render(&p, map[string]string{"c": "x", "a": strings.Repeat("x", library.MaxValueLength+1)})

	var re *library.RenderError
	if !errors.As(err, &re) {
		t.Fatalf("got %v", err)
	}

	if !reflect.DeepEqual(re.Missing, []string{"b"}) || !reflect.DeepEqual(re.Unknown, []string{"c"}) ||
		!reflect.DeepEqual(re.TooLong, []string{"a"}) {
		t.Fatalf("%+v", re)
	}
}

func TestPromptValidate(t *testing.T) {
	t.Parallel()

	cases := map[string]library.Prompt{
		"name":            {Name: "X", Description: "d", Body: "x"},
		"no body":         {Name: "a", Description: "d", Body: " \n"},
		"no description":  {Name: "a", Body: "x"},
		"default unknown": {Name: "a", Description: "d", Body: "x", Defaults: map[string]string{"q": "1"}},
		"default name":    {Name: "a", Description: "d", Body: "{{q}}", Defaults: map[string]string{"Q": "1"}},
		"body size":       {Name: "a", Description: "d", Body: strings.Repeat("x", library.MaxPromptBody+1)},
	}

	for name, p := range cases {
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestPromptRoundTrip(t *testing.T) {
	t.Parallel()

	p := library.Prompt{
		Name:        "intro",
		Description: "An intro",
		Tags:        []string{"intro"},
		Defaults:    map[string]string{"tone": "warm: very"},
		Actor:       "local:ui",
		Created:     "2026-11-19T10:00:00Z",
		Updated:     "2026-11-19T10:00:00Z",
		Body:        "Hi {{tone}}\n",
	}

	text, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	back, err := library.ParsePrompt(text)
	if err != nil || back.Defaults["tone"] != "warm: very" || back.Body != p.Body || back.Name != "intro" {
		t.Fatalf("%v %+v\n%s", err, back, text)
	}
}
