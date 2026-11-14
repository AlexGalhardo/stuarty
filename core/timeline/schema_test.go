package timeline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The committed schema is generated from the Go types (task codegen); a type
// or tag change without regenerating it fails here, and so does a generator
// that no longer produces the committed document.
func TestSchemaMatchesTheCommittedFile(t *testing.T) {
	t.Parallel()

	var got bytes.Buffer

	enc := json.NewEncoder(&got)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(Schema()); err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile(filepath.Join("..", "schema", "timeline.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got.Bytes()) {
		t.Error("core/schema/timeline.v1.schema.json is stale: run task codegen")
	}
}

func TestSchemaShape(t *testing.T) {
	t.Parallel()

	s := Schema()

	if s["$schema"] != "https://json-schema.org/draft/2020-12/schema" || s["x-status"] != "frozen" {
		t.Errorf("unexpected header: %v %v", s["$schema"], s["x-status"])
	}

	defs, ok := s["$defs"].(map[string]any)
	if !ok || len(defs) == 0 {
		t.Fatalf("$defs = %v", s["$defs"])
	}

	for _, name := range []string{"Track", "Item", "Rational"} {
		if _, ok := defs[name]; !ok {
			t.Errorf("$defs has no %s", name)
		}
	}
}
