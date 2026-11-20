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
// chapters, and a chapter that would last no time is dropped. When the first labelled marker is not
// at the first exported frame, an untitled chapter covers the time before it: an MP4 chapter list
// starts at 0 (the FFmpeg reader moves a first chapter to 0, and the chapter track has no sample
// before its first), so without it the first marker would be read as starting the file.
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

	if len(out) > 0 && out[0].StartMS > 0 {
		out = append([]Chapter{{StartMS: 0, EndMS: out[0].StartMS}}, out...)
	}

	return out
}

// frameMS is the start of a frame in milliseconds, rounded to the nearest.
func frameMS(frame int, fps timeline.Rational) int64 {
	den := int64(fps.Num) * 2

	return (int64(frame)*int64(fps.Den)*2000 + den/2) / den
}

// chapterTitle is a label made safe for a one-line title (oneLine) with its ends trimmed, and with no
// backslash at its end: the FFmpeg ffmetadata reader takes a backslash before the end of a line as the
// escape of that end, even when the backslash is itself escaped, and the next line of the file then
// joins the title (a label "C:\" would swallow the next chapter).
func chapterTitle(label string) string {
	return strings.TrimRightFunc(strings.TrimSpace(oneLine(label)), func(r rune) bool {
		return r == '\\' || unicode.IsSpace(r)
	})
}

// oneLine replaces with a space every character that could end a line of an ffmetadata file or of a
// player's title: the control characters (the FFmpeg reader ends a line at a NUL and at a CR as well
// as at a LF) and the Unicode line and paragraph separators. Format characters such as the
// bidirectional marks stay: a right-to-left title needs them and they mean nothing to the reader
// (SDD-007 T-102).
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp) {
			return ' '
		}

		return r
	}, s)
}

// FFMetadata is the text of the ffmetadata file that carries the chapters to FFmpeg (version 1,
// millisecond time base). A title is data, never a key, whoever built the chapters: it is made one
// line with no backslash at its end (chapterTitle, which leaves a planned title as it is) and the
// characters that mean something in the format (`=`, `;`, `#`, `\`) are escaped with a backslash
// (SDD-007 T-102).
func FFMetadata(chapters []Chapter) string {
	var b strings.Builder

	b.WriteString(";FFMETADATA1\n")

	esc := strings.NewReplacer(`\`, `\\`, "=", `\=`, ";", `\;`, "#", `\#`)

	for _, c := range chapters {
		fmt.Fprintf(
			&b,
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n",
			c.StartMS,
			c.EndMS,
			esc.Replace(chapterTitle(c.Title)),
		)
	}

	return b.String()
}
