package resources

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedFilesAreServed(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	if _, err := fs.Stat(Public(), "robots.txt"); err != nil {
		t.Errorf("Public() has no robots.txt: %v", err)
	}

	// The pages link it, so the browser does not ask for /favicon.ico and log a 404 (VV-124).
	if _, err := fs.Stat(Public(), "icon.png"); err != nil {
		t.Errorf("Public() has no icon.png: %v", err)
	}

	if Views() == nil || Lang() == nil {
		t.Error("Views() or Lang() is nil")
	}
}

// In development the resources directory next to the process wins, so a
// template edit shows on refresh; in production the embedded files always do.
func TestDirectoryWinsOnlyOutsideProduction(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "resources", "public"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "resources", "public", "marker.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)

	t.Setenv("APP_ENV", "development")

	if _, err := fs.Stat(Public(), "marker.txt"); err != nil {
		t.Errorf("development did not use the directory: %v", err)
	}

	t.Setenv("APP_ENV", "production")

	if _, err := fs.Stat(Public(), "marker.txt"); err == nil {
		t.Error("production used the directory instead of the embedded files")
	}
}
