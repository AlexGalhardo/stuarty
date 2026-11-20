package publish

import (
	"strings"
	"testing"
	"time"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()

	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}

	return v
}

func TestTargetStateMachine(t *testing.T) {
	t.Parallel()

	allowed := map[[2]TargetState]bool{
		{TargetDraft, TargetWaitingApproval}:           true,
		{TargetDraft, TargetCancelled}:                 true,
		{TargetWaitingApproval, TargetApproved}:        true,
		{TargetWaitingApproval, TargetScheduled}:       true,
		{TargetWaitingApproval, TargetMissed}:          true,
		{TargetWaitingApproval, TargetDraft}:           true,
		{TargetWaitingApproval, TargetWaitingApproval}: true,
		{TargetWaitingApproval, TargetCancelled}:       true,
		{TargetApproved, TargetPublishing}:             true,
		{TargetApproved, TargetMissed}:                 true,
		{TargetApproved, TargetWaitingApproval}:        true,
		{TargetApproved, TargetCancelled}:              true,
		{TargetScheduled, TargetPublishing}:            true,
		{TargetScheduled, TargetMissed}:                true,
		{TargetScheduled, TargetWaitingApproval}:       true,
		{TargetScheduled, TargetCancelled}:             true,
		{TargetPublishing, TargetPublished}:            true,
		{TargetPublishing, TargetFailed}:               true,
		{TargetPublishing, TargetScheduled}:            true,
		{TargetFailed, TargetWaitingApproval}:          true,
		{TargetFailed, TargetCancelled}:                true,
		{TargetMissed, TargetWaitingApproval}:          true,
		{TargetMissed, TargetCancelled}:                true,
	}

	for _, from := range TargetStates {
		for _, to := range TargetStates {
			if got := CanTarget(from, to); got != allowed[[2]TargetState{from, to}] {
				t.Errorf("CanTarget(%s, %s) = %v", from, to, got)
			}
		}
	}

	for _, s := range []TargetState{TargetPublished, TargetCancelled} {
		if !s.Final() {
			t.Errorf("%s is final", s)
		}
	}

	for _, s := range []TargetState{TargetWaitingApproval, TargetApproved, TargetScheduled} {
		if !s.Waiting() {
			t.Errorf("%s is waiting", s)
		}
	}

	if TargetPublishing.Editable() || TargetPublished.Editable() || !TargetFailed.Editable() {
		t.Error("Editable is wrong")
	}
}

func TestLocalInstant(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, local, zone string
		want              string
		adj               Adjustment
	}{
		{"summer in Sao Paulo (no daylight saving)", "2026-11-20T09:00", "America/Sao_Paulo", "2026-11-20T12:00:00Z", AdjustNone},
		{"New York in winter", "2026-01-15T09:00", "America/New_York", "2026-01-15T14:00:00Z", AdjustNone},
		{"New York in summer", "2026-07-15T09:00", "America/New_York", "2026-07-15T13:00:00Z", AdjustNone},
		{"the hour before the spring jump", "2026-03-08T01:59", "America/New_York", "2026-03-08T06:59:00Z", AdjustNone},
		{"the hour that does not exist, New York", "2026-03-08T02:30", "America/New_York", "2026-03-08T07:30:00Z", AdjustGap},
		{"the hour that does not exist, Lisbon", "2026-03-29T01:30", "Europe/Lisbon", "2026-03-29T02:30:00+01:00", AdjustGap},
		{"after the spring jump", "2026-03-08T03:00", "America/New_York", "2026-03-08T07:00:00Z", AdjustNone},
		{"the hour that happens twice, New York", "2026-11-01T01:30", "America/New_York", "2026-11-01T05:30:00Z", AdjustAmbiguous},
		{"the hour that happens twice, Berlin", "2026-10-25T02:30", "Europe/Berlin", "2026-10-25T00:30:00Z", AdjustAmbiguous},
		{"after the fall back", "2026-11-01T02:30", "America/New_York", "2026-11-01T07:30:00Z", AdjustNone},
		{"a zone half an hour off", "2026-06-01T10:00", "Asia/Kolkata", "2026-06-01T04:30:00Z", AdjustNone},
		{"UTC", "2026-06-01T10:00", "UTC", "2026-06-01T10:00:00Z", AdjustNone},
		{"seconds", "2026-06-01T10:00:30", "UTC", "2026-06-01T10:00:30Z", AdjustNone},
		{"Lord Howe, a jump of half an hour", "2026-10-04T02:15", "Australia/Lord_Howe", "2026-10-03T15:45:00Z", AdjustGap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			loc, err := LoadZone(tc.zone)
			if err != nil {
				t.Fatal(err)
			}

			got, adj, err := LocalInstant(tc.local, loc)
			if err != nil {
				t.Fatal(err)
			}

			if want := at(t, tc.want); !got.Equal(want) {
				t.Errorf("instant = %s, want %s", got.Format(time.RFC3339), want.UTC().Format(time.RFC3339))
			}

			if got.Location() != time.UTC {
				t.Errorf("the instant is in %s, not UTC", got.Location())
			}

			if adj != tc.adj {
				t.Errorf("adjustment = %q, want %q", adj, tc.adj)
			}
		})
	}
}

func TestLocalInstantRefusesWhatIsNotATime(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", "tomorrow", "2026-13-01T10:00", "2026-02-30T10:00", "10:00"} {
		if _, _, err := LocalInstant(s, time.UTC); err == nil {
			t.Errorf("%q was accepted", s)
		}
	}

	for _, z := range []string{"Mars/Olympus", "Local", "../etc", "America/ New_York"} {
		if _, err := LoadZone(z); err == nil {
			t.Errorf("zone %q was accepted", z)
		}
	}
}

func TestParseSlot(t *testing.T) {
	t.Parallel()

	s, err := ParseSlot("2026-11-20T09:00", "America/Sao_Paulo", "UTC")
	if err != nil {
		t.Fatal(err)
	}

	if !s.At.Equal(at(t, "2026-11-20T12:00:00Z")) || s.Zone != "America/Sao_Paulo" || s.Local() != "2026-11-20 09:00" {
		t.Errorf("slot = %+v (%s)", s, s.Local())
	}

	// An instant keeps its value and is shown in the zone the caller named.
	s, err = ParseSlot("2026-11-20T12:00:00Z", "America/Sao_Paulo", "UTC")
	if err != nil || !s.At.Equal(at(t, "2026-11-20T12:00:00Z")) || s.Local() != "2026-11-20 09:00" ||
		s.Adjustment != AdjustNone {
		t.Errorf("slot = %+v, %v", s, err)
	}

	// An offset moves the instant, whatever the zone.
	s, err = ParseSlot("2026-11-20T09:00:00-03:00", "", "UTC")
	if err != nil || !s.At.Equal(at(t, "2026-11-20T12:00:00Z")) || s.Zone != "UTC" {
		t.Errorf("slot = %+v, %v", s, err)
	}

	// The default zone reads a local time.
	s, err = ParseSlot("2026-03-08T02:30", "", "America/New_York")
	if err != nil || s.Adjustment != AdjustGap || s.Zone != "America/New_York" {
		t.Errorf("slot = %+v, %v", s, err)
	}

	for _, bad := range [][2]string{{"", "UTC"}, {"2026-11-20T09:00", "Nowhere/Land"}, {"soon", "UTC"}} {
		if _, err := ParseSlot(bad[0], bad[1], ""); err == nil {
			t.Errorf("%v was accepted", bad)
		}
	}
}

func TestDecide(t *testing.T) {
	t.Parallel()

	slot := at(t, "2026-11-20T12:00:00Z")
	mk := func(state TargetState, late int) Candidate {
		return Candidate{ID: "tgt_a", State: state, At: slot, LateWindowMinutes: late}
	}

	for _, tc := range []struct {
		name string
		t    Candidate
		now  string
		want Action
	}{
		{"before its time", mk(TargetScheduled, 0), "2026-11-20T11:59:59Z", ActionWait},
		{"at its time", mk(TargetScheduled, 0), "2026-11-20T12:00:00Z", ActionSend},
		{"approved, at its time", mk(TargetApproved, 0), "2026-11-20T12:00:00Z", ActionSend},
		{"within the grace of the loop", mk(TargetScheduled, 0), "2026-11-20T12:01:00Z", ActionSend},
		{"one second past the grace, no window", mk(TargetScheduled, 0), "2026-11-20T12:01:01Z", ActionMiss},
		{"the product was stopped for an hour", mk(TargetScheduled, 0), "2026-11-20T13:00:00Z", ActionMiss},
		{"inside the late window", mk(TargetScheduled, 30), "2026-11-20T12:30:00Z", ActionSend},
		{"inside the late window plus the grace", mk(TargetScheduled, 30), "2026-11-20T12:31:00Z", ActionSend},
		{"past the late window", mk(TargetScheduled, 30), "2026-11-20T12:31:01Z", ActionMiss},
		{"a draft is not due", mk(TargetDraft, 0), "2026-11-20T12:00:00Z", ActionWait},
		{"one waiting for approval is not due", mk(TargetWaitingApproval, 0), "2026-11-20T12:00:00Z", ActionWait},
		{"a published one is done", mk(TargetPublished, 0), "2026-11-20T12:00:00Z", ActionWait},
		{"a missed one stays missed", mk(TargetMissed, 0), "2026-11-20T12:00:00Z", ActionWait},
		{"one being sent, lease alive", Candidate{State: TargetPublishing, LeaseUntil: slot.Add(time.Minute)}, "2026-11-20T12:00:30Z", ActionWait},
		{"one being sent, lease ended", Candidate{State: TargetPublishing, LeaseUntil: slot.Add(time.Minute)}, "2026-11-20T12:01:01Z", ActionResume},
		{
			"a retry that is not yet due",
			Candidate{State: TargetScheduled, At: slot, Attempts: 1, NextAttempt: slot.Add(3 * time.Minute)},
			"2026-11-20T12:02:00Z", ActionWait,
		},
		{
			"a retry that is due is not judged by its first time",
			Candidate{State: TargetScheduled, At: slot, Attempts: 1, NextAttempt: slot.Add(3 * time.Minute)},
			"2026-11-20T12:03:00Z", ActionSend,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := Decide(tc.t, at(t, tc.now)); got != tc.want {
				t.Errorf("Decide = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDueAndMissed(t *testing.T) {
	t.Parallel()

	now := at(t, "2026-11-20T12:00:00Z")
	list := []Candidate{
		{ID: "late", State: TargetScheduled, At: now.Add(-time.Hour)},
		{ID: "second", State: TargetScheduled, At: now},
		{ID: "first", State: TargetApproved, At: now.Add(-30 * time.Second)},
		{ID: "future", State: TargetScheduled, At: now.Add(time.Minute)},
		{ID: "lated", State: TargetScheduled, At: now.Add(-time.Hour), LateWindowMinutes: 90},
		{ID: "draft", State: TargetDraft, At: now.Add(-time.Hour)},
	}

	var ids []string
	for _, d := range Due(list, now) {
		ids = append(ids, d.ID)
	}

	if got := join(ids); got != "lated first second" {
		t.Errorf("Due = %s", got)
	}

	var missed []string
	for _, d := range Missed(list, now) {
		missed = append(missed, d.ID)
	}

	if got := join(missed); got != "late" {
		t.Errorf("Missed = %s", got)
	}
}

func join(s []string) string { return strings.Join(s, " ") }

func TestWake(t *testing.T) {
	t.Parallel()

	slot := at(t, "2026-11-20T12:00:00Z")

	if w, ok := Wake(Candidate{State: TargetScheduled, At: slot}); !ok || !w.Equal(slot) {
		t.Errorf("Wake = %v, %v", w, ok)
	}

	next := slot.Add(2 * time.Minute)
	if w, ok := Wake(
		Candidate{State: TargetApproved, At: slot, Attempts: 1, NextAttempt: next},
	); !ok ||
		!w.Equal(next) {
		t.Errorf("Wake of a retry = %v, %v", w, ok)
	}

	if w, ok := Wake(Candidate{State: TargetPublishing, LeaseUntil: next}); !ok || !w.Equal(next) {
		t.Errorf("Wake of a lease = %v, %v", w, ok)
	}

	for _, s := range []TargetState{TargetDraft, TargetWaitingApproval, TargetPublished, TargetFailed, TargetMissed, TargetCancelled} {
		if _, ok := Wake(Candidate{State: s, At: slot}); ok {
			t.Errorf("%s wakes the scheduler", s)
		}
	}
}

func TestRetryPolicy(t *testing.T) {
	t.Parallel()

	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}
	for i, w := range want {
		if got, ok := RetryDelay(i + 1); !ok || got != w {
			t.Errorf("RetryDelay(%d) = %v, %v; want %v", i+1, got, ok, w)
		}
	}

	for _, n := range []int{0, -1, MaxAttempts, MaxAttempts + 3} {
		if _, ok := RetryDelay(n); ok {
			t.Errorf("RetryDelay(%d) retries", n)
		}
	}

	if !Transient("unavailable") {
		t.Error("unavailable is transient")
	}

	for _, k := range []string{"quota", "auth", "not_eligible", "invalid", "rejected", ""} {
		if Transient(k) {
			t.Errorf("%q is not transient", k)
		}
	}
}

func TestItemStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		states []TargetState
		want   string
	}{
		{"no targets", nil, ""},
		{"a draft", []TargetState{TargetDraft}, ""},
		{"one waits for a person", []TargetState{TargetPublished, TargetWaitingApproval}, ItemPendingApproval},
		{"all published", []TargetState{TargetPublished, TargetPublished}, ItemPublished},
		{"a cancelled one does not count", []TargetState{TargetPublished, TargetCancelled}, ItemPublished},
		{"only cancelled ones", []TargetState{TargetCancelled}, ""},
		{"one still scheduled", []TargetState{TargetPublished, TargetScheduled}, ""},
		{"one failed", []TargetState{TargetPublished, TargetFailed}, ""},
	} {
		if got := ItemStatus(tc.states); got != tc.want {
			t.Errorf("%s: ItemStatus = %q, want %q", tc.name, got, tc.want)
		}
	}
}
