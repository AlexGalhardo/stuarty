package skills_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/web/resources/skills"
)

// The skill of a recipe asks the questions of its kind and writes the brief with brief:set, because the briefing
// operations serve the short video only (SDD-018 section 17). The brief is in the schema of SDD-005 section
// 5.12.1 only when its headings are the headings of the set, in the order of the set: the skill names them in code
// spans on its "Required:" line, and that list must be the set.
var recipeKinds = []struct {
	skill string
	kind  project.ProductionKind
}{
	{"product-commercial", project.ProductionCommercial},
	{"long-talking-video", project.ProductionLong},
}

var codeSpanRE = regexp.MustCompile("`([^`\n]+)`")

func headingsOf(t *testing.T, k project.ProductionKind) []string {
	t.Helper()

	set, verr := project.BriefingQuestionsFor(k)
	if verr != nil {
		t.Fatal(verr)
	}

	out := make([]string, 0, len(set))
	for _, q := range set {
		out = append(out, q.Heading)
	}

	return out
}

func TestRecipeSkillsNameTheHeadingsOfTheirBriefing(t *testing.T) {
	t.Parallel()

	for _, r := range recipeKinds {
		data, err := fs.ReadFile(skills.FS(), r.skill+"/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}

		var line string

		for l := range strings.SplitSeq(string(data), "\n") {
			if strings.Contains(l, "Required: `") {
				line = l

				break
			}
		}

		if line == "" {
			t.Errorf("%s has no line that lists the questions of its brief (Required: ...)", r.skill)

			continue
		}

		var named []string
		for _, m := range codeSpanRE.FindAllStringSubmatch(line[strings.Index(line, "Required: `"):], -1) {
			named = append(named, m[1])
		}

		if want := headingsOf(t, r.kind); !slices.Equal(named, want) {
			t.Errorf("%s names the questions %q, want the headings of the %s set in its order %q",
				r.skill, named, r.kind, want)
		}
	}
}

// The scripted run of each recipe stores a brief that the briefing of its kind reads back, and that the briefing
// of another kind does not take for its own.
func TestRecipeScriptsWriteABriefOfTheirKind(t *testing.T) {
	t.Parallel()

	scripts := map[project.ProductionKind]string{
		project.ProductionCommercial: "recipe-commercial.sh",
		project.ProductionLong:       "recipe-long-video.sh",
	}

	for k, name := range scripts {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}

		text := strings.ReplaceAll(string(data), "\r\n", "\n")

		start := strings.Index(text, "/brief.md\" <<'EOF'\n")
		if start < 0 {
			t.Errorf("%s writes no brief.md", name)

			continue
		}

		body := text[start+len("/brief.md\" <<'EOF'\n"):]
		body = body[:strings.Index(body, "\nEOF\n")+1]

		if _, ok := project.ParseBriefingFor(k, body); !ok {
			t.Errorf("%s writes a brief that the %s briefing does not read:\n%s", name, k, body)
		}

		for _, other := range project.ProductionKinds() {
			if other == k {
				continue
			}

			if _, ok := project.ParseBriefingFor(other, body); ok {
				t.Errorf("%s writes a brief that the %s briefing also reads", name, other)
			}
		}
	}
}
