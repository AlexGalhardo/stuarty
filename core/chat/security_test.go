package chat

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

// SDD-007 T-71: whatever an agent or a person wrote, the Markdown export keeps it inside a fence:
// no text becomes a heading, a link, markup or a way out of its block.
func TestSecurity_T71_MarkdownExportCannotBeBrokenOutOf(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(14, 71))
	pieces := []string{
		"`",
		"``",
		"```",
		"````",
		"#",
		"## ",
		"<",
		">",
		"\n",
		"\r\n",
		" ",
		"[x](javascript:alert(1))",
		"<img src=x>",
		"a",
		"-",
		"\u202e",
		"\x00",
	}

	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	for range 3000 {
		var b strings.Builder

		b.WriteString("MARK")

		for range rng.IntN(30) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}

		b.WriteString("MARK")

		text := b.String()

		md := RenderMarkdown(Transcript{
			SessionID: "cht_0000000001", ProjectID: "prj_0000000001", Profile: "p", Title: text, StartedAt: at,
			Events: []Event{
				{Seq: 1, Kind: KindUserMessage, At: at, Text: text},
				{Seq: 2, Kind: KindAgentMessage, At: at, Text: text},
				{Seq: 3, Kind: KindToolCall, At: at, Tool: &Tool{Title: text, Status: ToolCompleted, Output: text}},
				{Seq: 4, Kind: KindPermissionAsked, At: at, Permission: &Permission{Title: text}},
				{Seq: 5, Kind: KindNotice, At: at, Text: text},
			},
		})

		if leaked := outsideFences(md); strings.Contains(leaked, "MARK") {
			t.Fatalf("text escaped its fence for %q:\n%s", text, md)
		}
	}
}

// outsideFences is what a Markdown reader sees as prose: every line that is not inside a fenced
// block, joined.
func outsideFences(md string) string {
	var out []string

	fence := 0

	for line := range strings.SplitSeq(md, "\n") {
		ticks := len(line) - len(strings.TrimLeft(line, "`"))
		bare := strings.Trim(line, "`") == "" && ticks >= 3

		switch {
		case fence == 0 && bare:
			fence = ticks
		case fence != 0 && bare && ticks >= fence:
			fence = 0
		case fence == 0:
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

// SDD-007 T-74: an answer of a person selects a one-time option or cancels, whatever the agent
// offered; a standing option (allow_always, reject_always) is never selected for the person.
func TestSecurity_T74_PermissionAnswerIsNeverAStandingPermission(t *testing.T) {
	t.Parallel()

	kinds := []OptionKind{AllowOnce, AllowAlways, RejectOnce, RejectAlways, OptionKind("allow_forever"), OptionKind("")}
	rng := rand.New(rand.NewPCG(14, 74))

	for range 3000 {
		var options []Option

		for i := range rng.IntN(6) {
			options = append(
				options,
				Option{ID: "o" + string(rune('a'+i)), Name: "n", Kind: kinds[rng.IntN(len(kinds))]},
			)
		}

		for _, answer := range []Answer{AnswerAllow, AnswerReject, AnswerCancel, Answer("always"), Answer("allow_always"), Answer("")} {
			id, cancelled, err := PickOption(options, answer)
			if err != nil || cancelled {
				continue
			}

			for _, o := range options {
				if o.ID == id && o.Kind != AllowOnce && o.Kind != RejectOnce {
					t.Fatalf("answer %q selected the %s option of %+v", answer, o.Kind, options)
				}
			}

			if answer == AnswerAllow && !hasKind(options, id, AllowOnce) ||
				answer == AnswerReject && !hasKind(options, id, RejectOnce) {
				t.Fatalf("answer %q selected %q of %+v", answer, id, options)
			}
		}
	}
}

func hasKind(options []Option, id string, k OptionKind) bool {
	for _, o := range options {
		if o.ID == id && o.Kind == k {
			return true
		}
	}

	return false
}
