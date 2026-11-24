package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadDetailKeepsTheAllowlist(t *testing.T) {
	in := json.RawMessage(`{"project_id":"prj_a4k9m2x7bq","name":"Intro","token":"sk-secret","operations":[` +
		`{"op":"clip.trim","args":{"api_key":"sk-in-args"}},{"op":"clip.split","args":{}}]}`)

	d := ReadDetail("mcp__stuarty__edit", in)
	if d == nil || d.Project != "prj_a4k9m2x7bq" || d.Item != "Intro" || strings.Join(d.Operations, ",") != "clip.trim,clip.split" {
		t.Fatalf("detail = %+v", d)
	}

	raw, _ := json.Marshal(d)
	for _, secret := range []string{"sk-secret", "sk-in-args", "api_key", "token"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("detail carries %q: %s", secret, raw)
		}
	}
}

func TestReadDetailOfATool(t *testing.T) {
	d := ReadDetail("stuarty/library", json.RawMessage(`{"action":"get","kind":"skill","name":"caption-style"}`))
	if d == nil || d.Item != "caption-style" || len(d.Operations) != 1 || d.Operations[0] != "get" {
		t.Errorf("detail = %+v", d)
	}
}

func TestReadDetailRefusesWhatIsNotStuartys(t *testing.T) {
	in := json.RawMessage(`{"project_id":"prj_a4k9m2x7bq"}`)

	for _, title := range []string{"", "Run shell", "library", "my_stuarty_edit", "mcp__other__edit"} {
		if d := ReadDetail(title, in); d != nil {
			t.Errorf("title %q: detail = %+v, want none", title, d)
		}
	}
}

func TestReadDetailDropsWhatIsNotAPlainString(t *testing.T) {
	cases := map[string]string{
		"a number":                        `{"project_id":42}`,
		"an object":                       `{"project_id":{"id":"prj_a4k9m2x7bq"}}`,
		"an array":                        `{"item_id":["a","b"]}`,
		"null":                            `{"name":null}`,
		"not an object":                   `["project_id"]`,
		"not json":                        `project_id=prj_a4k9m2x7bq`,
		"an empty input":                  ``,
		"operations of a wrong shape":     `{"operations":"clip.trim"}`,
		"operations that are not objects": `{"operations":["clip.trim"]}`,
	}

	for name, in := range cases {
		if d := ReadDetail("mcp__stuarty__edit", json.RawMessage(in)); d != nil {
			t.Errorf("%s: detail = %+v, want none", name, d)
		}
	}
}

func TestReadDetailCleansAndClips(t *testing.T) {
	long := strings.Repeat("x", 500)
	in, _ := json.Marshal(map[string]any{"item_id": "a\u0000b‮\nc", "name": long, "project_id": long})

	d := ReadDetail("mcp__stuarty__project_item", in)
	if d == nil {
		t.Fatal("no detail")
	}

	if d.Item != "ab c" {
		t.Errorf("Item = %q, want the control and bidi characters gone and one line", d.Item)
	}

	if len(d.Project) > MaxDetailValue {
		t.Errorf("Project is %d bytes, over %d", len(d.Project), MaxDetailValue)
	}
}

func TestReadDetailCapsTheOperations(t *testing.T) {
	var ops []map[string]any
	for range 20 {
		ops = append(ops, map[string]any{"op": "clip.trim", "args": map[string]any{}})
	}

	in, _ := json.Marshal(map[string]any{"operations": ops})

	if d := ReadDetail("mcp__stuarty__edit", in); d == nil || len(d.Operations) != MaxDetailOperations {
		t.Errorf("detail = %+v, want %d operations", d, MaxDetailOperations)
	}

	big := json.RawMessage(`{"project_id":"` + strings.Repeat("a", MaxDetailInput) + `"}`)
	if d := ReadDetail("mcp__stuarty__edit", big); d != nil {
		t.Error("an input over the limit was read")
	}
}
