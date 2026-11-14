package command

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
		"bad phase":      {withSpec(good, func(s *Spec) { s.Phase = "later" }), "phase must be"},
		"zero value":     {Command{}, "no arguments schema"},
		"several problems": {
			withSpec(good, func(s *Spec) { s.Example, s.Errors = "", nil }),
			"no example; no error list",
		},
	}
	for name, c := range cases {
		r := NewRegistry(Prototype)
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

func TestRegistryPhaseAndDuplicates(t *testing.T) {
	r := NewRegistry(Prototype)
	cmds := testCommands()
	for i := range cmds {
		if err := r.Register(cmds[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := r.Get("test.font"); ok {
		t.Error("an alpha command is registered in a prototype build")
	}
	if err := r.Register(testCommands()[0]); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate: %v", err)
	}
	alpha := NewRegistry(Alpha)
	alpha.MustRegister(testCommands()...)
	if _, ok := alpha.Get("test.font"); !ok {
		t.Error("an alpha build must have the alpha command")
	}
	if _, ok := alpha.Lookup("test.name"); !ok {
		t.Error("Lookup")
	}
}

func TestMustRegisterPanicsOnIncomplete(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	NewRegistry(Prototype).MustRegister(Command{})
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
		"duration (a whole number, required) between 1 and 108000", `Example: {"op": "test.item_add"`, "Errors: E_VALIDATION",
		"Scope: edit, phase: prototype", "text (a string, optional)",
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
