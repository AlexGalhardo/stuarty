package project

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem/z"
)

// The context of a project (SDD-005 section 5.12.2, epic 56): a name, a description and a SMART goal that every
// agent reads before it does anything else, the way an engineer reads the AGENTS.md of a repository. The rules are
// written with the validation package z; its JSON Schema export is the schema the web form is built from.
const (
	NameMin        = 4
	NameMax        = 32
	DescriptionMin = 64
	DescriptionMax = 1024
	GoalPartMin    = 16
	GoalPartMax    = 280

	// DateLayout is the form of the due date of a goal.
	DateLayout = "2006-01-02"

	// NamePattern is a name: Unicode letters, marks, digits and single spaces, with no space at either end. It
	// also refuses the control and direction characters of VV-049, which are none of those classes.
	NamePattern = `^[\p{L}\p{M}\p{N}]+( [\p{L}\p{M}\p{N}]+)*$`
	// DatePattern is the shape of a due date; a real calendar date is checked besides.
	DatePattern = `^\d{4}-\d{2}-\d{2}$`
)

// Goal is the SMART goal of a project: one text for each letter, and the date it is due.
type Goal struct {
	Specific   string `json:"specific"`
	Measurable string `json:"measurable"`
	Achievable string `json:"achievable"`
	Relevant   string `json:"relevant"`
	TimeBound  string `json:"time_bound"`
	Due        string `json:"due"` // YYYY-MM-DD
}

// Context is what a project says about itself.
type Context struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Goal        Goal   `json:"goal"`
}

// Complete reports whether the description and every part of the goal are set. A project made before epic 56
// has none of them, and the surfaces say so instead of inventing a value.
func (c Context) Complete() bool {
	g := c.Goal

	return c.Description != "" && g.Specific != "" && g.Measurable != "" && g.Achievable != "" &&
		g.Relevant != "" && g.TimeBound != "" && g.Due != ""
}

func goalPart(what string) z.StringSchema {
	return z.String().Min(GoalPartMin).Max(GoalPartMax).Describe(what)
}

var contextSchema = z.Struct[Context](z.Shape{
	"name": z.String().Min(NameMin).Max(NameMax).Regex(regexp.MustCompile(NamePattern)).
		Message("string.regex", "The name field must have only letters, digits and single spaces, with no space at either end.").
		Describe("The name of the project."),
	"description": z.String().Min(DescriptionMin).Max(DescriptionMax).
		Describe("What the project is, for whom, and what it makes."),
	"goal": z.Struct[Goal](z.Shape{
		"specific":   goalPart("What exactly the project will achieve."),
		"measurable": goalPart("How progress and success are counted."),
		"achievable": goalPart("Why it can be done with what the creator has."),
		"relevant":   goalPart("Why it matters to the creator or the audience."),
		"time_bound": goalPart("The schedule and the milestones."),
		"due": z.String().Regex(regexp.MustCompile(DatePattern)).
			Message("string.regex", "The due field must be a date written as YYYY-MM-DD.").
			Refine(func(v string) error {
				if _, err := time.Parse(DateLayout, v); err != nil {
					return z.FieldError("", "The due field must be a real date.")
				}

				return nil
			}).
			Describe("The date the goal is due, YYYY-MM-DD."),
	}),
})

// Schema is the JSON Schema of a Context, exported by z. Two members say what the schema language cannot: the due
// date is a date, and it is not before the day the project is made or changed.
func Schema() map[string]any {
	m := z.JSONSchema(contextSchema)

	if props, ok := m["properties"].(map[string]any); ok {
		if goal, ok := props["goal"].(map[string]any); ok {
			if gp, ok := goal["properties"].(map[string]any); ok {
				if due, ok := gp["due"].(map[string]any); ok {
					due["format"] = "date"
					due["x-not-before"] = "today"
				}
			}
		}
	}

	return m
}

// hints are the advice of each field, for the error that names it (SDD-000 section 8: an error says what to do).
var hints = map[string]string{
	"name":            "send a name of 4 to 32 letters, digits and single spaces, such as Launch teaser",
	"description":     "send 64 to 1024 characters that say what the project is, for whom, and what it makes",
	"goal":            "send goal with specific, measurable, achievable, relevant, time_bound and due",
	"goal.specific":   "say in 16 to 280 characters what exactly the project will achieve",
	"goal.measurable": "say in 16 to 280 characters how success is counted",
	"goal.achievable": "say in 16 to 280 characters why it can be done with what you have",
	"goal.relevant":   "say in 16 to 280 characters why it matters",
	"goal.time_bound": "say in 16 to 280 characters the schedule and the milestones",
	"goal.due":        "send a date written as YYYY-MM-DD, today or later",
}

// CheckOptions says how a Context is checked.
type CheckOptions struct {
	// Today is the day of the service clock, YYYY-MM-DD.
	Today string
	// CheckDue refuses a due date before Today. It is set when the project is created and when the date changes.
	CheckDue bool
}

var fieldNames = []string{
	"name", "description", "goal", "goal.specific", "goal.measurable", "goal.achievable", "goal.relevant",
	"goal.time_bound", "goal.due",
}

func fieldOrder(path string) int {
	if i := slices.Index(fieldNames, path); i >= 0 {
		return i
	}

	return len(fieldNames)
}

// CheckContext applies every rule and returns every broken one, one per field, in the order of the fields. It
// returns nil when the context is valid.
func CheckContext(c Context, o CheckOptions) []*RuleError {
	var out []*RuleError

	if err := contextSchema.Check(c); err != nil {
		if is, ok := z.AsIssues(err); ok {
			for _, i := range is {
				// A short name breaks the length and the pattern: the first of them says it.
				if slices.ContainsFunc(out, func(e *RuleError) bool { return e.Path == i.Path }) {
					continue
				}

				out = append(out, &RuleError{
					Code:    "E_VALIDATION",
					Path:    i.Path,
					Message: strings.TrimSuffix(i.Message, "."),
					Hint:    hints[i.Path],
				})
			}
		}
	}

	if o.CheckDue && o.Today != "" && c.Goal.Due < o.Today &&
		!slices.ContainsFunc(out, func(e *RuleError) bool { return e.Path == "goal.due" }) {
		out = append(out, &RuleError{
			Code: "E_VALIDATION", Path: "goal.due", Hint: hints["goal.due"],
			Message: fmt.Sprintf("The due field is %s, before today (%s)", c.Goal.Due, o.Today),
		})
	}

	slices.SortStableFunc(out, func(a, b *RuleError) int { return fieldOrder(a.Path) - fieldOrder(b.Path) })

	return out
}
