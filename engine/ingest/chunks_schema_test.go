package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// chunkIndexErr checks an index against docs/schemas/chunks-1.schema.json, the contract of the served file
// (SDD-011 4.4.1), and the one rule a schema cannot state: the four arrays have one entry per sample.
func chunkIndexErr(t *testing.T, idx *ChunkIndex) error {
	t.Helper()

	f, err := os.Open(filepath.Join("..", "..", "docs", "schemas", "chunks-1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := jsonschema.UnmarshalJSON(f)
	_ = f.Close()

	if err != nil {
		t.Fatal(err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("mem:///chunks-1.json", doc); err != nil {
		t.Fatal(err)
	}

	schema, err := c.Compile("mem:///chunks-1.json")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}

	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	if err := schema.Validate(inst); err != nil {
		return err
	}

	if n := len(idx.Sizes); len(idx.Offsets) != n || len(idx.TimestampsUS) != n {
		return fmt.Errorf("%d sizes, %d offsets, %d times", n, len(idx.Offsets), len(idx.TimestampsUS))
	}

	return nil
}

func TestChunkIndexMatchesItsSchema(t *testing.T) {
	t.Parallel()

	stts, stsc, stsz, stco := goodTables()
	file := tableOf(stts, stsc, stsz, stco)

	idx, err := readChunkIndex(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}

	if err := chunkIndexErr(t, idx); err != nil {
		t.Fatalf("the index of the base table: %v", err)
	}

	for name, mutate := range map[string]func(*ChunkIndex){
		"an H.264 codec string":   func(i *ChunkIndex) { i.Codec = "avc1.64001f" },
		"version 2":               func(i *ChunkIndex) { i.Version = 2 },
		"no width":                func(i *ChunkIndex) { i.Width = 0 },
		"a negative offset":       func(i *ChunkIndex) { i.Offsets[0] = -1 },
		"a missing time":          func(i *ChunkIndex) { i.TimestampsUS = i.TimestampsUS[:1] },
		"a repeated key frame":    func(i *ChunkIndex) { i.Keyframes = []int{0, 0} },
		"an index without arrays": func(i *ChunkIndex) { i.Keyframes = nil },
	} {
		bad := *idx
		bad.Offsets = append([]int64(nil), idx.Offsets...)
		mutate(&bad)

		if chunkIndexErr(t, &bad) == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
