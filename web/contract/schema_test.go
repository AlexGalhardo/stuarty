package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/AlexGalhardo/stuarty/service"
)

// resultSchemas names the component of the OpenAPI document that describes the
// result of each Service operation the scenario calls: the document a command
// prints with --json is the HTTP body of the same operation (SDD-003 section 5).
var resultSchemas = map[string]string{
	"ProjectList":   "ProjectListResult",
	"ProjectCreate": "ProjectCreateResult",
	"ProjectGet":    "ProjectGetResult",
	"Edit":          "EditResult",
	"HistoryList":   "HistoryListResult",
	"Undo":          "UndoResult",
	"Redo":          "UndoResult",
	"Lint":          "LintResult",
	"MediaImport":   "MediaImportResult",
	"MediaList":     "MediaListResult",
	"MediaInspect":  "MediaInspectResult",
	"JobStart":      "Job",
	"JobGet":        "Job",
	"JobList":       "JobListResult",
}

// schemaDriver checks every successful result of the driver it wraps against
// the schema of the committed OpenAPI document.
type schemaDriver struct {
	driver
	schemas map[string]*jsonschema.Schema
	checked map[string]int
}

func (d *schemaDriver) do(t *testing.T, s step) (json.RawMessage, string, *service.ErrorDocument) {
	t.Helper()

	raw, text, fail := d.driver.do(t, s)
	if fail != nil {
		return raw, text, fail
	}

	sch, ok := d.schemas[s.Call]
	if !ok {
		t.Fatalf("no result schema for %s", s.Call)
	}

	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("%s: the output is not one JSON document: %v\n%s", s.Call, err, raw)
	}

	if err := sch.Validate(inst); err != nil {
		t.Errorf("%s: the --json output does not validate against %s: %v\n%s", s.Call, resultSchemas[s.Call], err, raw)
	}

	d.checked[s.Call]++

	return raw, text, fail
}

func loadResultSchemas(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()

	f, err := os.Open(filepath.FromSlash(openAPIFile))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("openapi.json", doc); err != nil {
		t.Fatal(err)
	}

	out := map[string]*jsonschema.Schema{}

	for call, component := range resultSchemas {
		sch, err := c.Compile("openapi.json#/components/schemas/" + component)
		if err != nil {
			t.Fatalf("%s: %v", component, err)
		}

		out[call] = sch
	}

	return out
}

// TestCLIJSONValidatesAgainstTheResultSchemas runs the scenario through the CLI
// with --json and validates every document it prints (P1-AGT-001).
func TestCLIJSONValidatesAgainstTheResultSchemas(t *testing.T) {
	steps, fixtures := loadScenario(t, "prototype")
	d := &schemaDriver{driver: newCLI(t), schemas: loadResultSchemas(t), checked: map[string]int{}}

	run(t, d, steps, fixtures)

	for call := range resultSchemas {
		if d.checked[call] == 0 && call != "Redo" && call != "JobGet" {
			t.Errorf("the scenario never validated a %s result", call)
		}
	}
}
