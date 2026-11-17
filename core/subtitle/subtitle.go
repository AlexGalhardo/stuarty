// Package subtitle reads and writes SubRip (SRT) and WebVTT subtitle text. It is pure: it never opens a
// file, the caller supplies and receives the text (SDD-002 C.70).
package subtitle

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Formats the package reads and writes.
const (
	SRT = "srt"
	VTT = "vtt"
)

// MaxCues bounds the cues of one text, so that a hostile file cannot cost more than it is worth.
const MaxCues = 5000

// Cue is one subtitle: the time it shows and hides, in milliseconds from the start, and its text with
// the line breaks kept as "\n".
type Cue struct {
	StartMs int
	EndMs   int
	Text    string
}

var (
	timeRe = regexp.MustCompile(`^(?:(\d{1,3}):)?(\d{1,2}):(\d{2})[.,](\d{1,3})$`)
	tagRe  = regexp.MustCompile(`<[^>]*>|\{\\[^}]*\}`)
)

// Detect returns VTT for a text that starts with the WEBVTT signature and SRT otherwise.
func Detect(text string) string {
	if strings.HasPrefix(strings.TrimPrefix(text, "\ufeff"), "WEBVTT") {
		return VTT
	}
	return SRT
}

// Parse reads the cues of an SRT or WebVTT text, in file order. Cue numbers, VTT identifiers, settings,
// NOTE, STYLE and REGION blocks and the markup tags are dropped; a cue with no text is skipped. A block
// with a time line that is not valid is an error naming the cue.
func Parse(format, text string) ([]Cue, error) {
	if format != SRT && format != VTT {
		return nil, fmt.Errorf("format %q is not srt or vtt", format)
	}
	text = strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n"), "\r", "\n")
	var cues []Cue
	for block := range strings.SplitSeq(text, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		at := -1
		for i, l := range lines {
			if strings.Contains(l, "-->") {
				at = i
				break
			}
		}
		if at < 0 {
			continue // the header, a NOTE or a block without a time line
		}
		start, end, err := parseTimes(lines[at])
		if err != nil {
			return nil, fmt.Errorf("cue %d: %w", len(cues)+1, err)
		}
		var kept []string
		for _, l := range lines[at+1:] {
			if l = strings.TrimSpace(tagRe.ReplaceAllString(l, "")); l != "" {
				kept = append(kept, l)
			}
		}
		if len(kept) == 0 {
			continue
		}
		if len(cues) == MaxCues {
			return nil, fmt.Errorf("more than %d cues", MaxCues)
		}
		cues = append(cues, Cue{StartMs: start, EndMs: end, Text: strings.Join(kept, "\n")})
	}
	return cues, nil
}

func parseTimes(line string) (start, end int, err error) {
	left, right, _ := strings.Cut(line, "-->")
	right = strings.TrimSpace(right)
	if i := strings.IndexAny(right, " \t"); i >= 0 {
		right = right[:i] // VTT cue settings
	}
	if start, err = parseTime(strings.TrimSpace(left)); err != nil {
		return 0, 0, err
	}
	if end, err = parseTime(right); err != nil {
		return 0, 0, err
	}
	if end <= start {
		return 0, 0, fmt.Errorf("the cue ends at %dms, not after it starts at %dms", end, start)
	}
	return start, end, nil
}

func parseTime(s string) (int, error) {
	m := timeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("%q is not a time (hh:mm:ss,mmm)", s)
	}
	h, _ := strconv.Atoi(m[1])
	mins, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	frac := m[4]
	for len(frac) < 3 {
		frac += "0"
	}
	ms, _ := strconv.Atoi(frac)
	if mins > 59 || sec > 59 {
		return 0, fmt.Errorf("%q has minutes or seconds above 59", s)
	}
	return ((h*60+mins)*60+sec)*1000 + ms, nil
}

// Format writes the cues as SRT (numbered, comma before the milliseconds) or WebVTT (WEBVTT header, a
// dot before the milliseconds).
func Format(format string, cues []Cue) (string, error) {
	if format != SRT && format != VTT {
		return "", fmt.Errorf("format %q is not srt or vtt", format)
	}
	var b strings.Builder
	sep := ","
	if format == VTT {
		b.WriteString("WEBVTT\n\n")
		sep = "."
	}
	for i, c := range cues {
		if format == SRT {
			fmt.Fprintf(&b, "%d\n", i+1)
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", stamp(c.StartMs, sep), stamp(c.EndMs, sep), c.Text)
	}
	return b.String(), nil
}

func stamp(ms int, sep string) string {
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", ms/3600000, ms/60000%60, ms/1000%60, sep, ms%1000)
}
