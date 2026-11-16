package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// cargoMetadataFor builds the `cargo metadata` document of a workspace "app" that depends on
// "lib" (normal), "mac" (a proc macro) and "dev" (dev-only); "mac" needs "helper".
func cargoMetadataFor(dirs map[string]string) string {
	pkg := func(name, kind, source string) string {
		return `{"id":"` + name + `","name":"` + name + `","version":"1.0.0","license":"MIT","source":` + source +
			`,"manifest_path":"` + filepath.ToSlash(filepath.Join(dirs[name], "Cargo.toml")) +
			`","targets":[{"kind":["` + kind + `"]}]}`
	}

	reg := `"registry+https://github.com/rust-lang/crates.io-index"`
	dep := func(id string, kind string) string {
		return `{"pkg":"` + id + `","dep_kinds":[{"kind":` + kind + `}]}`
	}

	return `{"packages":[` + strings.Join([]string{
		pkg("app", "cdylib", "null"), pkg("lib", "lib", reg), pkg("mac", "proc-macro", reg),
		pkg("helper", "lib", reg), pkg("dev", "lib", reg),
	}, ",") + `],"workspace_members":["app"],"resolve":{"nodes":[` +
		`{"id":"app","deps":[` + dep("lib", "null") + "," + dep("mac", "null") + "," + dep("dev", `"dev"`) + `]},` +
		`{"id":"lib","deps":[]},{"id":"mac","deps":[` + dep("helper", "null") + `]},` +
		`{"id":"helper","deps":[]},{"id":"dev","deps":[]}]}}`
}

func TestRustCratesListsOnlyWhatIsShipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirs := map[string]string{}

	for _, n := range []string{"app", "lib", "mac", "helper", "dev"} {
		dirs[n] = filepath.Join(root, "crate-"+n)
		if err := os.MkdirAll(dirs[n], 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dirs[n], "LICENSE-MIT"), []byte(mitText), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.MkdirAll(filepath.Join(root, "renderer"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "renderer", "Cargo.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	var got []string

	run := func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)

		return []byte(cargoMetadataFor(dirs)), nil
	}

	comps, err := rustCrates(t.Context(), run, root)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, c := range comps {
		names = append(names, c.Name)
	}

	if !slices.Equal(names, []string{"lib"}) {
		t.Fatalf(
			"crates = %v, want only [lib]: the workspace member, the proc macro, what only it needs and the dev dependency are not shipped",
			names,
		)
	}

	if c := comps[0]; c.Group != groupCrates || c.Copyright != "Copyright (c) 2024 Example Authors" ||
		c.Licenses[0] != "licenses/crates/lib@1.0.0/LICENSE-MIT" {
		t.Fatalf("component = %+v", c)
	}

	if !slices.Contains(got, "--locked") || !slices.Contains(got, rendererTarget) {
		t.Fatalf("cargo was run as %v, want --locked and the wasm32 target", got)
	}
}

func TestRustCratesWithoutAWorkspaceIsEmpty(t *testing.T) {
	t.Parallel()

	run := func(context.Context, string, []string, string, ...string) ([]byte, error) {
		t.Fatal("cargo must not run without renderer/Cargo.lock")

		return nil, nil
	}

	if comps, err := rustCrates(t.Context(), run, t.TempDir()); err != nil || comps != nil {
		t.Fatalf("got %v, %v", comps, err)
	}
}

// TestDenyAllowlistComesFromTheNoticesAllowlist: renderer/deny.toml may only allow a licence that
// internal/tools/notices allows, and never one that must not link into the renderer.
func TestDenyAllowlistComesFromTheNoticesAllowlist(t *testing.T) {
	t.Parallel()

	deny := readFile(t, repoRoot(t), "renderer", "deny.toml")

	block := regexp.MustCompile(`(?s)\nallow = \[(.*?)\]`).FindStringSubmatch(deny)
	if block == nil {
		t.Fatal("renderer/deny.toml has no [licenses] allow list")
	}

	ids := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(block[1], -1)
	if len(ids) == 0 {
		t.Fatal("empty allow list")
	}

	for _, m := range ids {
		id := m[1]

		switch {
		case !allowed[id]:
			t.Errorf("deny.toml allows %s, which the notices allow-list does not", id)
		case id == "OFL-1.1" || strings.HasPrefix(id, "LGPL") || id == "MPL-2.0":
			t.Errorf("deny.toml must not allow %s in the renderer", id)
		}
	}
}
