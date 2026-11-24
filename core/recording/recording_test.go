package recording

import (
	"errors"
	"testing"
)

var allStates = []State{Prepared, Recording, Finishing, Ready, Failed, Cancelled}

var allEvents = []Event{EventStarted, EventStop, EventHelperEnded, EventFinished, EventFailed, EventCancel}

// allowed is the table of SDD-023 section 3, written out: every pair not in it must be refused.
var allowed = map[State]map[Event]State{
	Prepared:  {EventStarted: Recording, EventCancel: Cancelled, EventFailed: Failed},
	Recording: {EventStop: Finishing, EventCancel: Cancelled, EventHelperEnded: Failed, EventFailed: Failed},
	Finishing: {EventFinished: Ready, EventFailed: Failed, EventHelperEnded: Failed, EventCancel: Cancelled},
}

func TestNextEveryPair(t *testing.T) {
	for _, from := range allStates {
		for _, ev := range allEvents {
			want, ok := allowed[from][ev]
			got, err := Next(from, ev)

			if ok {
				if err != nil || got != want {
					t.Errorf("Next(%s, %s) = %s, %v; want %s", from, ev, got, err, want)
				}

				continue
			}

			if !errors.Is(err, ErrTransition) {
				t.Errorf("Next(%s, %s) = %s, %v; want ErrTransition", from, ev, got, err)
			}

			if got != from {
				t.Errorf("Next(%s, %s) moved to %s on a refusal", from, ev, got)
			}
		}
	}
}

func TestFinalStatesLeaveNowhere(t *testing.T) {
	for _, s := range allStates {
		if s.Final() != (len(allowed[s]) == 0) {
			t.Errorf("%s: Final() = %v but it has %d exits", s, s.Final(), len(allowed[s]))
		}
	}
}

func TestSettle(t *testing.T) {
	for _, from := range []State{Prepared, Recording, Finishing} {
		if got, err := Settle(from, true); err != nil || got != Ready {
			t.Errorf("Settle(%s, frames) = %s, %v", from, got, err)
		}

		if got, err := Settle(from, false); err != nil || got != Failed {
			t.Errorf("Settle(%s, no frames) = %s, %v", from, got, err)
		}
	}

	for _, from := range []State{Ready, Failed, Cancelled, State("bogus")} {
		if _, err := Settle(from, true); !errors.Is(err, ErrTransition) {
			t.Errorf("Settle(%s) = %v; want ErrTransition", from, err)
		}
	}
}

func TestSettleNeverLeavesRecording(t *testing.T) {
	for _, hasFrames := range []bool{true, false} {
		for _, from := range []State{Prepared, Recording, Finishing} {
			got, err := Settle(from, hasFrames)
			if err != nil || got == Recording || got == Prepared || got == Finishing {
				t.Errorf("Settle(%s, %v) = %s, %v", from, hasFrames, got, err)
			}
		}
	}
}

func TestReviewRules(t *testing.T) {
	if r, err := Confirm(Ready, ReviewPending); err != nil || r != ReviewAdded {
		t.Errorf("Confirm pending = %s, %v", r, err)
	}

	if r, err := Confirm(Ready, ReviewDismissed); err != nil || r != ReviewAdded {
		t.Errorf("Confirm dismissed = %s, %v", r, err)
	}

	for _, c := range []struct {
		s State
		r Review
	}{{Ready, ReviewAdded}, {Failed, ReviewPending}, {Recording, ""}, {Ready, ""}} {
		if _, err := Confirm(c.s, c.r); !errors.Is(err, ErrReview) {
			t.Errorf("Confirm(%s, %q) = %v; want ErrReview", c.s, c.r, err)
		}
	}

	if r, err := Dismiss(Ready, ReviewPending); err != nil || r != ReviewDismissed {
		t.Errorf("Dismiss pending = %s, %v", r, err)
	}

	for _, r := range []Review{ReviewAdded, ReviewDismissed, ""} {
		if _, err := Dismiss(Ready, r); !errors.Is(err, ErrReview) {
			t.Errorf("Dismiss(ready, %q) = %v; want ErrReview", r, err)
		}
	}

	if _, err := Dismiss(Failed, ReviewPending); !errors.Is(err, ErrReview) {
		t.Errorf("Dismiss failed = %v", err)
	}
}

func TestVisibilityPlacementAndDelete(t *testing.T) {
	if VisibleTo(ReviewDismissed, false) || !VisibleTo(ReviewDismissed, true) {
		t.Error("a dismissed recording must be for a person only")
	}

	for _, r := range []Review{ReviewPending, ReviewAdded, ""} {
		if !VisibleTo(r, false) {
			t.Errorf("review %q must be visible to everyone", r)
		}
	}

	if !Placeable(Ready, ReviewAdded) || Placeable(Ready, ReviewPending) || Placeable(Ready, ReviewDismissed) ||
		Placeable(Failed, ReviewAdded) {
		t.Error("only a ready, added recording is placeable")
	}

	if !CanDelete(Ready, ReviewPending) || !CanDelete(Ready, ReviewDismissed) || !CanDelete(Failed, "") ||
		CanDelete(Ready, ReviewAdded) || CanDelete(Recording, "") || CanDelete(Cancelled, "") {
		t.Error("CanDelete does not follow SDD-023 section 3")
	}
}

func TestLimitsCheck(t *testing.T) {
	l := Limits{MaxSeconds: 10, MinFreeBytes: 1000, MaxBytes: 5000}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name                string
		elapsed, bytes, fre int64
		want                StopReason
	}{
		{"fine", 9999, 4999, 1000, StopNone},
		{"duration", 10000, 0, 5000, StopDuration},
		{"size", 1, 5000, 5000, StopSize},
		{"disk", 1, 1, 999, StopDisk},
		{"duration wins over disk", 10000, 1, 0, StopDuration},
	}

	for _, c := range cases {
		if got := l.Check(c.elapsed, c.bytes, c.fre); got != c.want {
			t.Errorf("%s: Check = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLimitsStartAndValidate(t *testing.T) {
	l := Limits{MaxSeconds: 10, MinFreeBytes: 1000, MaxBytes: 5000}
	if err := l.CheckStart(1000); err != nil {
		t.Errorf("exactly the limit is enough: %v", err)
	}

	if err := l.CheckStart(999); !errors.Is(err, ErrDiskLow) {
		t.Errorf("CheckStart(999) = %v", err)
	}

	for _, bad := range []Limits{
		{MaxSeconds: 0, MaxBytes: 1},
		{MaxSeconds: MaxSecondsCeiling + 1, MaxBytes: 1},
		{MaxSeconds: 1, MaxBytes: 0},
		{MaxSeconds: 1, MaxBytes: 1, MinFreeBytes: -1},
	} {
		if bad.Validate() == nil {
			t.Errorf("%+v must be refused", bad)
		}
	}
}

func TestFitSize(t *testing.T) {
	if w, h, err := FitSize(1921, 1081); err != nil || w != 1920 || h != 1080 {
		t.Errorf("FitSize(1921,1081) = %d,%d,%v", w, h, err)
	}

	for _, c := range [][2]int{{10, 1080}, {1920, 8}, {8194, 100}, {8192, 8192}} {
		if _, _, err := FitSize(c[0], c[1]); err == nil {
			t.Errorf("FitSize(%d,%d) must be refused", c[0], c[1])
		}
	}
}

func TestValidateStart(t *testing.T) {
	ok := StartSpec{ProjectID: "prj_0123456789", SourceKind: SourceWindow, SourceID: "w-1234", FPS: 60}
	if got := ValidateStart(ok); len(got) != 0 {
		t.Fatalf("a valid request was refused: %v", got)
	}

	ok.FPS = 0
	if got := ValidateStart(ok); len(got) != 0 || FPSOrDefault(ok.FPS) != DefaultFPS {
		t.Errorf("the default frame rate: %v", got)
	}

	bad := StartSpec{ProjectID: "x", SourceKind: "camera", SourceID: "a b", FPS: 59}

	fields := map[string]bool{}
	for _, f := range ValidateStart(bad) {
		fields[f.Field] = true
	}

	for _, want := range []string{"project_id", "source.kind", "source.id", "fps"} {
		if !fields[want] {
			t.Errorf("field %s not refused", want)
		}
	}
}

func TestCleanLabel(t *testing.T) {
	rlo := string(rune(0x202e))
	got := CleanLabel("  Doc\t\x00uments "+rlo+"  - Editor\n", 100)
	if got != "Doc uments - Editor" {
		t.Errorf("CleanLabel = %q", got)
	}

	if got := CleanLabel("abcdef", 3); got != "abc" {
		t.Errorf("cut = %q", got)
	}
}
