package project

import (
	"slices"
	"testing"
)

func stepOf(t *testing.T, w *Workflow, id StepID) Step {
	t.Helper()

	st, ok := w.Step(id)
	if !ok {
		t.Fatalf("the workflow has no step %q", id)
	}

	return st
}

func TestWorkflowNamesTheSkillOfAStepOnlyWhenTheLibraryOffersIt(t *testing.T) {
	t.Parallel()

	items := []Item{item(Text, TextKindPost, "words")}
	facts := &Facts{Items: items, PublicationsKnown: true, Skills: []string{"write-a-hook"}}

	w := ComputeWorkflow(KindText, facts)
	if got := stepOf(t, &w, StepWriting).Skill; got != "write-a-hook" {
		t.Errorf("writing skill = %q", got)
	}

	if got := stepOf(t, &w, StepBriefing).Skill; got != "" {
		t.Errorf("a skill the library does not offer is not named: %q", got)
	}

	if got := stepOf(t, &w, StepReview).Skill; got != "" {
		t.Errorf("a step with no skill in the table has none: %q", got)
	}

	none := ComputeWorkflow(KindText, &Facts{Items: items, PublicationsKnown: true})
	for _, st := range none.Steps {
		if st.Skill != "" {
			t.Errorf("no skills offered, yet %q names %q", st.ID, st.Skill)
		}
	}
}

func TestWorkflowSpentFollowsTheJobKindToItsStep(t *testing.T) {
	t.Parallel()

	spent := []Spend{
		{JobKind: "text.generate", Currency: "USD", Micros: 3000},
		{JobKind: "text.generate", Currency: "USD", Micros: 2000},
		{JobKind: "text.generate", Currency: "EUR", Micros: 1000},
		{JobKind: "image.generate", Currency: "USD", Micros: 70000},
		{JobKind: "publish.upload", Currency: "USD", Micros: 500},
		{JobKind: "unknown.kind", Currency: "USD", Micros: 9},
		{JobKind: "text.generate", Currency: "USD", Micros: 0},
	}

	text := ComputeWorkflow(KindText, &Facts{
		Items: []Item{item(Text, TextKindPost, "w")}, PublicationsKnown: true, Spent: spent,
	})

	want := []Cost{{Micros: 1000, Currency: "EUR"}, {Micros: 5000, Currency: "USD"}}
	if got := stepOf(t, &text, StepWriting).Spent; !slices.Equal(got, want) {
		t.Errorf("writing spent = %v, want %v", got, want)
	}

	if got := stepOf(t, &text, StepPublication).Spent; !slices.Equal(got, []Cost{{Micros: 500, Currency: "USD"}}) {
		t.Errorf("publication spent = %v", got)
	}

	// A text project has no editing step: the picture cost belongs to no step rather than to a wrong one.
	for _, st := range text.Steps {
		if st.ID != StepWriting && st.ID != StepPublication && len(st.Spent) != 0 {
			t.Errorf("%q spent %v", st.ID, st.Spent)
		}
	}

	video := ComputeWorkflow(KindVideo, &Facts{
		Items: []Item{item(Timeline, "", "")}, PublicationsKnown: true, TimelineItems: 1, Spent: spent,
	})

	if got := stepOf(t, &video, StepEditing).Spent; !slices.Equal(got, []Cost{{Micros: 70000, Currency: "USD"}}) {
		t.Errorf("editing spent = %v", got)
	}

	if got := stepOf(t, &video, StepScript).Spent; !slices.Equal(got, want) {
		t.Errorf("script spent = %v, want the generated text %v", got, want)
	}
}
