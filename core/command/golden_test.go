package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The golden-case runner of SDD-002 §A.11 and SDD-008 §3. A case is a directory
// testdata/<command>/<case>/ with before.json (the document, canonical), op.json,
// and either after.json, inverse.json, diff.json and effective.json, or error.json.
// `go test ./core/command/... -update` rewrites the expected files.

func goldenBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

func compareFile(t *testing.T, dir, name string, got []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%s: %v", path, err)
		return
	}
	if !bytes.Equal(bytes.TrimRight(want, "\n"), bytes.TrimRight(got, "\n")) {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

// runGolden runs every case found under root for the commands of reg and returns
// the number of cases run.
func runGolden(t *testing.T, reg *Registry, root string) int {
	t.Helper()
	n := 0
	for _, cmd := range reg.List() {
		cases, err := os.ReadDir(filepath.Join(root, cmd.Name))
		if err != nil {
			continue
		}
		for _, c := range cases {
			if !c.IsDir() {
				continue
			}
			n++
			dir := filepath.Join(root, cmd.Name, c.Name())
			t.Run(cmd.Name+"/"+c.Name(), func(t *testing.T) { runGoldenCase(t, reg, dir) })
		}
	}
	return n
}

func runGoldenCase(t *testing.T, reg *Registry, dir string) {
	t.Helper()
	beforeBytes, err := os.ReadFile(filepath.Join(dir, "before.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := timeline.Decode(beforeBytes)
	if err != nil {
		t.Fatal(err)
	}
	if got := timeline.Canonical(doc); !bytes.Equal(bytes.TrimRight(beforeBytes, "\n"), got) {
		t.Fatalf("before.json is not canonical:\n%s", got)
	}
	opBytes, err := os.ReadFile(filepath.Join(dir, "op.json"))
	if err != nil {
		t.Fatal(err)
	}
	var op Operation
	if err := json.Unmarshal(opBytes, &op); err != nil {
		t.Fatal(err)
	}
	// Identifiers in cases are supplied, never generated, so files are stable.
	// A case whose command draws ids from the generator lists them in newids.json.
	var generated []string
	if b, err := os.ReadFile(filepath.Join(dir, "newids.json")); err == nil {
		if err := json.Unmarshal(b, &generated); err != nil {
			t.Fatal(err)
		}
	}
	env := Env{NewIDs: func(prefix string) string {
		if len(generated) == 0 {
			t.Fatalf("the case needs a generated %s id; supply it in op.json or newids.json", prefix)
		}
		id := generated[0]
		generated = generated[1:]
		return id
	}}
	if b, err := os.ReadFile(filepath.Join(dir, "timelines.json")); err == nil {
		env.Timelines = caseGraphOf(t, doc, b)
	}
	res, applyErr := reg.Apply(doc, op, env)
	checkCaseName(t, filepath.Base(dir), applyErr)
	if applyErr == nil {
		checkDiffPatchesFetchedDocument(t, doc, res)
	}
	if applyErr != nil {
		e := AsError(applyErr)
		if cerr := CheckError(e); cerr != nil {
			t.Errorf("error breaks the rules of SDD-002 §A.8: %v", cerr)
		}
		compareError(t, dir, e)
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "error.json")); err == nil && !*update {
		t.Fatal("error.json exists but the operation succeeded")
	}
	if len(res.Spawned) > 0 {
		docs := make([]json.RawMessage, len(res.Spawned))
		for i, s := range res.Spawned {
			docs[i] = timeline.Canonical(s)
		}
		compareFile(t, dir, "spawned.json", goldenBytes(t, docs))
	}
	compareFile(t, dir, "after.json", append(timeline.Canonical(res.Doc), '\n'))
	compareFile(t, dir, "inverse.json", goldenBytes(t, nonNil(res.Inverse)))
	compareFile(t, dir, "diff.json", goldenBytes(t, nonNil(res.Diff)))
	compareFile(t, dir, "effective.json", goldenBytes(t, res.Effective))

	// Undo is exact: the inverse restores before.json byte for byte.
	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), timeline.Canonical(doc)) {
		t.Errorf("the inverse does not restore the document (skipped %v)", skipped)
	}
}

// caseGraph is the production of a case: the document under test and the others of timelines.json.
type caseGraph map[string]*timeline.Document

func (g caseGraph) Doc(id string) (*timeline.Document, bool) { d, ok := g[id]; return d, ok }

func (g caseGraph) Users(id string) []string {
	var out []string
	for uid, d := range g {
		if slices.Contains(timeline.CompositionTimelines(d), id) {
			out = append(out, uid)
		}
	}
	slices.Sort(out)
	return out
}

func caseGraphOf(t *testing.T, doc *timeline.Document, timelines []byte) caseGraph {
	t.Helper()
	var raws []json.RawMessage
	if err := json.Unmarshal(timelines, &raws); err != nil {
		t.Fatal(err)
	}
	g := caseGraph{doc.ID: doc}
	for _, r := range raws {
		d, err := timeline.Decode(r)
		if err != nil {
			t.Fatal(err)
		}
		g[d.ID] = d
	}
	return g
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// compareError compares error.json on code, path, op_index and the keys of details;
// the message is compared as text, because messages are part of the contract.
func compareError(t *testing.T, dir string, e *Error) {
	t.Helper()
	if *update {
		compareFile(t, dir, "error.json", goldenBytes(t, e))
		return
	}
	b, err := os.ReadFile(filepath.Join(dir, "error.json"))
	if err != nil {
		t.Errorf("the operation failed with %v and there is no error.json", e)
		return
	}
	var want Error
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if e.Code != want.Code || e.Path != want.Path || e.Message != want.Message || e.Hint != want.Hint ||
		!equalIndex(e.OpIndex, want.OpIndex) || !slices.Equal(detailKeys(e), detailKeys(&want)) {
		t.Errorf("error differs:\n--- got\n%s\n--- want\n%s", goldenBytes(t, e), b)
	}
}

func equalIndex(a, b *int) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func detailKeys(e *Error) []string {
	keys := make([]string, 0, len(e.Details))
	for k := range e.Details {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestGoldenRunner(t *testing.T) {
	if n := runGolden(t, testRegistry(t), "testdata/runner"); n < 5 {
		t.Errorf("only %d runner cases ran", n)
	}
}

// The real catalogue: its cases arrive with the commands (P1-ENG-002).
func TestGoldenCases(t *testing.T) {
	runGolden(t, fullRegistry(), "testdata")
}

// checkCaseName holds a case to its name: ok_ succeeds, err_<code> fails with that code.
func checkCaseName(t *testing.T, name string, err error) {
	t.Helper()
	switch {
	case strings.HasPrefix(name, "ok_"):
		if err != nil {
			t.Errorf("an ok_ case failed: %v", err)
		}
	case strings.HasPrefix(name, "err_"):
		e := AsError(err)
		if err == nil || !caseHasCode(name, e.Code) {
			t.Errorf("case %s should fail with its code, got %v", name, err)
		}
	default:
		t.Errorf("case name %q starts with neither ok_ nor err_", name)
	}
}
