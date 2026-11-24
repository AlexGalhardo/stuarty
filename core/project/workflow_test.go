package project

import (
	"slices"
	"testing"
)

func ids(w *Workflow) []StepID { return stepIDsOf(w) }

func states(w *Workflow) map[StepID]StepState {
	out := map[StepID]StepState{}
	for i := range w.Steps {
		out[w.Steps[i].ID] = w.Steps[i].State
	}

	return out
}

func item(kind Kind, textKind, text string) Item {
	return Item{ID: string(kind) + "-" + textKind, Kind: kind, Text: text, TextKind: textKind}
}

func pub(status string) Item { return Item{ID: "pub-" + status, Kind: Publication, Status: status} }

func TestStepsOfEveryKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stored Intent
		items  []Item
		want   []StepID
	}{
		{"undecided", KindUndecided, nil, []StepID{StepBriefing, StepChoice}},
		{
			"undecided with a brief only",
			KindUndecided,
			[]Item{item(Brief, "", "b")},
			[]StepID{StepBriefing, StepChoice},
		},
		{
			"text", KindText, nil,
			[]StepID{StepBriefing, StepWriting, StepReview, StepApproval, StepPublication},
		},
		{
			"video", KindVideo, nil,
			[]StepID{
				StepBriefing, StepScript, StepRecording, StepEditing, StepFinishing, StepApproval, StepPublication,
			},
		},
		{"image", KindImage, nil, []StepID{StepBriefing, StepEditing, StepApproval, StepPublication}},
		{
			"derived text", KindUndecided,
			[]Item{item(Text, "copy", "words")},
			[]StepID{StepBriefing, StepWriting, StepReview, StepApproval, StepPublication},
		},
		{
			"derived video", KindUndecided,
			[]Item{item(Timeline, "", "")},
			[]StepID{
				StepBriefing, StepScript, StepRecording, StepEditing, StepFinishing, StepApproval, StepPublication,
			},
		},
		{
			"a text that gets a timeline keeps its steps and gains the video ones", KindText,
			[]Item{item(Timeline, "", "")},
			[]StepID{
				StepBriefing, StepScript, StepWriting, StepRecording, StepEditing, StepFinishing, StepReview,
				StepApproval, StepPublication,
			},
		},
		{
			"a trashed timeline holds nothing", KindText,
			[]Item{{Kind: Timeline, Trashed: true}},
			[]StepID{StepBriefing, StepWriting, StepReview, StepApproval, StepPublication},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := ComputeWorkflow(tc.stored, &Facts{Items: tc.items, PublicationsKnown: true})
			if got := ids(&w); !slices.Equal(got, tc.want) {
				t.Fatalf("steps = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEffectiveKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stored Intent
		items  []Item
		want   Intent
	}{
		{"nothing", KindUndecided, nil, KindUndecided},
		{"a stored kind wins over the items", KindText, []Item{item(Timeline, "", "")}, KindText},
		{"a timeline", KindUndecided, []Item{item(Timeline, "", "")}, KindVideo},
		{"an image", KindUndecided, []Item{item(Image, "", "")}, KindImage},
		{"a text", KindUndecided, []Item{item(Text, "post", "x")}, KindText},
		{"a timeline beats a text", KindUndecided, []Item{item(Text, "copy", "x"), item(Timeline, "", "")}, KindVideo},
		{"an output decides nothing", KindUndecided, []Item{item(Output, "", "")}, KindUndecided},
		{"a trashed text holds nothing", KindUndecided, []Item{{Kind: Text, Trashed: true}}, KindUndecided},
		{"a kind that is not one reads from the items", Intent("x"), []Item{item(Text, "copy", "x")}, KindText},
	}

	for _, tc := range tests {
		if got := EffectiveKind(tc.stored, tc.items); got != tc.want {
			t.Errorf("%s: kind = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestKindToStore(t *testing.T) {
	t.Parallel()

	copyText := []Item{item(Text, "copy", "x")}

	tests := []struct {
		name   string
		stored Intent
		before []Item
		added  Kind
		want   Intent
	}{
		{"a stored kind stays", KindVideo, nil, Text, KindVideo},
		{"the first text decides text", KindUndecided, nil, Text, KindText},
		{"the first timeline decides video", KindUndecided, nil, Timeline, KindVideo},
		{"what it already reads as wins over the new item", KindUndecided, copyText, Timeline, KindText},
		{"an output decides nothing", KindUndecided, nil, Output, KindUndecided},
	}

	for _, tc := range tests {
		if got := KindToStore(tc.stored, tc.before, tc.added); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestParseKind(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]Intent{
		"": KindUndecided, "undecided": KindUndecided, "text": KindText, "video": KindVideo, "image": KindImage,
	} {
		got, err := ParseKind(in)
		if err != nil || got != want {
			t.Errorf("ParseKind(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	_, err := ParseKind("audio")
	if err == nil || err.Code != "E_VALIDATION" || err.Path != "kind" {
		t.Fatalf("an unknown kind: %+v", err)
	}
}

func TestTextWorkflowStates(t *testing.T) {
	t.Parallel()

	brief := item(Brief, "", "# Brief")
	post := item(Text, TextKindPost, "words here")
	blank := item(Text, TextKindCopy, "  \n")

	tests := []struct {
		name  string
		items []Item
		known bool
		want  map[StepID]StepState
		cur   StepID
		next  StepID
	}{
		{
			"a new text project starts at the briefing", nil, true,
			map[StepID]StepState{
				StepBriefing: StepCurrent, StepWriting: StepUpcoming, StepReview: StepUpcoming,
				StepApproval: StepUpcoming, StepPublication: StepUpcoming,
			},
			StepBriefing, StepWriting,
		},
		{
			"a blank text is not there",
			[]Item{brief, blank},
			true,
			map[StepID]StepState{StepBriefing: StepDone, StepWriting: StepCurrent},
			StepWriting, StepReview,
		},
		{
			"written",
			[]Item{brief, post},
			true,
			map[StepID]StepState{StepBriefing: StepDone, StepWriting: StepDone, StepReview: StepCurrent},
			StepReview, StepApproval,
		},
		{
			"a publication that waits for a person",
			[]Item{brief, post, pub(StatusPendingApproval)},
			true,
			map[StepID]StepState{
				StepReview: StepDone, StepApproval: StepWaiting, StepPublication: StepUpcoming,
			},
			StepApproval, StepPublication,
		},
		{
			"published",
			[]Item{brief, post, pub(StatusPublished)},
			true,
			map[StepID]StepState{StepApproval: StepDone, StepPublication: StepDone},
			"", "",
		},
		{
			"a publication in another state is not read as approved",
			[]Item{brief, post, pub("")},
			true,
			map[StepID]StepState{StepReview: StepDone, StepApproval: StepUnknown, StepPublication: StepUnknown},
			"", "",
		},
		{
			"publications that could not be read are unknown, never done",
			[]Item{brief, post, pub(StatusPublished)},
			false,
			map[StepID]StepState{StepReview: StepUnknown, StepApproval: StepUnknown, StepPublication: StepUnknown},
			"", "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := ComputeWorkflow(KindText, &Facts{Items: tc.items, PublicationsKnown: tc.known})
			got := states(&w)

			for id, want := range tc.want {
				if got[id] != want {
					t.Errorf("%s = %q, want %q (all: %v)", id, got[id], want, got)
				}
			}

			if w.Current != tc.cur || w.Next != tc.next {
				t.Errorf("current, next = %q, %q; want %q, %q", w.Current, w.Next, tc.cur, tc.next)
			}

			for i := range w.Steps {
				if w.Steps[i].State == stepOpen {
					t.Errorf("%s left the workflow open", w.Steps[i].ID)
				}
			}
		})
	}
}

// A text project that gets a timeline: the video steps appear and what was finished stays finished.
func TestAddingAVideoKeepsWhatIsDone(t *testing.T) {
	t.Parallel()

	items := []Item{item(Brief, "", "b"), item(Text, TextKindCopy, "the words")}
	before := ComputeWorkflow(KindText, &Facts{Items: items, PublicationsKnown: true})
	after := ComputeWorkflow(KindText, &Facts{
		Items: append(slices.Clone(items), item(Timeline, "", "")), PublicationsKnown: true,
	})

	for _, id := range []StepID{StepRecording, StepEditing, StepFinishing, StepScript} {
		if _, ok := before.Step(id); ok {
			t.Errorf("a text project had the step %s", id)
		}

		if _, ok := after.Step(id); !ok {
			t.Errorf("the video step %s did not appear", id)
		}
	}

	for _, id := range []StepID{StepBriefing, StepWriting} {
		b, _ := before.Step(id)
		a, _ := after.Step(id)

		if b.State != StepDone || a.State != StepDone {
			t.Errorf("%s: %q before, %q after; want done in both", id, b.State, a.State)
		}
	}

	if after.Kind != KindText {
		t.Errorf("the kind changed to %q", after.Kind)
	}
}

func TestVideoWorkflowFacts(t *testing.T) {
	t.Parallel()

	items := []Item{item(Brief, "", "b"), item(Text, TextKindScript, "script"), item(Timeline, "", "")}

	w := ComputeWorkflow(KindVideo, &Facts{Items: items, PublicationsKnown: true, TimelineAssets: 2, TimelineItems: 5})
	got := states(&w)

	if got[StepRecording] != StepDone || got[StepEditing] != StepDone || got[StepFinishing] != StepCurrent {
		t.Fatalf("video states: %v", got)
	}

	// No timeline items yet: the clips are in, the cut is not.
	w = ComputeWorkflow(KindVideo, &Facts{Items: items, PublicationsKnown: true, TimelineAssets: 2})
	if got := states(&w); got[StepEditing] != StepCurrent {
		t.Fatalf("editing: %v", got)
	}

	// A caption is not a script.
	w = ComputeWorkflow(KindVideo, &Facts{
		Items: []Item{item(Text, TextKindCaption, "words")}, PublicationsKnown: true,
	})
	if got := states(&w); got[StepScript] == StepDone {
		t.Fatalf("a caption made the script done: %v", got)
	}
}

func TestImageEditingIsUnknownNotDone(t *testing.T) {
	t.Parallel()

	w := ComputeWorkflow(KindImage, &Facts{Items: []Item{item(Brief, "", "b")}, PublicationsKnown: true})

	editing, _ := w.Step(StepEditing)
	if editing.State != StepUnknown || editing.Tool != "ImageEdit" {
		t.Fatalf("editing = %+v", editing)
	}

	if w.Current != StepApproval {
		t.Fatalf("an unknown step must not block the current one: %q", w.Current)
	}
}

func TestOptionalSteps(t *testing.T) {
	t.Parallel()

	optionalOf := optionalIDsOf
	post := item(Text, TextKindPost, "p")

	text := ComputeWorkflow(KindText, &Facts{PublicationsKnown: true})
	if got := optionalOf(&text); !slices.Equal(got, []StepID{StepPublication}) {
		t.Errorf("a copy ends in copy or export, so its publication is optional: %v", got)
	}

	blog := ComputeWorkflow(KindText, &Facts{Items: []Item{post}, PublicationsKnown: true})
	if got := optionalOf(&blog); len(got) != 0 {
		t.Errorf("a blog post is published: %v", got)
	}

	video := ComputeWorkflow(KindVideo, &Facts{PublicationsKnown: true})
	if got := optionalOf(&video); !slices.Equal(got, []StepID{StepRecording, StepFinishing}) {
		t.Errorf("video: %v", got)
	}
}

func TestSkippedSteps(t *testing.T) {
	t.Parallel()

	facts := Facts{PublicationsKnown: true, Skipped: []StepID{StepRecording, StepScript, StepFinishing}}
	w := ComputeWorkflow(KindVideo, &facts)
	got := states(&w)

	if got[StepRecording] != StepSkipped || got[StepFinishing] != StepSkipped {
		t.Errorf("optional steps are skipped: %v", got)
	}

	if got[StepScript] == StepSkipped {
		t.Errorf("a required step cannot be skipped by a stored value: %v", got)
	}

	if w.Current != StepBriefing || w.Next != StepScript {
		t.Errorf("current, next = %q, %q", w.Current, w.Next)
	}

	// A step that is done stays done.
	facts = Facts{
		PublicationsKnown: true, Skipped: []StepID{StepFinishing}, Items: []Item{item(Output, "", "")},
	}

	w = ComputeWorkflow(KindVideo, &facts)
	if got := states(&w); got[StepFinishing] != StepDone {
		t.Errorf("finishing: %v", got)
	}
}

func TestCheckStep(t *testing.T) {
	t.Parallel()

	video := ComputeWorkflow(KindVideo, &Facts{PublicationsKnown: true, Items: []Item{item(Output, "", "")}})
	text := ComputeWorkflow(KindText, &Facts{PublicationsKnown: true})

	tests := []struct {
		name    string
		w       *Workflow
		step    string
		restore bool
		code    string
		path    string
	}{
		{"an optional step", &video, "recording", false, "", ""},
		{"restore an optional step", &video, "recording", true, "", ""},
		{"a required step is refused", &video, "editing", false, "E_VALIDATION", "step"},
		{"a required step is refused on restore", &video, "editing", true, "E_VALIDATION", "step"},
		{"an unknown step", &video, "dancing", false, "E_VALIDATION", "step"},
		{"a step of another kind", &text, "recording", false, "E_VALIDATION", "step"},
		{"a step that is done", &video, "finishing", false, "E_INVALID_STATE", "step"},
		{"restoring a step that is done is harmless", &video, "finishing", true, "", ""},
	}

	for _, tc := range tests {
		id, err := CheckStep(tc.w, tc.step, tc.restore)

		switch {
		case tc.code == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.code == "" && string(id) != tc.step:
			t.Errorf("%s: id %q", tc.name, id)
		case tc.code != "" && (err == nil || err.Code != tc.code || err.Path != tc.path):
			t.Errorf("%s: %+v", tc.name, err)
		}
	}
}

func TestCostIsOnTheApprovalStep(t *testing.T) {
	t.Parallel()

	costs := []Cost{{Micros: 1500000, Currency: "USD"}}
	w := ComputeWorkflow(KindText, &Facts{PublicationsKnown: true, Costs: costs})

	for i := range w.Steps {
		st := &w.Steps[i]
		if (st.ID == StepApproval) != (len(st.Cost) == 1) {
			t.Errorf("%s has the cost %v", st.ID, st.Cost)
		}
	}

	// The cost is a copy: the facts are not shared with the answer.
	approval, _ := w.Step(StepApproval)
	approval.Cost[0].Micros = 1

	if costs[0].Micros != 1500000 {
		t.Error("the cost of the workflow aliases the facts")
	}

	none := ComputeWorkflow(KindText, &Facts{PublicationsKnown: true})
	if a, _ := none.Step(StepApproval); a.Cost != nil {
		t.Errorf("a step with no approvals has no cost: %v", a.Cost)
	}
}

func TestToolsAreNamedByTheTable(t *testing.T) {
	t.Parallel()

	for _, k := range Intents() {
		w := ComputeWorkflow(k, &Facts{PublicationsKnown: true})

		for i := range w.Steps {
			if w.Steps[i].Tool == "" || w.Steps[i].Actor == "" {
				t.Errorf("%s step %s has no tool or no actor", k, w.Steps[i].ID)
			}
		}
	}
}

func TestSkippedColumnRoundTrip(t *testing.T) {
	t.Parallel()

	got := JoinSkipped([]StepID{StepFinishing, StepRecording, StepRecording})
	if got != "recording,finishing" {
		t.Fatalf("JoinSkipped = %q", got)
	}

	if back := ParseSkipped(
		"finishing,nonsense,recording,finishing",
	); !slices.Equal(
		back,
		[]StepID{StepFinishing, StepRecording},
	) {
		t.Fatalf("ParseSkipped = %v", back)
	}

	if len(ParseSkipped("")) != 0 || JoinSkipped(nil) != "" {
		t.Fatal("an empty column holds no step")
	}
}
