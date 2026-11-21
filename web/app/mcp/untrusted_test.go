package mcp

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// TestUntrustedText is §9 check 11 through the MCP adapter: names with
// delimiters, markup, control characters, a long run and prompt-like sentences
// are escaped and delimited, truncated with a marker, and the structured result
// keeps the original under `untrusted`.
func TestUntrustedText(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project := createProject(t, cs, "Hostile")

	names := testenv.HostileNames(t, "testdata/untrusted/names.json")
	paths := make([]any, 0, len(names))

	for i, n := range names {
		// Distinct bytes, so that no file is a duplicate of another.
		paths = append(paths, env.WriteFile(t, n.Name, strings.Repeat("x", 64+i)))
	}

	imported := ok(t, call(t, cs, "media_import", map[string]any{
		"project_id": project, "paths": paths, "wait_ms": 20000,
	}))
	assets, _ := structured(t, imported)["assets"].([]any)

	if len(assets) != len(names) {
		t.Fatalf("imported %d of %d files: %s", len(assets), len(names), textOf(t, imported))
	}

	for _, problem := range testenv.UntrustedTextProblems(textOf(t, imported)) {
		t.Errorf("media_import: %s", problem)
	}

	listed := ok(t, call(t, cs, "media_inspect", map[string]any{"project_id": project}))
	for _, problem := range testenv.UntrustedTextProblems(textOf(t, listed)) {
		t.Errorf("media list: %s", problem)
	}

	byName := map[string]string{}

	for _, a := range assets {
		m, _ := a.(map[string]any)
		id, _ := m["asset_id"].(string)

		res := ok(t, call(t, cs, "media_inspect", map[string]any{"project_id": project, "asset_id": id}))
		for _, problem := range testenv.UntrustedTextProblems(textOf(t, res)) {
			t.Errorf("media_inspect %s: %s", id, problem)
		}

		un, _ := structured(t, res)["untrusted"].(map[string]any)
		name, _ := un["name"].(string)
		byName[name] = id
	}

	for _, n := range names {
		_, kept := byName[n.Name]
		_, cleaned := byName[testenv.StoredName(n.Name)]

		if !kept && !cleaned {
			t.Errorf("the structured result lost the name %q (have %q)", n.Name, byName)
		}
	}

	if text := textOf(t, listed); strings.Contains(text, strings.Repeat("x", 150)) || !strings.Contains(text, "…") {
		t.Errorf("a 150 character name must be cut with the marker: %s", text)
	}
}
