package textview

import (
	"fmt"
	"strings"
)

// StepLine is one step of a workflow in the text of a project.
type StepLine struct {
	ID    string
	State string
}

// Workflow is the line of a project that says where it stands: its kind, how many steps are done, the step to do
// now and the one after it. It is short on purpose, for the budget of the text: every step with its state is in the
// structured result. Every word is a name the service chose, never text of the user.
func Workflow(kind string, steps []StepLine, current, next string) string {
	counts := map[string]int{}

	for i := range steps {
		counts[steps[i].State]++
	}

	var b strings.Builder

	fmt.Fprintf(&b, "workflow (%s): %d of %d steps done", kind, counts["done"], len(steps))

	for _, state := range []string{"skipped", "unknown"} {
		if counts[state] > 0 {
			fmt.Fprintf(&b, ", %d %s", counts[state], state)
		}
	}

	switch {
	case current != "":
		b.WriteString("; now " + current)

		for i := range steps {
			if steps[i].ID == current && steps[i].State == "waiting" {
				b.WriteString(" (waits for a person)")
			}
		}

		if next != "" {
			b.WriteString(", then " + next)
		}
	case counts["done"]+counts["skipped"]+counts["unknown"] == len(steps):
		b.WriteString("; nothing is open")
	}

	return b.String()
}

// StepSkipped is the text of a skipped or restored step.
func StepSkipped(projectID, step string, restore, changed, dry bool) string {
	switch {
	case !changed:
		return fmt.Sprintf("%s\n%s: the step %s is already as asked; nothing changed", Note, projectID, step)
	case restore && dry:
		return fmt.Sprintf("%s\n%s: dry run, would restore the step %s", Note, projectID, step)
	case dry:
		return fmt.Sprintf("%s\n%s: dry run, would skip the step %s", Note, projectID, step)
	case restore:
		return fmt.Sprintf("%s\n%s: restored the step %s", Note, projectID, step)
	}

	return fmt.Sprintf("%s\n%s: skipped the step %s", Note, projectID, step)
}
