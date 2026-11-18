package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/web/app/agentdocs"
)

// TestCutCaptionScenarioIsTheSameThroughEverySurface (P2-AGT-009): the first
// recipe of the skill, import, cut, caption and export, is the same through
// Direct, HTTP, the CLI and MCP. The recipe is run as CLI text by
// agentdocs.TestRecipesRun; this runs its operations through every surface.
func TestCutCaptionScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "cut-caption", "prj_7k2m9x4q1b")

	checkRecordedHash(t, "cut-caption", docs["prj_7k2m9x4q1b"])
}

var recipeOp = regexp.MustCompile(`(?:"op":"|\s)([a-z]+\.[a-z_]+)(?:"|\s)`)

// TestEveryRecipeHasItsScenario (D-31, SDD-003 section 8.2 item 4): each recipe
// names a scenario, and every operation and job kind the recipe uses appears in
// that scenario, so changing a recipe without the scenario that runs it through
// the four surfaces fails here.
func TestEveryRecipeHasItsScenario(t *testing.T) {
	recipes, err := agentdocs.Recipes()
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range recipes {
		t.Run(r.Title, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("scenarios", r.Scenario+".json"))
			if err != nil {
				t.Fatalf("scenario %q: %v", r.Scenario, err)
			}

			scenario := strings.Join(strings.Fields(string(raw)), "")

			for _, s := range r.Steps {
				text := s.Cmd + " " + s.Stdin
				if s.Run != "" {
					text = s.Run + " " + s.Stdin
				}

				var want []string

				for _, m := range recipeOp.FindAllStringSubmatch(text, -1) {
					want = append(want, `"op":"`+m[1]+`"`)
				}

				switch strings.Fields(s.Cmd)[0] {
				case "export", "transcribe":
					want = append(want, `"kind":"`+strings.Fields(s.Cmd)[0]+`"`)
				}

				for _, w := range want {
					if !strings.Contains(scenario, w) {
						t.Errorf("recipe step %q uses %s, which scenario %s does not run", s.Cmd, w, r.Scenario)
					}
				}
			}
		})
	}
}
