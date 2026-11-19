package publish

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	// The zone database travels with the binary: a creator's machine has no GOROOT, and a schedule that
	// reads its zone wrongly sends a post at the wrong hour (SDD-020 section 17).
	_ "time/tzdata"
)

// TargetState is the state of one target of a publication (SDD-020 section 16).
type TargetState string

// The states of a target.
const (
	TargetDraft           TargetState = "draft"
	TargetWaitingApproval TargetState = "waiting_approval"
	TargetApproved        TargetState = "approved"
	TargetScheduled       TargetState = "scheduled"
	TargetPublishing      TargetState = "publishing"
	TargetPublished       TargetState = "published"
	TargetFailed          TargetState = "failed"
	TargetMissed          TargetState = "missed"
	TargetCancelled       TargetState = "cancelled"
)

// TargetStates lists every state of a target.
var TargetStates = []TargetState{
	TargetDraft, TargetWaitingApproval, TargetApproved, TargetScheduled, TargetPublishing, TargetPublished,
	TargetFailed, TargetMissed, TargetCancelled,
}

var targetMoves = map[TargetState][]TargetState{
	TargetDraft: {TargetWaitingApproval, TargetCancelled},
	TargetWaitingApproval: {
		TargetApproved,
		TargetScheduled,
		TargetMissed,
		TargetDraft,
		TargetWaitingApproval,
		TargetCancelled,
	},
	TargetApproved:   {TargetPublishing, TargetMissed, TargetWaitingApproval, TargetCancelled},
	TargetScheduled:  {TargetPublishing, TargetMissed, TargetWaitingApproval, TargetCancelled},
	TargetPublishing: {TargetPublished, TargetFailed, TargetScheduled},
	TargetFailed:     {TargetWaitingApproval, TargetCancelled},
	TargetMissed:     {TargetWaitingApproval, TargetCancelled},
}

// CanTarget says whether a target may move from one state to another. A final state moves nowhere.
func CanTarget(from, to TargetState) bool { return slices.Contains(targetMoves[from], to) }

// Final says whether a state is the end of a target.
func (s TargetState) Final() bool { return s == TargetPublished || s == TargetCancelled }

// Waiting says whether a person's yes was asked or given and the post has not gone yet: editing such a
// target makes it another post, so it goes back to waiting_approval (SDD-020 section 16).
func (s TargetState) Waiting() bool {
	return s == TargetWaitingApproval || s == TargetApproved || s == TargetScheduled
}

// Editable says whether the fields of a target may change.
func (s TargetState) Editable() bool { return s != TargetPublishing && s != TargetPublished }

// The timing constants (SDD-020 sections 17 and 19).
const (
	// Grace is how late the loop may be woken and still be on time.
	Grace = 60 * time.Second
	// MaxLateWindowMinutes is the longest "send late up to" a person can choose: a day.
	MaxLateWindowMinutes = 1440
	// Lease is how long a scheduler holds a target it is sending without renewing.
	Lease = 2 * time.Minute
	// MaxAttempts is the most sends of one target, the first included.
	MaxAttempts = 5
)

// Adjustment says what a local time had to do to become an instant.
type Adjustment string

// The adjustments of LocalInstant.
const (
	// AdjustNone: the wall time exists once.
	AdjustNone Adjustment = ""
	// AdjustGap: the wall time does not exist, the clock jumped over it; the instant is the one after the jump.
	AdjustGap Adjustment = "gap"
	// AdjustAmbiguous: the wall time happens twice, the clock went back; the first of the two was taken.
	AdjustAmbiguous Adjustment = "ambiguous"
)

// ErrTime is a time or a zone that cannot be read.
var ErrTime = errors.New("publish: invalid time")

// LoadZone reads an IANA zone name. An empty name is UTC.
func LoadZone(name string) (*time.Location, error) {
	if name == "" || strings.EqualFold(name, "utc") {
		return time.UTC, nil
	}

	// "Local" would be the zone of the server, which is not a name another machine can read back.
	if strings.EqualFold(name, "local") || len(name) > 64 || strings.ContainsAny(name, "\\ \t\r\n") ||
		strings.Contains(name, "..") {
		return nil, fmt.Errorf("%w: %q is not a zone name such as America/Sao_Paulo", ErrTime, name)
	}

	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a zone name such as America/Sao_Paulo", ErrTime, name)
	}

	return loc, nil
}

var localLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02 15:04:05"}

// LocalInstant reads a local date and time in a zone as an instant. A wall time that does not exist (the
// clock jumped forward over it) becomes the instant after the jump, and one that happens twice (the clock
// went back) the first of them; the adjustment says which.
func LocalInstant(local string, loc *time.Location) (time.Time, Adjustment, error) {
	var (
		wall time.Time
		err  error
	)

	for _, layout := range localLayouts {
		if wall, err = time.Parse(layout, local); err == nil {
			break
		}
	}

	if err != nil {
		return time.Time{}, AdjustNone, fmt.Errorf(
			"%w: %q is not a date and time such as 2026-11-20T09:00",
			ErrTime,
			local,
		)
	}

	// The wall time read as if it were UTC; the offsets in force a day before and after bracket any change
	// of the clock that could touch it.
	utc := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, time.UTC)
	_, before := utc.Add(-36 * time.Hour).In(loc).Zone()
	_, after := utc.Add(36 * time.Hour).In(loc).Zone()

	var found []time.Time

	for _, off := range []int{before, after} {
		inst := utc.Add(-time.Duration(off) * time.Second)
		if sameWall(inst.In(loc), utc) && !slices.ContainsFunc(found, inst.Equal) {
			found = append(found, inst)
		}
	}

	switch len(found) {
	case 0:
		return utc.Add(-time.Duration(before) * time.Second).UTC(), AdjustGap, nil
	case 1:
		return found[0].UTC(), AdjustNone, nil
	default:
		first := found[0]
		if found[1].Before(first) {
			first = found[1]
		}

		return first.UTC(), AdjustAmbiguous, nil
	}
}

func sameWall(a, wall time.Time) bool {
	return a.Year() == wall.Year() && a.Month() == wall.Month() && a.Day() == wall.Day() &&
		a.Hour() == wall.Hour() && a.Minute() == wall.Minute() && a.Second() == wall.Second()
}

// Slot is a time as a person chose it: the instant, in UTC, and the zone they thought it in.
type Slot struct {
	At         time.Time  `json:"at"`
	Zone       string     `json:"zone"`
	Adjustment Adjustment `json:"adjustment,omitempty"`
}

// ParseSlot reads what a person or an agent wrote: an instant with an offset or Z (RFC 3339), or a local
// date and time read in zone. defaultZone is used when zone is empty; an instant keeps the zone the
// caller named, or defaultZone, as the zone to show it in.
func ParseSlot(at, zone, defaultZone string) (Slot, error) {
	at = strings.TrimSpace(at)
	if at == "" {
		return Slot{}, fmt.Errorf("%w: a time is needed", ErrTime)
	}

	if zone == "" {
		zone = defaultZone
	}

	if zone == "" {
		zone = "UTC"
	}

	loc, err := LoadZone(zone)
	if err != nil {
		return Slot{}, err
	}

	if t, perr := time.Parse(time.RFC3339, at); perr == nil {
		return Slot{At: t.UTC(), Zone: zone}, nil
	}

	inst, adj, err := LocalInstant(at, loc)
	if err != nil {
		return Slot{}, err
	}

	return Slot{At: inst, Zone: zone, Adjustment: adj}, nil
}

// Local is the instant as the wall clock of its zone shows it, for a person to read.
func (s Slot) Local() string {
	loc, err := LoadZone(s.Zone)
	if err != nil {
		loc = time.UTC
	}

	return s.At.In(loc).Format("2006-01-02 15:04")
}

// Action is what the scheduler does with a target at a moment.
type Action string

// The actions of Decide.
const (
	// ActionWait: nothing to do yet.
	ActionWait Action = "wait"
	// ActionSend: the target is due and on time.
	ActionSend Action = "send"
	// ActionResume: the target was being sent and its lease ran out; send it again with the same key.
	ActionResume Action = "resume"
	// ActionMiss: the time passed and the person did not allow a late send.
	ActionMiss Action = "miss"
)

// Candidate is the part of a target the schedule rules read.
type Candidate struct {
	ID    string
	State TargetState
	// At is the time the person chose.
	At time.Time
	// LateWindowMinutes is how late after At the person allowed the post to go without asking.
	LateWindowMinutes int
	// Attempts are the sends made so far, an interrupted one included.
	Attempts int
	// NextAttempt is when the next automatic retry may go; zero when none is waiting.
	NextAttempt time.Time
	// LeaseUntil is when the lease of a publishing target ends.
	LeaseUntil time.Time
}

// OnTime says whether now is within the grace and the late window of a time.
func OnTime(at time.Time, lateMinutes int, now time.Time) bool {
	return !now.After(at.Add(Grace + time.Duration(lateMinutes)*time.Minute))
}

// EffectiveLate is the late window a target is judged by: the one the person chose, or a day for a target
// they asked to send now, since a person answers the request later than the instant it names.
func EffectiveLate(minutes int, sendNow bool) int {
	if sendNow {
		return MaxLateWindowMinutes
	}

	return minutes
}

// Decide is what to do with one target at now (SDD-020 sections 17 and 19). A target that is already
// being retried is bounded by its attempts and not by its first time; a target that was being sent
// resumes; one whose time passed beyond its window is missed and never sent on its own (RF-406).
func Decide(t Candidate, now time.Time) Action {
	if t.State == TargetPublishing {
		if !t.LeaseUntil.IsZero() && t.LeaseUntil.Before(now) {
			return ActionResume
		}

		return ActionWait
	}

	if t.State != TargetApproved && t.State != TargetScheduled {
		return ActionWait
	}

	if t.Attempts > 0 && !t.NextAttempt.IsZero() {
		if t.NextAttempt.After(now) {
			return ActionWait
		}

		return ActionSend
	}

	if t.At.After(now) {
		return ActionWait
	}

	if !OnTime(t.At, t.LateWindowMinutes, now) {
		return ActionMiss
	}

	return ActionSend
}

// Due returns the targets to send at now, the ones to resume included, in the order of their time.
func Due(ts []Candidate, now time.Time) []Candidate {
	var out []Candidate

	for i := range ts {
		if a := Decide(ts[i], now); a == ActionSend || a == ActionResume {
			out = append(out, ts[i])
		}
	}

	slices.SortStableFunc(out, func(a, b Candidate) int { return a.At.Compare(b.At) })

	return out
}

// Missed returns the targets whose time passed beyond their window at now.
func Missed(ts []Candidate, now time.Time) []Candidate {
	var out []Candidate

	for i := range ts {
		if Decide(ts[i], now) == ActionMiss {
			out = append(out, ts[i])
		}
	}

	return out
}

// Wake is the next moment something about the target needs the scheduler, or false when nothing does: the
// time of a target that waits, the next attempt of one that failed in a transient way, or the end of a
// lease.
func Wake(t Candidate) (time.Time, bool) {
	if t.State == TargetPublishing && !t.LeaseUntil.IsZero() {
		return t.LeaseUntil, true
	}

	if t.State != TargetApproved && t.State != TargetScheduled {
		return time.Time{}, false
	}

	if t.Attempts > 0 && !t.NextAttempt.IsZero() {
		return t.NextAttempt, true
	}

	return t.At, true
}

// Transient says whether a kind of error of a connector may pass by itself: the scheduler retries only
// those. A quota comes back at a time of the network's, a refused sign-in and a refused video need a
// person, so the person retries them (SDD-020 section 19).
func Transient(kind string) bool { return kind == "unavailable" }

var retryDelays = []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}

// RetryDelay is how long to wait after the attempts-th failed send, or false when the attempts are used.
func RetryDelay(attempts int) (time.Duration, bool) {
	if attempts < 1 || attempts >= MaxAttempts || attempts > len(retryDelays) {
		return 0, false
	}

	return retryDelays[attempts-1], true
}
