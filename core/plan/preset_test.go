package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func easeOut(u float64) float64 { return 1 - (1-u)*(1-u)*(1-u) }
func easeIn(u float64) float64  { return u * u * u }
func easeInOut(u float64) float64 {
	if u < 0.5 {
		return 4 * u * u * u
	}
	return 1 - (-2*u+2)*(-2*u+2)*(-2*u+2)/2
}

func lerp(a, b, e float64) float64 { return a + (b-a)*e }

// The documents of the text.apply_preset golden cases, evaluated by the plan, give the
// closed forms of SDD-002 C.30 at the first, middle and last frame of the edge (and at
// the overshoot frame of pop). The item is 90 frames, 1080x1920, n is 12.
func TestPresetGoldensMatchClosedForms(t *testing.T) {
	t.Parallel()

	const (
		n  = 12
		d  = 90
		by = 960.0
		h  = 96.0
	)

	type want struct{ opacity, y, scale float64 }

	cases := []struct {
		name   string
		frames []int
		f      func(r int) want
	}{
		{"ok_fade_up_in_default_duration", []int{0, 6, 12}, func(r int) want {
			e := easeOut(float64(r) / n)

			return want{lerp(0, 1, e), lerp(by+h, by, e), 1}
		}},
		{"ok_fade_down_in", []int{0, 6, 12}, func(r int) want {
			e := easeOut(float64(r) / n)

			return want{lerp(0, 1, e), lerp(by-h, by, e), 1}
		}},
		{"ok_fade_up_out", []int{d - n, d - n/2, d}, func(r int) want {
			e := easeIn(float64(r-(d-n)) / n)

			return want{lerp(1, 0, e), lerp(by, by-h, e), 1}
		}},
		{"ok_fade_down_out", []int{d - n, d - n/2, d}, func(r int) want {
			e := easeIn(float64(r-(d-n)) / n)

			return want{lerp(1, 0, e), lerp(by, by+h, e), 1}
		}},
		{"ok_zoom_in_in", []int{0, 6, 12}, func(r int) want {
			e := easeOut(float64(r) / n)

			return want{lerp(0, 1, e), by, lerp(0.8, 1, e)}
		}},
		{"ok_zoom_in_out", []int{d - n, d - n/2, d}, func(r int) want {
			e := easeIn(float64(r-(d-n)) / n)

			return want{lerp(1, 0, e), by, lerp(1, 1.2, e)}
		}},
		{"ok_pop_in", []int{0, 6, 8, 12}, func(r int) want {
			const q = 8 // round(0.7 x 12)

			scale := lerp(1.1, 1, easeInOut(float64(r-q)/float64(n-q)))
			if r <= q {
				scale = lerp(0.6, 1.1, easeOut(float64(r)/q))
			}

			return want{lerp(0, 1, easeOut(float64(r)/n)), by, scale}
		}},
	}

	for _, c := range cases {
		data, err := os.ReadFile(filepath.Join("..", "command", "testdata", "text.apply_preset", c.name, "after.json"))
		if err != nil {
			t.Fatal(err)
		}

		doc, err := timeline.Decode(data)
		if err != nil {
			t.Fatal(err)
		}

		it := doc.Items["itm_s6t7v8w9x0"]

		for _, r := range c.frames {
			got, w := evaluate(&it, r, 1080, 1920, 540, 960), c.f(r)
			near(t, c.name+" opacity", got.opacity, w.opacity)
			near(t, c.name+" y", got.y, w.y)
			near(t, c.name+" scale", got.scale, w.scale)
		}
	}
}
