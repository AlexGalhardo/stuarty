package publish

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// parseICS is the reader of the test: it unfolds the lines and unescapes the values the way RFC 5545 says.
func parseICS(t *testing.T, body string) []map[string]string {
	t.Helper()

	if strings.Contains(strings.ReplaceAll(body, "\r\n", ""), "\n") {
		t.Fatal("a bare line feed outside a CRLF")
	}

	unfolded := strings.ReplaceAll(body, "\r\n ", "")

	var events []map[string]string

	var cur map[string]string

	for line := range strings.SplitSeq(strings.TrimSuffix(unfolded, "\r\n"), "\r\n") {
		switch line {
		case "BEGIN:VEVENT":
			cur = map[string]string{}
		case "END:VEVENT":
			events = append(events, cur)
			cur = nil
		default:
			if cur == nil {
				continue
			}

			name, value, ok := strings.Cut(line, ":")
			if !ok {
				t.Fatalf("a line with no colon: %q", line)
			}

			cur[name] = value
		}
	}

	if cur != nil {
		t.Fatal("an event that never ends")
	}

	return events
}

func unescapeICS(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++

			if s[i] == 'n' {
				b.WriteByte('\n')

				continue
			}
		}

		b.WriteByte(s[i])
	}

	return b.String()
}

func TestICSListsTheTargetsInUTCWithEscapedText(t *testing.T) {
	t.Parallel()

	now := at(t, "2026-11-25T10:00:00Z")
	events := []Event{
		{
			ID:        "tgt_b",
			At:        at(t, "2026-12-02T15:00:00-03:00"),
			Network:   "youtube",
			Title:     "Second",
			State:     "scheduled",
			Confirmed: true,
		},
		{
			ID: "tgt_a", At: at(t, "2026-12-01T09:00:00Z"), Network: "instagram",
			Title: "Hello, world; a \\ backslash", Text: "line one\nline two", State: "waiting_approval",
		},
	}

	body := WriteICS(events, now)

	if !strings.HasPrefix(body, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n") || !strings.HasSuffix(body, "END:VCALENDAR\r\n") {
		t.Fatalf("not a calendar: %q", body)
	}

	got := parseICS(t, body)
	if len(got) != 2 {
		t.Fatalf("%d events", len(got))
	}

	if got[0]["DTSTART"] != "20261201T090000Z" || got[1]["DTSTART"] != "20261202T180000Z" {
		t.Errorf("times %q, %q (UTC, in the order of the time)", got[0]["DTSTART"], got[1]["DTSTART"])
	}

	if got[0]["STATUS"] != "TENTATIVE" || got[1]["STATUS"] != "CONFIRMED" {
		t.Errorf("status %q, %q", got[0]["STATUS"], got[1]["STATUS"])
	}

	if s := unescapeICS(got[0]["SUMMARY"]); s != "instagram: Hello, world; a \\ backslash" {
		t.Errorf("summary %q", s)
	}

	if d := unescapeICS(got[0]["DESCRIPTION"]); d != "waiting_approval\nline one\nline two" {
		t.Errorf("description %q", d)
	}

	if got[0]["UID"] != ICSUID("tgt_a") || strings.Contains(got[0]["UID"], "tgt_a") {
		t.Errorf("uid %q", got[0]["UID"])
	}

	again := ICSUID("tgt_a")
	if ICSUID("tgt_a") != again || ICSUID("tgt_a") == ICSUID("tgt_b") {
		t.Error("the uid is stable and one per target")
	}
}

func TestICSTextCannotAddALineOrAProperty(t *testing.T) {
	t.Parallel()

	evil := "x\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nATTACH:http://evil.invalid/\r\nUID:y\x00\x1b[31m"
	body := WriteICS([]Event{{ID: "tgt_1", At: at(t, "2026-12-01T09:00:00Z"), Network: "x", Title: evil, Text: evil}},
		at(t, "2026-11-25T10:00:00Z"))

	if n := strings.Count("\r\n"+body, "\r\nBEGIN:VEVENT\r\n"); n != 1 {
		t.Errorf("%d events, the text started another:\n%s", n, body)
	}

	for line := range strings.SplitSeq(body, "\r\n") {
		if strings.HasPrefix(line, "ATTACH") || strings.HasPrefix(line, "UID:y") {
			t.Errorf("a line made by the text: %q", line)
		}
	}

	if strings.ContainsAny(body, "\x00\x1b") {
		t.Error("a control character in the file")
	}

	if got := parseICS(t, body); len(got) != 1 {
		t.Errorf("%d events parsed", len(got))
	}
}

func TestICSFoldsLongLinesInsideTheLimitAndNeverInsideACharacter(t *testing.T) {
	t.Parallel()

	title := strings.Repeat("ação 日本語 ", 30)
	body := WriteICS([]Event{{ID: "tgt_1", At: at(t, "2026-12-01T09:00:00Z"), Network: "x", Title: title}},
		at(t, "2026-11-25T10:00:00Z"))

	for line := range strings.SplitSeq(strings.TrimSuffix(body, "\r\n"), "\r\n") {
		if len(line) > icsLineOctets {
			t.Errorf("a line of %d octets: %q", len(line), line)
		}

		if !utf8.ValidString(line) {
			t.Errorf("a line cut inside a character: %q", line)
		}
	}

	if !strings.Contains(body, "\r\n ") {
		t.Fatal("the long line was not folded")
	}

	got := parseICS(t, body)
	if want := "x: " + string([]rune(title)[:ICSTitleRunes]); unescapeICS(got[0]["SUMMARY"]) != want {
		t.Errorf("the title does not survive the fold: %q", got[0]["SUMMARY"])
	}
}

func TestICSCutsTheWordsOfAPersonAndTakesNothingElse(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 5000)
	body := WriteICS(
		[]Event{{ID: "tgt_secret", At: at(t, "2026-12-01T09:00:00Z"), Network: "x", Title: long, Text: long}},
		at(t, "2026-11-25T10:00:00Z"),
	)

	ev := parseICS(t, body)[0]
	if n := len(unescapeICS(ev["SUMMARY"])); n > len("x: ")+ICSTitleRunes {
		t.Errorf("a title of %d characters", n)
	}

	if n := strings.Count(ev["DESCRIPTION"], "a"); n > ICSTextRunes {
		t.Errorf("a text of %d characters", n)
	}

	if strings.Contains(body, "tgt_secret") || strings.Contains(body, "con_") || strings.Contains(body, "apr_") {
		t.Error("an id of the product is in the file")
	}

	if WriteICS(nil, time.Time{}) == "" {
		t.Error("an empty calendar is still a file")
	}
}
