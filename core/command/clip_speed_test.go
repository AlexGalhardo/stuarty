package command

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
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

// A reversed item keeps showing the same source frames when its end is trimmed, and its
// split and start trim stay within a frame of the unsplit mapping (SDD-004 section 3.12).
func TestReverseTrimAndSplitKeepTheMapping(t *testing.T) {
	reg := Default()
	doc := base().clip(itemA, vidTr, 0, 60, 100, func(it *timeline.Item) {
		it.Speed, it.Reverse = timeline.Rational{Num: 3, Den: 2}, true
	}).d
	orig := doc.Items[itemA]

	res, err := reg.Apply(
		doc,
		op2("item.trim", fmt.Sprintf(`{"item_id": %q, "edge": "end", "to": "41f"}`, itemA)),
		Env{},
	)
	if err != nil {
		t.Fatal(err)
	}
	n := res.Doc.Items[itemA]
	for k := range n.Duration {
		if got, want := n.SourceIn+n.SourceOffset(k), orig.SourceIn+orig.SourceOffset(k); got != want {
			t.Fatalf("trim end: frame %d shows %d, was %d", k, got, want)
		}
	}

	res, err = reg.Apply(
		doc,
		op2("item.trim", fmt.Sprintf(`{"item_id": %q, "edge": "start", "to": "10f"}`, itemA)),
		Env{},
	)
	if err != nil {
		t.Fatal(err)
	}
	n = res.Doc.Items[itemA]
	for k := range n.Duration {
		if d := n.SourceIn + n.SourceOffset(k) - (orig.SourceIn + orig.SourceOffset(k+10)); d < -1 || d > 1 {
			t.Fatalf("trim start: frame %d is %d frames off", k, d)
		}
	}

	res, err = reg.Apply(
		doc,
		op2("item.split", fmt.Sprintf(`{"item_id": %q, "at": "25f", "new_id": %q}`, itemA, itemB)),
		Env{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if vs := timeline.Validate(res.Doc); len(vs) != 0 {
		t.Fatal(vs)
	}
	l, r := res.Doc.Items[itemA], res.Doc.Items[itemB]
	for k := range l.Duration {
		if d := l.SourceIn + l.SourceOffset(k) - (orig.SourceIn + orig.SourceOffset(k)); d < -1 || d > 1 {
			t.Fatalf("split left: frame %d is %d frames off", k, d)
		}
	}
	for k := range r.Duration {
		if d := r.SourceIn + r.SourceOffset(k) - (orig.SourceIn + orig.SourceOffset(25+k)); d < -1 || d > 1 {
			t.Fatalf("split right: frame %d is %d frames off", k, d)
		}
	}
}

// A speed or a curve that lengthens a clip over its neighbour says how long the clip became
// and that ripple "track" moves the later items (VV-118, VV-155).
func TestSetSpeedOverlapNamesTheResultDuration(t *testing.T) {
	for name, args := range map[string]string{
		"constant": `"speed": "0.5x"`,
		"curve":    `"curve": [{"t": 0, "speed": "1x"}, {"t": 0.5, "speed": "0.4x"}, {"t": 1, "speed": "1x"}]`,
	} {
		doc := base().clip(itemA, vidTr, 0, 90, 0).clip(itemB, vidTr, 90, 30, 0).d
		_, err := Default().Apply(doc, op2("clip.set_speed", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, args)), Env{})
		ce := AsError(err)
		if ce == nil || ce.Code != CodeOverlap {
			t.Fatalf("%s: want E_OVERLAP, got %v", name, err)
		}
		if _, ok := ce.Details["result_duration"]; !ok {
			t.Errorf("%s: no details.result_duration in %+v", name, ce.Details)
		}
		if !strings.Contains(ce.Hint, "ripple") {
			t.Errorf("%s: hint %q does not suggest ripple", name, ce.Hint)
		}
	}
}

// A bad point of a speed curve is reported at its own path (VV-156).
func TestSetSpeedCurvePointErrorPath(t *testing.T) {
	doc := base().clip(itemA, vidTr, 0, 90, 0).d
	_, err := Default().Apply(doc, op2("clip.set_speed", fmt.Sprintf(
		`{"item_id": %q, "curve": [{"t": 0, "speed": "1x"}, {"t": 0.5, "speed": 0.4}, {"t": 1, "speed": "1x"}]}`,
		itemA,
	)), Env{})
	ce := AsError(err)
	if ce == nil || ce.Path != "args.curve[1].speed" || !strings.Contains(ce.Message, "args.curve[1].speed") {
		t.Fatalf("got %+v", ce)
	}
}

// A curve that shortens a clip with ripple none leaves a gap: the result says so and the
// linter reports the black frames (VV-163).
func TestSetSpeedCurveShortenLeavesReportedGap(t *testing.T) {
	doc := base().clip(itemA, vidTr, 0, 90, 0).clip(itemB, vidTr, 150, 30, 0).d
	res, err := Default().Apply(doc, op2("clip.set_speed", fmt.Sprintf(
		`{"item_id": %q, "curve": [{"t": 0, "speed": "1x"}, {"t": 0.5, "speed": "3x"}, {"t": 1, "speed": "1x"}]}`,
		itemA,
	)), Env{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Effective["result_duration"]; !ok {
		t.Errorf("no effective.result_duration in %+v", res.Effective)
	}
	if _, ok := res.Effective["gap"]; !ok {
		t.Errorf("no effective.gap in %+v", res.Effective)
	}
	found := false
	for _, f := range lint.Lint(res.Doc, lint.Env{}, lint.Fast) {
		found = found || f.Code == lint.CodeBlackFrames || f.Code == lint.CodeGap
	}
	if !found {
		t.Error("the linter does not report the gap")
	}
}
