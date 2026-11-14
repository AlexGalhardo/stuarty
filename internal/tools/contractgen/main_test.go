package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// The committed contracts are what the generator writes now: editing a Go type
// without running `task codegen` fails here as it fails the CI step.
func TestCommittedContractsAreCurrent(t *testing.T) {
	tmp := t.TempDir()
	if err := run(tmp); err != nil {
		t.Fatal(err)
	}
	committed := filepath.Join("..", "..", "..", "core", "schema")
	n := 0
	err := filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		rel, _ := filepath.Rel(tmp, path)
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(committed, rel))
		if err != nil {
			t.Errorf("%s is not committed: %v", rel, err)
			return nil
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s is stale: run task codegen", rel)
		}
		return nil
	})
	if err != nil || n < 20 {
		t.Fatalf("%d files: %v", n, err)
	}
}
