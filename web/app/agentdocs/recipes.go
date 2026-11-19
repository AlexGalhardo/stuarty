package agentdocs

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed recipes/*.json
var recipeFS embed.FS

// RecipeStep is one command of a recipe. Cmd is what the skill shows, after
// `stuarty`; Run replaces it when the test must run something concrete (a
// minted id); Stdin is the input of the command; Exit is the exit code the step
// is expected to end with (0 when absent); Note is a comment line above it.
type RecipeStep struct {
	Cmd   string `json:"cmd"`
	Run   string `json:"run,omitempty"`
	Stdin string `json:"stdin,omitempty"`
	Exit  int    `json:"exit,omitempty"`
	Note  string `json:"note,omitempty"`

	// Hidden marks a setup step the test runs but the skill does not show: the
	// recipe says what it starts from in a note, to keep the skill short.
	Hidden bool `json:"hidden,omitempty"`
}

// Recipe is a short sequence of commands that does one job. The skill renders
// each recipe as commands and the contract tests run it (SDD-003 section 8.2
// item 4), so a recipe that stops working breaks the build.
type Recipe struct {
	Title string `json:"title"`

	// Scenario names the file of web/contract/scenarios that does the same job
	// through the Service, and so through the six runs.
	Scenario string `json:"scenario"`

	Steps []RecipeStep `json:"steps"`
}

// Recipes reads the recipes in file name order.
func Recipes() ([]Recipe, error) {
	entries, err := recipeFS.ReadDir("recipes")
	if err != nil {
		return nil, fmt.Errorf("agentdocs: %w", err)
	}

	out := make([]Recipe, 0, len(entries))

	for _, e := range entries {
		raw, err := recipeFS.ReadFile("recipes/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("agentdocs: %w", err)
		}

		var r Recipe
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("agentdocs: recipe %s: %w", e.Name(), err)
		}

		if r.Title == "" || r.Scenario == "" || len(r.Steps) == 0 {
			return nil, fmt.Errorf("agentdocs: recipe %s has no title, scenario or steps", e.Name())
		}

		out = append(out, r)
	}

	return out, nil
}

// Text is the recipe as the commands of the skill: a note above its command, and
// the standard input of a command as a here-document.
func (r Recipe) Text() string {
	var b strings.Builder

	for _, s := range r.Steps {
		if s.Hidden {
			continue
		}

		if s.Note != "" {
			b.WriteString("# " + s.Note + "\n")
		}

		b.WriteString("stuarty " + s.Cmd)

		if s.Stdin != "" {
			b.WriteString(" <<'EOF'\n" + s.Stdin + "\nEOF")
		}

		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
