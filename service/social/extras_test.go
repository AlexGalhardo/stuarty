package social

import (
	"fmt"
	"testing"
)

func TestRotationChainedThreeWayAndBounded(t *testing.T) {
	t.Parallel()

	r := NewRotation()

	// 3-way concurrent send scenario:
	// Send A rotates R0 -> R1; Send B (which held R0) rotates R0 -> R2; Send C arrives holding R1.
	r.Remember("con_1", "R0", "R1")
	r.Remember("con_1", "R0", "R2")

	if got := r.Current("con_1", "R1"); got != "R2" {
		t.Errorf("Current(R1) = %q, want R2", got)
	}

	if got := r.Current("con_1", "R0"); got != "R2" {
		t.Errorf("Current(R0) = %q, want R2", got)
	}

	if got := r.Current("con_1", "R2"); got != "R2" {
		t.Errorf("Current(R2) = %q, want R2", got)
	}

	if got := r.Current("con_1", "unrelated"); got != "unrelated" {
		t.Errorf("Current(unrelated) = %q, want unrelated", got)
	}

	// Memory bound: rotating more than maxRotationHistory times evicts oldest tokens.
	prev := "T0"
	for i := 1; i <= maxRotationHistory+5; i++ {
		next := fmt.Sprintf("T%d", i)
		r.Remember("con_cap", prev, next)
		prev = next
	}

	r.mu.Lock()
	gotLen := len(r.last["con_cap"].seen)
	r.mu.Unlock()

	if gotLen > maxRotationHistory {
		t.Errorf("seen len = %d, exceeds maxRotationHistory %d", gotLen, maxRotationHistory)
	}

	if got := r.Current("con_cap", "T0"); got != "T0" {
		t.Errorf("evicted token T0 returned %q, want T0", got)
	}

	recent := fmt.Sprintf("T%d", maxRotationHistory+4)
	if got := r.Current("con_cap", recent); got != prev {
		t.Errorf("recent token %s returned %q, want %q", recent, got, prev)
	}
}
