package oauth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
)

func TestHostIDIsMadeOnceAndKept(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "providers", "host-id")

	first, err := oauth.HostID(path)
	if err != nil || !strings.HasPrefix(first, "urn:uuid:") || len(first) != len("urn:uuid:")+36 {
		t.Fatalf("HostID = %q, %v", first, err)
	}

	again, err := oauth.HostID(path)
	if err != nil || again != first {
		t.Fatalf("the identifier changed: %q then %q (%v)", first, again, err)
	}

	other, err := oauth.HostID(filepath.Join(t.TempDir(), "host-id"))
	if err != nil || other == first {
		t.Fatalf("two hosts must have distinct identifiers: %q and %q", first, other)
	}
}

func TestHostIDRefusesAFileThatIsNotOne(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "host-id")
	if err := os.WriteFile(path, []byte("not a uuid"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := oauth.HostID(path); err == nil {
		t.Fatal("a corrupt file was taken for an identifier")
	}
}
