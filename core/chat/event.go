package chat

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Kind is the kind of a transcript event (SDD-017 section 8).
type Kind string

// The kinds of event.
const (
	KindUserMessage      Kind = "user_message"
	KindAgentMessage     Kind = "agent_message"
	KindAgentThought     Kind = "agent_thought"
	KindToolCall         Kind = "tool_call"
	KindPermissionAsked  Kind = "permission_request"
	KindPermissionAnswer Kind = "permission_answer"
	KindPlan             Kind = "plan"
	KindNotice           Kind = "notice"
	KindState            Kind = "state"
)

// Valid reports whether k is a kind of event.
func (k Kind) Valid() bool {
	switch k {
	case KindUserMessage, KindAgentMessage, KindAgentThought, KindToolCall, KindPermissionAsked,
		KindPermissionAnswer, KindPlan, KindNotice, KindState:
		return true
	}

	return false
}

// ToolStatus is the status of a tool call as the protocol names it.
type ToolStatus string

// The statuses of a tool call.
const (
	ToolPending    ToolStatus = "pending"
	ToolInProgress ToolStatus = "in_progress"
	ToolCompleted  ToolStatus = "completed"
	ToolFailed     ToolStatus = "failed"
)

// Tool is a tool call of the agent. Every member is text from the agent.
type Tool struct {
	CallID string     `json:"tool_call_id"`
	Title  string     `json:"title"`
	Kind   string     `json:"tool_kind,omitempty"`
	Status ToolStatus `json:"status"`
	Output string     `json:"output,omitempty"`
}

// Permission is a permission request and, once a person answered, the answer.
type Permission struct {
	RequestID string   `json:"request_id"`
	CallID    string   `json:"tool_call_id,omitempty"`
	Title     string   `json:"title,omitempty"`
	ToolKind  string   `json:"tool_kind,omitempty"`
	Options   []Option `json:"options,omitempty"`
	Answer    Answer   `json:"answer,omitempty"`
}

// StateInfo is the body of a state event.
type StateInfo struct {
	State     State     `json:"state"`
	EndReason EndReason `json:"end_reason,omitempty"`
}

// Event is one entry of a transcript and one frame of the stream. Text is never markup.
type Event struct {
	Seq        int64       `json:"seq"`
	Kind       Kind        `json:"kind"`
	At         time.Time   `json:"at"`
	Text       string      `json:"text,omitempty"`
	Append     bool        `json:"append,omitempty"`
	Tool       *Tool       `json:"tool,omitempty"`
	Permission *Permission `json:"permission,omitempty"`
	State      *StateInfo  `json:"state,omitempty"`
}

// CleanText makes text from an agent safe to store and to send: valid UTF-8, no NUL or other
// control character but the newline and the tab (a carriage return becomes a newline), and none
// of the characters that reorder what a person reads (the bidirectional overrides and
// isolates) or hide it (the zero-width and tag characters).
func CleanText(s string) string {
	if s == "" {
		return s
	}

	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}

	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == '\r':
			return '\n'
		case unicode.IsControl(r):
			return -1
		case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0x200B, r == 0x200C, r == 0x200D,
			r == 0x2060, r == 0xFEFF, r >= 0xE0000 && r <= 0xE007F:
			return -1
		}

		return r
	}, s)
}

// ClipMarker ends a text that was cut.
const ClipMarker = "\n[cut]"

// Clip cuts s to at most limit bytes at a character boundary and says whether it cut; a cut text
// ends with ClipMarker, which is inside the budget.
func Clip(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}

	keep := max(limit-len(ClipMarker), 0)

	for keep > 0 && !utf8.RuneStart(s[keep]) {
		keep--
	}

	return s[:keep] + ClipMarker, true
}

// Title is the title of a session: the first message of the person, cleaned, with newlines and
// runs of blanks folded to one space, and clipped.
func Title(first string) string {
	line := strings.Join(strings.Fields(CleanText(first)), " ")
	t, _ := Clip(line, MaxTitleLength)

	return t
}
