package transcribe

import (
	"math"
	"strings"
	"unicode/utf8"
)

// Constants of SDD-011 section 10.4.
const (
	// PauseMinMS is the shortest silence that counts as a pause.
	PauseMinMS = 300
	// TailPadMS is what stays of a word that spans a pause.
	TailPadMS = 120
	// MinWordMS is the shortest word. The limit is an initial value (ADR-0021).
	MinWordMS = 100
	// DegenerateMS is the length under which a word counts as zero-length.
	DegenerateMS = 20

	postprocessVersion = 1
)

// Postprocess turns the words of a provider into the transcript: onset snap and tail trim, repair of
// zero-length words, minimum duration, confidence (SDD-011 section 10.4). It is pure: the same input
// gives the same output. silences are in milliseconds; audioMS is the length of the audio.
func Postprocess(raw Raw, silences []Range, audioMS int64) (Transcript, Stats) {
	stats := Stats{Version: postprocessVersion}
	words := make([]Word, len(raw.Words))
	for i, w := range raw.Words {
		words[i] = Word{Text: w.Text, StartMS: w.StartMS, EndMS: w.EndMS, Confidence: confidence(w)}
	}

	var pauses []Range
	for _, s := range silences {
		if s.EndMS-s.StartMS >= PauseMinMS {
			pauses = append(pauses, s)
		}
	}
	stats.Pauses = len(pauses)

	snapOnsets(words, pauses, &stats)
	repairZeroLength(words, &stats)
	minDuration(words, &stats)
	// Steps 2 and 3 can move a boundary back into a silence that step 1 had cleared.
	snapOnsets(words, pauses, &stats)
	minDuration(words, &stats)
	words = enforce(words, audioMS, &stats)

	return Transcript{Language: raw.Language, Words: words, Segments: Segments(words)}, stats
}

// confidence is the minimum token probability of the word, or the provider's value, to 3 decimals.
func confidence(w RawWord) *float64 {
	var v float64
	switch {
	case len(w.TokenProbs) > 0:
		v = w.TokenProbs[0]
		for _, p := range w.TokenProbs[1:] {
			v = min(v, p)
		}
	case !math.IsNaN(w.Confidence) && w.Confidence != 0:
		v = w.Confidence
	default:
		return nil
	}
	v = math.Round(min(max(v, 0), 1)*1000) / 1000
	return &v
}

// snapOnsets is step 1: a word that starts inside a pause starts where the pause ends, and the word
// before a pause loses the pause from its tail.
func snapOnsets(w []Word, pauses []Range, stats *Stats) {
	for _, p := range pauses {
		s, e := p.StartMS, p.EndMS
		k := -1
		for i := range w {
			if w[i].EndMS > e {
				k = i
				break
			}
		}
		if k < 0 {
			continue
		}
		var j int
		switch {
		case s <= w[k].StartMS && w[k].StartMS < e:
			w[k].StartMS = e
			if w[k].EndMS < e+MinWordMS {
				limit := e + MinWordMS
				if k+1 < len(w) {
					limit = min(limit, w[k+1].StartMS)
				}
				w[k].EndMS = max(w[k].EndMS, limit)
			}
			if k > 0 && w[k-1].EndMS > e {
				w[k-1].EndMS = e
			}
			stats.OnsetsSnapped++
			j = k - 1
		case w[k].StartMS < s:
			j = k
		default:
			j = k - 1
		}
		if j >= 0 && w[j].StartMS < s && s < w[j].EndMS && w[j].EndMS > s+TailPadMS {
			w[j].EndMS = max(w[j].StartMS+MinWordMS, s+TailPadMS)
			stats.TailsTrimmed++
		}
	}
}

// repairZeroLength is step 2: a run of degenerate words shares the time of its neighbour, in proportion
// to the length of the texts, with no gap and no overlap.
func repairZeroLength(w []Word, stats *Stats) {
	degenerate := func(i int) bool { return w[i].EndMS-w[i].StartMS < DegenerateMS }
	for a := 0; a < len(w); a++ {
		if !degenerate(a) {
			continue
		}
		b := a
		for b+1 < len(w) && degenerate(b+1) {
			b++
		}
		first, last := a, b
		switch {
		case b+1 < len(w):
			last = b + 1
		case a > 0:
			first = a - 1
		default:
			a = b
			continue
		}
		from, to := w[first].StartMS, w[last].EndMS
		if to > from {
			split(w[first:last+1], from, to)
			stats.ZeroLengthRepaired += b - a + 1
		}
		a = last
	}
}

// split covers [from, to] with the words in proportion to their length in characters (at least 1 each);
// the last word takes the remainder of the integer rounding.
func split(w []Word, from, to int64) {
	var total int64
	for i := range w {
		total += int64(max(utf8.RuneCountInString(w[i].Text), 1))
	}
	var cum int64
	cursor := from
	for i := range w {
		cum += int64(max(utf8.RuneCountInString(w[i].Text), 1))
		end := from + (to-from)*cum/total
		if i == len(w)-1 {
			end = to
		}
		w[i].StartMS, w[i].EndMS = cursor, end
		cursor = end
	}
}

// minDuration is step 3: a short word grows into the gap after it, and then, when it touches the word
// before it, takes the difference from that word. Words that do not touch are never stretched backwards:
// that would swallow a pause.
func minDuration(w []Word, stats *Stats) {
	for i := range w {
		if w[i].EndMS-w[i].StartMS >= MinWordMS {
			continue
		}
		before := w[i]
		end := w[i].StartMS + MinWordMS
		if i+1 < len(w) {
			end = min(end, w[i+1].StartMS)
		}
		w[i].EndMS = max(w[i].EndMS, end)
		if need := MinWordMS - (w[i].EndMS - w[i].StartMS); need > 0 && i > 0 &&
			w[i-1].EndMS == w[i].StartMS && w[i-1].EndMS-w[i-1].StartMS-need >= MinWordMS {
			w[i-1].EndMS -= need
			w[i].StartMS -= need
		}
		if !sameWord(w[i], before) {
			stats.MinDurationExtend++
		}
	}
}

func sameWord(a, b Word) bool { return a.StartMS == b.StartMS && a.EndMS == b.EndMS }

// enforce makes the invariants true: 0 <= start < end <= audio, no overlap, order kept. A word that
// cannot satisfy them (it starts at the end of the audio) is dropped. Each change is counted.
func enforce(w []Word, audioMS int64, stats *Stats) []Word {
	out := w[:0]
	var prevEnd int64
	for _, x := range w {
		fixed := x
		fixed.StartMS = min(max(fixed.StartMS, prevEnd, 0), audioMS)
		fixed.EndMS = min(fixed.EndMS, audioMS)
		if fixed.EndMS <= fixed.StartMS {
			fixed.EndMS = min(fixed.StartMS+1, audioMS)
		}
		if fixed.EndMS <= fixed.StartMS {
			stats.InvariantFixes++
			continue
		}
		if fixed.StartMS != x.StartMS || fixed.EndMS != x.EndMS {
			stats.InvariantFixes++
		}
		prevEnd = fixed.EndMS
		out = append(out, fixed)
	}
	return out
}

// Segments derives the segments of the transcript: one ends after a word whose text ends a sentence,
// when the gap to the next word is at least 700 ms, or when it has lasted 15 s.
func Segments(words []Word) []Segment {
	const (
		gapMS  = 700
		maxSeg = 15000
	)
	segs := []Segment{}
	if len(words) == 0 {
		return segs
	}
	first := 0
	for i, w := range words {
		last := i == len(words)-1
		cut := last || endsSentence(w.Text) || words[i+1].StartMS-w.EndMS >= gapMS ||
			words[i+1].EndMS-words[first].StartMS > maxSeg
		if cut {
			segs = append(segs, Segment{StartMS: words[first].StartMS, EndMS: w.EndMS, FirstWord: first, LastWord: i})
			first = i + 1
		}
	}
	return segs
}

func endsSentence(text string) bool {
	return strings.HasSuffix(text, ".") || strings.HasSuffix(text, "?") || strings.HasSuffix(text, "!") ||
		strings.HasSuffix(text, "…") || strings.HasSuffix(text, "。") || strings.HasSuffix(text, "？") ||
		strings.HasSuffix(text, "！")
}
