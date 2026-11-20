package chat

import "errors"

// OptionKind is the kind the protocol gives to an option of a permission request.
type OptionKind string

// The kinds of the Agent Client Protocol.
const (
	AllowOnce    OptionKind = "allow_once"
	AllowAlways  OptionKind = "allow_always"
	RejectOnce   OptionKind = "reject_once"
	RejectAlways OptionKind = "reject_always"
)

// Option is one choice the agent offered.
type Option struct {
	ID   string     `json:"option_id"`
	Name string     `json:"name"`
	Kind OptionKind `json:"kind"`
}

// Answer is what a person may answer (SDD-017 section 7): allow once, reject once or cancel.
// A standing choice is not an answer: a permission that outlives the call is a rule, and
// rules are written in the Approvals page (SDD-015).
type Answer string

// The answers.
const (
	AnswerAllow  Answer = "allow"
	AnswerReject Answer = "reject"
	AnswerCancel Answer = "cancel"
)

// Valid reports whether a is an answer a person may give.
func (a Answer) Valid() bool { return a == AnswerAllow || a == AnswerReject || a == AnswerCancel }

// The refusals of PickOption.
var (
	ErrAnswer          = errors.New("the answer is allow, reject or cancel")
	ErrNoOneTimeOption = errors.New("the agent offered no one-time option to allow")
)

// PickOption maps an answer to what is sent to the agent: the id of the option to select, or
// cancelled. It never selects a standing option. A reject with no one-time reject option is
// sent as cancelled, which the protocol defines as a refusal; an allow with no one-time allow
// option is refused, so a person is never made to grant more than they chose.
func PickOption(options []Option, a Answer) (optionID string, cancelled bool, err error) {
	switch a {
	case AnswerCancel:
		return "", true, nil
	case AnswerAllow:
		for _, o := range options {
			if o.Kind == AllowOnce {
				return o.ID, false, nil
			}
		}

		return "", false, ErrNoOneTimeOption
	case AnswerReject:
		for _, o := range options {
			if o.Kind == RejectOnce {
				return o.ID, false, nil
			}
		}

		return "", true, nil
	}

	return "", false, ErrAnswer
}

// CleanOptions keeps the options a client may show: at most MaxOptions, with their ids and
// names cleaned and clipped, an unknown kind or an empty id dropped.
func CleanOptions(in []Option) []Option {
	out := make([]Option, 0, len(in))

	for _, o := range in {
		switch o.Kind {
		case AllowOnce, AllowAlways, RejectOnce, RejectAlways:
		default:
			continue
		}

		id, _ := Clip(CleanText(o.ID), MaxOptionName)
		name, _ := Clip(CleanText(o.Name), MaxOptionName)

		if id == "" {
			continue
		}

		out = append(out, Option{ID: id, Name: name, Kind: o.Kind})
		if len(out) == MaxOptions {
			break
		}
	}

	return out
}
