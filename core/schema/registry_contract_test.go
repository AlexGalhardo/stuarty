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

// mediaCommands are the commands whose scope is media; every other command is edit.
var mediaCommands = map[string]bool{"asset.add": true, "asset.remove": true, "font.add": true, "font.remove": true}

// The registry snapshot has the 77 commands of SDD-002 Part C; every scope is edit or media, and
// asset.add is the only internal command.
func TestRegistrySnapshotShape(t *testing.T) {
	t.Parallel()

	cmds := loadRegistry(t)
	for _, c := range cmds {
		if c.Scope != "edit" && c.Scope != "media" {
			t.Errorf("%s: scope %q is neither edit nor media", c.Name, c.Scope)
		}

		if (c.Scope == "media") != mediaCommands[c.Name] {
			t.Errorf("%s: scope %q is wrong; only asset.* and font.* are media", c.Name, c.Scope)
		}

		if c.Internal != (c.Name == "asset.add") {
			t.Errorf("%s: internal is %v; asset.add is the only internal command", c.Name, c.Internal)
		}
	}

	if len(cmds) != 77 {
		t.Errorf("%d commands, want 77", len(cmds))
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

// Every section of Part C has a registry entry, and the other way round, and both count 74.
func TestPartCSectionsMatchTheRegistry(t *testing.T) {
	t.Parallel()

	b, err := os.ReadFile("../../docs/sdds/002-command-catalogue.md")
	if err != nil {
		t.Fatal(err)
	}

	sections := regexp.MustCompile(`(?m)^### C\.\d+ \x60([a-z_.]+)\x60\s*$`).
		FindAllStringSubmatch(string(b), -1)
	want := map[string]bool{}

	for _, s := range sections {
		want[s[1]] = true
	}

	got := map[string]bool{}
	for _, c := range loadRegistry(t) {
		got[c.Name] = true
	}

	if len(want) != 77 {
		t.Fatalf("Part C has %d sections, want 77", len(want))
	}

	if len(sections) != len(want) {
		t.Errorf("Part C has %d sections for %d distinct commands", len(sections), len(want))
	}

	if len(got) != len(want) {
		t.Errorf("the registry snapshot has %d commands, Part C has %d", len(got), len(want))
	}

	for name := range want {
		if !got[name] {
			t.Errorf("Part C has a section for %s, which the registry does not have", name)
		}
	}

	for name := range got {
		if !want[name] {
			t.Errorf("%s is registered but has no section in Part C", name)
		}
	}
}

// Every command of the snapshot names a domain of SDD-000 section 1.1: the timeline registry is video, the
// image registry image (epic 13).
func TestRegistrySnapshotsCarryTheDomain(t *testing.T) {
	t.Parallel()

	known := []string{"project", "video", "image", "writing", "asset", "publish", "provider", "memory", "workspace"}

	for file, want := range map[string]string{"commands.json": "video", "image/commands.json": "image"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		var cmds []struct {
			Name   string `json:"name"`
			Domain string `json:"domain"`
		}
		if err := json.Unmarshal(b, &cmds); err != nil {
			t.Fatal(err)
		}

		if len(cmds) == 0 {
			t.Errorf("%s lists no command", file)
		}

		for _, c := range cmds {
			if !slices.Contains(known, c.Domain) {
				t.Errorf("%s: %s has domain %q, which SDD-000 section 1.1 does not list", file, c.Name, c.Domain)
			}

			if c.Domain != want {
				t.Errorf("%s: %s has domain %q, want %q", file, c.Name, c.Domain, want)
			}
		}
	}
}
