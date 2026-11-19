package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot is the repository root, two levels above internal/tools/notices.
func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	return root
}

func readFile(t *testing.T, root string, rel ...string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(append([]string{root}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}

	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// TestRendererToolchainIsPinned: rule 2 of .agents/rules/rust.md.
func TestRendererToolchainIsPinned(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	tc := readFile(t, root, "rust-toolchain.toml")

	if !regexp.MustCompile(`(?m)^channel = "\d+\.\d+\.\d+"$`).MatchString(tc) {
		t.Error("rust-toolchain.toml must pin an exact stable version")
	}

	for _, want := range []string{`"wasm32-unknown-unknown"`, `"rustfmt"`, `"clippy"`} {
		if !strings.Contains(tc, want) {
			t.Errorf("rust-toolchain.toml lacks %s", want)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "renderer", "Cargo.lock")); err != nil {
		t.Errorf("Cargo.lock must be committed: %v", err)
	}

	dep := regexp.MustCompile(`(?m)^([a-z0-9_-]+) = "([^"]+)"$`)

	for _, m := range dep.FindAllStringSubmatch(readFile(t, root, "renderer", "stuarty-renderer", "Cargo.toml"), -1) {
		if !strings.HasPrefix(m[2], "=") && m[1] != "name" && m[1] != "description" && m[1] != "version" {
			t.Errorf("crate %s is at %q, want an exact version (=x.y.z)", m[1], m[2])
		}
	}
}
