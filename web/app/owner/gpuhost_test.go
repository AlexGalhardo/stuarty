package owner

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, p string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestLocateGPUHost(t *testing.T) {
	t.Run("next to the executable", func(t *testing.T) {
		root := t.TempDir()
		bin := filepath.Join(root, "bin")
		want := filepath.Join(bin, "stuarty-render.exe")
		touch(t, want)

		if got := locateGPUHost(bin, "windows"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("in libexec", func(t *testing.T) {
		root := t.TempDir()
		bin := filepath.Join(root, "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}

		want := filepath.Join(root, "libexec", "stuarty", "stuarty-render")
		touch(t, want)

		if got := locateGPUHost(bin, "linux"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if got := locateGPUHost(t.TempDir(), "linux"); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})

	t.Run("only for the gpu renderer", func(t *testing.T) {
		root := t.TempDir()
		touch(t, filepath.Join(root, "stuarty-render"))
		touch(t, filepath.Join(root, "stuarty-render.exe"))

		if got := gpuHostForRenderer("rust", root); got != "" {
			t.Fatalf("rust renderer got %q", got)
		}

		if got := gpuHostForRenderer("gpu", root); got == "" {
			t.Fatal("gpu renderer found nothing")
		}
	})
}
