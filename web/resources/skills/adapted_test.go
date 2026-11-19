package skills_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The set of epic 16: the skills that lead a creator and an agent through the first flow (SDD-018 section 16), and
// the skill of epic 33 for an animation made by an external motion engine.
var wantSkills = []string{
	"start-here", "brief-a-production", "short-vertical-video", "captions-for-shorts", "thumbnail",
	"recall-learnings", "production-retro", "fetch-clip", "write-a-hook", "short-script", "caption-per-network",
	"motion-with-an-external-engine",
}

func TestTheSetIsComplete(t *testing.T) {
	t.Parallel()

	var have []string
	for _, s := range loadAll(t) {
		have = append(have, s.name)
	}

	slices.Sort(have)

	want := slices.Clone(wantSkills)
	slices.Sort(want)

	if !slices.Equal(have, want) {
		t.Fatalf("the built-in skills are\n %v\nwant\n %v", have, want)
	}
}

// The sources D-071 approved: each has its licence text in licenses/vendored/<id>-<SPDX id>.txt.
var sources = map[string]string{
	"marketingskills": "coreyhaines31/marketingskills",
	"hyperframes":     "heygen-com/hyperframes",
}

const adaptedFrom = "stuarty-adapted-from"

// A skill adapted from a source names it, its licence and where it was read; the licence text of that source is
// in licenses/vendored/ (D-071, SDD-018 section 16).
func TestAdaptedSkillsNameASourceWhoseLicenceIsKept(t *testing.T) {
	t.Parallel()

	adapted := 0

	for _, s := range loadAll(t) {
		id, ok := s.skill.Metadata[adaptedFrom]
		if !ok {
			if s.skill.License != "" || s.skill.Source() != "" {
				t.Errorf("%s names a licence or a source but no %s", s.name, adaptedFrom)
			}

			continue
		}

		adapted++

		repo, known := sources[id]
		if !known {
			t.Errorf("%s: %s %q is not an approved source", s.name, adaptedFrom, id)

			continue
		}

		if s.skill.License == "" {
			t.Errorf("%s: no licence in the front matter", s.name)
		}

		if src := s.skill.Source(); !strings.HasPrefix(src, repo+" ") || !regexp.MustCompile(`@ [0-9a-f]{40}$`).MatchString(src) {
			t.Errorf("%s: the source %q is not %q, a path and @ and a commit", s.name, src, repo)
		}

		text, err := os.ReadFile(filepath.Join("..", "..", "..", "licenses", "vendored", id+"-"+s.skill.License+".txt"))
		if err != nil || len(text) < 500 {
			t.Errorf("%s: the licence text of %s is not in licenses/vendored: %v", s.name, id, err)

			continue
		}

		if !strings.Contains(string(text), licenceMarker(s.skill.License)) {
			t.Errorf("%s: the licence %q is not the text kept for %s", s.name, s.skill.License, id)
		}
	}

	// The writing skills of the first flow come from the approved sources.
	if adapted < 3 {
		t.Errorf("%d adapted skills; the three writing skills are adapted", adapted)
	}

	if _, err := os.Stat(filepath.Join("..", "..", "..", "licenses", "vendored", "NOTICE-skills.md")); err != nil {
		t.Errorf("licenses/vendored/NOTICE-skills.md: %v", err)
	}
}

// licenceMarker is a phrase that only the text of the named licence has.
func licenceMarker(spdx string) string {
	switch spdx {
	case "MIT":
		return "Permission is hereby granted, free of charge"
	case "Apache-2.0":
		return "Apache License"
	}

	return "\x00no such licence"
}
