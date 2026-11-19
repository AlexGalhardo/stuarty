package skills_test

import (
	"regexp"
	"strings"
	"testing"

	library "github.com/AlexGalhardo/stuarty/core/library"
)

// Every built-in skill passes the skill lint of epic 15 with no finding at all, and its name is its folder.
func TestEveryBuiltInSkillPassesTheLint(t *testing.T) {
	t.Parallel()

	for _, s := range loadAll(t) {
		if s.skill.Name != s.name {
			t.Errorf("%s: the name is %q", s.name, s.skill.Name)
		}

		if f := library.LintSkill(s.name, s.files); len(f) != 0 {
			t.Errorf("%s: the lint reports %+v", s.name, f)
		}

		if o := s.skill.Origin(); o != "" {
			t.Errorf("%s: the file claims the origin %q; the product records it", s.name, o)
		}

		if s.skill.ReviewedMark() != "" {
			t.Errorf("%s: the file carries a review mark", s.name)
		}
	}
}

var (
	// Phrases that tell an agent to go around the approval queue or to approve for a person.
	bypassRE = regexp.MustCompile(
		`(?i)\b(approve (it|them|this|the \w+) yourself|publish (it |them )?(directly|without)|` +
			`skip(ping)? the (approval|queue)|bypass|auto-?publish|without (an )?approval|` +
			`approve (the )?(publication|post|cost|spend|generation))\b`,
	)
	// An imperative verb to approve or to publish at the start of a sentence, a list item or a table cell.
	imperativeRE = regexp.MustCompile(`(?im)(?:^|[.!?:|]\s*|^\s*(?:[-*]|\d+\.)\s+)(approve|publish|post it|send it live)\b`)
	costRE       = regexp.MustCompile(`(?i)\b(publish|publication)`)
)

// No built-in skill tells an agent to approve, to publish without approval or to reveal a credential, and a
// skill that speaks of publishing names the approval queue as the way (RF-389, ADR-0032, ADR-0033).
func TestNoBuiltInSkillBypassesApprovalOrTouchesACredential(t *testing.T) {
	t.Parallel()

	for _, s := range loadAll(t) {
		for _, f := range s.files {
			text := string(f.Data)

			if m := bypassRE.FindString(text); m != "" {
				t.Errorf("%s/%s: %q", s.name, f.Path, m)
			}

			if m := imperativeRE.FindString(text); m != "" {
				t.Errorf("%s/%s: the text starts a sentence with %q", s.name, f.Path, strings.TrimSpace(m))
			}

			if costRE.MatchString(text) && !strings.Contains(strings.ToLower(text), "approval queue") {
				t.Errorf("%s/%s speaks of publishing and does not name the approval queue", s.name, f.Path)
			}
		}

		// The lint already refuses a request for a credential; the words of it must not appear at all.
		for _, f := range library.LintSkill(s.name, s.files) {
			if f.Code == library.LintCredential || f.Code == library.LintRemoteExec || f.Code == library.LintHiddenText {
				t.Errorf("%s: %+v", s.name, f)
			}
		}
	}

	if imperativeRE.MatchString("Publish it now.") == false || bypassRE.MatchString("bypass the queue") == false {
		t.Fatal("the checks of this test do not catch what they are meant to catch")
	}
}
