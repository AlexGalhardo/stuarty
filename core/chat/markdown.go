package chat

import (
	"fmt"
	"strings"
	"time"
)

// Transcript is a session with its events, as the export reads it.
type Transcript struct {
	SessionID string
	ProjectID string
	Profile   string
	Title     string
	StartedAt time.Time
	EndedAt   *time.Time
	EndReason EndReason
	Events    []Event
}

// RenderMarkdown writes a transcript as Markdown (SDD-017 section 9). Everything an agent or a
// person wrote goes inside a fenced block whose fence is longer than any run of backticks in
// the text, so no text can become a heading, a link or HTML when the file is opened; only
// identifiers, names that passed their own rules and times are written outside a fence, and
// those are reduced to a safe alphabet first.
func RenderMarkdown(t Transcript) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Chat session %s\n\n", safeToken(t.SessionID))
	fmt.Fprintf(&b, "- Project: %s\n", safeToken(t.ProjectID))
	fmt.Fprintf(&b, "- Agent profile: %s\n", safeToken(t.Profile))
	fmt.Fprintf(&b, "- Started: %s\n", stamp(t.StartedAt))

	if t.EndedAt != nil {
		fmt.Fprintf(&b, "- Ended: %s (%s)\n", stamp(*t.EndedAt), safeToken(string(t.EndReason)))
	}

	if t.Title != "" {
		b.WriteString("\nTitle:\n\n")
		b.WriteString(fence(t.Title))
	}

	for i := range t.Events {
		renderEvent(&b, &t.Events[i])
	}

	return b.String()
}

func renderEvent(b *strings.Builder, e *Event) {
	head := func(who string) {
		fmt.Fprintf(b, "\n## %s, %s\n\n", who, stamp(e.At))
	}

	switch e.Kind {
	case KindUserMessage:
		head("You")
		b.WriteString(fence(e.Text))
	case KindAgentMessage:
		head("Agent")
		b.WriteString(fence(e.Text))
	case KindAgentThought:
		head("Agent (thinking)")
		b.WriteString(fence(e.Text))
	case KindPlan:
		head("Agent plan")
		b.WriteString(fence(e.Text))
	case KindToolCall:
		head("Tool call")

		if e.Tool != nil {
			fmt.Fprintf(b, "Status: %s\n\n", safeToken(string(e.Tool.Status)))
			b.WriteString(fence(e.Tool.Title))

			if e.Tool.Output != "" {
				b.WriteString("\nOutput:\n\n")
				b.WriteString(fence(e.Tool.Output))
			}
		}
	case KindPermissionAsked, KindPermissionAnswer:
		head("Permission")

		if e.Permission != nil {
			b.WriteString(fence(e.Permission.Title))

			if e.Permission.Answer != "" {
				fmt.Fprintf(b, "\nAnswer: %s\n", safeToken(string(e.Permission.Answer)))
			}
		}
	case KindState:
		if e.State != nil {
			fmt.Fprintf(b, "\n_State: %s %s_\n", safeToken(string(e.State.State)), safeToken(string(e.State.EndReason)))
		}
	default:
		head("Notice")
		b.WriteString(fence(e.Text))
	}
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// safeToken keeps the characters of an identifier and drops the rest.
func safeToken(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
			return r
		}

		return -1
	}, s)
}

// fence wraps text in a fenced block that the text cannot close: the fence is one backtick
// longer than the longest run in the text, and at least three.
func fence(text string) string {
	text = CleanText(text)

	longest, run := 0, 0

	for _, r := range text {
		if r == '`' {
			run++

			if run > longest {
				longest = run
			}

			continue
		}

		run = 0
	}

	f := strings.Repeat("`", max(3, longest+1))

	return f + "\n" + strings.TrimRight(text, "\n") + "\n" + f + "\n"
}
