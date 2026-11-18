package plan

import (
	"math"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// The vectors were computed with an independent implementation (Python integers masked to 32 bits).
func TestHash3Vectors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		a, b, c uint32
		want    uint32
	}{{0, 0, 0, 2488251732}, {1, 2, 3, 3114175480}, {65535, 100, 2, 3848593025}} {
		if got := Hash3(tc.a, tc.b, tc.c); got != tc.want {
			t.Errorf("Hash3(%d, %d, %d) = %d, want %d", tc.a, tc.b, tc.c, got, tc.want)
		}
	}
}

func TestShakeOffsetVectors(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30, Den: 1}

	for _, tc := range []struct {
		r, speed, seed int
		amount         float64
		dx, dy, rot    int
	}{
		{0, 8, 0, 0.3, 2, 11, -438},
		{7, 8, 5, 1, -23, -31, 1265},
		{100, 30, 65535, 0.5, -5, 24, 792},
	} {
		e := &timeline.Effect{
			Kind:   timeline.EffectShake,
			Amount: new(tc.amount),
			Speed:  new(tc.speed),
			Seed:   new(tc.seed),
		}

		dx, dy, rot := ShakeOffset(e, tc.r, fps, 1080, 1920)
		if dx != tc.dx || dy != tc.dy || rot != tc.rot {
			t.Errorf("frame %d: got (%d, %d, %d), want (%d, %d, %d)", tc.r, dx, dy, rot, tc.dx, tc.dy, tc.rot)
		}
	}

	// Amount 0 never moves anything.
	e := &timeline.Effect{Kind: timeline.EffectShake, Amount: new(float64(0)), Speed: new(8), Seed: new(1)}
	if dx, dy, rot := ShakeOffset(e, 5, fps, 1080, 1920); dx != 0 || dy != 0 || rot != 0 {
		t.Errorf("amount 0 moved the layer: %d %d %d", dx, dy, rot)
	}
}

func TestEffectsAndNewTransitionsInThePlan(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 0, 30)
	c.Effects = []timeline.Effect{
		{Kind: timeline.EffectVignette, Amount: new(0.5), Softness: new(0.5)},
		{Kind: timeline.EffectShake, Amount: new(0.3), Speed: new(8), Seed: new(0)},
	}
	c.TransitionIn = &timeline.Transition{Kind: "wipe", Duration: 9, Direction: "up"}
	c.TransitionOut = &timeline.Transition{Kind: "dip", Duration: 4, Color: "#FF8800"}
	put(d, c)

	l := Frame(d, 0).Layers[0]
	if len(l.Effects) != 1 || l.Effects[0].Kind != timeline.EffectVignette {
		t.Errorf("effects of the layer: %+v (shake must not be listed)", l.Effects)
	}

	if tr := l.Transition; tr == nil || tr.Kind != "wipe" || tr.Edge != "in" || tr.Direction != "up" || tr.P != 6554 {
		t.Errorf("wipe state at frame 0: %+v, want p 1/10 = 6554", tr)
	}

	if l.CX == 540 && l.CY == 960 {
		t.Errorf("the shake did not move the layer: %v %v", l.CX, l.CY)
	}

	if tr := Frame(d, 10).Layers[0].Transition; tr != nil {
		t.Errorf("no transition between the edges: %+v", tr)
	}

	if tr := Frame(d, 29).Layers[0].Transition; tr == nil || tr.Kind != "dip" || tr.Edge != "out" ||
		tr.Color != "#FF8800" ||
		tr.P != 13107 {
		t.Errorf("dip state at the last frame: %+v, want p 1/5 = 13107", tr)
	}

	w := Window(d, 0, 12)
	if len(w.Layers) != 1 || len(w.Layers[0].Effects) != 1 || len(w.Layers[0].Transition) != 12 ||
		w.Layers[0].Transition[0] == nil || w.Layers[0].Transition[11] != nil {
		t.Errorf("window layer: %+v", w.Layers)
	}
}

func TestZoomAndWhipValues(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 0, 30)
	c.TransitionIn = &timeline.Transition{Kind: "zoom", Duration: 9}
	c.TransitionOut = &timeline.Transition{Kind: "whip", Duration: 9, Direction: "right"}
	put(d, c)

	// p = 1/10: scale 1 + (1 - s(0.1)) / 2 with s(p) = 1 - (1 - p)^3, opacity 0.1.
	l := Frame(d, 0).Layers[0]
	if want := round3(1 + 0.729/2); l.Scale != want || l.Opacity != 0.1 {
		t.Errorf("zoom in at frame 0: scale %v opacity %v, want %v and 0.1", l.Scale, l.Opacity, want)
	}

	// p = 1/10 at the last frame: x = 540 + (1 - s5(0.1)) x 1080 with s5(p) = 1 - (1 - p)^5.
	l = Frame(d, 29).Layers[0]
	if want := round3(540 + math.Pow(0.9, 5)*1080); l.CX != want {
		t.Errorf("whip out right at the last frame: cx %v, want %v", l.CX, want)
	}
}
