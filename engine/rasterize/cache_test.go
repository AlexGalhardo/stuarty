package rasterize

import (
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

func TestCacheHitSharesPixels(t *testing.T) {
	t.Parallel()

	c := NewCache(Bundled(), 0)
	spec := textSpec("Cached", plainStyle())

	a, err := c.Get(spec)
	if err != nil {
		t.Fatal(err)
	}

	b, err := c.Get(spec)
	if err != nil {
		t.Fatal(err)
	}

	if &a.Pix[0] != &b.Pix[0] {
		t.Error("the second Get rendered again instead of hitting the cache")
	}

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}

	other := spec
	other.Style.Size = 90

	if _, err := c.Get(other); err != nil {
		t.Fatal(err)
	}

	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2 after another spec", c.Len())
	}
}

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()

	one := render(t, textSpec("Evict", plainStyle()))
	c := NewCache(Bundled(), len(one.Pix)*5/2) // room for two bitmaps of this size

	specs := make([]plan.BitmapSpec, 3)
	for i := range specs {
		specs[i] = textSpec("Evict", plainStyle())
		specs[i].Style.Color = C(255, 255, 255, uint8(250-i)) // same size, different keys
	}

	get := func(s plan.BitmapSpec) Bitmap {
		b, err := c.Get(s)
		if err != nil {
			t.Fatal(err)
		}

		return b
	}

	first := get(specs[0])
	get(specs[1])
	get(specs[0]) // specs[1] is now the least recently used
	get(specs[2]) // evicts specs[1]

	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2 within the budget", c.Len())
	}

	if again := get(specs[0]); &again.Pix[0] != &first.Pix[0] {
		t.Error("specs[0] was evicted although it was used more recently than specs[1]")
	}
}

func TestCacheConcurrent(t *testing.T) {
	t.Parallel()

	c := NewCache(Bundled(), 0)
	spec := captionSpec([]string{"Race", "free"}, 1, styledForAllEffects())

	var wg sync.WaitGroup

	for range 16 {
		wg.Go(func() {
			if _, err := c.Get(spec); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestCacheError(t *testing.T) {
	t.Parallel()

	if _, err := NewCache(Bundled(), 0).Get(plan.BitmapSpec{}); err == nil {
		t.Error("Get of an invalid spec succeeded")
	}
}

// Warm may be called from many goroutines at start-up and while a render runs.
func TestWarmIsSafeToCallConcurrently(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for range 4 {
		wg.Go(Warm)
	}

	wg.Go(func() {
		if _, err := Render(textSpec("Warm", plainStyle()), Bundled()); err != nil {
			t.Error(err)
		}
	})

	wg.Wait()
}
