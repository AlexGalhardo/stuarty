package transcript

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The labelled fixture of epic 69 (SDD-023 section 7): a synthetic speech recording per language whose
// fillers and repetitions are known (testdata/gen-speech.ps1 writes the labels), and the transcript the
// pinned whisper.cpp (v1.9.2, model small) made of it, taken from the transcript artifact. The tests below
// measure the detector on those transcripts and hold it to the thresholds of the report in docs/perf/.

type labelWord struct {
	Text    string `json:"text"`
	Kind    string `json:"kind"`
	StartMS int64  `json:"start_ms"`
}

type labels struct {
	Lang  string      `json:"lang"`
	Words []labelWord `json:"words"`
}

type artifact struct {
	Language string `json:"language"`
	Words    []struct {
		Text    string `json:"text"`
		StartMS int64  `json:"start_ms"`
		EndMS   int64  `json:"end_ms"`
	} `json:"words"`
}

// labelled is something a person would remove: a filler word or the first copy of a repeated sequence.
type labelled struct {
	kind         Kind
	from, to     int
	startMS, end int64
}

func loadFixture(t *testing.T, lang string) (labels, []SpokenWord, string) {
	t.Helper()
	var l labels
	var a artifact
	for name, into := range map[string]any{lang + ".labels.json": &l, lang + ".transcript.json": &a} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, into); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	words := make([]SpokenWord, len(a.Words))
	for i, w := range a.Words {
		words[i] = SpokenWord{Text: w.Text, StartMS: w.StartMS, EndMS: w.EndMS}
	}
	return l, words, a.Language
}

// items groups the labelled words: runs of fillers and runs of first-copy words become one item each. An
// item lasts from the onset of its first word to the onset of the word after it, at most 800 ms a word.
func items(l labels) []labelled {
	var out []labelled
	for i := 0; i < len(l.Words); i++ {
		var kind Kind
		switch l.Words[i].Kind {
		case "filler":
			kind = KindFiller
		case "repeat":
			kind = KindRepetition
		default:
			continue
		}
		j := i
		for j+1 < len(l.Words) && l.Words[j+1].Kind == l.Words[i].Kind {
			j++
		}
		end := l.Words[i].StartMS + 800*int64(j-i+1)
		if j+1 < len(l.Words) {
			end = min(end, l.Words[j+1].StartMS)
		}
		out = append(out, labelled{kind: kind, from: i, to: j, startMS: l.Words[i].StartMS, end: end})
		i = j
	}
	return out
}

// toleranceMS is how far apart a suggestion and a label may be and still be the same thing. The post-processing
// of word timings (SDD-011 section 10.4) moves the onset of a word after a pause to the end of the pause,
// but a word inside a run still starts up to a few hundred milliseconds before the audible onset.
const toleranceMS = 300

func near(g Suggestion, w labelled) bool {
	return g.Kind == w.kind && g.StartMS-toleranceMS < w.end && w.startMS < g.EndMS+toleranceMS
}

type score struct {
	items, found                   int
	suggestions, right             int
	strongSuggestions, strongRight int
	fillers, fillersFound          int
	repeats, repeatsFound          int
	timingN                        int
	timingMedian, timingP90        float64
	within100, within200           int
	startsN                        int
	startsMedian, startsP90        float64
}

func evaluate(t *testing.T, lang string) score {
	t.Helper()
	l, words, language := loadFixture(t, lang)
	suggestions := Detect(words, Options{Language: language})
	want := items(l)
	var s score
	s.items = len(want)
	for _, w := range want {
		hit := slices.ContainsFunc(suggestions, func(g Suggestion) bool { return near(g, w) })
		if !hit {
			t.Logf("%s: missed: %s %q at %d-%d ms", lang, w.kind, l.Words[w.from].Text, w.startMS, w.end)
		}
		if w.kind == KindFiller {
			s.fillers++
			if hit {
				s.fillersFound++
			}
		} else {
			s.repeats++
			if hit {
				s.repeatsFound++
			}
		}
		if hit {
			s.found++
		}
	}
	for _, g := range suggestions {
		s.suggestions++
		right := slices.ContainsFunc(want, func(w labelled) bool { return near(g, w) })
		if right {
			s.right++
		} else {
			t.Logf("%s: not in the labels: %s %q at %d-%d ms", lang, g.Kind, g.Text, g.StartMS, g.EndMS)
		}
		if g.Strength == Strong {
			s.strongSuggestions++
			if right {
				s.strongRight++
			}
		}
	}

	// Word timing: the words both sides agree on, found by the longest common subsequence of the
	// normalised texts, compared by onset.
	truth := make([]string, len(l.Words))
	for i, w := range l.Words {
		truth[i] = Normalize(w.Text)
	}
	got := make([]string, len(words))
	for i, w := range words {
		got[i] = Normalize(w.Text)
	}
	var errs, starts []float64
	for _, p := range lcs(truth, got) {
		e := math.Abs(float64(words[p[1]].StartMS - l.Words[p[0]].StartMS))
		errs = append(errs, e)
		// A word that follows a pause of 900 ms or more between onsets starts a sentence.
		if p[0] > 0 && l.Words[p[0]].StartMS-l.Words[p[0]-1].StartMS >= 900 {
			starts = append(starts, e)
		}
	}
	slices.Sort(errs)
	slices.Sort(starts)
	s.startsN = len(starts)
	if len(starts) > 0 {
		s.startsMedian = starts[len(starts)/2]
		s.startsP90 = starts[min(len(starts)-1, len(starts)*9/10)]
	}
	s.timingN = len(errs)
	if len(errs) > 0 {
		s.timingMedian = errs[len(errs)/2]
		s.timingP90 = errs[min(len(errs)-1, len(errs)*9/10)]
		for _, e := range errs {
			if e <= 100 {
				s.within100++
			}
			if e <= 200 {
				s.within200++
			}
		}
	}
	return s
}

// lcs returns the index pairs of a longest common subsequence.
func lcs(a, b []string) [][2]int {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			out = append(out, [2]int{i, j})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}

// floors are the thresholds fixed by the measurement of docs/perf/2026-11-23-transcript-fillers.md: about ten
// points under what the pinned helper gave, so that a change of the rules or of the post-processing that
// makes detection worse fails here. They are floors of a regression test, not a promise to the user: the
// report says what the detector can and cannot find. Portuguese has no floor for fillers: whisper writes the
// hesitations "hum" and "ahn" as the words "um" and "a", which the rule cannot tell from the article.
var floors = map[string]struct {
	precision, recall, fillers, repetitions, sentenceStartMedianMS float64
}{
	"en": {precision: 0.85, recall: 0.70, fillers: 0.60, repetitions: 0.90, sentenceStartMedianMS: 60},
	"pt": {precision: 0.85, recall: 0.20, fillers: 0, repetitions: 0.70, sentenceStartMedianMS: 60},
}

func TestLabelledFixture(t *testing.T) {
	for _, lang := range []string{"en", "pt"} {
		s := evaluate(t, lang)
		f := floors[lang]
		precision, recall := ratio(s.right, s.suggestions), ratio(s.found, s.items)
		fillers, repeats := ratio(s.fillersFound, s.fillers), ratio(s.repeatsFound, s.repeats)
		t.Logf("%s: items %d (fillers %d, repetitions %d); found %d (fillers %d, repetitions %d)", lang,
			s.items, s.fillers, s.repeats, s.found, s.fillersFound, s.repeatsFound)
		t.Logf("%s: suggestions %d, right %d (strong %d, right %d)", lang,
			s.suggestions, s.right, s.strongSuggestions, s.strongRight)
		t.Logf("%s: precision %.3f, recall %.3f (fillers %.3f, repetitions %.3f), strong precision %.3f", lang,
			precision, recall, fillers, repeats, ratio(s.strongRight, s.strongSuggestions))
		t.Logf("%s: timing over %d matched words: median %.0f ms, p90 %.0f ms, within 100 ms %.3f, within 200 ms %.3f",
			lang, s.timingN, s.timingMedian, s.timingP90, ratio(s.within100, s.timingN), ratio(s.within200, s.timingN))
		t.Logf("%s: onset of the %d words that start a sentence: median %.0f ms, p90 %.0f ms", lang, s.startsN,
			s.startsMedian, s.startsP90)
		for name, c := range map[string]struct{ got, floor float64 }{
			"precision": {precision, f.precision}, "recall": {recall, f.recall},
			"filler recall": {fillers, f.fillers}, "repetition recall": {repeats, f.repetitions},
		} {
			if c.got < c.floor {
				t.Errorf("%s: %s is %.3f, below the floor %.2f", lang, name, c.got, c.floor)
			}
		}
		if s.startsMedian > f.sentenceStartMedianMS {
			t.Errorf("%s: the onset of sentence starts is %.0f ms off, above %.0f ms", lang, s.startsMedian,
				f.sentenceStartMedianMS)
		}
	}
}
