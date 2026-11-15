package schema

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"testing"
)

type registryEntry struct {
	Name     string   `json:"name"`
	Phase    string   `json:"phase"`
	Scope    string   `json:"scope"`
	Internal bool     `json:"internal"`
	Errors   []string `json:"errors"`
}

func loadRegistry(t *testing.T) []registryEntry {
	t.Helper()

	b, err := Files.ReadFile("commands.json")
	if err != nil {
		t.Fatal(err)
	}

	var cmds []registryEntry
	if err := json.Unmarshal(b, &cmds); err != nil {
		t.Fatal(err)
	}

	return cmds
}

// The registry snapshot has the 48 commands of SDD-002 Part C: 16 of the Prototype and
// 32 of ALPHA, every scope is edit or media, and asset.add is the only internal command.
func TestRegistrySnapshotShape(t *testing.T) {
	t.Parallel()

	cmds := loadRegistry(t)
	phases := map[string]int{}

	for _, c := range cmds {
		phases[c.Phase]++

		if c.Scope != "edit" && c.Scope != "media" {
			t.Errorf("%s: scope %q is neither edit nor media", c.Name, c.Scope)
		}

		if (c.Scope == "media") != (c.Name == "asset.add" || c.Name == "asset.remove") {
			t.Errorf("%s: scope %q is wrong; only asset.add and asset.remove are media", c.Name, c.Scope)
		}

		if c.Internal != (c.Name == "asset.add") {
			t.Errorf("%s: internal is %v; asset.add is the only internal command", c.Name, c.Internal)
		}
	}

	if len(cmds) != 48 || phases["prototype"] != 16 || phases["alpha"] != 32 {
		t.Errorf("%d commands (%v), want 48 with 16 prototype and 32 alpha (the exposed build)", len(cmds), phases)
	}
}

// Every error code a command lists is a code of SDD-002 A.8 (errors.json).
func TestCommandErrorCodesExist(t *testing.T) {
	t.Parallel()

	b, err := Files.ReadFile("errors.json")
	if err != nil {
		t.Fatal(err)
	}

	var codes []struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(b, &codes); err != nil {
		t.Fatal(err)
	}

	known := make([]string, len(codes))
	for i, c := range codes {
		known[i] = c.Code
	}

	for _, c := range loadRegistry(t) {
		for _, code := range c.Errors {
			if !slices.Contains(known, code) {
				t.Errorf("%s lists %s, which is not in errors.json", c.Name, code)
			}
		}
	}
}

// Every section of Part C has a registry entry of the same phase, and the other way round.
func TestPartCSectionsMatchTheRegistry(t *testing.T) {
	t.Parallel()

	b, err := os.ReadFile("../../docs/sdds/002-command-catalogue.md")
	if err != nil {
		t.Fatal(err)
	}

	sections := regexp.MustCompile(`(?m)^### C\.\d+ \x60([a-z_.]+)\x60 \(phase ([PAB])\b`).
		FindAllStringSubmatch(string(b), -1)
	want := map[string]string{}

	for _, s := range sections {
		want[s[1]] = map[string]string{"P": "prototype", "A": "alpha", "B": "beta"}[s[2]]
	}

	got := map[string]string{}
	for _, c := range loadRegistry(t) {
		got[c.Name] = c.Phase
	}

	if len(want) != 54 {
		t.Fatalf("Part C has %d sections, want 54", len(want))
	}

	for name, phase := range want {
		if phase == "beta" {
			// A Beta command is registered but not exposed until the BETA phase switch.
			if _, exposed := got[name]; exposed {
				t.Errorf("Part C says %s is beta but the exposed build lists it", name)
			}

			delete(want, name)

			continue
		}

		if got[name] != phase {
			t.Errorf("Part C says %s is %q, the registry has %q", name, phase, got[name])
		}
	}

	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s is registered but has no section in Part C", name)
		}
	}
}
