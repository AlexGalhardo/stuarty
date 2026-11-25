package recording

import (
	"errors"
	"fmt"
	"slices"
)

// ProposalState is where a proposal of cuts is.
type ProposalState string

// The states of a proposal. Accepted and Rejected are final.
const (
	ProposalPending  ProposalState = "pending"
	ProposalAccepted ProposalState = "accepted"
	ProposalRejected ProposalState = "rejected"
)

// Decision is what a person decided about one range.
type Decision string

// The decisions of a range; the empty string is not decided yet.
const (
	DecisionAccepted Decision = "accepted"
	DecisionRejected Decision = "rejected"
)

// CutRange is a stretch of a take to cut out, in active milliseconds of the recording, with the reason the
// proposer gave. The reason is untrusted text.
type CutRange struct {
	FromMS   int64    `json:"from_ms"`
	ToMS     int64    `json:"to_ms"`
	Reason   string   `json:"reason"`
	Decision Decision `json:"decision,omitempty"`
}

// Proposal is a list of ranges to cut, made by any caller and decided by a person (SDD-025 section 19.2).
type Proposal struct {
	ID        string        `json:"id"`
	Actor     string        `json:"actor"`
	CreatedMs int64         `json:"created_ms"`
	State     ProposalState `json:"state"`
	Ranges    []CutRange    `json:"ranges"`
	DecidedBy string        `json:"decided_by,omitempty"`
	DecidedMs int64         `json:"decided_ms,omitempty"`
	// Revision is the revision of the timeline the accepted ranges produced; 0 when nothing was applied.
	Revision int64 `json:"revision,omitempty"`
}

// ErrProposalState is returned when a proposal is not pending.
var ErrProposalState = errors.New("recording: the proposal is not pending")

// ErrDecisions is returned when the decisions do not match the ranges of the proposal.
var ErrDecisions = errors.New("recording: there must be one decision for every range")

// ValidateRanges checks the ranges of a new proposal against the span they may cut, [loMS, hiMS]: the part of
// the take every track shares. Every range is sorted after the one before and does not overlap it, is at least
// MinRangeMS long and has a reason. It returns every refused field, named `ranges[i].field`.
func ValidateRanges(rs []CutRange, loMS, hiMS int64) []FieldError {
	if len(rs) == 0 || len(rs) > MaxRanges {
		return []FieldError{{"ranges", "count"}}
	}

	var out []FieldError

	prevEnd := int64(-1)

	for i, r := range rs {
		at := func(f string) string { return fmt.Sprintf("ranges[%d].%s", i, f) }

		switch {
		case r.FromMS < loMS || r.ToMS > hiMS:
			out = append(out, FieldError{at("from_ms"), "out_of_range"})
		case r.ToMS-r.FromMS < MinRangeMS:
			out = append(out, FieldError{at("to_ms"), "too_short"})
		case r.FromMS < prevEnd:
			out = append(out, FieldError{at("from_ms"), "overlaps"})
		}

		if reason := CleanLabel(r.Reason, MaxNoteRunes); reason == "" || len([]rune(r.Reason)) > MaxNoteRunes {
			out = append(out, FieldError{at("reason"), "invalid"})
		}

		prevEnd = r.ToMS
	}

	return out
}

// CheckNewProposal says whether a recording that holds the proposals can take one more.
func CheckNewProposal(have []Proposal, s State) error {
	if s != Ready {
		return fmt.Errorf("%w: a proposal for a %q recording", ErrState, s)
	}

	pending := 0

	for i := range have {
		if have[i].State == ProposalPending {
			pending++
		}
	}

	if pending >= MaxPendingProposals || len(have) >= MaxProposals {
		return &LimitError{Limit: LimitProposals, Ceiling: MaxPendingProposals}
	}

	return nil
}

// Decide applies the decisions of a person to a pending proposal: accept[i] is the decision of range i. It
// returns the decided proposal and the accepted ranges, in order. The proposal is accepted when at least one
// range is, and rejected when none is. The proposal given is not changed.
func Decide(p Proposal, accept []bool, by string, atMs int64) (Proposal, []CutRange, error) {
	if p.State != ProposalPending {
		return p, nil, fmt.Errorf("%w: it is %q", ErrProposalState, p.State)
	}

	if len(accept) != len(p.Ranges) {
		return p, nil, fmt.Errorf("%w: %d ranges, %d decisions", ErrDecisions, len(p.Ranges), len(accept))
	}

	out := p
	out.Ranges = slices.Clone(p.Ranges)

	var accepted []CutRange

	for i, yes := range accept {
		if yes {
			out.Ranges[i].Decision = DecisionAccepted
			accepted = append(accepted, out.Ranges[i])
		} else {
			out.Ranges[i].Decision = DecisionRejected
		}
	}

	out.State = ProposalRejected
	if len(accepted) > 0 {
		out.State = ProposalAccepted
	}

	out.DecidedBy, out.DecidedMs = by, atMs

	return out, accepted, nil
}

// SharedSpan is the part of a take that every track holds, [lo, hi] in active milliseconds of the shared clock:
// from the latest start to the earliest end. With no track it is [0, durationMS].
func SharedSpan(tracks []Track, durationMS int64) (lo, hi int64) {
	if len(tracks) == 0 {
		return 0, durationMS
	}

	lo, hi = tracks[0].StartMS, tracks[0].StartMS+tracks[0].DurationMS

	for _, t := range tracks[1:] {
		lo, hi = max(lo, t.StartMS), min(hi, t.StartMS+t.DurationMS)
	}

	if lo > hi {
		return 0, 0
	}

	return lo, hi
}
