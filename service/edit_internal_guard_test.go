package service

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// EditInternal lifts the refusal of internal commands; only tests may call it.
func TestSecurity_T05_EditInternalOnlyFromTests(t *testing.T) {
	root := ".."
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the module root is not at %s: %v", root, err)
	}

	skip := map[string]bool{
		".git": true, "node_modules": true, "testdata": true, "spikes": true, "development": true,
		"site": true, "dist": true, ".cache": true,
	}
	own := filepath.Join(root, "service", "edit.go")

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == own {
			return nil
		}

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		if strings.Contains(string(src), "EditInternal(") {
			t.Errorf("%s calls EditInternal; product code must not issue internal commands", path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
