package project_test

import (
	"maps"
	"reflect"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
)

func answersOf(t *testing.T, k project.ProductionKind) map[string]string {
	t.Helper()

	qs, v := project.BriefingQuestionsFor(k)
	if v != nil {
		t.Fatal(v)
	}

	out := map[string]string{}

	for _, q := range qs {
		if !q.Required {
			continue
		}

		switch q.Kind {
		case project.AnswerChoice:
			out[q.ID] = q.Options[0]
		case project.AnswerSeconds:
			out[q.ID] = strconv.Itoa(q.Min)
		case project.AnswerDate:
			out[q.ID] = "2026-12-01"
		default:
			out[q.ID] = "an answer for " + q.ID
		}
	}

	return out
}

func TestBriefingKindsListAndShortIsTheFirstFlow(t *testing.T) {
	t.Parallel()

	want := []project.ProductionKind{"short", "commercial", "long"}
	if got := project.ProductionKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds are %v", got)
	}

	short, v := project.BriefingQuestionsFor(project.ProductionShort)
	if v != nil || !reflect.DeepEqual(short, project.BriefingQuestions()) {
		t.Fatalf("the short set is not the briefing of the first flow: %v", v)
	}

	if project.ProductionKind("podcast").Known() || !project.ProductionLong.Known() {
		t.Fatal("Known is wrong")
	}

	if _, v := project.BriefingQuestionsFor("podcast"); v == nil || v.Code != "E_VALIDATION" || v.Path != "kind" {
		t.Fatalf("an unknown kind is %+v", v)
	}
}

func TestBriefingKindsEachSetIsWellFormed(t *testing.T) {
	t.Parallel()

	for _, k := range project.ProductionKinds() {
		qs, v := project.BriefingQuestionsFor(k)
		if v != nil {
			t.Fatal(v)
		}

		ids, headings, required := map[string]bool{}, map[string]bool{}, 0

		for _, q := range qs {
			if q.ID == "" || q.Heading == "" || q.Text == "" || ids[q.ID] || headings[q.Heading] {
				t.Errorf("%s: question %+v is empty or repeated", k, q)
			}

			ids[q.ID], headings[q.Heading] = true, true

			if q.Required {
				required++
			}

			switch q.Kind {
			case project.AnswerChoice:
				if len(q.Options) < 2 {
					t.Errorf("%s: %s is a choice with no options", k, q.ID)
				}
			case project.AnswerSeconds:
				if q.Min < 1 || q.Max <= q.Min {
					t.Errorf("%s: %s has the bounds %d to %d", k, q.ID, q.Min, q.Max)
				}
			case project.AnswerText:
				if q.MaxChars <= 0 {
					t.Errorf("%s: %s is a text with no limit", k, q.ID)
				}
			case project.AnswerDate:
			}
		}

		if required < 4 {
			t.Errorf("%s asks for only %d required answers", k, required)
		}
	}
}

func TestBriefingKindsTheSetsDiffer(t *testing.T) {
	t.Parallel()

	c, _ := project.BriefingQuestionsFor(project.ProductionCommercial)
	l, _ := project.BriefingQuestionsFor(project.ProductionLong)

	has := func(qs []project.Question, id string) bool {
		for _, q := range qs {
			if q.ID == id {
				return true
			}
		}

		return false
	}

	for _, id := range []string{"product", "problem", "offer", "call_to_action", "product_shots"} {
		if !has(c, id) || has(l, id) {
			t.Errorf("%s belongs to the commercial only", id)
		}
	}

	for _, id := range []string{"topic", "speakers", "cut_style", "chapters", "short_from_it"} {
		if !has(l, id) || has(c, id) {
			t.Errorf("%s belongs to the long video only", id)
		}
	}
}

func TestBriefingKinds(t *testing.T) {
	t.Parallel()

	for _, k := range project.ProductionKinds() {
		t.Run(string(k), func(t *testing.T) {
			t.Parallel()

			answers := answersOf(t, k)

			checked, v := project.CheckAnswersFor(k, answers)
			if v != nil {
				t.Fatal(v)
			}

			md := project.BriefingMarkdownFor(k, checked)

			back, ok := project.ParseBriefingFor(k, md)
			if !ok || !reflect.DeepEqual(back, checked) {
				t.Fatalf("the brief does not read back as it was written:\n%s", md)
			}

			// A required answer missing, and an id of another kind, are refused.
			qs, _ := project.BriefingQuestionsFor(k)

			for _, q := range qs {
				if !q.Required {
					continue
				}

				partial := map[string]string{}
				for id, a := range answers {
					if id != q.ID {
						partial[id] = a
					}
				}

				if _, v := project.CheckAnswersFor(k, partial); v == nil || v.Path != "answers."+q.ID {
					t.Errorf("%s: no answer to %s was accepted: %+v", k, q.ID, v)
				}
			}

			stray := map[string]string{"not_a_question": "x"}
			maps.Copy(stray, answers)

			if _, v := project.CheckAnswersFor(k, stray); v == nil || v.Path != "answers.not_a_question" {
				t.Errorf("%s: an id that is no question was accepted: %+v", k, v)
			}
		})
	}
}

func TestBriefingKindsABriefBelongsToItsKind(t *testing.T) {
	t.Parallel()

	checked, v := project.CheckAnswersFor(project.ProductionCommercial, answersOf(t, project.ProductionCommercial))
	if v != nil {
		t.Fatal(v)
	}

	md := project.BriefingMarkdownFor(project.ProductionCommercial, checked)

	if _, ok := project.ParseBriefing(md); ok {
		t.Error("a commercial brief was read as the brief of a short")
	}

	if _, ok := project.ParseBriefingFor(project.ProductionLong, md); ok {
		t.Error("a commercial brief was read as the brief of a long video")
	}

	if _, ok := project.ParseBriefingFor("podcast", md); ok {
		t.Error("a brief was read for an unknown kind")
	}

	if _, v := project.CheckAnswersFor(project.ProductionShort, answersOf(t, project.ProductionCommercial)); v == nil {
		t.Error("the answers of a commercial were taken for those of a short")
	}
}

func TestBriefingKindsDurationBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind    project.ProductionKind
		seconds string
		ok      bool
	}{
		{project.ProductionShort, "180", true},
		{project.ProductionShort, "181", false},
		{project.ProductionCommercial, "15", true},
		{project.ProductionCommercial, "30", true},
		{project.ProductionCommercial, "4", false},
		{project.ProductionCommercial, "91", false},
		{project.ProductionLong, "1800", true},
		{project.ProductionLong, "119", false},
		{project.ProductionLong, "14401", false},
		{project.ProductionLong, "20m", false},
	}

	for _, c := range cases {
		a := answersOf(t, c.kind)
		a["duration"] = c.seconds

		if _, v := project.CheckAnswersFor(c.kind, a); (v == nil) != c.ok {
			t.Errorf("%s %ss: ok=%v, got %+v", c.kind, c.seconds, c.ok, v)
		}
	}
}

func TestBriefingKindsAnUnknownKindWritesNothing(t *testing.T) {
	t.Parallel()

	if project.BriefingMarkdownFor("podcast", map[string]string{"audience": "x"}) != project.BriefTitle+"\n" {
		t.Error("an unknown kind wrote answers")
	}

	if _, v := project.CheckAnswersFor("podcast", nil); v == nil {
		t.Error("an unknown kind was accepted")
	}
}
