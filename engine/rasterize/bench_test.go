package rasterize

import (
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// referenceCaption is the caption of the reference scene of SDD-004 section 11:
// a 1080 wide vertical frame, six words on two lines, Inter 700 at 64 px with a
// stroke and a soft shadow, one word highlighted.
func referenceCaption() plan.BitmapSpec {
	st := plainStyle()
	st.Size, st.StrokeWidth = 64, 5
	st.ShadowDX, st.ShadowDY, st.ShadowBlur = 0, 4, 6
	st.MaxWidth = 0.8 * 1080
	hb := C(255, 45, 110, 255)
	st.HighlightBackground = hb

	spec := captionSpec([]string{"Never", "gonna", "give", "you", "up", "today"}, 2, st)
	spec.Breaks = []int{3}

	return spec
}

// BenchmarkCaptionMiss measures a cache miss: shaping, layout and drawing the
// caption of the reference scene. The budget is 5 ms (P1-RND-003).
func BenchmarkCaptionMiss(b *testing.B) {
	spec := referenceCaption()
	fonts := Bundled()

	if _, err := Render(spec, fonts); err != nil { // parse the font outside the timing
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Render(spec, fonts); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCaptionHit measures a cache hit: hashing the spec and finding the
// bitmap. The budget is 0.1 ms.
func BenchmarkCaptionHit(b *testing.B) {
	c := NewCache(Bundled(), 0)
	spec := referenceCaption()

	if _, err := c.Get(spec); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := c.Get(spec); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCaptionMissOversampled is the same caption at PixelScale 2, the
// scale a caption that zooms to twice its size needs.
func BenchmarkCaptionMissOversampled(b *testing.B) {
	spec := referenceCaption()
	spec.PixelScale = 2
	fonts := Bundled()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Render(spec, fonts); err != nil {
			b.Fatal(err)
		}
	}
}

// TestCaptionBudgets enforces the budgets of P1-RND-003 with a wide margin, so
// that a slow shared runner does not fail the suite but a regression of an
// order of magnitude does: 25 ms for a miss (the budget is 5 ms) and 1 ms for a
// hit (the budget is 0.1 ms). The benchmarks above give the real numbers.
func TestCaptionBudgets(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing test: skipped with -short and with the race detector")
	}

	c := NewCache(Bundled(), 0)
	spec := referenceCaption()

	if _, err := Render(spec, Bundled()); err != nil { // warm the font parse
		t.Fatal(err)
	}

	best := func(n int, f func()) time.Duration {
		d := time.Duration(1<<63 - 1)

		for range n {
			start := time.Now()
			f()
			d = min(d, time.Since(start))
		}

		return d
	}

	miss := best(20, func() {
		if _, err := Render(spec, Bundled()); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := c.Get(spec); err != nil {
		t.Fatal(err)
	}

	hit := best(200, func() {
		if _, err := c.Get(spec); err != nil {
			t.Fatal(err)
		}
	})

	t.Logf("miss %v, hit %v", miss, hit)

	if miss > 25*time.Millisecond {
		t.Errorf("a cache miss took %v, the budget is 5 ms", miss)
	}

	if hit > time.Millisecond {
		t.Errorf("a cache hit took %v, the budget is 0.1 ms", hit)
	}
}
