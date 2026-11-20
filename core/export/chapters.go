package export

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Chapter is one chapter of an exported file: a labelled marker of the timeline up to the next one
// (SDD-004 section 8.1, VV-290). Times are milliseconds from the first exported frame.
type Chapter struct {
	StartMS int64
	EndMS   int64
	Title   string
}

// Chapters plans the chapters of an export of the frames [start, end) of a timeline: every marker of
// the document that has a label and lies inside the range, in order. A chapter lasts until the next
// one starts, the last until the end of the range. Markers with no label (after trimming) are not
// chapters, and a chapter that would last no time is dropped.
func Chapters(markers []timeline.Marker, fps timeline.Rational, start, end int) []Chapter {
	if fps.Num <= 0 || fps.Den <= 0 || end <= start {
		return nil
	}

	type point struct {
		ms    int64
		title string
	}

	var pts []point

	for _, m := range markers {
		title := chapterTitle(m.Label)
		if title == "" || m.Frame < start || m.Frame >= end {
			continue
		}

		pts = append(pts, point{frameMS(m.Frame-start, fps), title})
	}

	total := frameMS(end-start, fps)

	var out []Chapter

	for i, p := range pts {
		stop := total
		if i+1 < len(pts) {
			stop = pts[i+1].ms
		}

		if stop <= p.ms {
			continue
		}

		out = append(out, Chapter{StartMS: p.ms, EndMS: stop, Title: p.title})
	}

	return out
}

// frameMS is the start of a frame in milliseconds, rounded to the nearest.
func frameMS(frame int, fps timeline.Rational) int64 {
	den := int64(fps.Num) * 2

	return (int64(frame)*int64(fps.Den)*2000 + den/2) / den
}

// chapterTitle is a label made safe for a one-line title: control characters become spaces and the
// ends are trimmed.
func chapterTitle(label string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}

		return r
	}, label)

	return strings.TrimSpace(s)
}

// FFMetadata is the text of the ffmetadata file that carries the chapters to FFmpeg (version 1,
// millisecond time base). A title is data, never a key: the characters that mean something in the
// format (`=`, `;`, `#`, `\`) are escaped with a backslash.
func FFMetadata(chapters []Chapter) string {
	var b strings.Builder

	b.WriteString(";FFMETADATA1\n")

	esc := strings.NewReplacer(`\`, `\\`, "=", `\=`, ";", `\;`, "#", `\#`, "\n", `\n`)

	for _, c := range chapters {
		fmt.Fprintf(
			&b,
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n",
			c.StartMS,
			c.EndMS,
			esc.Replace(c.Title),
		)
	}

	return b.String()
}
