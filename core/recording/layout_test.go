package recording

import (
	"fmt"
	"testing"
)

const eps = 0.001

var frames = []struct {
	w, h   int
	format Format
}{{1920, 1080, Format16x9}, {1080, 1080, Format1x1}, {1080, 1350, Format4x5}, {1080, 1920, Format9x16}}

func inside(r, outer Rect) bool {
	return r.X >= outer.X-eps && r.Y >= outer.Y-eps && r.X+r.W <= outer.X+outer.W+eps && r.Y+r.H <= outer.Y+outer.H+eps
}

func TestFormatOfTakesTheNearestFormat(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		w, h int
		want Format
	}{
		{1920, 1080, Format16x9},
		{1280, 720, Format16x9},
		{1080, 1080, Format1x1},
		{1080, 1350, Format4x5},
		{1080, 1920, Format9x16},
		{2560, 1080, Format16x9},
		{1500, 1200, Format1x1},
		{1000, 1500, Format9x16},
		{1080, 1300, Format4x5},
		{0, 0, Format16x9},
	} {
		if got := FormatOf(c.w, c.h); got != c.want {
			t.Errorf("FormatOf(%d, %d) = %s, want %s", c.w, c.h, got, c.want)
		}
	}
}

func TestPictureInPictureOfAWideFrameHasKnownValues(t *testing.T) {
	t.Parallel()

	res, err := PlanLayout(LayoutRequest{
		Layout: LayoutPip, Corner: CornerBottomRight, FrameW: 1920, FrameH: 1080,
		Screen: Size{1920, 1080}, Camera: Size{1920, 1080}, HasCamera: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if s := res.Screen; s.Scale != 1 || s.CX != 960 || s.CY != 540 || !s.Crop.IsZero() {
		t.Errorf("screen %+v, want scale 1 at the centre and no crop", s)
	}

	c := res.Camera
	if c.Scale != 0.26 || c.CX != 1736.4 || c.CY != 896.4 || c.Crop != (Crop{Left: 0.2188, Right: 0.2188}) {
		t.Errorf("camera %+v, want scale 0.26 at 1736.4, 896.4 cropped 0.2188 on both sides", *c)
	}

	if !res.CameraInFront || !res.Circle || res.Format != Format16x9 {
		t.Errorf("result %+v: the camera is in front, a circle, in a 16:9 frame", res)
	}
}

func TestEveryLayoutKeepsEveryClipInsideTheFrame(t *testing.T) {
	t.Parallel()

	assets := []Size{{1920, 1080}, {1280, 720}, {1080, 1920}, {1000, 1000}, {3840, 1600}, {640, 480}}

	for _, f := range frames {
		frame := Rect{W: float64(f.w), H: float64(f.h)}

		for _, layout := range Layouts {
			for _, corner := range Corners {
				for _, padding := range []float64{0, 0.04, 0.1, MaxLayoutPadding} {
					for _, screen := range assets {
						for _, camera := range assets {
							name := fmt.Sprintf("%s/%s/%s/%v/%v/%v", f.format, layout, corner, padding, screen, camera)

							res, err := PlanLayout(LayoutRequest{
								Layout: layout, Corner: corner, Padding: padding, FrameW: f.w, FrameH: f.h,
								Screen: screen, Camera: camera, HasCamera: layout != LayoutScreenOnly,
							})
							if err != nil {
								t.Fatalf("%s: %v", name, err)
							}

							pad := padding * float64(min(f.w, f.h))
							safe := Rect{X: pad, Y: pad, W: frame.W - 2*pad, H: frame.H - 2*pad}

							if res.Format != f.format {
								t.Fatalf("%s: format %s", name, res.Format)
							}

							if !inside(res.Screen.Shown, safe) || res.Screen.Scale <= 0 {
								t.Fatalf("%s: screen %+v leaves the safe area %+v", name, res.Screen.Shown, safe)
							}

							if res.Camera == nil {
								continue
							}

							if !inside(res.Camera.Shown, safe) || res.Camera.Scale <= 0 {
								t.Fatalf("%s: camera %+v leaves the safe area %+v", name, res.Camera.Shown, safe)
							}

							if layout == LayoutSplit && overlaps(res.Screen.Shown, res.Camera.Shown) {
								t.Fatalf(
									"%s: the two clips of a split overlap: %+v %+v",
									name,
									res.Screen.Shown,
									res.Camera.Shown,
								)
							}

							if layout == LayoutPip && !inside(res.Camera.Shown, res.Screen.Shown) {
								t.Fatalf(
									"%s: the camera %+v is not on the screen %+v",
									name,
									res.Camera.Shown,
									res.Screen.Shown,
								)
							}
						}
					}
				}
			}
		}
	}
}

func overlaps(a, b Rect) bool {
	return a.X < b.X+b.W-eps && b.X < a.X+a.W-eps && a.Y < b.Y+b.H-eps && b.Y < a.Y+a.H-eps
}

func TestSplitStacksTheTakesInAStackedFrame(t *testing.T) {
	t.Parallel()

	for _, f := range frames[1:] {
		res, err := PlanLayout(LayoutRequest{
			Layout: LayoutSplit, FrameW: f.w, FrameH: f.h,
			Screen: Size{1920, 1080}, Camera: Size{1280, 720}, HasCamera: true,
		})
		if err != nil {
			t.Fatal(err)
		}

		if res.Screen.Shown.Y >= res.Camera.Shown.Y || res.Screen.Shown.W < float64(f.w)*0.99 {
			t.Errorf("%s: screen %+v should be above camera %+v and as wide as the frame", f.format, res.Screen.Shown,
				res.Camera.Shown)
		}
	}
}

func TestSplitPutsTheScreenOnTheLeftOfAWideFrame(t *testing.T) {
	t.Parallel()

	res, err := PlanLayout(LayoutRequest{
		Layout:    LayoutSplit,
		FrameW:    1920,
		FrameH:    1080,
		Screen:    Size{1920, 1080},
		Camera:    Size{1280, 720},
		HasCamera: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Screen.Shown.X >= res.Camera.Shown.X || res.Screen.Shown.W <= res.Camera.Shown.W {
		t.Errorf("screen %+v should be left of and wider than camera %+v", res.Screen.Shown, res.Camera.Shown)
	}
}

func TestPlanLayoutIsDeterministic(t *testing.T) {
	t.Parallel()

	q := LayoutRequest{
		Layout: LayoutCameraFocus, Corner: CornerTopLeft, Padding: 0.04, FrameW: 1080, FrameH: 1920,
		Screen: Size{2560, 1440}, Camera: Size{1920, 1080}, HasCamera: true,
	}

	first, err := PlanLayout(q)
	if err != nil {
		t.Fatal(err)
	}

	for range 20 {
		again, _ := PlanLayout(q)
		if again.Screen != first.Screen || *again.Camera != *first.Camera {
			t.Fatal("the same request gave another layout")
		}
	}
}

func TestPlanLayoutRefusesWhatItCannotPlan(t *testing.T) {
	t.Parallel()

	ok := LayoutRequest{
		Layout:    LayoutPip,
		FrameW:    1920,
		FrameH:    1080,
		Screen:    Size{1920, 1080},
		Camera:    Size{640, 480},
		HasCamera: true,
	}

	for name, mod := range map[string]func(*LayoutRequest){
		"no frame":          func(q *LayoutRequest) { q.FrameW = 0 },
		"no screen size":    func(q *LayoutRequest) { q.Screen = Size{} },
		"no camera":         func(q *LayoutRequest) { q.HasCamera = false },
		"no camera size":    func(q *LayoutRequest) { q.Camera = Size{} },
		"padding too large": func(q *LayoutRequest) { q.Padding = 0.3 },
		"negative padding":  func(q *LayoutRequest) { q.Padding = -0.1 },
		"unknown layout":    func(q *LayoutRequest) { q.Layout = "grid" },
	} {
		q := ok
		mod(&q)

		if _, err := PlanLayout(q); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
