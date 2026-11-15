package timeline

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// FuzzDecode feeds bytes to the document decoder: it never panics, Check reports a
// problem as a violation, and a document that is valid encodes to canonical bytes
// that decode to the same bytes (SDD-001 §2 item 8).
func FuzzDecode(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "valid", "*.json"))
	for _, name := range files {
		if data, err := os.ReadFile(name); err == nil {
			f.Add(data)
		}
	}

	f.Add([]byte(`{}`))
	f.Add([]byte(`{"schema_version":1,"items":{"itm_m1n2p3q4r5":null}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(Check(data)) != 0 {
			return
		}

		doc, err := Decode(data)
		if err != nil {
			t.Fatalf("Check found no violation but Decode failed: %v", err)
		}

		first := Canonical(doc)

		again, err := Decode(first)
		if err != nil {
			t.Fatalf("the canonical bytes do not decode: %v", err)
		}

		if second := Canonical(again); !bytes.Equal(first, second) {
			t.Fatalf("canonical bytes are not stable:\n%s\n%s", first, second)
		}
	})
}
