package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
)

// TestCheckFailsOnAHandEdit: --check passes on what the generator wrote and
// fails, naming the file, once a generated file was edited by hand.
func TestCheckFailsOnAHandEdit(t *testing.T) {
	dir := t.TempDir()

	var out, errOut bytes.Buffer

	if code := run([]string{"-dir", dir}, &out, &errOut); code != 0 {
		t.Fatalf("generate: exit %d: %s", code, errOut.String())
	}

	out.Reset()

	if code := run([]string{"-check", "-dir", dir}, &out, &errOut); code != 0 || out.Len() != 0 {
		t.Fatalf("check on fresh files: exit %d: %s", code, out.String())
	}

	path := filepath.Join(dir, agentdocs.SkillFile)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, append(raw, "edited\n"...), 0o600); err != nil {
		t.Fatal(err)
	}

	code := run([]string{"-check", "-dir", dir}, &out, &errOut)
	if code != 1 || !bytes.Contains(out.Bytes(), []byte("SKILL.md")) {
		t.Fatalf("check after a hand edit: exit %d: %s", code, out.String())
	}
}
