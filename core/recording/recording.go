// Package recording holds the rules of a screen recording (SDD-023): the state machine, the review of a finished
// recording, the limits and the checks of a request. It is pure: the helper, the disk and the database are the
// business of the layers above it.
package recording

import (
	"errors"
	"fmt"
)

// State is where a recording is in its life (SDD-023 section 3).
type State string

// The states of a recording. Ready, Failed and Cancelled are final.
const (
	Prepared  State = "prepared"
	Recording State = "recording"
	Finishing State = "finishing"
	Ready     State = "ready"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

// Final reports whether no event leaves the state.
func (s State) Final() bool { return s == Ready || s == Failed || s == Cancelled }

// Valid reports whether s is one of the six states.
func (s State) Valid() bool {
	switch s {
	case Prepared, Recording, Finishing, Ready, Failed, Cancelled:
		return true
	}

	return false
}

// Event is what happens to a recording.
type Event string

// The events of the state machine.
const (
	// EventStarted is the helper's started message.
	EventStarted Event = "started"
	// EventStop is a stop requested by a caller or a limit reached.
	EventStop Event = "stop"
	// EventHelperEnded is the helper or the encoder ending with no stop.
	EventHelperEnded Event = "helper_ended"
	// EventFinished is the helper's finished message with a readable file.
	EventFinished Event = "finished"
	// EventFailed is a failure that ends the recording.
	EventFailed Event = "failed"
	// EventCancel discards the recording.
	EventCancel Event = "cancel"
)

// ErrTransition is returned when an event is not allowed in a state.
var ErrTransition = errors.New("recording: the event is not allowed in this state")

// Next is the state after an event, or ErrTransition (wrapped with both names) for any pair the table of
// SDD-023 section 3 does not list.
func Next(from State, ev Event) (State, error) {
	var to State

	switch {
	case from == Prepared && ev == EventStarted:
		to = Recording
	case from == Prepared && ev == EventCancel:
		to = Cancelled
	case from == Prepared && ev == EventFailed:
		to = Failed
	case from == Recording && ev == EventStop:
		to = Finishing
	case from == Recording && ev == EventCancel:
		to = Cancelled
	case from == Recording && (ev == EventHelperEnded || ev == EventFailed):
		to = Failed
	case from == Finishing && ev == EventFinished:
		to = Ready
	case from == Finishing && (ev == EventFailed || ev == EventHelperEnded):
		to = Failed
	case from == Finishing && ev == EventCancel:
		to = Cancelled
	default:
		return from, fmt.Errorf("%w: %q while %q", ErrTransition, ev, from)
	}

	return to, nil
}

// Settle is the state of a recording the server found open at start-up (SDD-023 section 10): Ready when its file
// has a decodable frame, Failed when not. A final recording is an error: there is nothing to settle.
func Settle(from State, hasFrames bool) (State, error) {
	if !from.Valid() || from.Final() {
		return from, fmt.Errorf("%w: settle while %q", ErrTransition, from)
	}

	if hasFrames {
		return Ready, nil
	}

	return Failed, nil
}

// Review is the decision of a person about a finished recording (SDD-023 section 4).
type Review string

// The reviews. A recording that is not Ready has none (the empty string).
const (
	ReviewPending   Review = "pending"
	ReviewAdded     Review = "added"
	ReviewDismissed Review = "dismissed"
)

// Valid reports whether r is one of the three reviews.
func (r Review) Valid() bool {
	return r == ReviewPending || r == ReviewAdded || r == ReviewDismissed
}

// ErrReview is returned when a review change is not allowed.
var ErrReview = errors.New("recording: the review does not allow this")

// Confirm is the review after a person adds the recording to the project: only a ready recording that is
// pending or dismissed.
func Confirm(s State, r Review) (Review, error) {
	if s != Ready || (r != ReviewPending && r != ReviewDismissed) {
		return r, fmt.Errorf("%w: confirm a %q recording that is %q", ErrReview, s, r)
	}

	return ReviewAdded, nil
}

// Dismiss is the review after "Not now": only a ready recording that is pending.
func Dismiss(s State, r Review) (Review, error) {
	if s != Ready || r != ReviewPending {
		return r, fmt.Errorf("%w: dismiss a %q recording that is %q", ErrReview, s, r)
	}

	return ReviewDismissed, nil
}

// Placeable reports whether the recording may be placed on a timeline: a ready one that a person added.
func Placeable(s State, r Review) bool { return s == Ready && r == ReviewAdded }

// CanDelete reports whether a person may delete a finished recording: a ready one that was not added (an added
// one is a library asset and is removed as an asset), or a failed one.
func CanDelete(s State, r Review) bool {
	return (s == Ready && r != ReviewAdded) || s == Failed
}

// VisibleTo reports whether a caller may read the recording (SDD-023 section 4): everyone, except that a
// dismissed recording is for a person only.
func VisibleTo(r Review, person bool) bool { return r != ReviewDismissed || person }
