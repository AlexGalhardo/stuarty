package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The same resolved words and ids give identical bytes on every run (the CI matrix
// runs this on the three operating systems).
func TestFromTranscriptDeterministic(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "caption.from_transcript", "ok_worked_example_replace_true")

	before, err := os.ReadFile(filepath.Join(dir, "before.json"))
	if err != nil {
		t.Fatal(err)
	}

	opBytes, err := os.ReadFile(filepath.Join(dir, "op.json"))
	if err != nil {
		t.Fatal(err)
	}

	var op Operation
	if err := json.Unmarshal(opBytes, &op); err != nil {
		t.Fatal(err)
	}

	var runs [2][]byte

	for i := range runs {
		doc, err := timeline.Decode(before)
		if err != nil {
			t.Fatal(err)
		}

		res, err := Default().Apply(doc, op, Env{})
		if err != nil {
			t.Fatal(err)
		}

		eff, err := json.Marshal(res.Effective)
		if err != nil {
			t.Fatal(err)
		}

		runs[i] = append(timeline.Canonical(res.Doc), eff...)
	}

	if !bytes.Equal(runs[0], runs[1]) {
		t.Error("two runs of the same operation gave different bytes")
	}
}
