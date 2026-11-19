package image

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// A golden case is a directory testdata/<operation>/<case>/ with op.json, newids.json when the case draws
// identifiers from the generator, and either after.json, effective.json, diff.json and inverse.json, or
// error.json (SDD-021 section 3). The documents the cases start from are built in code (baseDoc), so the
// directory holds the result, which is what a person reviews. `go test ./core/image -update` rewrites them.
type gcase struct {
	cmd    string
	name   string
	before func() *Document // nil means baseDoc
	args   string
	ids    []string
}

var cases []gcase

func gc(cmd, name, args string, ids ...string) gcase {
	return gcase{cmd: cmd, name: name, args: args, ids: ids}
}

func (c gcase) on(before func() *Document) gcase {
	c.before = before

	return c
}

func addCases(cs ...gcase) { cases = append(cases, cs...) }

func pretty(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.MarshalIndent(tree(v), "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	return append(b, '\n')
}

func compareFile(t *testing.T, dir, name string, got []byte) {
	t.Helper()

	path := filepath.Join(dir, name)

	if *update {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%s: %v (run with -update)", path, err)

		return
	}

	if !bytes.Equal(bytes.TrimRight(want, "\r\n"), bytes.TrimRight(got, "\r\n")) {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func opOf(c gcase) command.Operation {
	return command.Operation{Op: c.cmd, Args: json.RawMessage(c.args)}
}

func TestGolden(t *testing.T) {
	for _, c := range cases {
		t.Run(c.cmd+"/"+c.name, func(t *testing.T) {
			dir := filepath.Join("testdata", c.cmd, c.name)
			doc := baseDoc()

			if c.before != nil {
				doc = c.before()
			}

			ReorderAll(doc)

			frozen := Canonical(doc)
			op := opOf(c)

			var arg any
			if err := json.Unmarshal([]byte(c.args), &arg); err != nil {
				t.Fatalf("the arguments of the case are not JSON: %v", err)
			}

			compareFile(t, dir, "op.json", pretty(t, map[string]any{"op": c.cmd, "args": arg}))

			if len(c.ids) > 0 {
				compareFile(t, dir, "newids.json", pretty(t, c.ids))
			}

			res, err := ApplyBatch(doc, []command.Operation{op}, testEnv(c.ids...))

			if !bytes.Equal(frozen, Canonical(doc)) {
				t.Fatal("the batch changed the document it was given")
			}

			if err != nil {
				e := command.AsError(err)
				if cerr := command.CheckError(e); cerr != nil {
					t.Errorf("the error breaks the rules of SDD-002 section A.8: %v", cerr)
				}

				compareFile(t, dir, "error.json", pretty(t, e))

				return
			}

			if vs := Validate(res.Doc); len(vs) > 0 {
				t.Fatalf("the result is not valid: %v", vs)
			}

			compareFile(t, dir, "after.json", pretty(t, res.Doc))
			compareFile(t, dir, "effective.json", pretty(t, res.Effective[0]))
			compareFile(t, dir, "diff.json", pretty(t, nonNil(res.Diff)))
			compareFile(t, dir, "inverse.json", pretty(t, nonNilP(res.Inverse)))
		})
	}
}

func nonNil(c []command.Change) []command.Change {
	if c == nil {
		return []command.Change{}
	}

	return c
}

func nonNilP(p []command.Primitive) []command.Primitive {
	if p == nil {
		return []command.Primitive{}
	}

	return p
}

// TestEveryCommandHasCases is the rule of SDD-021 section 3: an operation with no golden case fails the build,
// and so does one that lists an error code no case reaches.
func TestEveryCommandHasCases(t *testing.T) {
	reached := map[string]map[string]bool{}

	for _, c := range cases {
		if reached[c.cmd] == nil {
			reached[c.cmd] = map[string]bool{}
		}

		_, err := ApplyBatch(baseOrCustom(c), []command.Operation{opOf(c)}, testEnv(c.ids...))
		if err == nil {
			reached[c.cmd]["ok"] = true

			continue
		}

		reached[c.cmd][command.AsError(err).Code] = true
	}

	for _, cmd := range Registry().List() {
		got := reached[cmd.Name]
		if !got["ok"] {
			t.Errorf("%s has no golden case that succeeds", cmd.Name)
		}

		for _, code := range cmd.Errors {
			if !got[code] {
				t.Errorf("%s lists %s but no golden case returns it", cmd.Name, code)
			}
		}

		for code := range got {
			if code != "ok" && !slices.Contains(cmd.Errors, code) {
				t.Errorf("%s returned %s in a case but does not list it", cmd.Name, code)
			}
		}
	}
}

func baseOrCustom(c gcase) *Document {
	if c.before != nil {
		return c.before()
	}

	return baseDoc()
}

func TestRegistryIsComplete(t *testing.T) {
	want := []string{
		"canvas.resize", "image.create", "layer.add_fill", "layer.add_image", "layer.add_shape", "layer.add_text",
		"layer.align", "layer.crop", "layer.duplicate", "layer.fit", "layer.group", "layer.move", "layer.remove",
		"layer.set", "layer.set_shape", "layer.set_text", "layer.transform", "layer.ungroup",
	}

	if got := Registry().Names(); !slices.Equal(got, want) {
		t.Errorf("the catalogue is %v, want %v", got, want)
	}
}
