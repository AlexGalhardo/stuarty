package transcribe

import (
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
)

func word(text string, from, to int64) RawWord {
	return RawWord{Text: text, StartMS: from, EndMS: to, Confidence: math.NaN()}
}

func rawOf(words ...RawWord) Raw { return Raw{Language: "en", Words: words} }

func spans(tr Transcript) [][2]int64 {
	out := make([][2]int64, len(tr.Words))
	for i, w := range tr.Words {
		out[i] = [2]int64{w.StartMS, w.EndMS}
	}
	return out
}

func checkInvariants(t *testing.T, tr Transcript, audioMS int64) {
	t.Helper()
	var prevEnd int64
	for i, w := range tr.Words {
		if w.StartMS < 0 || w.StartMS >= w.EndMS || w.EndMS > audioMS {
			t.Fatalf("word %d %q: %d-%d outside 0 < length, end <= %d", i, w.Text, w.StartMS, w.EndMS, audioMS)
		}
		if w.StartMS < prevEnd {
			t.Fatalf("word %d %q starts at %d before the end of the word before, %d", i, w.Text, w.StartMS, prevEnd)
		}
		prevEnd = w.EndMS
	}
}

// ADR-0021: a first word that whisper starts 300 ms before the audible onset after a pause starts at
// the onset; no word has zero length and no word is shorter than the minimum.
func TestWordTimingRepair(t *testing.T) {
	data, err := os.ReadFile("testdata/pause.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ParseWhisperJSON(data, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(raw.Words); got != 5 {
		t.Fatalf("%d words parsed, want 5: %+v", got, raw.Words)
	}
	if raw.Words[4].Text != "works." || len(raw.Words[4].TokenProbs) != 2 {
		t.Errorf("sub-word tokens were not merged: %+v", raw.Words[4])
	}

	const onset = 2500
	tr, stats := Postprocess(raw, []Range{{StartMS: 1000, EndMS: onset}}, 4000)
	checkInvariants(t, tr, 4000)

	world := tr.Words[1]
	if world.Text != "world" || math.Abs(float64(world.StartMS-onset)) > 100 {
		t.Errorf("world starts at %d, want within 100 ms of the onset %d", world.StartMS, onset)
	}
	if hello := tr.Words[0]; hello.EndMS > 1000+TailPadMS {
		t.Errorf("Hello ends at %d, the pause was not cut from its tail", hello.EndMS)
	}
	for _, w := range tr.Words {
		if w.EndMS-w.StartMS < MinWordMS {
			t.Errorf("%q is %d ms, under the minimum %d", w.Text, w.EndMS-w.StartMS, MinWordMS)
		}
	}
	if stats.OnsetsSnapped != 1 || stats.TailsTrimmed != 1 || stats.ZeroLengthRepaired != 1 ||
		stats.InvariantFixes != 0 {
		t.Errorf("stats %+v", stats)
	}
	if c := tr.Words[4].Confidence; c == nil || *c != 0.6 {
		t.Errorf("confidence of works. = %v, want the lowest token probability 0.6", c != nil)
	}
	if c := tr.Words[1].Confidence; c == nil || *c != 0.8 {
		t.Errorf("confidence of world is set %v, want 0.8 (the timestamp token does not count)", c != nil)
	}
}

// The worked examples of SDD-011 section 10.4.
func TestPostprocessWorkedExamples(t *testing.T) {
	tests := []struct {
		name string
		in   []RawWord
		want [][2]int64
	}{
		{
			"zero-length words share the window with the next word",
			[]RawWord{word("So", 7220, 7220), word("she", 7220, 7220), word("was", 7220, 7500)},
			[][2]int64{{7220, 7290}, {7290, 7395}, {7395, 7500}},
		},
		{
			"a short word grows into the gap after it",
			[]RawWord{word("a", 1000, 1040), word("big", 1340, 1600)},
			[][2]int64{{1000, 1100}, {1340, 1600}},
		},
		{
			"a short word touching the one before takes the difference from it",
			[]RawWord{word("the", 1000, 1300), word("a", 1300, 1340), word("cat", 1340, 1600)},
			[][2]int64{{1000, 1240}, {1240, 1340}, {1340, 1600}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, _ := Postprocess(rawOf(tt.in...), nil, 10000)
			got := spans(tr)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestPostprocessIsPure(t *testing.T) {
	in := rawOf(word("a", 0, 0), word("b", 0, 50), word("c", 900, 1000))
	a, sa := Postprocess(in, []Range{{StartMS: 100, EndMS: 800}}, 2000)
	b, sb := Postprocess(in, []Range{{StartMS: 100, EndMS: 800}}, 2000)
	if sa != sb || len(a.Words) != len(b.Words) {
		t.Fatalf("two runs differ: %+v and %+v", a, b)
	}
	for i := range a.Words {
		if a.Words[i].StartMS != b.Words[i].StartMS || a.Words[i].EndMS != b.Words[i].EndMS {
			t.Fatalf("two runs differ at word %d", i)
		}
	}
	if in.Words[1].EndMS != 50 {
		t.Error("the input was changed")
	}
}

// Whatever the provider says, the transcript keeps its invariants, and the first word that ends after a
// pause of 300 ms or more does not start inside it (SDD-011 section 10.4, A-17).
func TestPostprocessProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for range 300 {
		audio := int64(20000)
		var words []RawWord
		var cursor int64
		for cursor < audio-1500 {
			length := int64(rng.IntN(900))
			words = append(words, word(strings.Repeat("x", 1+rng.IntN(6)), cursor, cursor+length))
			cursor += length
		}
		var pauses []Range
		for at := int64(500); at < audio-1000; at += int64(1500 + rng.IntN(3000)) {
			pauses = append(pauses, Range{StartMS: at, EndMS: at + PauseMinMS + int64(rng.IntN(1200))})
		}
		tr, stats := Postprocess(rawOf(words...), pauses, audio)
		checkInvariants(t, tr, audio)
		if stats.InvariantFixes != 0 {
			t.Fatalf("the repair steps left %d invariant fixes", stats.InvariantFixes)
		}
		for _, p := range pauses {
			for _, w := range tr.Words {
				if w.EndMS <= p.EndMS {
					continue
				}
				if w.StartMS > p.StartMS && w.StartMS < p.EndMS {
					t.Fatalf("%q starts at %d inside the pause %v", w.Text, w.StartMS, p)
				}
				break
			}
		}
	}
}

func TestSegments(t *testing.T) {
	tr, _ := Postprocess(rawOf(
		word("One", 0, 400), word("two.", 400, 900), word("three", 900, 1400), word("four", 3000, 3400),
	), nil, 5000)
	if len(tr.Segments) != 3 {
		t.Fatalf("segments %+v, want 3 (a sentence end, then a gap of 1.6 s)", tr.Segments)
	}
	if s := tr.Segments[1]; s.FirstWord != 2 || s.LastWord != 2 {
		t.Errorf("segment 1 %+v", s)
	}
	if Segments(nil) == nil {
		t.Error("an empty transcript must have an empty list of segments, not null")
	}
}
