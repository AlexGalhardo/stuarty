package command

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Error messages of the item property commands quote identifiers and numbers, never
// the label a person gave an item.
func TestErrorsQuoteIdsNotNames(t *testing.T) {
	const label = "Secret label"
	named := func(it *timeline.Item) { it.Name = label }
	d := base().
		clip(itemA, vidTr, 0, 150, 0, named, func(it *timeline.Item) {
			it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 100}
			it.FadeOut = 60
			it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(30, 2)}, "volume": {kf(0, 1)}}
		}).
		text(itemC, vidTr, 200, 60, named).
		audio(itemB, 0, 60, 0).d
	d.Items[itemB] = func() timeline.Item { it := d.Items[itemB]; it.Name = label; return it }()

	for _, o := range []Operation{
		op2("item.set_transform", `{"item_id": "`+itemB+`", "x": 1}`),
		op2("item.set_transform", `{"item_id": "`+itemA+`", "scale": 2}`),
		op2("item.set_transition", `{"item_id": "`+itemB+`", "edge": "in", "kind": "fade"}`),
		op2("item.set_transition", `{"item_id": "`+itemA+`", "edge": "in", "kind": "fade", "duration": "60f"}`),
		op2("item.set_audio", `{"item_id": "`+itemC+`", "volume": 1}`),
		op2("item.set_audio", `{"item_id": "`+itemB+`", "audio_muted": true}`),
		op2("item.set_audio", `{"item_id": "`+itemA+`", "volume": 2}`),
		op2("item.set_audio", `{"item_id": "`+itemA+`", "fade_in": "100f"}`),
	} {
		_, err := Default().Apply(d, o, Env{})
		if err == nil {
			t.Errorf("%s %s succeeded", o.Op, o.Args)
			continue
		}
		e := AsError(err)
		if strings.Contains(e.Message+e.Hint, label) {
			t.Errorf("%s quotes a name: %s %s", o.Op, e.Message, e.Hint)
		}
	}
}

func TestSetTransformRoundsToZeroScale(t *testing.T) {
	d := base().text(itemC, vidTr, 0, 60).d
	_, err := Default().Apply(d, op2("item.set_transform", `{"item_id": "`+itemC+`", "scale": 0.0001}`), Env{})
	if e := AsError(err); err == nil || e.Code != CodeValidation {
		t.Errorf("%v", err)
	}
	if _, err := Default().Apply(d, op2("item.set_transform", `{"item_id": "`+itemC+`", "x": 1e999}`), Env{}); err == nil {
		t.Error("an out-of-range number was accepted")
	}
}

// An extreme number is E_VALIDATION, never a document that cannot be encoded (VV-003).
func TestExtremeNumbersAreRefused(t *testing.T) {
	d := base().text(itemC, vidTr, 0, 60).d
	for _, args := range []string{
		`{"item_id": "` + itemC + `", "x": 1e308}`,
		`{"item_id": "` + itemC + `", "y": -1e308}`,
		`{"item_id": "` + itemC + `", "x": 1e300}`,
		`{"item_id": "` + itemC + `", "rotation": 1e10}`,
	} {
		_, err := Default().Apply(d, op2("item.set_transform", args), Env{})
		if e := AsError(err); err == nil || e.Code != CodeValidation {
			t.Errorf("%s: %v", args, err)
		}
	}
	shape := `{"track_id": "` + vidTr + `", "id": "itm_s1t2u3v4w5", "shape": "rect", "width": 1e308, "height": 10}`
	_, err := Default().Apply(d, op2("shape.add", shape), Env{})
	if e := AsError(err); err == nil || e.Code != CodeValidation {
		t.Errorf("shape.add: %v", err)
	}
	if got := round3(1e308); got != 1e308 {
		t.Errorf("round3(1e308) = %v", got)
	}
}

// Without track_id an add command takes the highest free visual track, and makes a new one on
// top when the clip occupies the only one (VV-008).
func TestAddWithoutTrackPicksOrMakesAFreeVisualTrack(t *testing.T) {
	d := base().clip(itemA, vidTr, 0, 150, 0).d
	n := 0
	env := Env{NewIDs: func(prefix string) string {
		n++
		return prefix + "_" + strings.Repeat("a", 9) + string(rune('a'+n))
	}}
	for id := range d.Tracks {
		if id != vidTr {
			delete(d.Tracks, id)
		}
	}
	nTracks := len(d.Tracks)

	for _, o := range []Operation{
		op2("text.add", `{"start": "1s", "duration": "2s", "text": "Hi"}`),
		op2("shape.add", `{"start": "1s", "duration": "2s", "shape": "rect", "width": 10, "height": 10}`),
		op2("caption.add", `{"start": "1s", "duration": "2s", "text": "Hi there"}`),
	} {
		res, err := Default().Apply(d, o, env)
		if err != nil {
			t.Fatalf("%s: %v", o.Op, err)
		}

		tr, _ := res.Effective["track_id"].(string)
		if tr == "" || tr == vidTr {
			t.Fatalf("%s: effective track %q", o.Op, tr)
		}

		if len(res.Doc.Tracks) != nTracks+1 || res.Doc.Tracks[tr].Order >= d.Tracks[vidTr].Order {
			t.Errorf("%s: tracks %d, new order %q, clip track %q", o.Op, len(res.Doc.Tracks),
				res.Doc.Tracks[tr].Order, d.Tracks[vidTr].Order)
		}
	}

	// A second overlay at the same time lands on the first free track, not on top of the first.
	one, err := Default().Apply(d, op2("text.add", `{"start": "1s", "duration": "2s", "text": "A"}`), env)
	if err != nil {
		t.Fatal(err)
	}

	two, err := Default().Apply(one.Doc, op2("text.add", `{"start": "1s", "duration": "2s", "text": "B"}`), env)
	if err != nil {
		t.Fatal(err)
	}

	if one.Effective["track_id"] == two.Effective["track_id"] {
		t.Error("two overlapping texts share a track")
	}
}

// A family with no 700 face works without naming a weight; an explicit missing weight still fails (VV-057).
func TestTextAddDefaultWeightFallsToTheFamily(t *testing.T) {
	d := base().d
	n := 0
	env := Env{NewIDs: func(prefix string) string {
		n++
		return prefix + "_" + strings.Repeat("b", 9) + string(rune('a'+n))
	}}

	ok := op2(
		"text.add",
		`{"track_id": "`+vidTr+`", "start": "0s", "duration": "1s", "text": "x", "style": {"font_family": "Anton"}}`,
	)
	if _, err := Default().Apply(d, ok, env); err != nil {
		t.Errorf("default weight: %v", err)
	}

	bad := op2(
		"text.add",
		`{"track_id": "`+vidTr+`", "start": "0s", "duration": "1s", "text": "x", "style": {"font_family": "Anton", "font_weight": 700}}`,
	)
	if _, err := Default().Apply(d, bad, env); AsError(err) == nil || AsError(err).Code != CodeFontUnknown {
		t.Errorf("explicit weight: %v", err)
	}
}

// Without start, duplicating twice walks to the next free position instead of E_OVERLAP (VV-066).
func TestDuplicateWithoutStartTakesTheNextFreeSlot(t *testing.T) {
	d := base().text(itemC, vidTr, 0, 60).d
	n := 0
	env := Env{NewIDs: func(prefix string) string {
		n++
		return prefix + "_" + strings.Repeat("c", 9) + string(rune('a'+n))
	}}

	var starts []int

	for range 3 {
		res, err := Default().Apply(d, op2("item.duplicate", `{"item_id": "`+itemC+`"}`), env)
		if err != nil {
			t.Fatal(err)
		}

		d = res.Doc
		id, _ := res.Effective["new_id"].(string)
		starts = append(starts, d.Items[id].Start)
	}

	if starts[0] != 60 || starts[1] != 120 || starts[2] != 180 {
		t.Errorf("starts = %v, want 60 120 180", starts)
	}
}

// Changing the frame rate back and forth keeps the words of a caption touching (VV-058).
func TestSetFPSRoundTripKeepsWordsTouching(t *testing.T) {
	words := []timeline.Word{
		{Text: "a", Start: 0, Duration: 8},
		{Text: "b", Start: 8, Duration: 8},
		{Text: "c", Start: 16, Duration: 7},
		{Text: "d", Start: 23, Duration: 7},
	}
	d := base().d
	n := 0
	env := Env{NewIDs: func(prefix string) string {
		n++
		return prefix + "_" + strings.Repeat("d", 9) + string(rune('a'+n))
	}}

	var raw []map[string]string
	for _, w := range words {
		raw = append(raw, map[string]string{
			"text": w.Text, "start": strconv.Itoa(w.Start) + "f", "duration": strconv.Itoa(w.Duration) + "f",
		})
	}

	args, err := json.Marshal(map[string]any{"track_id": vidTr, "start": "0f", "words": raw})
	if err != nil {
		t.Fatal(err)
	}

	for _, o := range []Operation{op2("caption.add", string(args))} {
		res, err := Default().Apply(d, o, env)
		if err != nil {
			t.Fatal(err)
		}

		d = res.Doc
	}

	var capID string
	for id, it := range d.Items {
		if it.Type == timeline.ItemCaption {
			capID = id
		}
	}

	for _, fps := range []string{`"24"`, `"30000/1001"`, `"60"`, `"30"`} {
		res, err := Default().Apply(d, op2("settings.set_fps", `{"fps": `+fps+`}`), env)
		if err != nil {
			t.Fatalf("%s: %v", fps, err)
		}

		d = res.Doc
	}

	// Rounding each frame rate loses information, so the exact start of a word may move by a
	// frame; the words keep touching and the last one keeps ending with the caption.
	got := d.Items[capID].Words
	end := 0

	for i, w := range got {
		if w.Start != end {
			t.Errorf("word %d starts at %d, the previous one ended at %d", i, w.Start, end)
		}

		end = w.Start + w.Duration
	}

	if want := words[len(words)-1].Start + words[len(words)-1].Duration; end != want {
		t.Errorf("the last word ends at %d, want %d", end, want)
	}
}

// caption.set_words with text changes the word count and keeps the caption's duration (VV-043).
func TestCaptionSetWordsFromText(t *testing.T) {
	d := base().caption(itemA, 0, 6).d

	res, err := Default().Apply(d, op2("caption.set_words", `{"item_id": "`+itemA+`", "text": "just two"}`), Env{})
	if err != nil {
		t.Fatal(err)
	}

	it := res.Doc.Items[itemA]
	if len(it.Words) != 2 || it.Duration != 60 || it.Words[1].Start+it.Words[1].Duration != 60 {
		t.Errorf("words = %+v, duration %d", it.Words, it.Duration)
	}

	both := `{"item_id": "` + itemA + `", "text": "x", "words": [{"text": "a", "start": "0f", "duration": "5f"}]}`
	if _, err := Default().Apply(d, op2("caption.set_words", both), Env{}); AsError(err) == nil {
		t.Error("words and text together were accepted")
	}
}

// A voice-over on an audio track can be captioned from its transcript (VV-041).
func TestCaptionFromTranscriptOnAnAudioItem(t *testing.T) {
	d := base().audio(itemB, 0, 90, 0).d
	args := `{"clip_item_id": "` + itemB + `", "track_id": "` + capTr + `", "new_ids": ["itm_z9y8x7w6v5"], ` +
		`"resolved": {"words": [{"text": "Hello", "start": 2, "end": 10}, {"text": "world", "start": 12, "end": 20}]}}`

	res, err := Default().Apply(d, op2("caption.from_transcript", args), Env{})
	if err != nil {
		t.Fatal(err)
	}

	it := res.Doc.Items["itm_z9y8x7w6v5"]
	if it.Type != timeline.ItemCaption || it.LinkedItem != itemB || len(it.Words) != 2 {
		t.Errorf("caption = %+v", it)
	}
}

// A locked track refuses every edit of its items, and only those (VV-036).
func TestLockedTrackRefusesEditsOfItsItems(t *testing.T) {
	d := base().clip(itemA, vidTr, 0, 150, 0).audio(itemB, 0, 60, 0).d
	tr := d.Tracks[vidTr]
	tr.Locked = true
	d.Tracks[vidTr] = tr

	refused := map[string]string{
		"item.move":      `{"item_id": "` + itemA + `", "start": "10f"}`,
		"item.delete":    `{"item_ids": ["` + itemA + `"]}`,
		"item.trim":      `{"item_id": "` + itemA + `", "edge": "end", "to": "100f"}`,
		"item.rename":    `{"item_id": "` + itemA + `", "name": "x"}`,
		"clip.set_color": `{"item_id": "` + itemA + `", "brightness": 0.1}`,
		"clip.add":       `{"track_id": "` + vidTr + `", "asset_id": "` + vidAsset + `", "id": "itm_x000000009", "start": "200f", "duration": "30f"}`,
	}
	for name, args := range refused {
		_, err := Default().Apply(d, op2(name, args), Env{})
		if e := AsError(err); e == nil || e.Code != CodeInvalidState || !strings.Contains(e.Message, "locked") {
			t.Errorf("%s on a locked track: %v", name, err)
		}
	}

	allowed := map[string]string{
		"item.rename":  `{"item_id": "` + itemB + `", "name": "x"}`,
		"track.update": `{"track_id": "` + vidTr + `", "locked": false}`,
	}
	for name, args := range allowed {
		if _, err := Default().Apply(d, op2(name, args), Env{}); err != nil {
			t.Errorf("%s should be allowed: %v", name, err)
		}
	}
}
