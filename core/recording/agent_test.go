package recording

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func okSpec() StartSpec {
	return StartSpec{ProjectID: "prj_0000000001", SourceKind: SourceDisplay, SourceID: "d1"}
}

func TestValidatePrepare(t *testing.T) {
	if got := ValidatePrepare(okSpec(), "pit_0000000001", "say hello"); len(got) != 0 {
		t.Fatalf("valid plan refused: %v", got)
	}

	bad := okSpec()
	bad.SourceKind = "tab"

	got := ValidatePrepare(bad, "nope", strings.Repeat("x", MaxPlanNotesRunes+1))

	want := map[string]bool{"source.kind": true, "script_id": true, "notes": true}
	for _, f := range got {
		delete(want, f.Field)
	}

	if len(want) != 0 {
		t.Errorf("fields not refused: %v (got %v)", want, got)
	}
}

func TestPlanSpecRoundTrip(t *testing.T) {
	p := Plan{SourceKind: SourceWindow, SourceID: "w9", FPS: 60, Microphone: "default", Events: true}
	s := p.Spec("prj_0000000002")

	if s.ProjectID != "prj_0000000002" || s.SourceKind != SourceWindow || s.FPS != 60 || !s.Events ||
		s.Microphone != "default" {
		t.Errorf("spec = %+v", s)
	}
}

func TestCleanNote(t *testing.T) {
	cases := map[string]string{
		"  hello\r\n\r\n\r\nworld  ": "hello\n\nworld",
		"a\x00b\u202ec":              "abc",
		"tab\there":                  "tab here",
		"\n\n\nstart":                "start",
	}

	for in, want := range cases {
		if got := CleanNote(in, 100); got != want {
			t.Errorf("CleanNote(%q) = %q, want %q", in, got, want)
		}
	}

	if got := CleanNote(strings.Repeat("é", 300), 10); len([]rune(got)) != 10 {
		t.Errorf("not cut: %d runes", len([]rune(got)))
	}
}

func TestIsPrepared(t *testing.T) {
	if !IsPrepared(Prepared, "{}") || IsPrepared(Prepared, "") || IsPrepared(Recording, "{}") {
		t.Error("IsPrepared is wrong")
	}
}

func TestValidateMark(t *testing.T) {
	type tc struct {
		name   string
		kind   MarkKind
		at, to int64
		state  State
		have   int
		field  string
		err    error
		limit  bool
	}

	cases := []tc{
		{name: "ok point", kind: MarkGood, at: 10, state: Ready},
		{name: "ok while recording", kind: MarkMistake, at: 5_000_000, state: Recording},
		{name: "ok stretch", kind: MarkBad, at: 10, to: 900, state: Ready},
		{name: "unknown kind", kind: "great", at: 1, state: Ready, field: "kind"},
		{name: "negative", kind: MarkGood, at: -1, state: Ready, field: "at_ms"},
		{name: "past the end", kind: MarkGood, at: 1001, state: Ready, field: "at_ms"},
		{name: "end before start", kind: MarkGood, at: 100, to: 50, state: Ready, field: "end_ms"},
		{name: "end past the end", kind: MarkGood, at: 100, to: 2000, state: Ready, field: "end_ms"},
		{name: "prepared", kind: MarkGood, at: 1, state: Prepared, err: ErrState},
		{name: "cancelled", kind: MarkGood, at: 1, state: Cancelled, err: ErrState},
		{name: "full", kind: MarkGood, at: 1, state: Ready, have: MaxMarks, limit: true},
	}

	for _, c := range cases {
		fields, err := ValidateMark(c.kind, c.at, c.to, c.state, 1000, c.have)

		var le *LimitError

		switch {
		case c.err != nil:
			if !errors.Is(err, c.err) {
				t.Errorf("%s: err = %v", c.name, err)
			}
		case c.limit:
			if !errors.As(err, &le) || le.Limit != LimitMarks {
				t.Errorf("%s: err = %v", c.name, err)
			}
		case c.field != "":
			if err != nil || len(fields) == 0 || fields[0].Field != c.field {
				t.Errorf("%s: fields = %v, err = %v", c.name, fields, err)
			}
		default:
			if err != nil || len(fields) != 0 {
				t.Errorf("%s: fields = %v, err = %v", c.name, fields, err)
			}
		}
	}
}

func TestValidateRanges(t *testing.T) {
	r := func(from, to int64, reason string) CutRange { return CutRange{FromMS: from, ToMS: to, Reason: reason} }

	if got := ValidateRanges([]CutRange{r(100, 500, "um"), r(500, 900, "pause")}, 0, 1000); len(got) != 0 {
		t.Errorf("valid ranges refused: %v", got)
	}

	cases := map[string][]CutRange{
		"ranges":              nil,
		"ranges[0].from_ms":   {r(-1, 500, "x")},
		"ranges[0].to_ms":     {r(100, 150, "short")},
		"ranges[1].from_ms":   {r(100, 500, "x"), r(400, 900, "overlap")},
		"ranges[0].reason":    {r(100, 500, "   ")},
		"ranges[0].from_ms 2": {r(100, 1100, "past the end")},
	}

	for field, rs := range cases {
		field, _, _ = strings.Cut(field, " ")

		got := ValidateRanges(rs, 0, 1000)

		found := false

		for _, f := range got {
			found = found || f.Field == field
		}

		if !found {
			t.Errorf("%s: not refused: %v", field, got)
		}
	}

	many := make([]CutRange, MaxRanges+1)
	if got := ValidateRanges(many, 0, 1<<40); len(got) != 1 || got[0].Field != "ranges" {
		t.Errorf("too many ranges: %v", got)
	}

	if got := ValidateRanges([]CutRange{r(10, 500, strings.Repeat("x", MaxNoteRunes+1))}, 0, 1000); len(got) == 0 {
		t.Error("a reason over the limit was accepted")
	}
}

func TestCheckNewProposal(t *testing.T) {
	pending := make([]Proposal, MaxPendingProposals)
	for i := range pending {
		pending[i].State = ProposalPending
	}

	var le *LimitError
	if err := CheckNewProposal(pending, Ready); !errors.As(err, &le) || le.Limit != LimitProposals {
		t.Errorf("five pending: %v", err)
	}

	if err := CheckNewProposal(pending[:4], Ready); err != nil {
		t.Errorf("four pending: %v", err)
	}

	decided := make([]Proposal, 4)
	for i := range decided {
		decided[i].State = ProposalRejected
	}

	if err := CheckNewProposal(decided, Ready); err != nil {
		t.Errorf("decided ones do not count: %v", err)
	}

	if err := CheckNewProposal(nil, Recording); !errors.Is(err, ErrState) {
		t.Errorf("a recording still running: %v", err)
	}
}

func TestDecide(t *testing.T) {
	p := Proposal{
		ID:     "prp_0000000001",
		State:  ProposalPending,
		Ranges: []CutRange{{FromMS: 0, ToMS: 200}, {FromMS: 300, ToMS: 600}},
	}

	got, acc, err := Decide(p, []bool{false, true}, "user:1", 7)
	if err != nil || got.State != ProposalAccepted || len(acc) != 1 || acc[0].FromMS != 300 ||
		got.Ranges[0].Decision != DecisionRejected || got.Ranges[1].Decision != DecisionAccepted ||
		got.DecidedBy != "user:1" || got.DecidedMs != 7 {
		t.Fatalf("decide = %+v %v %v", got, acc, err)
	}

	if p.Ranges[0].Decision != "" {
		t.Error("the proposal given was changed")
	}

	if got, acc, _ := Decide(p, []bool{false, false}, "u", 1); got.State != ProposalRejected || len(acc) != 0 {
		t.Errorf("reject all: %+v", got)
	}

	if _, _, err := Decide(p, []bool{true}, "u", 1); !errors.Is(err, ErrDecisions) {
		t.Errorf("short decisions: %v", err)
	}

	if _, _, err := Decide(got, []bool{true, true}, "u", 1); !errors.Is(err, ErrProposalState) {
		t.Errorf("decided twice: %v", err)
	}
}

func TestSharedSpan(t *testing.T) {
	if lo, hi := SharedSpan(nil, 900); lo != 0 || hi != 900 {
		t.Errorf("no tracks: %d %d", lo, hi)
	}

	tr := []Track{{StartMS: 0, DurationMS: 1000}, {StartMS: 80, DurationMS: 900}, {StartMS: 20, DurationMS: 1100}}
	if lo, hi := SharedSpan(tr, 1000); lo != 80 || hi != 980 {
		t.Errorf("span = %d %d", lo, hi)
	}

	if lo, hi := SharedSpan(
		[]Track{{StartMS: 0, DurationMS: 10}, {StartMS: 50, DurationMS: 10}},
		60,
	); lo != 0 ||
		hi != 0 {
		t.Errorf("disjoint: %d %d", lo, hi)
	}
}

// ids makes deterministic ids for the planner.
func ids() func() string {
	n := 0

	return func() string {
		n++

		return fmt.Sprintf("itm_new%d", n)
	}
}

func TestMSToFrames(t *testing.T) {
	if got := MSToFrames(1000, 30, 1); got != 30 {
		t.Errorf("1000 ms at 30 = %d", got)
	}

	if got := MSToFrames(1000, 30000, 1001); got != 30 {
		t.Errorf("1000 ms at 29.97 = %d", got)
	}

	if got := MSToFrames(16, 30, 1); got != 0 || MSToFrames(17, 30, 1) != 1 {
		t.Errorf("rounding: %d %d", MSToFrames(16, 30, 1), MSToFrames(17, 30, 1))
	}
}

func TestPlanCutsMiddleOfOneClip(t *testing.T) {
	clips := []Clip{{ID: "itm_a", Track: "trk_1", Kind: TrackScreen, Start: 0, Duration: 300}}
	rs := []CutRange{{FromMS: 2000, ToMS: 3000}}

	steps, err := PlanCuts(rs, clips, []Track{{Kind: TrackScreen}}, 30, 1, ids())
	if err != nil {
		t.Fatal(err)
	}

	want := []Step{
		{StepSplit, "itm_a", 90, "itm_new1"}, {StepSplit, "itm_a", 60, "itm_new2"}, {StepDelete, "itm_new2", 0, ""},
	}
	if fmt.Sprint(steps) != fmt.Sprint(want) {
		t.Errorf("steps = %v, want %v", steps, want)
	}
}

func TestPlanCutsEdgesAndWhole(t *testing.T) {
	clips := []Clip{{ID: "itm_a", Track: "trk_1", Kind: TrackScreen, Start: 0, Duration: 300}}
	screen := []Track{{Kind: TrackScreen}}

	// the start of the clip: the left part goes
	steps, err := PlanCuts([]CutRange{{FromMS: 0, ToMS: 1000}}, clips, screen, 30, 1, ids())
	if err != nil ||
		fmt.Sprint(steps) != fmt.Sprint([]Step{{StepSplit, "itm_a", 30, "itm_new1"}, {StepDelete, "itm_a", 0, ""}}) {
		t.Errorf("start: %v %v", steps, err)
	}

	// the end of the clip: the right part goes
	steps, err = PlanCuts([]CutRange{{FromMS: 9000, ToMS: 10000}}, clips, screen, 30, 1, ids())
	if err != nil ||
		fmt.Sprint(
			steps,
		) != fmt.Sprint(
			[]Step{{StepSplit, "itm_a", 270, "itm_new1"}, {StepDelete, "itm_new1", 0, ""}},
		) {
		t.Errorf("end: %v %v", steps, err)
	}

	// the whole clip
	steps, err = PlanCuts([]CutRange{{FromMS: 0, ToMS: 10000}}, clips, screen, 30, 1, ids())
	if err != nil || fmt.Sprint(steps) != fmt.Sprint([]Step{{StepDelete, "itm_a", 0, ""}}) {
		t.Errorf("whole: %v %v", steps, err)
	}
}

func TestPlanCutsTwoRangesLastFirstAndTwoTracksStayAligned(t *testing.T) {
	clips := []Clip{
		{ID: "itm_s", Track: "trk_v", Kind: TrackScreen, Start: 0, Duration: 300},
		{ID: "itm_m", Track: "trk_a", Kind: TrackMicrophone, Start: 3, Duration: 297},
	}
	// the microphone starts 100 ms (3 frames) after the screen
	tracks := []Track{{Kind: TrackScreen}, {Kind: TrackMicrophone, StartMS: 100}}
	rs := []CutRange{{FromMS: 1000, ToMS: 2000}, {FromMS: 5000, ToMS: 6000}}

	steps, err := PlanCuts(rs, clips, tracks, 30, 1, ids())
	if err != nil {
		t.Fatal(err)
	}

	// replay the plan on the geometry and check the two tracks end where they should: 60 frames shorter
	type piece struct{ start, dur, srcIn int }

	state := map[string]piece{"itm_s": {0, 300, 0}, "itm_m": {3, 297, 0}}
	track := map[string]string{"itm_s": "v", "itm_m": "a"}

	for _, st := range steps {
		p := state[st.ItemID]

		switch st.Op {
		case StepSplit:
			left := st.At - p.start
			state[st.NewID] = piece{st.At, p.dur - left, p.srcIn + left}
			track[st.NewID] = track[st.ItemID]
			state[st.ItemID] = piece{p.start, left, p.srcIn}
		case StepDelete:
			delete(state, st.ItemID)

			for id, q := range state {
				if track[id] == track[st.ItemID] && q.start >= p.start+p.dur {
					q.start -= p.dur
					state[id] = q
				}
			}
		}
	}

	ends := map[string]int{}

	for id, p := range state {
		ends[track[id]] = max(ends[track[id]], p.start+p.dur)
	}

	if ends["v"] != 240 || ends["a"] != 240 {
		t.Errorf("ends = %v, want both 240 (steps %v)", ends, steps)
	}

	// last range first: the first split is at the later range
	if steps[0].Op != StepSplit || steps[0].At < 150 {
		t.Errorf("the later range must be cut first: %v", steps[0])
	}
}

func TestPlanCutsRefusals(t *testing.T) {
	slow := []Clip{{ID: "itm_a", Track: "trk_1", Kind: TrackScreen, Duration: 100, Slow: true}}
	if _, err := PlanCuts([]CutRange{{FromMS: 0, ToMS: 500}}, slow, nil, 30, 1, ids()); !errors.Is(err, ErrSlowClip) {
		t.Errorf("slow: %v", err)
	}

	fast := []Clip{{ID: "itm_a", Track: "trk_1", Kind: TrackScreen, Start: 0, Duration: 30}}
	if _, err := PlanCuts(
		[]CutRange{{FromMS: 5000, ToMS: 6000}},
		fast,
		nil,
		30,
		1,
		ids(),
	); !errors.Is(
		err,
		ErrNothingToCut,
	) {
		t.Errorf("off the clip: %v", err)
	}

	if _, err := PlanCuts(
		[]CutRange{{FromMS: 0, ToMS: 500}},
		nil,
		nil,
		30,
		1,
		ids(),
	); !errors.Is(
		err,
		ErrNothingToCut,
	) {
		t.Errorf("no clip: %v", err)
	}
}

func TestPlanCutsASecondCutSeesTheFirstOnTheSameTrack(t *testing.T) {
	// two clips of the take on one track, the second after the first; a range in each
	clips := []Clip{
		{ID: "itm_a", Track: "trk_1", Kind: TrackScreen, Start: 0, Duration: 100, SourceIn: 0},
		{ID: "itm_b", Track: "trk_1", Kind: TrackScreen, Start: 100, Duration: 100, SourceIn: 100},
	}
	rs := []CutRange{{FromMS: 0, ToMS: 1000}, {FromMS: 4000, ToMS: 5000}}

	steps, err := PlanCuts(rs, clips, []Track{{Kind: TrackScreen}}, 30, 1, ids())
	if err != nil {
		t.Fatal(err)
	}

	// the later range cuts the second clip (frames 120 to 150 of the source) first
	if steps[0].ItemID != "itm_b" {
		t.Errorf("first step = %v", steps[0])
	}
}
