package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The .ics export of the calendar (SDD-020 section 30A.5): read only, with times in UTC, escaped and folded text and
// nothing that grants access.

const (
	// ICSTitleRunes cuts the title of a post before it is written.
	ICSTitleRunes = 200
	// ICSTextRunes cuts the text of a post before it is written.
	ICSTextRunes  = 500
	icsLineOctets = 75
	icsUIDHost    = "stuarty.invalid"
)

// Event is one target with a time, as the file shows it.
type Event struct {
	// ID is the id of the target. It never reaches the file: the UID is a digest of it.
	ID      string
	At      time.Time
	Network string
	Title   string
	Text    string
	// State is the state of the target, in words a person reads.
	State string
	// Confirmed is true once a person approved the post.
	Confirmed bool
}

// ICSUID is the stable identifier of the event of a target: the start of a digest, so it reveals neither the id
// nor the host.
func ICSUID(targetID string) string {
	sum := sha256.Sum256([]byte("stuarty-ics:" + targetID))

	return hex.EncodeToString(sum[:])[:32] + "@" + icsUIDHost
}

// ICSText makes a person's words safe for a value of the file: control characters go, the text is cut to max
// characters and the characters RFC 5545 reserves are escaped. A line break becomes \n.
func ICSText(s string, maxRunes int) string {
	var b strings.Builder

	n := 0

	for _, r := range strings.ToValidUTF8(s, "") {
		if n >= maxRunes {
			break
		}

		switch {
		case r == '\n' || r == '\r':
			if r == '\r' {
				continue
			}

			b.WriteString(`\n`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == ';':
			b.WriteString(`\;`)
		case r == ',':
			b.WriteString(`\,`)
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
		}

		n++
	}

	return b.String()
}

// icsFold folds a content line at 75 octets with a CRLF and a space, never inside a UTF-8 sequence.
func icsFold(line string) string {
	if len(line) <= icsLineOctets {
		return line
	}

	var b strings.Builder

	size := 0

	for _, r := range line {
		w := utf8.RuneLen(r)
		if size+w > icsLineOctets {
			b.WriteString("\r\n ")

			// The space that starts a continuation counts toward its 75 octets.
			size = 1
		}

		b.WriteRune(r)

		size += w
	}

	return b.String()
}

func icsStamp(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// WriteICS writes the events as an iCalendar file with CRLF line ends, in the order of their time. now is the
// stamp of the file.
func WriteICS(events []Event, now time.Time) string {
	sorted := slices.SortedStableFunc(slices.Values(events), func(a, b Event) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}

		return strings.Compare(ICSUID(a.ID), ICSUID(b.ID))
	})

	var b strings.Builder

	line := func(s string) { b.WriteString(icsFold(s) + "\r\n") }

	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Stuarty//Publishing calendar//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:Stuarty publishing")

	for _, e := range sorted {
		status := "TENTATIVE"
		if e.Confirmed {
			status = "CONFIRMED"
		}

		summary := ICSText(e.Network, 40)
		if e.Title != "" {
			summary += ": " + ICSText(e.Title, ICSTitleRunes)
		}

		desc := ICSText(e.State, 40)
		if e.Text != "" {
			desc += `\n` + ICSText(e.Text, ICSTextRunes)
		}

		line("BEGIN:VEVENT")
		line("UID:" + ICSUID(e.ID))
		line("DTSTAMP:" + icsStamp(now))
		line("DTSTART:" + icsStamp(e.At))
		line("DURATION:PT15M")
		line("SUMMARY:" + summary)
		line("DESCRIPTION:" + desc)
		line("STATUS:" + status)
		line("TRANSP:TRANSPARENT")
		line("END:VEVENT")
	}

	line("END:VCALENDAR")

	return b.String()
}
