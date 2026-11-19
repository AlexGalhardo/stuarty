package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compile(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	b, err := Files.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("mem:///"+name, doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("mem:///" + name)
	if err != nil {
		t.Fatalf("%s is not a valid schema: %v", name, err)
	}
	return s
}

func instance(t *testing.T, path string) any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	v, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// ROADMAP P1-SCH-003: every valid example validates, and every invalid example that
// is a shape error fails the generated schema.
func TestTimelineSchemaAcceptsTheValidExamples(t *testing.T) {
	s := compile(t, "timeline.v1.schema.json")
	files, err := filepath.Glob("../timeline/testdata/valid/*.json")
	if err != nil || len(files) < 4 {
		t.Fatalf("examples: %v %v", files, err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".canonical.json") {
			continue
		}
		if err := s.Validate(instance(t, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestTimelineSchemaRejectsTheShapeErrors(t *testing.T) {
	s := compile(t, "timeline.v1.schema.json")
	for _, name := range []string{"start-not-integer", "fps-den", "odd-width", "bad-id"} {
		if err := s.Validate(instance(t, "../timeline/testdata/invalid/"+name+".json")); err == nil {
			t.Errorf("%s: the schema accepted a document with a shape error", name)
		}
	}
}

func TestTimelineSchemaKeepsUnknownFieldsAndRejectsLimits(t *testing.T) {
	s := compile(t, "timeline.v1.schema.json")
	ok := instance(t, "../timeline/testdata/valid/unknown-fields.json")
	if err := s.Validate(ok); err != nil {
		t.Errorf("unknown fields must be allowed: %v", err)
	}
	doc, _ := ok.(map[string]any)
	doc["schema_version"] = json.Number("2")
	if err := s.Validate(doc); err == nil {
		t.Error("schema_version 2 must be rejected")
	}
}

func TestCommandSchemasAcceptTheirExamples(t *testing.T) {
	b, err := Files.ReadFile("commands.json")
	if err != nil {
		t.Fatal(err)
	}
	var cmds []struct {
		Name     string          `json:"name"`
		Example  json.RawMessage `json:"example"`
		Internal bool            `json:"internal"`
	}
	if err := json.Unmarshal(b, &cmds); err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 74 {
		t.Fatalf("commands.json lists %d commands, want 74", len(cmds))
	}
	internal := 0
	for _, c := range cmds {
		if c.Internal {
			internal++
		}
		s := compile(t, "commands/"+c.Name+".schema.json")
		ex, err := jsonschema.UnmarshalJSON(bytes.NewReader(c.Example))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(ex); err != nil {
			t.Errorf("%s: its example breaks its schema: %v", c.Name, err)
		}
		if s.Validate(map[string]any{"zzz_unknown": true}) == nil {
			t.Errorf("%s: the schema must reject unknown arguments", c.Name)
		}
	}
	if internal != 1 {
		t.Errorf("one command is internal (asset.add), got %d", internal)
	}
}

func TestErrorsAndLimitsFiles(t *testing.T) {
	var errs []struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
		HTTP      int    `json:"http"`
	}
	b, _ := Files.ReadFile("errors.json")
	if err := json.Unmarshal(b, &errs); err != nil || len(errs) != 17 {
		t.Fatalf("%d codes: %v", len(errs), err)
	}
	for _, e := range errs {
		if e.HTTP < 400 || e.Retryable != (e.Code == "E_REVISION_CONFLICT") {
			t.Errorf("%+v", e)
		}
	}
	var limits map[string]int
	b, _ = Files.ReadFile("limits.json")
	if err := json.Unmarshal(b, &limits); err != nil || limits["items"] != 500 || limits["max_frames"] != 108000 {
		t.Errorf("%v %v", limits, err)
	}
}
