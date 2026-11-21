package project

import (
	"strings"
	"testing"
)

func validContext() Context {
	return Context{
		Name:        "Launch teaser",
		Description: strings.Repeat("d", DescriptionMin),
		Goal: Goal{
			Specific: strings.Repeat("s", GoalPartMin), Measurable: strings.Repeat("m", GoalPartMin),
			Achievable: strings.Repeat("a", GoalPartMin), Relevant: strings.Repeat("r", GoalPartMin),
			TimeBound: strings.Repeat("t", GoalPartMin), Due: "2026-12-31",
		},
	}
}

func paths(errs []*RuleError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Path
	}

	return out
}

func TestCheckContextValid(t *testing.T) {
	if errs := CheckContext(validContext(), CheckOptions{Today: "2026-11-20", CheckDue: true}); errs != nil {
		t.Fatalf("a valid context was refused: %v", paths(errs))
	}
}

func TestCheckContextBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(c *Context)
		field string // the path expected, "" when the context is valid
	}{
		{"name 3", func(c *Context) { c.Name = "abc" }, "name"},
		{"name 4", func(c *Context) { c.Name = "abcd" }, ""},
		{"name 32", func(c *Context) { c.Name = strings.Repeat("n", 32) }, ""},
		{"name 33", func(c *Context) { c.Name = strings.Repeat("n", 33) }, "name"},
		{"name accents count as characters", func(c *Context) { c.Name = "Vídeo ação" }, ""},
		{"name accents 32 runes", func(c *Context) { c.Name = strings.Repeat("é", 32) }, ""},
		{"name decomposed accent", func(c *Context) { c.Name = "Cafe\u0301 novo" }, ""},
		{"name two spaces", func(c *Context) { c.Name = "Launch  teaser" }, "name"},
		{"name leading space", func(c *Context) { c.Name = " Launch teaser" }, "name"},
		{"name trailing space", func(c *Context) { c.Name = "Launch teaser " }, "name"},
		{"name digits", func(c *Context) { c.Name = "Take 2 final" }, ""},
		{"name punctuation", func(c *Context) { c.Name = "Launch, teaser" }, "name"},
		{"name bidi", func(c *Context) { c.Name = "Launch\u202eteaser" }, "name"},
		{"name control", func(c *Context) { c.Name = "Launch\x07teaser" }, "name"},
		{"name tab", func(c *Context) { c.Name = "Launch\tteaser" }, "name"},
		{"name no-break space", func(c *Context) { c.Name = "Launch\u00a0teaser" }, "name"},
		{"description 63", func(c *Context) { c.Description = strings.Repeat("d", 63) }, "description"},
		{"description 64", func(c *Context) { c.Description = strings.Repeat("d", 64) }, ""},
		{"description 1024", func(c *Context) { c.Description = strings.Repeat("d", 1024) }, ""},
		{"description 1025", func(c *Context) { c.Description = strings.Repeat("d", 1025) }, "description"},
		{"description counts characters", func(c *Context) { c.Description = strings.Repeat("ç", 64) }, ""},
		{
			"description newline",
			func(c *Context) { c.Description = strings.Repeat("d", 40) + "\n" + strings.Repeat("d", 40) },
			"",
		},
		{"goal 15", func(c *Context) { c.Goal.Specific = strings.Repeat("s", 15) }, "goal.specific"},
		{"goal 16", func(c *Context) { c.Goal.Specific = strings.Repeat("s", 16) }, ""},
		{"goal 280", func(c *Context) { c.Goal.TimeBound = strings.Repeat("t", 280) }, ""},
		{"goal 281", func(c *Context) { c.Goal.TimeBound = strings.Repeat("t", 281) }, "goal.time_bound"},
		{"goal measurable", func(c *Context) { c.Goal.Measurable = "" }, "goal.measurable"},
		{"goal achievable", func(c *Context) { c.Goal.Achievable = "short" }, "goal.achievable"},
		{"goal relevant", func(c *Context) { c.Goal.Relevant = "short" }, "goal.relevant"},
		{"due past", func(c *Context) { c.Goal.Due = "2026-11-19" }, "goal.due"},
		{"due today", func(c *Context) { c.Goal.Due = "2026-11-20" }, ""},
		{"due not a date", func(c *Context) { c.Goal.Due = "2026-13-40" }, "goal.due"},
		{"due words", func(c *Context) { c.Goal.Due = "tomorrow" }, "goal.due"},
		{"due empty", func(c *Context) { c.Goal.Due = "" }, "goal.due"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validContext()
			tc.edit(&c)

			errs := CheckContext(c, CheckOptions{Today: "2026-11-20", CheckDue: true})
			if tc.field == "" {
				if errs != nil {
					t.Fatalf("refused: %v %s", paths(errs), errs[0].Message)
				}

				return
			}

			if len(errs) != 1 || errs[0].Path != tc.field {
				t.Fatalf("paths = %v, want [%s]", paths(errs), tc.field)
			}

			if errs[0].Code != "E_VALIDATION" || errs[0].Hint == "" || errs[0].Message == "" {
				t.Fatalf("incomplete error: %+v", errs[0])
			}
		})
	}
}

func TestCheckContextEveryFieldAtOnce(t *testing.T) {
	errs := CheckContext(Context{}, CheckOptions{Today: "2026-11-20", CheckDue: true})
	want := []string{
		"name",
		"description",
		"goal.specific",
		"goal.measurable",
		"goal.achievable",
		"goal.relevant",
		"goal.time_bound",
		"goal.due",
	}

	got := paths(errs)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

func TestCheckContextDueIsOptionalOnUpdate(t *testing.T) {
	c := validContext()
	c.Goal.Due = "2020-01-01"

	if errs := CheckContext(c, CheckOptions{Today: "2026-11-20"}); errs != nil {
		t.Fatalf("an unchanged past due date was refused: %v", paths(errs))
	}
}

func TestComplete(t *testing.T) {
	if !validContext().Complete() {
		t.Fatal("a full context is not complete")
	}

	if (Context{Name: "Launch teaser"}).Complete() {
		t.Fatal("a context with no description is complete")
	}

	c := validContext()
	c.Goal.Due = ""

	if c.Complete() {
		t.Fatal("a context with no due date is complete")
	}
}

func TestSchemaExport(t *testing.T) {
	m := Schema()
	props, _ := m["properties"].(map[string]any)

	name, _ := props["name"].(map[string]any)
	if name["minLength"] != NameMin || name["maxLength"] != NameMax || name["pattern"] == nil {
		t.Fatalf("name schema = %v", name)
	}

	goal, _ := props["goal"].(map[string]any)
	gp, _ := goal["properties"].(map[string]any)
	due, _ := gp["due"].(map[string]any)

	if due["format"] != "date" || due["x-not-before"] != "today" {
		t.Fatalf("due schema = %v", due)
	}

	spec, _ := gp["specific"].(map[string]any)
	if spec["minLength"] != GoalPartMin || spec["maxLength"] != GoalPartMax {
		t.Fatalf("specific schema = %v", spec)
	}
}
