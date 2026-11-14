package cli

import (
	"slices"
	"testing"
)

// TestSecurity_T10_NoOutboundCapability checks that no command lets a caller
// name a destination: a flag called url, output, dest or to would be a way to
// make the product send data somewhere (SDD-007 T10, ADR-0020).
func TestSecurity_T10_NoOutboundCapability(t *testing.T) {
	t.Parallel()

	forbidden := []string{"url", "output", "dest", "to"}

	for _, s := range registry() {
		for _, d := range s.flagDefs() {
			if slices.Contains(forbidden, d.Name) {
				t.Errorf("%s declares the flag --%s", s.name, d.Name)
			}
		}
	}
}

// TestSecurity_ActorCannotImpersonate checks that the CLI never sends an actor
// that names a user, a key or the browser (SDD-003 section 5.1).
func TestSecurity_ActorCannotImpersonate(t *testing.T) {
	t.Parallel()

	for _, actor := range []string{"user:u1", "key:k1", "ui", "local:ui"} {
		h := newHarness(t)

		if code := h.run("project:list", "--actor", actor, "--json"); code != ExitFailure {
			t.Errorf("actor %q accepted (exit %d)", actor, code)
		}

		if h.res.resolved != 0 {
			t.Errorf("actor %q reached the resolver", actor)
		}
	}
}
