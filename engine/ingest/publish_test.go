package ingest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPublishFileIsReadOnlyAndKeepsAWinner(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "work.bin")
	dst := filepath.Join(dir, "final.bin")
	if err := os.WriteFile(src, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err == nil {
		t.Error("the work file is still there")
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(dst); st.Mode().Perm() != 0o400 {
			t.Errorf("mode %v", st.Mode().Perm())
		}
	}
	// A second job publishing the same name loses: the first file stays.
	src2 := filepath.Join(dir, "work2.bin")
	if err := os.WriteFile(src2, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishFile(src2, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "first" {
		t.Errorf("the winner was overwritten: %q", b)
	}
}

func TestRemoveArtifactsClearsReadOnlyFiles(t *testing.T) {
	root := t.TempDir()
	hash := "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"
	dir := filepath.Join(root, hash)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"proxy-1.mp4", "peaks-1.bin"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveArtifacts(root, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("the folder survived")
	}
	if err := RemoveArtifacts(root, hash); err != nil {
		t.Errorf("removing twice: %v", err)
	}
}

func TestArtifactName(t *testing.T) {
	for _, c := range []struct {
		kind    string
		profile int
		variant string
		ext     string
		want    string
	}{
		{"normalized", 1, "", "mov", "normalized-1.mov"},
		{"thumbnails", 1, "w160", "jpg", "thumbnails-1-w160.jpg"},
		{"peaks", 1, "", "bin", "peaks-1.bin"},
	} {
		if got := ArtifactName(c.kind, c.profile, c.variant, c.ext); got != c.want {
			t.Errorf("%s", got)
		}
	}
}
