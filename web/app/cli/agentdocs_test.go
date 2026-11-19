package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
)

func TestAgentDocsPrintEqualsTheCommittedFile(t *testing.T) {
	t.Parallel()

	for kind, name := range agentdocs.Kinds {
		want, err := agentdocs.File(name)
		if err != nil {
			t.Fatal(err)
		}

		h := newHarness(t)

		if code := h.run("agent:docs", "--print", kind); code != ExitOK {
			t.Fatalf("--print %s: exit %d", kind, code)
		}

		if !bytes.Equal(h.stdout.Bytes(), want) {
			t.Errorf("--print %s differs from out/%s", kind, name)
		}
	}
}

func TestAgentDocsWriteAndBadUse(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "skill")
	h := newHarness(t)

	if code := h.run("agent:docs", "--write", dir); code != ExitOK {
		t.Fatalf("--write: exit %d", code)
	}

	for _, name := range agentdocs.Files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}

		want, _ := agentdocs.File(name)
		if !bytes.Equal(got, want) {
			t.Errorf("%s was written with other bytes", name)
		}
	}

	for _, args := range [][]string{{}, {"--print", "nope"}, {"--print", "skill", "--write", dir}} {
		if code := newHarness(t).run(append([]string{"agent:docs"}, args...)...); code != ExitFailure {
			t.Errorf("agent:docs %s: exit %d, want 1", strings.Join(args, " "), code)
		}
	}
}
