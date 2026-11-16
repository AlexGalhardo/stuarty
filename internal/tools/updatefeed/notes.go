package main

import (
	"regexp"
	"strings"
)

const (
	// maxNotesBytes and maxNoteLines bound the release notes summary of the feed: a launcher shows it in
	// a dialog, so it is a few lines, not the changelog.
	maxNotesBytes = 1200
	maxNoteLines  = 10
)

var markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// summarizeNotes turns the Markdown of a release into a short plain text: the bullets (the headings
// that group them are dropped), links reduced to their text, at most maxNoteLines lines and
// maxNotesBytes bytes, cut at a character boundary.
func summarizeNotes(markdown string) string {
	var lines []string

	for line := range strings.SplitSeq(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") {
			continue
		}

		line = markdownLink.ReplaceAllString(line, "$1")
		line = strings.NewReplacer("**", "", "`", "").Replace(line)

		lines = append(lines, "- "+strings.TrimSpace(line[2:]))
		if len(lines) == maxNoteLines {
			break
		}
	}

	out := strings.Join(lines, "\n")
	if len(out) > maxNotesBytes {
		out = strings.ToValidUTF8(out[:maxNotesBytes], "")
	}

	return out
}
