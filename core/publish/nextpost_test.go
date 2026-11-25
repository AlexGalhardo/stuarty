package publish

import (
	"testing"
	"time"
)

func TestNextPost(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	at := func(h int) *time.Time { v := now.Add(time.Duration(h) * time.Hour); return &v }

	cases := []struct {
		name    string
		targets []Planned
		want    *time.Time
	}{
		{"none", nil, nil},
		{"the earliest that still goes out", []Planned{
			{TargetScheduled, at(48)}, {TargetApproved, at(24)}, {TargetWaitingApproval, at(72)},
		}, at(24)},
		{"a past instant is not next", []Planned{{TargetScheduled, at(-1)}, {TargetScheduled, at(5)}}, at(5)},
		{"final and draft targets are not next", []Planned{
			{TargetPublished, at(1)}, {TargetFailed, at(1)}, {TargetMissed, at(1)}, {TargetCancelled, at(1)},
			{TargetSentAsDraft, at(1)}, {TargetDraft, at(1)}, {TargetPublishing, at(1)},
		}, nil},
		{"no instant", []Planned{{TargetScheduled, nil}}, nil},
		{"now itself", []Planned{{TargetScheduled, &now}}, &now},
	}

	for _, c := range cases {
		got := NextPost(c.targets, now)

		switch {
		case got == nil && c.want == nil:
		case got == nil || c.want == nil || !got.Equal(*c.want):
			t.Errorf("%s: NextPost = %v, want %v", c.name, got, c.want)
		}
	}
}
