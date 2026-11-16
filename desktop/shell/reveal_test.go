package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRevealCommand(t *testing.T) {
	tests := []struct {
		goos, path string
		name       string
		args       []string
	}{
		{"windows", `C:\out\a b.mp4`, "explorer.exe", []string{`/select,C:\out\a b.mp4`}},
		{"darwin", "/out/a.mp4", "open", []string{"-R", "/out/a.mp4"}},
		{"linux", "/out/a.mp4", "xdg-open", []string{filepath.Dir("/out/a.mp4")}},
	}
	for _, tc := range tests {
		name, args := RevealCommand(tc.goos, tc.path)
		if name != tc.name || !reflect.DeepEqual(args, tc.args) {
			t.Errorf("RevealCommand(%s) = %s %q; want %s %q", tc.goos, name, args, tc.name, tc.args)
		}
	}
}

func TestCheckRevealPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckRevealPath(file); err != nil {
		t.Errorf("a real file was refused: %v", err)
	}
	for _, bad := range []string{"", "a.mp4", dir, filepath.Join(dir, "missing.mp4"), file + "\x00"} {
		if CheckRevealPath(bad) == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
