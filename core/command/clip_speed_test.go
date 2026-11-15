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
