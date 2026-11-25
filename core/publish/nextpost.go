package publish

import "time"

// Planned is the part of a target that tells when it goes out: its state and its instant.
type Planned struct {
	State       TargetState
	ScheduledAt *time.Time
}

// NextPost is the instant of the next post of a project: the earliest scheduled instant, not yet past, of a
// target that still goes out (waiting for approval, approved or scheduled). A target that has been published,
// failed, missed or cancelled is not next, nor is a draft that has no instant. It is nil when there is none
// (SDD-020 section 47).
func NextPost(targets []Planned, now time.Time) *time.Time {
	var next *time.Time

	for i := range targets {
		t := &targets[i]

		switch t.State {
		case TargetWaitingApproval, TargetApproved, TargetScheduled:
		default:
			continue
		}

		if t.ScheduledAt == nil || t.ScheduledAt.Before(now) {
			continue
		}

		if next == nil || t.ScheduledAt.Before(*next) {
			at := *t.ScheduledAt
			next = &at
		}
	}

	return next
}
