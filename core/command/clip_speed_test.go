package command

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestSpeedBoundaries(t *testing.T) {
	for _, s := range []string{"0.25x", "4x", "1x", "0.333x", "3.999x"} {
		if _, err := parseSpeed(s); err != nil {
			t.Errorf("%s rejected: %v", s, err)
		}
	}
	for _, s := range []any{
		"4.001x", "0.249x", "x", "1.5", "1.5xx", "-1x", "+1x", ".x", "1.2345x", "0x", 2.0, nil,
		map[string]any{"num": "1", "den": "2"},
	} {
		if _, err := parseSpeed(s); err == nil || err.Code != CodeValidation {
			t.Errorf("%v accepted", s)
		}
	}
	if r, err := parseSpeed(map[string]any{"num": jn("8"), "den": jn("4")}); err != nil || r.Num != 2 || r.Den != 1 {
		t.Errorf("%v %v", r, err)
	}
}

// For random clips and speeds the result is valid and its source range holds (R9).
func TestSetSpeedProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(25))
	reg := Default()
	ok := 0
	for i := range 500 {
		srcIn, dur := rng.Intn(800), 1+rng.Intn(100)
		doc := base().clip(itemA, vidTr, rng.Intn(500), dur, srcIn, func(it *timeline.Item) {
			it.Speed = timeline.Rational{Num: 1 + rng.Intn(8), Den: 1 + rng.Intn(4)}
			if it.Speed.Num*4 < it.Speed.Den || it.Speed.Num > 4*it.Speed.Den {
				it.Speed = timeline.Rational{Num: 1, Den: 1}
			}
			it.FadeIn = rng.Intn(dur + 1)
			it.FadeOut = rng.Intn(dur - it.FadeIn + 1)
		}).d
		if vs := timeline.Validate(doc); len(vs) != 0 {
			continue // the random clip was not valid to begin with
		}
		speed := fmt.Sprintf("%dx", 1+rng.Intn(4))
		if rng.Intn(2) == 0 {
			speed = fmt.Sprintf("0.%dx", 25+rng.Intn(75))
		}
		res, err := reg.Apply(
			doc,
			op2("clip.set_speed", fmt.Sprintf(`{"item_id": %q, "speed": %q}`, itemA, speed)),
			Env{},
		)
		if err != nil {
			if e := AsError(err); e.Code != CodeOutOfSource && e.Code != CodeOverlap && e.Code != CodeInvalidTime {
				t.Fatalf("case %d %s: %v", i, speed, err)
			}
			continue
		}
		if vs := timeline.Validate(res.Doc); len(vs) != 0 {
			t.Fatalf("case %d %s: %v", i, speed, vs)
		}
		ok++
	}
	if ok < 100 {
		t.Errorf("only %d random speed changes succeeded", ok)
	}
}

// A speed curve keeps the invariants: the speed is the mean, the source range the clip read
// before is the one it reads after (up to the ceiling), and the document stays valid.
func TestSetSpeedCurveKeepsSourceRange(t *testing.T) {
	reg := Default()
	for _, curve := range []string{
		`[{"t":0,"speed":"1x"},{"t":0.5,"speed":"3x"},{"t":1,"speed":"1x"}]`,
		`[{"t":0,"speed":"0.25x"},{"t":1,"speed":"4x"}]`,
		`[{"t":0,"speed":"2x"},{"t":0.333,"speed":"0.75x"},{"t":1,"speed":"1.337x"}]`,
	} {
		doc := base().clip(itemA, vidTr, 0, 90, 10).d
		res, err := reg.Apply(
			doc,
			op2("clip.set_speed", fmt.Sprintf(`{"item_id": %q, "curve": %s}`, itemA, curve)),
			Env{},
		)
		if err != nil {
			t.Fatalf("%s: %v", curve, err)
		}
		if vs := timeline.Validate(res.Doc); len(vs) != 0 {
			t.Fatalf("%s: %v", curve, vs)
		}
		it := res.Doc.Items[itemA]
		if it.Speed != timeline.CurveMean(it.SpeedCurve) {
			t.Errorf("%s: speed %v is not the mean %v", curve, it.Speed, timeline.CurveMean(it.SpeedCurve))
		}
		if got := timeline.CurveSourceFrames(
			it.SpeedCurve,
			it.Duration,
		); got < 90 ||
			got > 90+it.Speed.Num/it.Speed.Den+1 {
			t.Errorf("%s: reads %d source frames for 90 before", curve, got)
		}
	}
}

func TestSpeedCurveRefusesSourceMappingEdits(t *testing.T) {
	reg := Default()
	doc := base().clip(itemA, vidTr, 0, 90, 100, func(it *timeline.Item) {
		it.SpeedCurve = []timeline.SpeedPoint{
			{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}},
			{T: 1, Speed: timeline.Rational{Num: 2, Den: 1}},
		}
		it.Speed = timeline.CurveMean(it.SpeedCurve)
	}).d
	for _, tc := range []struct{ name, args string }{
		{"item.split", fmt.Sprintf(`{"item_id": %q, "at": "30f", "new_id": %q}`, itemA, itemB)},
		{"item.trim", fmt.Sprintf(`{"item_id": %q, "edge": "start", "to": "10f"}`, itemA)},
	} {
		_, err := reg.Apply(doc, op2(tc.name, tc.args), Env{})
		if e := AsError(err); e == nil || e.Code != CodeInvalidState {
			t.Errorf("%s on a curve item: %v", tc.name, err)
		}
	}
	// Trimming the end keeps the curve over the shorter item.
	if _, err := reg.Apply(
		doc,
		op2("item.trim", fmt.Sprintf(`{"item_id": %q, "edge": "end", "to": "60f"}`, itemA)),
		Env{},
	); err != nil {
		t.Errorf("trim end: %v", err)
	}
}
