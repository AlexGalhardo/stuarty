//go:build linux

package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SDD-007 section 6.4: no child sees an application variable, and the namespace probe of bwrap is a child.
func TestSecurity_T16_BwrapProbeEnvironmentMinimal(t *testing.T) {
	bin, out := t.TempDir(), filepath.Join(t.TempDir(), "env.txt")
	script := "#!/bin/sh\n/usr/bin/env > '" + out + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "bwrap"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("STUARTY_KEY", "s3cret")
	t.Setenv("APP_KEY", "s3cret")

	if _, err := ResolveSandbox(context.Background(), SandboxBwrap); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the probe did not run: %v", err)
	}
	if strings.Contains(string(got), "s3cret") || strings.Contains(string(got), "STUARTY_KEY") {
		t.Errorf("the bwrap probe inherited the application environment:\n%s", got)
	}
}
