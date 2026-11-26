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

	for _, lock := range [][]string{
		{"renderer", "Cargo.lock"},
		{"aihelper", "Cargo.lock"},
		{"capture", "Cargo.lock"},
	} {
		if _, err := os.Stat(filepath.Join(append([]string{root}, lock...)...)); err != nil {
			t.Errorf("%s must be committed: %v", filepath.Join(lock...), err)
		}
	}

	bareDep := regexp.MustCompile(`^([a-z0-9_-]+) = "([^"]+)"$`)
	tableDep := regexp.MustCompile(`^([a-z0-9_-]+) = \{[^}]*version = "([^"]+)"`)

	manifests := [][]string{
		{"renderer", "stuarty-renderer", "Cargo.toml"},
		{"renderer", "stuarty-renderer-gpu", "Cargo.toml"},
		{"renderer", "stuarty-renderer-text", "Cargo.toml"},
		{"aihelper", "Cargo.toml"},
		{"capture", "Cargo.toml"},
	}

	for _, rel := range manifests {
		content := readFile(t, root, rel...)
		label := filepath.Join(rel...)
		inDeps := false

		for line := range strings.SplitSeq(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") {
				inDeps = strings.HasSuffix(trimmed, "dependencies]")
				continue
			}

			if !inDeps || trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}

			if m := bareDep.FindStringSubmatch(trimmed); m != nil {
				if !strings.HasPrefix(m[2], "=") {
					t.Errorf("%s: crate %s is at %q, want an exact version (=x.y.z)", label, m[1], m[2])
				}
			} else if m := tableDep.FindStringSubmatch(trimmed); m != nil {
				if !strings.HasPrefix(m[2], "=") {
					t.Errorf("%s: crate %s is at %q, want an exact version (=x.y.z)", label, m[1], m[2])
				}
			}
		}
	}
}
