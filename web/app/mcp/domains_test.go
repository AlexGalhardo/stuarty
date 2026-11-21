package mcp

import (
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

func TestRouteDomain(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		tool, domain, want, hint string
	}{
		{"edit", "", "video", ""},
		{"edit", "video", "video", ""},
		{"edit", "image", "image", ""},
		{"history", "image", "image", ""},
		{"command_help", "publish", "publish", ""},
		{"edit", "writing", "", "the domains of edit are video, image"},
		{"history", "memory", "", "the domains of history are video, image"},
		{"edit", "videos", "", "use one of project, video, image"},
		{"edit", "Image", "", "use one of project, video, image"},
	} {
		got, err := routeDomain(tc.tool, tc.domain)

		if tc.want == "" {
			se, ok := err.(*service.Error) //nolint:errorlint // built by routeDomain
			if !ok || se.Code != command.CodeValidation || se.Path != "domain" || !strings.Contains(se.Hint, tc.hint) {
				t.Errorf(
					"%s %q: got %v, want E_VALIDATION on domain with the hint %q",
					tc.tool,
					tc.domain,
					err,
					tc.hint,
				)
			}

			continue
		}

		if err != nil || got != tc.want {
			t.Errorf("%s %q: got %q, %v, want %q", tc.tool, tc.domain, got, err, tc.want)
		}
	}
}

// The tools that take a domain are the ones SDD-003 section 4.8 lists, and each of them declares the argument in
// its definition: the router and the definition cannot drift.
func TestDomainArgumentIsDeclaredWhereItIsRouted(t *testing.T) {
	t.Parallel()

	defs, err := loadDefinitions()
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range order {
		props, _ := defs[name].tool.InputSchema.(map[string]any)["properties"].(map[string]any)
		_, has := props["domain"]
		_, routed := served[name]

		if has != routed {
			t.Errorf("%s: the definition has domain %v, the router serves it %v", name, has, routed)
		}
	}

	for name := range served {
		if _, ok := defs[name]; !ok {
			t.Errorf("the router serves %s, which is no tool", name)
		}
	}
}

// The definition of edit does not list the operations: it stays the same size as the registry grows, and the
// operations are found through command_help (ADR-0043 item 2).
func TestEditDefinitionListsNoOperation(t *testing.T) {
	t.Parallel()

	defs, err := loadDefinitions()
	if err != nil {
		t.Fatal(err)
	}

	raw := string(marshal(t, defs["edit"].tool.InputSchema))
	for _, name := range command.Default().Names() {
		if strings.Contains(raw, `"`+name+`"`) {
			t.Errorf("the definition of edit names the operation %s", name)
		}
	}
}

func TestEditAndHistoryRouteByDomain(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")

	project := createProject(t, cs, "P")
	img, _ := structured(t, ok(t, call(t, cs, "image", map[string]any{
		"action": "create", "preset": "video_thumbnail", "name": "Cover",
	})))["image_id"].(string)

	fill := []any{map[string]any{"op": "layer.add_fill", "args": map[string]any{"color": "#203040"}}}
	track := []any{map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual", "name": "T"}}}

	// With no domain and with domain video, edit is the call it was before domains existed.
	ok(t, call(t, cs, "edit", map[string]any{"project_id": project, "base_revision": 0, "operations": track}))
	ok(t, call(t, cs, "edit", map[string]any{
		"domain": "video", "project_id": project, "base_revision": 1,
		"operations": []any{map[string]any{"op": "track.add", "args": map[string]any{"kind": "audio", "name": "A"}}},
	}))

	// domain image reaches ImageEdit, the same operation as the image tool.
	res := ok(t, call(t, cs, "edit", map[string]any{
		"domain": "image", "image_id": img, "base_revision": 1, "operations": fill,
	}))
	if !strings.Contains(textOf(t, res), "added fill") {
		t.Errorf("the image edit did not run the operation: %q", textOf(t, res))
	}

	described := textOf(t, ok(t, call(t, cs, "image", map[string]any{"action": "show", "image_id": img})))
	if !strings.Contains(described, "fill") {
		t.Errorf("the image has no fill after edit with domain image: %q", described)
	}

	undone := ok(t, call(t, cs, "history", map[string]any{"domain": "image", "image_id": img, "action": "undo"}))
	if textOf(t, undone) == "" {
		t.Error("the image undo through history said nothing")
	}

	// The refusals name the member, the domain and what to use.
	for name, tc := range map[string]struct {
		tool string
		args map[string]any
		path string
		code string
	}{
		"video needs project_id":   {"edit", map[string]any{"base_revision": 0, "operations": track}, "project_id", command.CodeValidation},
		"video refuses image_id":   {"edit", map[string]any{"project_id": project, "image_id": img, "base_revision": 2, "operations": track}, "image_id", command.CodeValidation},
		"image needs image_id":     {"edit", map[string]any{"domain": "image", "base_revision": 1, "operations": fill}, "image_id", command.CodeValidation},
		"image refuses project_id": {"edit", map[string]any{"domain": "image", "image_id": img, "project_id": project, "base_revision": 1, "operations": fill}, "project_id", command.CodeValidation},
		"unserved domain":          {"edit", map[string]any{"domain": "publish", "project_id": project, "base_revision": 2, "operations": track}, "domain", command.CodeValidation},
		"history list of image":    {"history", map[string]any{"domain": "image", "image_id": img}, "action", command.CodeValidation},
		"history video no id":      {"history", map[string]any{}, "project_id", command.CodeValidation},
		"history image refuses":    {"history", map[string]any{"domain": "image", "image_id": img, "action": "undo", "limit": 3}, "limit", command.CodeValidation},
	} {
		res := call(t, cs, tc.tool, tc.args)
		text := textOf(t, res)

		if !res.IsError || !strings.Contains(text, "error "+tc.code+":") ||
			!strings.Contains(text, `"path":"`+tc.path+`"`) {
			t.Errorf("%s: %q", name, text)
		}
	}
}

// Every tool of the list belongs to a domain, and the domains it routes are among them.
func TestEveryToolHasItsDomains(t *testing.T) {
	t.Parallel()

	for _, name := range order {
		domains := ToolDomains(name)
		if len(domains) == 0 {
			t.Errorf("%s belongs to no domain: add it to toolDomains", name)
		}

		for _, d := range domains {
			if !command.KnownDomain(d) {
				t.Errorf("%s: %q is not a domain", name, d)
			}
		}

		for _, d := range served[name] {
			if !slices.Contains(domains, d) {
				t.Errorf("%s routes the domain %s and does not list it in toolDomains", name, d)
			}
		}
	}

	for name := range toolDomains {
		if !slices.Contains(order, name) {
			t.Errorf("toolDomains names %s, which is no tool", name)
		}
	}
}
