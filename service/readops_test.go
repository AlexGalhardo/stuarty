package service

import (
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// A second call at the same revision is served from the cache; another
// revision, another project and the words flag are other entries; a failure
// stores nothing.
func TestSnapPointsCacheServesTheSameRevisionOnce(t *testing.T) {
	t.Parallel()

	var c snapCache

	compute := func() (plan.Snap, error) { return plan.Snap{Points: []plan.SnapPoint{{Frame: 1}}}, nil }

	for range 3 {
		if _, err := c.get(snapKey{"prj_a", 4, false}, compute); err != nil {
			t.Fatal(err)
		}
	}

	if c.computes != 1 {
		t.Errorf("computed %d times for one revision, want 1", c.computes)
	}

	for _, k := range []snapKey{{"prj_a", 5, false}, {"prj_b", 4, false}, {"prj_a", 4, true}} {
		if _, err := c.get(k, compute); err != nil {
			t.Fatal(err)
		}
	}

	if c.computes != 4 {
		t.Errorf("computed %d times for four keys, want 4", c.computes)
	}

	boom := errors.New("boom")
	failing := func() (plan.Snap, error) { return plan.Snap{}, boom }

	if _, err := c.get(snapKey{"prj_a", 9, false}, failing); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}

	if c.computes != 4 || len(c.m) != 4 {
		t.Error("a failed computation was stored")
	}

	for i := range maxSnapEntries + 1 {
		_, _ = c.get(snapKey{"prj_x", int64(i), false}, compute)
	}

	if len(c.m) > maxSnapEntries {
		t.Errorf("the cache holds %d entries, the bound is %d", len(c.m), maxSnapEntries)
	}
}
