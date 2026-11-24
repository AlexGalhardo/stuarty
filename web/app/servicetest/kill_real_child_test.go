//go:build crashtest

package servicetest

import (
	"encoding/hex"
	"os"
	"testing"
)

func hexDecode(s string) ([]byte, error) { return hex.DecodeString(s) }

// TestKillRealChild is the body of the child of the real-kill cases; as a normal test it does nothing. The role
// says which case the parent is running.
func TestKillRealChild(t *testing.T) {
	switch os.Getenv(killRealRoleEnv) {
	case "":
		t.Skip("only runs as the child of a real-kill test")
	case "publication":
		killRealPublicationChild(t)
	case "export-video":
		killRealExportChild(t, "video")
	case "export-image":
		killRealExportChild(t, "image")
	default:
		t.Fatalf("unknown role %q", os.Getenv(killRealRoleEnv))
	}
}
