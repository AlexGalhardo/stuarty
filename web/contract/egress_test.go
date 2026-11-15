package contract

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

// TestSecurity_T10_DefaultRunOpensNoOutboundConnection (SDD-007 section 10.2): with every
// VUKEM_VIDEO_* variable unset, the prototype scenario (create, import, edit, lint, view, export,
// undo) makes no dial through any outbound purpose.
func TestSecurity_T10_DefaultRunOpensNoOutboundConnection(t *testing.T) {
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(name, "VUKEM_VIDEO_") {
			continue
		}

		_ = os.Unsetenv(name)

		t.Cleanup(func() { _ = os.Setenv(name, value) })
	}

	var dials atomic.Int32

	restore := outbound.SetDialRecorder(func(p outbound.Purpose, address string) error {
		dials.Add(1)
		t.Errorf("outbound dial for %s to %s", p, address)

		return outbound.ErrNotLoopback
	})
	defer restore()

	sameThroughEverySurface(t, "prototype", "prj_7k2m9x4q1b")

	if n := dials.Load(); n != 0 {
		t.Fatalf("%d outbound dials in a default run, want 0", n)
	}
}
