package mcp

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// TestTemplateToolSavesListsAndInstantiates: save makes a template from a
// project, list shows it, new makes a project from it.
func TestTemplateToolSavesListsAndInstantiates(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	project, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{"name": "Base"})))["project_id"].(string)

	saved := structured(t, ok(t, call(t, cs, "template", map[string]any{
		"action": "save", "project_id": project, "name": "Spring sale",
	})))

	tpl, _ := saved["template"].(map[string]any)
	id, _ := tpl["template_id"].(string)

	if !strings.HasPrefix(id, "tpl_") {
		t.Fatalf("save: %v", saved)
	}

	listed := structured(t, ok(t, call(t, cs, "template", map[string]any{"action": "list"})))
	if templates, _ := listed["templates"].([]any); len(templates) != 1 {
		t.Fatalf("list: %v", listed)
	}

	made := structured(t, ok(t, call(t, cs, "template", map[string]any{
		"action": "new", "template_id": id, "name": "From template",
	})))
	if got, _ := made["project_id"].(string); !strings.HasPrefix(got, "prj_") || got == project {
		t.Fatalf("new: %v", made)
	}
}

// TestTemplateToolRefusesMembersTheActionDoesNotUse: a missing member and a
// member of another action are E_VALIDATION with the path of the member; the
// import and export are in the schema now.
func TestTemplateToolRefusesMembersTheActionDoesNotUse(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")

	for _, tc := range []struct {
		args map[string]any
		path string
	}{
		{map[string]any{"action": "save", "name": "x"}, "project_id"},
		{map[string]any{"action": "new", "template_id": "tpl_0000000000"}, "name"},
		{map[string]any{"action": "list", "name": "x"}, "name"},
		{map[string]any{"action": "save", "project_id": "prj_0000000000", "name": "x", "fill": map[string]any{}}, "fill"},
		{map[string]any{"action": "export"}, "template_id"},
		{map[string]any{"action": "import", "name": "x"}, "name"},
		{map[string]any{"action": "import"}, "path"},
	} {
		res := call(t, cs, "template", tc.args)
		text := textOf(t, res)

		if !res.IsError || !strings.Contains(text, "E_VALIDATION") || !strings.Contains(text, tc.path) {
			t.Errorf("%v: %s", tc.args, text)
		}
	}
}

// TestTemplateToolExportsAndImports: export writes a template file and import
// reads it back; a relative path is E_PATH_NOT_ALLOWED from the Service.
func TestTemplateToolExportsAndImports(t *testing.T) {
	t.Parallel()

	cs := connect(t, testenv.New(t, testenv.Options{}).Service, Options{}, "claude-code")
	project, _ := structured(t, ok(t, call(t, cs, "project_create", map[string]any{"name": "Base"})))["project_id"].(string)

	saved := structured(t, ok(t, call(t, cs, "template", map[string]any{
		"action": "save", "project_id": project, "name": "Round trip",
	})))
	tpl, _ := saved["template"].(map[string]any)
	id, _ := tpl["template_id"].(string)

	exported := structured(t, ok(t, call(t, cs, "template", map[string]any{"action": "export", "template_id": id})))
	if path, _ := exported["path"].(string); path == "" {
		t.Fatalf("export: %v", exported)
	}

	relative := call(t, cs, "template", map[string]any{"action": "import", "path": "x.stuarty-template.zip"})
	if !relative.IsError || !strings.Contains(textOf(t, relative), "E_PATH_NOT_ALLOWED") {
		t.Errorf("a relative path: %s", textOf(t, relative))
	}
}
