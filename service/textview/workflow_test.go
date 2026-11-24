package textview

import "testing"

func TestWorkflowLine(t *testing.T) {
	t.Parallel()

	steps := func(states ...string) []StepLine {
		ids := []string{"briefing", "writing", "review", "approval"}
		out := make([]StepLine, len(states))

		for i, s := range states {
			out[i] = StepLine{ID: ids[i], State: s}
		}

		return out
	}

	tests := []struct {
		name          string
		kind          string
		steps         []StepLine
		current, next string
		want          string
	}{
		{
			"a new project", "text", steps("current", "upcoming", "upcoming", "upcoming"), "briefing", "writing",
			"workflow (text): 0 of 4 steps done; now briefing, then writing",
		},
		{
			"a person is asked", "video", steps("done", "done", "done", "waiting"), "approval", "",
			"workflow (video): 3 of 4 steps done; now approval (waits for a person)",
		},
		{
			"skipped and unknown steps are counted",
			"image",
			steps("done", "skipped", "unknown", "current"),
			"approval",
			"",
			"workflow (image): 1 of 4 steps done, 1 skipped, 1 unknown; now approval",
		},
		{
			"nothing is open", "text", steps("done", "skipped", "unknown", "done"), "", "",
			"workflow (text): 2 of 4 steps done, 1 skipped, 1 unknown; nothing is open",
		},
	}

	for _, tc := range tests {
		if got := Workflow(tc.kind, tc.steps, tc.current, tc.next); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}

func TestStepSkippedText(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		restore, changed, dry bool
		want                  string
	}{
		{false, true, false, "skipped the step recording"},
		{true, true, false, "restored the step recording"},
		{false, true, true, "dry run, would skip the step recording"},
		{true, true, true, "dry run, would restore the step recording"},
		{false, false, false, "the step recording is already as asked; nothing changed"},
	} {
		got := StepSkipped("prj_1", "recording", tc.restore, tc.changed, tc.dry)
		if want := Note + "\nprj_1: " + tc.want; got != want {
			t.Errorf("%+v:\n got %q\nwant %q", tc, got, want)
		}
	}
}
