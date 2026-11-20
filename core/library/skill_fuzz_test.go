package library

import "testing"

// FuzzParseSkill feeds any bytes as the SKILL.md of a skill folder, with one more file (SDD-008 section 13.2):
// no panic in the parser or the linter, and a skill that parses names every file but SKILL.md.
func FuzzParseSkill(f *testing.F) {
	f.Add([]byte("---\nname: demo\ndescription: A demo.\n---\nBody\n"), "scripts/run.sh")
	f.Add([]byte("---\nname: [unclosed\n"), "../escape")
	f.Add([]byte("no front matter"), "SKILL.md")
	f.Add([]byte("---\n---\n"), "")

	f.Fuzz(func(t *testing.T, skill []byte, other string) {
		files := []File{{Path: SkillFile, Data: skill}, {Path: other, Data: []byte("x")}}

		s, err := ParseSkill(files)
		_ = LintSkill("demo", files)

		if err != nil {
			return
		}

		for _, file := range s.Files {
			if file.Path == SkillFile {
				t.Fatalf("the files of the skill include %s", SkillFile)
			}
		}
	})
}
