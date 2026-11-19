package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// ROADMAP P1-ENG-001: a command registered without a schema, an example or its error
// list fails TestRegistryComplete. The Default registry is the one every surface
// reads; the test registry stands for the machinery.
func TestRegistryComplete(t *testing.T) {
	for name, r := range map[string]*Registry{"default": Default(), "test": testRegistry(t)} {
		for _, c := range r.List() {
			if err := c.Check(); err != nil {
				t.Errorf("%s registry: %v", name, err)
			}
			if _, err := r.Schema(c.Name); err != nil {
				t.Errorf("%s: no schema: %v", c.Name, err)
			}
			if h, err := r.Help(
				c.Name,
			); err != nil || !strings.Contains(h, c.Example) ||
				!strings.Contains(h, c.Summary) {
				t.Errorf("%s: help text %q: %v", c.Name, h, err)
			}
		}
	}
}

func TestRegisterRejectsIncompleteCommands(t *testing.T) {
	good := testCommands()[0]
	noSchema := good
	noSchema.argsType = nil
	notStruct := Define(good.Spec, func(*Context, *string) error { return nil })
	noApply := good
	noApply.apply = nil
	cases := map[string]struct {
		cmd  Command
		want string
	}{
		"no schema":       {noSchema, "no arguments schema"},
		"args not struct": {notStruct, "no arguments schema"},
		"no apply":        {noApply, "no apply function"},
		"no example":      {withSpec(good, func(s *Spec) { s.Example = "" }), "no example"},
		"bad example":     {withSpec(good, func(s *Spec) { s.Example = `{"nope": 1}` }), "example does not satisfy"},
		"no error list":   {withSpec(good, func(s *Spec) { s.Errors = nil }), "no error list"},
		"unknown error":   {withSpec(good, func(s *Spec) { s.Errors = []string{"E_NOPE"} }), "unknown error code"},
		"repeated error": {
			withSpec(good, func(s *Spec) { s.Errors = []string{CodeValidation, CodeValidation} }),
			"listed twice",
		},
		"bad name":       {withSpec(good, func(s *Spec) { s.Name = "Rename" }), "namespace.action"},
		"no summary":     {withSpec(good, func(s *Spec) { s.Summary = " " }), "no summary"},
		"no description": {withSpec(good, func(s *Spec) { s.Description = "" }), "no description"},
		"bad scope":      {withSpec(good, func(s *Spec) { s.Scope = "root" }), "scope must be"},
		"zero value":     {Command{}, "no arguments schema"},
		"several problems": {
			withSpec(good, func(s *Spec) { s.Example, s.Errors = "", nil }),
			"no example; no error list",
		},
	}
	for name, c := range cases {
		r := NewRegistry()
		err := r.Register(c.cmd)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
		if len(r.List()) != 0 {
			t.Errorf("%s: the command was stored", name)
		}
	}
}

func withSpec(c Command, f func(*Spec)) Command {
	f(&c.Spec)
	return c
}

func TestRegistryDuplicates(t *testing.T) {
	r := NewRegistry()
	cmds := testCommands()
	for i := range cmds {
		if err := r.Register(cmds[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := r.Get("test.font"); !ok {
		t.Error("a registered command must be found")
	}
	if err := r.Register(testCommands()[0]); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate: %v", err)
	}
	if _, ok := r.Lookup("test.name"); !ok {
		t.Error("Lookup")
	}
}

func TestMustRegisterPanicsOnIncomplete(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	NewRegistry().MustRegister(Command{})
}

func TestRegisterOnDefaultPanicsOnIncomplete(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	Register(Command{})
}

func TestListNamesSchemaHelp(t *testing.T) {
	r := testRegistry(t)
	names := r.Names()
	list := r.List()
	if len(names) != len(list) || len(names) == 0 {
		t.Fatal(names)
	}
	for i := range names {
		if names[i] != list[i].Name || i > 0 && names[i-1] >= names[i] {
			t.Errorf("not sorted at %d: %v", i, names)
		}
	}
	raw, err := r.Schema("test.item_add")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if s["$schema"] != "https://json-schema.org/draft/2020-12/schema" || s["additionalProperties"] != false {
		t.Errorf("%s", raw)
	}
	help, err := r.Help("test.item_add")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"test.item_add: Adds a text item.", "Arguments:", "track_id (a string, required)",
		"duration (a whole number, required) between 1 and 108000", `Example: {"op": "test.item_add"`, "ids in the example are placeholders", "Errors: E_VALIDATION",
		"Scope: edit", "text (a string, optional)",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
	if h, _ := r.Help("test.track_add"); !strings.Contains(h, "one of visual, audio") {
		t.Errorf("enum missing:\n%s", h)
	}
	if h, _ := r.Help("test.noop"); !strings.Contains(h, "  none") {
		t.Errorf("no-argument help:\n%s", h)
	}
	for _, f := range []func(string) error{
		func(n string) error { _, err := r.Schema(n); return err },
		func(n string) error { _, err := r.Help(n); return err },
	} {
		err := f("nope.nope")
		e, ok := errors.AsType[*Error](err)
		if !ok || e.Code != CodeUnknownCommand || e.Details["known"] == nil || CheckError(e) != nil {
			t.Errorf("unknown: %v", err)
		}
	}
}

// snapshotEntry is a command of core/schema/commands.json, the contract the surfaces read.
type snapshotEntry struct {
	Name     string `json:"name"`
	Internal bool   `json:"internal"`
}

// TestRegistryMatchesSnapshot compares the registry, which the init function of every
// command file fills, with the generated commands.json: a command that is registered
// but absent from the snapshot, or listed there but not registered, fails.
func TestRegistryMatchesSnapshot(t *testing.T) {
	b, err := os.ReadFile("../schema/commands.json")
	if err != nil {
		t.Fatal(err)
	}

	var snapshot []snapshotEntry
	if err := json.Unmarshal(b, &snapshot); err != nil {
		t.Fatal(err)
	}

	want := map[string]snapshotEntry{}
	for _, e := range snapshot {
		want[e.Name] = e
	}

	if len(want) != len(snapshot) {
		t.Error("commands.json lists a command twice")
	}

	for _, c := range Default().List() {
		e, ok := want[c.Name]

		switch {
		case !ok:
			t.Errorf("%s is registered but absent from commands.json (run go run ./internal/tools/contractgen)", c.Name)
		case e.Internal != c.Internal:
			t.Errorf("%s: the snapshot says internal %v, the registry %v", c.Name, e.Internal, c.Internal)
		}

		delete(want, c.Name)
	}

	for name := range want {
		t.Errorf("%s is in commands.json but not registered", name)
	}
}

// goldenCaseDirs lists the case directories of a command, split by outcome.
func goldenCaseDirs(t *testing.T, name string) (ok, failing []string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join("testdata", name))
	if err != nil {
		t.Errorf("%s: no golden directory: %v", name, err)

		return nil, nil
	}

	for _, e := range entries {
		switch {
		case !e.IsDir():
		case strings.HasPrefix(e.Name(), "ok_"):
			ok = append(ok, e.Name())
		case strings.HasPrefix(e.Name(), "err_"):
			failing = append(failing, e.Name())
		}
	}

	return ok, failing
}

// PRD-001 metric 3: every command of the registry has a golden directory with at least
// one success and one error case, one error case per code it lists, and every success
// case carries the inverse that the runner applies to get the starting bytes back.
func TestEveryCommandHasGoldenAndUndo(t *testing.T) {
	cmds := fullRegistry().List()
	if len(cmds) != 74 {
		t.Errorf("the registry has %d commands, want 74", len(cmds))
	}

	for _, c := range cmds {
		ok, failing := goldenCaseDirs(t, c.Name)
		if len(ok) == 0 || len(failing) == 0 {
			t.Errorf("%s: %d success and %d error cases; it needs at least one of each", c.Name, len(ok), len(failing))
		}

		for _, code := range c.Errors {
			if !coveredElsewhere(c.Name, code) &&
				!slices.ContainsFunc(failing, func(n string) bool { return caseHasCode(n, code) }) {
				t.Errorf("%s lists %s but has no err_ case for it", c.Name, code)
			}
		}

		for _, n := range ok {
			if _, err := os.Stat(filepath.Join("testdata", c.Name, n, "inverse.json")); err != nil {
				t.Errorf("%s/%s has no inverse.json: %v", c.Name, n, err)
			}
		}
	}
}

// timePaths collects the paths, inside args decoded from JSON, of the values that the
// arguments struct types as Time.
func timePaths(t reflect.Type, v any, path []any, out *[][]any) {
	switch {
	case t == timeType:
		*out = append(*out, slices.Clone(path))
	case t.Kind() == reflect.Pointer:
		timePaths(t.Elem(), v, path, out)
	case t.Kind() == reflect.Slice:
		if list, ok := v.([]any); ok {
			for i, e := range list {
				timePaths(t.Elem(), e, append(slices.Clone(path), i), out)
			}
		}
	case t.Kind() == reflect.Struct:
		if m, ok := v.(map[string]any); ok {
			for _, f := range argFields(t) {
				if e, has := m[f.name]; has {
					timePaths(f.fieldType, e, append(slices.Clone(path), f.name), out)
				}
			}
		}
	}
}

func setAt(root any, path []any, value any) {
	cur := root

	for _, p := range path[:len(path)-1] {
		switch k := p.(type) {
		case string:
			cur = cur.(map[string]any)[k]
		case int:
			cur = cur.([]any)[k]
		}
	}

	switch k := path[len(path)-1].(type) {
	case string:
		cur.(map[string]any)[k] = value
	case int:
		cur.([]any)[k] = value
	}
}

func decodeJSON(t *testing.T, raw []byte) any {
	t.Helper()

	var v any

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}

	return v
}

// RF-113: a time given as a JSON number or as a string without a unit is E_INVALID_TIME,
// for every time argument of every command. The argument is changed in the operations of
// the success cases, which reach the time check with every other precondition met.
func TestNoBareNumberTimes(t *testing.T) {
	checked := map[string]int{}

	for _, c := range fullRegistry().List() {
		ok, _ := goldenCaseDirs(t, c.Name)
		for _, name := range ok {
			dir := filepath.Join("testdata", c.Name, name)

			before, err := os.ReadFile(filepath.Join(dir, "before.json"))
			if err != nil {
				t.Fatal(err)
			}

			opBytes, err := os.ReadFile(filepath.Join(dir, "op.json"))
			if err != nil {
				t.Fatal(err)
			}

			var probe Operation
			if err := json.Unmarshal(opBytes, &probe); err != nil {
				t.Fatal(err)
			}

			var paths [][]any
			timePaths(c.argsType, decodeJSON(t, probe.Args), nil, &paths)

			for _, p := range paths {
				for _, bare := range []any{json.Number("5"), "5"} {
					doc, err := timeline.Decode(before)
					if err != nil {
						t.Fatal(err)
					}

					mutated := decodeJSON(t, probe.Args)
					setAt(mutated, p, bare)

					raw, err := json.Marshal(mutated)
					if err != nil {
						t.Fatal(err)
					}

					_, err = fullRegistry().Apply(doc, Operation{Op: c.Name, Args: raw}, Env{NewIDs: (&seqIDs{}).next})
					if e := AsError(err); err == nil || e.Code != CodeInvalidTime {
						t.Errorf("%s/%s: %v given as %v gave %v, want E_INVALID_TIME", c.Name, name, p, bare, err)
					}

					checked[c.Name]++
				}
			}
		}
	}

	for _, c := range fullRegistry().List() {
		if hasTimeField(c.argsType) && checked[c.Name] == 0 {
			t.Errorf("%s has a time argument that no success case sets", c.Name)
		}
	}
}

// hasTimeField says whether a struct type, or one nested in it, has a Time field.
func hasTimeField(t reflect.Type) bool {
	switch {
	case t == timeType:
		return true
	case t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice:
		return hasTimeField(t.Elem())
	case t.Kind() == reflect.Struct:
		return slices.ContainsFunc(argFields(t), func(f argField) bool { return hasTimeField(f.fieldType) })
	}
	return false
}

// nameLikeValues returns the strings that a document stores under a key called name or
// text: labels and content that a message must not repeat.
func nameLikeValues(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if s, ok := e.(string); ok && (k == "name" || k == "text") && len(s) >= 4 {
				out[s] = true
			}

			if k == "slot" {
				continue // a slot name is an identifier, quoted like an id
			}

			nameLikeValues(e, out)
		}
	case []any:
		for _, e := range x {
			nameLikeValues(e, out)
		}
	}
}

// P2-SEC-005 (shared): the message and the hint of every golden error quote identifiers
// and numbers, never a name or a text of the document the case runs on.
func TestGoldenErrorsQuoteIdsNotNames(t *testing.T) {
	n := 0

	for _, c := range fullRegistry().List() {
		_, failing := goldenCaseDirs(t, c.Name)
		for _, name := range failing {
			dir := filepath.Join("testdata", c.Name, name)

			before, err := os.ReadFile(filepath.Join(dir, "before.json"))
			if err != nil {
				t.Fatal(err)
			}

			errBytes, err := os.ReadFile(filepath.Join(dir, "error.json"))
			if err != nil {
				t.Fatal(err)
			}

			var e Error
			if err := json.Unmarshal(errBytes, &e); err != nil {
				t.Fatal(err)
			}

			values := map[string]bool{}
			nameLikeValues(decodeJSON(t, before), values)

			for v := range values {
				if strings.Contains(e.Message+" "+e.Hint, v) {
					t.Errorf("%s/%s quotes %q of its document: %s %s", c.Name, name, v, e.Message, e.Hint)
				}
			}

			n++
		}
	}

	if n < 350 {
		t.Errorf("only %d error cases were checked", n)
	}
}

func TestHelpCarriesTheSchemaAndTheShapeOfListItems(t *testing.T) {
	r := FullRegistry()

	for name, want := range map[string][]string{
		"caption.add": {
			"each item of words: {text (a string, required), start (a time, required), duration (a time, required)}",
			`JSON Schema: {"$schema":"https://json-schema.org/draft/2020-12/schema"`,
		},
		"font.add": {"each item of files: {weight (a whole number, required), hash (a string, required)}"},
	} {
		help, err := r.Help(name)
		if err != nil {
			t.Fatal(err)
		}

		for _, w := range want {
			if !strings.Contains(help, w) {
				t.Errorf("%s: help lacks %q:\n%s", name, w, help)
			}
		}
	}
}
