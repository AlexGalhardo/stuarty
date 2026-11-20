package command

import (
	"bytes"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// speedDoc is a document with one composition item of 90 frames at frame 30, with a text item after it.
func speedDoc(mods ...func(*timeline.Item)) *timeline.Document {
	b := base()
	compItem(tlB, 30, 90, mods...)(b.d)
	b.text(itemB, vidTr, 120, 30)

	return b.d
}

func setSpeed(t *testing.T, doc *timeline.Document, args string) (Result, error) {
	t.Helper()

	return fullRegistry().Apply(doc, opJSON("clip.set_speed", args), Env{})
}

// Epic 53 criterion 1: speed 2x halves the duration by ceil(D x old / new) and stores the speed.
func TestCompositionSpeedDoubleHalvesTheDuration(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		speed string
		dur   int
		num   int
		den   int
	}{{"2x", 45, 2, 1}, {"0.5x", 180, 1, 2}, {"1.5x", 60, 3, 2}, {"0.333x", 271, 333, 1000}, {"4x", 23, 4, 1}} {
		doc := speedDoc()
		delete(doc.Items, itemB)

		res, err := setSpeed(t, doc, `{"item_id":"`+itemA+`","speed":"`+c.speed+`"}`)
		if err != nil {
			t.Fatalf("%s: %v", c.speed, err)
		}

		it := res.Doc.Items[itemA]
		if it.Duration != c.dur || it.Speed != (timeline.Rational{Num: c.num, Den: c.den}) {
			t.Errorf("%s: duration %d speed %v, want %d and %d/%d", c.speed, it.Duration, it.Speed, c.dur, c.num, c.den)
		}
	}
}

func TestCompositionSpeedRippleMovesTheItemsAfter(t *testing.T) {
	t.Parallel()

	res, err := setSpeed(t, speedDoc(), `{"item_id":"`+itemA+`","speed":"2x","ripple":"track"}`)
	if err != nil {
		t.Fatal(err)
	}

	if got := res.Doc.Items[itemB].Start; got != 75 {
		t.Errorf("the item after is at %d, want 120 - 45 = 75", got)
	}

	// Without ripple a longer composition lands on its neighbour.
	_, err = setSpeed(t, speedDoc(), `{"item_id":"`+itemA+`","speed":"0.5x"}`)
	if e := AsError(err); e == nil || e.Code != CodeOverlap {
		t.Errorf("slowing down onto a neighbour: %v, want %s", err, CodeOverlap)
	}
}

// Epic 53 criterion 2: a curve or a reverse is refused with E_VALIDATION and a hint; nothing is approximated.
func TestCompositionSpeedRefusesACurveAndAReverse(t *testing.T) {
	t.Parallel()

	for name, args := range map[string]string{
		"curve":             `{"item_id":"` + itemA + `","curve":[{"t":0,"speed":"1x"},{"t":1,"speed":"2x"}]}`,
		"reverse":           `{"item_id":"` + itemA + `","reverse":true}`,
		"reverse false":     `{"item_id":"` + itemA + `","reverse":false}`,
		"speed and reverse": `{"item_id":"` + itemA + `","speed":"2x","reverse":true}`,
	} {
		doc := speedDoc()
		before := timeline.Canonical(doc)

		_, err := setSpeed(t, doc, args)
		e := AsError(err)

		switch {
		case e == nil:
			t.Errorf("%s: accepted", name)
		case e.Code != CodeValidation || e.Hint == "" || e.Details["reason"] != "composition_speed_constant":
			t.Errorf("%s: %+v, want %s with a hint and the reason composition_speed_constant", name, e, CodeValidation)
		}

		if !bytes.Equal(timeline.Canonical(doc), before) {
			t.Errorf("%s: the document changed", name)
		}
	}
}

// A speed and then the old speed again gives the duration back within the rounding of C.25, and undo restores
// the composition exactly.
func TestCompositionSpeedThereAndBackAndUndo(t *testing.T) {
	t.Parallel()

	for _, speed := range []string{"0.25x", "0.333x", "0.5x", "0.75x", "1.5x", "2x", "3x", "4x"} {
		doc := speedDoc(func(i *timeline.Item) { i.FadeIn, i.FadeOut = 6, 9 })
		before := timeline.Canonical(doc)

		res, err := setSpeed(t, doc, `{"item_id":"`+itemA+`","speed":"`+speed+`","ripple":"track"}`)
		if err != nil {
			t.Fatalf("%s: %v", speed, err)
		}

		back, err := setSpeed(t, res.Doc, `{"item_id":"`+itemA+`","speed":"1x","ripple":"track"}`)
		if err != nil {
			t.Fatalf("%s back: %v", speed, err)
		}

		// ceil(ceil(90 / s) x s) is within one frame of 90 and a speed at most 4 loses at most 4 frames.
		if d := back.Doc.Items[itemA].Duration; d < 90 || d > 90+4 {
			t.Errorf("%s and back: duration %d, want 90 within the rounding", speed, d)
		}

		undone, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
		if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(undone.Doc), before) {
			t.Errorf("%s: undo did not restore the item exactly (skipped %v)", speed, skipped)
		}
	}
}

// A composition at a speed cannot be expanded back into its items: they would play at their own speed.
func TestCompositionSpeedIsNotExpandable(t *testing.T) {
	t.Parallel()

	nested := nestedTL(tlB, 300)
	doc := speedDoc(func(i *timeline.Item) { i.Duration, i.Speed = 45, timeline.Rational{Num: 2, Den: 1} })

	_, err := fullRegistry().Apply(doc, opJSON("composition.expand", `{"item_id":"`+itemA+`"}`),
		Env{NewIDs: (&seqIDs{}).next, Timelines: production(doc, nested)})

	e := AsError(err)
	if e == nil || e.Code != CodeInvalidState || e.Details["feature"] != "speed" {
		t.Fatalf("expand of a composition at 2x: %v, want %s with the feature speed", err, CodeInvalidState)
	}
}
