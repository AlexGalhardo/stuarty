package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const repoURL = "https://github.com/AlexGalhardo/vukem-video"

var (
	versionRe  = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?$`)
	linkLineRe = regexp.MustCompile(`(?m)^\[[^\]]+\]: \S+\n?`)
)

// isPatchRelease reports whether v is a patch bump: its patch number is not 0
// (SDD-009 section 4 item 4: patches never carry a breaking change).
func isPatchRelease(v string) bool {
	m := versionRe.FindStringSubmatch(v)

	return len(m) > 3 && m[3] != "0"
}

// breakingSubjectRe matches a Conventional Commits header marked with "!", and
// breakingFooterRe a "BREAKING CHANGE:" footer.
var (
	breakingSubjectRe = regexp.MustCompile(`^[a-z]+(\([^)]*\))?!:`)
	breakingFooterRe  = regexp.MustCompile(`^BREAKING[ -]CHANGE:`)
)

// breakingSubjects returns the lines of log (full commit messages) that mark a
// breaking change: a header with "!" or a "BREAKING CHANGE:" footer.
func breakingSubjects(log string) []string {
	var out []string

	for line := range strings.SplitSeq(log, "\n") {
		if line = strings.TrimSpace(line); breakingSubjectRe.MatchString(line) || breakingFooterRe.MatchString(line) {
			out = append(out, line)
		}
	}

	return out
}

// isMajorBump reports whether version raises the major number over previous.
// An empty previous counts as one: there is nothing to break.
func isMajorBump(version, previous string) bool {
	return previous == "" || core(version)[0] > core(previous)[0]
}

// stable reports whether v is at 1.0 or later, where a breaking change
// needs a major bump (SDD-009 section 4, ADR-0025 item 1).
func stable(v string) bool { return core(v)[0] >= 1 }

// validVersion reports whether v is a SemVer tag with a leading "v".
func validVersion(v string) bool { return versionRe.MatchString(v) }

// newerThan reports whether version is a higher SemVer than previous (both
// "vX.Y.Z" tags; a pre-release suffix is ignored). An empty previous is
// older than any version.
func newerThan(version, previous string) bool {
	if previous == "" {
		return true
	}

	a, b := core(version), core(previous)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}

	return false
}

func core(v string) [3]int {
	m := versionRe.FindStringSubmatch(v)

	var out [3]int

	if m == nil {
		return out
	}

	for i := range out {
		out[i], _ = strconv.Atoi(m[i+1])
	}

	return out
}

// insertRelease adds section (a "## [x.y.z] - date" block) right below the
// "## [Unreleased]" heading and rewrites the link references at the end of
// the file. Released sections and the links of older releases are kept as
// they are: the Studio links keep the repository name they were written with.
func insertRelease(changelog, section, version, previous string) (string, error) {
	const unreleased = "## [Unreleased]"

	idx := strings.Index(changelog, unreleased)
	if idx < 0 {
		return "", errors.New("CHANGELOG.md has no \"## [Unreleased]\" heading")
	}

	bare := strings.TrimPrefix(version, "v")
	if strings.Contains(changelog, "## ["+bare+"]") {
		return "", fmt.Errorf("CHANGELOG.md already has a section for %s", bare)
	}

	section = strings.TrimSpace(section)
	if !strings.HasPrefix(section, "## ["+bare+"]") {
		return "", fmt.Errorf("generated section does not start with \"## [%s]\"", bare)
	}

	oldLinks := linkLineRe.FindAllString(changelog, -1)
	body := strings.TrimRight(linkLineRe.ReplaceAllString(changelog, ""), "\n")

	after := idx + len(unreleased)

	// Keep any [Unreleased] notes written by hand (rare) above the release.
	nextSection := strings.Index(body[after:], "\n## [")

	var unreleasedNotes, rest string
	if nextSection < 0 {
		unreleasedNotes, rest = body[after:], ""
	} else {
		unreleasedNotes, rest = body[after:after+nextSection], body[after+nextSection:]
	}

	var b strings.Builder

	b.WriteString(body[:after])

	if notes := strings.TrimSpace(unreleasedNotes); notes != "" {
		b.WriteString("\n\n" + notes)
	}

	b.WriteString("\n\n" + section + "\n")
	b.WriteString(strings.TrimRight(rest, "\n"))
	b.WriteString("\n\n")
	b.WriteString(links(oldLinks, version, previous))

	return b.String(), nil
}

// links renders the link references: a fresh [Unreleased] and one for the new
// version, then every older line unchanged (the old [Unreleased] is dropped).
func links(old []string, latest, previous string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "[Unreleased]: %s/compare/%s...HEAD\n", repoURL, latest)

	if previous == "" {
		fmt.Fprintf(&b, "[%s]: %s/releases/tag/%s\n", strings.TrimPrefix(latest, "v"), repoURL, latest)
	} else {
		fmt.Fprintf(&b, "[%s]: %s/compare/%s...%s\n", strings.TrimPrefix(latest, "v"), repoURL, previous, latest)
	}

	for _, line := range old {
		if strings.HasPrefix(line, "[Unreleased]:") {
			continue
		}

		b.WriteString(strings.TrimRight(line, "\n") + "\n")
	}

	return b.String()
}

// releaseNotes extracts the body of version's section, without its heading.
// It is the text release.yml publishes: the same section, cut at the next
// heading or at the link block.
func releaseNotes(changelog, version string) string {
	heading := "## [" + strings.TrimPrefix(version, "v") + "]"

	start := strings.Index(changelog, heading)
	if start < 0 {
		return ""
	}

	rest := changelog[start:]

	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		return ""
	}

	rest = rest[nl+1:]

	end := len(rest)
	if i := strings.Index(rest, "\n## ["); i >= 0 {
		end = i
	}

	if loc := linkLineRe.FindStringIndex(rest[:end]); loc != nil {
		end = loc[0]
	}

	notes := strings.TrimSpace(rest[:end])
	if notes == "" {
		return ""
	}

	return notes + "\n"
}
