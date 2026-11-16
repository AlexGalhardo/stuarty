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

// cargoMetadataFor builds the `cargo metadata` document of a workspace "vukem-renderer" that depends on
// "lib" (normal), "mac" (a proc macro) and "dev" (dev-only); "mac" needs "helper". A second member,
// "vukem-renderer-gpu", is the native host and brings "gpudep", which is listed for the native targets only.
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
		pkg("vukem-renderer", "cdylib", "null"),
		pkg("lib", "lib", reg),
		pkg("mac", "proc-macro", reg),
		pkg(
			"helper",
			"lib",
			reg,
		),
		pkg("dev", "lib", reg),
		pkg("vukem-renderer-gpu", "bin", "null"),
		pkg("gpudep", "lib", reg),
	}, ",") + `],"workspace_members":["vukem-renderer","vukem-renderer-gpu"],"resolve":{"nodes":[` +
		`{"id":"vukem-renderer","deps":[` + dep("lib", "null") + "," + dep("mac", "null") + "," + dep("dev", `"dev"`) + `]},` +
		`{"id":"lib","deps":[]},{"id":"mac","deps":[` + dep("helper", "null") + `]},` +
		`{"id":"helper","deps":[]},{"id":"dev","deps":[]},` +
		`{"id":"vukem-renderer-gpu","deps":[` + dep("gpudep", "null") + `]},{"id":"gpudep","deps":[]}]}}`
}

func TestRustCratesListsOnlyWhatIsShipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirs := map[string]string{}

	for _, n := range []string{"vukem-renderer", "lib", "mac", "helper", "dev", "vukem-renderer-gpu", "gpudep"} {
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

	var got [][]string

	run := func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		got = append(got, append([]string{name}, args...))

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

	if !slices.Equal(names, []string{"gpudep", "lib"}) {
		t.Fatalf(
			"crates = %v, want [gpudep lib]: the workspace members, the proc macro, what only it needs and the dev dependency are not shipped",
			names,
		)
	}

	if c := comps[1]; c.Group != groupCrates || c.Copyright != "Copyright (c) 2024 Example Authors" ||
		c.Licenses[0] != "licenses/crates/lib@1.0.0/LICENSE-MIT" {
		t.Fatalf("component = %+v", c)
	}

	if len(got) != 1+len(gpuHostTargets) {
		t.Fatalf("cargo ran %d times, want the wasm32 target and %d native targets", len(got), len(gpuHostTargets))
	}

	for _, run := range got {
		if !slices.Contains(run, "--locked") {
			t.Fatalf("cargo was run as %v, want --locked", run)
		}
	}

	if !slices.Contains(got[0], rendererTarget) || !slices.Contains(got[1], gpuHostTargets[0]) {
		t.Fatalf("cargo was run as %v, want the wasm32 target first and then the native ones", got)
	}
}

// TestRustCratesUsesTheKeptTextWhenACrateShipsNone: a crate whose package has no licence file takes the
// text kept in licenses/crate-texts/<crate>, and the legacy "A/B" expression reads as "A OR B".
func TestRustCratesUsesTheKeptTextWhenACrateShipsNone(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pkgDir := filepath.Join(root, "crate-bare")
	kept := filepath.Join(root, filepath.FromSlash(crateTextsDir), "bare")

	for _, d := range []string{pkgDir, kept} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(kept, "LICENSE-MIT"), []byte(mitText), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := crateComponent(root, cargoPackage{
		Name: "bare", Version: "2.0.0", License: "MIT/Apache-2.0", ManifestPath: filepath.Join(pkgDir, "Cargo.toml"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if c.SPDX != "MIT OR Apache-2.0" || len(c.Licenses) != 1 ||
		c.Licenses[0] != "licenses/crates/bare@2.0.0/LICENSE-MIT" {
		t.Fatalf("component = %+v", c)
	}

	if _, err := crateComponent(
		t.TempDir(),
		cargoPackage{Name: "bare", ManifestPath: filepath.Join(pkgDir, "Cargo.toml")},
	); err == nil {
		t.Fatal("a crate with no text anywhere must fail")
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
