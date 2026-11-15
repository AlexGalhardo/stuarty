package cli

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()

	defs := []flagDef{
		{Name: "json", Kind: kindBool, Default: "false"},
		{Name: "wait", Kind: kindBool, Default: "true"},
		{Name: "limit", Kind: kindInt, Default: "20"},
		{Name: "name", Kind: kindString},
		{Name: "asset-id", Kind: kindString, Repeat: true},
		{Name: "timeout", Kind: kindDuration, Default: "10m"},
	}

	p, err := parseArgs(
		defs,
		[]string{
			"a",
			"--json",
			"b",
			"--name=x y",
			"--limit",
			"5",
			"--asset-id",
			"ast_1",
			"--asset-id=ast_2",
			"--no-wait",
			"-5",
			"--",
			"--json",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(p.Positional, []string{"a", "b", "-5", "--json"}) {
		t.Fatalf("positional = %v", p.Positional)
	}

	if !p.boolean("json") || p.boolean("wait") || p.integer("limit") != 5 || p.str("name") != "x y" {
		t.Fatalf("flags not read: %+v", p.values)
	}

	if !reflect.DeepEqual(p.all("asset-id"), []string{"ast_1", "ast_2"}) {
		t.Fatalf("a repeatable flag keeps its order: %v", p.all("asset-id"))
	}

	if p.timeout().String() != "10m0s" {
		t.Fatalf("the default must apply: %v", p.timeout())
	}
}

func TestParseArgsErrors(t *testing.T) {
	t.Parallel()

	defs := []flagDef{
		{Name: "limit", Kind: kindInt},
		{Name: "json", Kind: kindBool},
		{Name: "timeout", Kind: kindDuration},
	}

	for name, args := range map[string][]string{
		"unknown":           {"--nope"},
		"missing value":     {"--limit"},
		"bad integer":       {"--limit", "x"},
		"bad boolean":       {"--json=maybe"},
		"bad duration":      {"--timeout", "soon"},
		"no- of a non-bool": {"--no-limit"},
	} {
		if _, err := parseArgs(defs, args); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestAbsMakesPathsAbsoluteAgainstTheCurrentDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rt := &Runtime{Getwd: func() (string, error) { return dir, nil }}

	got, err := rt.abs(filepath.Join("clips", "..", "a.mov"))
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(dir, "a.mov"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	abs := filepath.Join(dir, "x", "..", "b.mov")

	if got, _ := rt.abs(abs); got != filepath.Join(dir, "b.mov") {
		t.Fatalf("an absolute path is cleaned: %q", got)
	}
}
