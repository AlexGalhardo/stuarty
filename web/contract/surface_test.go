package contract

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/app/cli"
)

const surfacePath = "testdata/surface.txt"

// currentSurface lists every name an agent can depend on, one "kind name" line
// each: the MCP tools of tools/list, the CLI commands (stubs included, their name
// is already public) and the HTTP routes of the OpenAPI document.
func currentSurface(t *testing.T) []string {
	t.Helper()

	var lines []string

	for _, name := range listedTools(t) {
		lines = append(lines, "tool "+name)
	}

	commands, _ := cli.CommandNames()
	for _, name := range commands {
		lines = append(lines, "command "+name)
	}

	for _, route := range httpRoutes(t, newHTTP(t).(*httpDriver)) {
		lines = append(lines, "route "+route)
	}

	slices.Sort(lines)

	return slices.Compact(lines)
}

// diffSurface returns the names of the snapshot that are gone and the names that
// the snapshot does not know.
func diffSurface(snapshot, current []string) (removed, added []string) {
	for _, line := range snapshot {
		if !slices.Contains(current, line) {
			removed = append(removed, line)
		}
	}

	for _, line := range current {
		if !slices.Contains(snapshot, line) {
			added = append(added, line)
		}
	}

	return removed, added
}

func readSurface(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.FromSlash(surfacePath))
	if err != nil {
		t.Fatalf("%v (UPDATE_SURFACE=1 go test ./web/contract/ -run TestSurfaceFrozen records it)", err)
	}

	var lines []string

	for line := range strings.SplitSeq(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

// TestSurfaceDiffRules proves both directions of the freeze on a synthetic
// surface: a removed or renamed name is reported as removed, a new name as added.
func TestSurfaceDiffRules(t *testing.T) {
	t.Parallel()

	snapshot := []string{"command edit", "route GET /projects", "tool edit"}

	removed, added := diffSurface(snapshot, snapshot)
	if len(removed)+len(added) != 0 {
		t.Fatalf("an unchanged surface differs: removed %v, added %v", removed, added)
	}

	removed, added = diffSurface(snapshot, []string{"command edit", "route GET /projects"})
	if !slices.Equal(removed, []string{"tool edit"}) || len(added) != 0 {
		t.Errorf("a removed tool: removed %v, added %v", removed, added)
	}

	removed, added = diffSurface(snapshot, []string{"command edit", "route GET /projects", "tool edit_batch"})
	if !slices.Equal(removed, []string{"tool edit"}) || !slices.Equal(added, []string{"tool edit_batch"}) {
		t.Errorf("a renamed tool: removed %v, added %v", removed, added)
	}

	removed, added = diffSurface(snapshot, append(slices.Clone(snapshot), "route POST /x"))
	if len(removed) != 0 || !slices.Equal(added, []string{"route POST /x"}) {
		t.Errorf("an added route: removed %v, added %v", removed, added)
	}
}

// TestSurfaceFrozen compares the tool list, the CLI command table and the HTTP
// routes with the committed snapshot (SDD-003 section 9, P4-AGT-001). A removed or
// renamed name fails. An added name fails until the snapshot changes in the same
// change: UPDATE_SURFACE=1 appends additions, but never drops a name, so a removal
// needs a deliberate edit of testdata/surface.txt and a major version.
func TestSurfaceFrozen(t *testing.T) {
	current := currentSurface(t)

	if os.Getenv("UPDATE_SURFACE") == "1" {
		snapshot := readSurfaceIfAny(t)
		merged := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(snapshot), current...))))

		body := []byte(strings.Join(merged, "\n") + "\n")

		if err := os.WriteFile(filepath.FromSlash(surfacePath), body, 0o600); err != nil {
			t.Fatal(err)
		}

		removed, _ := diffSurface(merged, current)

		for _, line := range removed {
			t.Errorf("%s is in the snapshot but no longer served: removing a name is a breaking change", line)
		}

		return
	}

	removed, added := diffSurface(readSurface(t), current)

	for _, line := range removed {
		t.Errorf("%s was removed or renamed: the agent surface is frozen", line)
	}

	for _, line := range added {
		t.Errorf("%s is new: record it with UPDATE_SURFACE=1 go test ./web/contract/ -run TestSurfaceFrozen", line)
	}
}

func readSurfaceIfAny(t *testing.T) []string {
	t.Helper()

	if _, err := os.Stat(filepath.FromSlash(surfacePath)); err != nil {
		return nil
	}

	return readSurface(t)
}
