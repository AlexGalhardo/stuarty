package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// The measurement of epic 89 (docs/perf/2026-11-24-transcript-snap.md): on the labelled fixture of epic 69, how
// often a cut of one word clips a neighbouring word or leaves part of the word it removes, at the whisper times and
// snapped to the silences of the asset. The silence artifacts are the ones the product's own ingest wrote for the
// fixture audio (testdata/<lang>.silence.json, the -35 dB set, frames of 30 fps like the default project).

const (
	fixtureFPS      = 30
	snapWindowFrame = 9 // the default window of 0.3 s at 30 fps
	snapToleranceMS = 40
)

func loadSilences(t *testing.T, lang string) []Silence {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", lang+".silence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SampleRate int64 `json:"sample_rate"`
		Sets       []struct {
			ThresholdDB int `json:"threshold_db"`
			Ranges      []struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"ranges"`
		} `json:"sets"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	fps := timeval.FPS{Num: fixtureFPS, Den: 1}
	var out []Silence
	for _, set := range doc.Sets {
		if set.ThresholdDB != -35 {
			continue
		}
		for _, r := range set.Ranges {
			start, end := fps.FramesInside(r.Start, r.End, doc.SampleRate)
			if end > start {
				out = append(out, Silence{int(start), int(end)})
			}
		}
	}
	return out
}

func frameMS(f int) int64 { return int64(f) * 1000 / fixtureFPS }

// truthEnd is the end of a labelled word: the next onset, or the start of the first silence after its onset
// when there is one before the next onset (the synthesizer reports onsets only).
func truthEnd(l labels, i int, silences []Silence) int64 {
	onset := l.Words[i].StartMS
	next := onset + 800
	if i+1 < len(l.Words) {
		next = l.Words[i+1].StartMS
	}
	end := next
	for _, s := range silences {
		if sm := frameMS(s.Start); sm > onset+snapToleranceMS && sm <= next {
			end = min(end, sm)
		}
	}
	return end
}

// snapScore counts the one-word cuts of a fixture by what they do to the speech around them.
type snapScore struct {
	cuts int
	// clips cut a neighbouring word: the range starts before the previous word ended or ends after the next began.
	clips int
	// slivers leave part of the removed word: the range starts after it began or ends before it ended.
	slivers int
	clean   int
	// touched are the cuts whose range the snap changed, and how many of those clip or leave a sliver.
	touched, touchedClips, touchedSlivers int
}

func (s snapScore) share(n int) float64 { return ratio(n, s.cuts) }

func (s *snapScore) add(l labels, silences []Silence, a, b, i int) (clips, sliver bool) {
	start, end := frameMS(a), frameMS(b)
	clips = start < truthEnd(l, i-1, silences)-snapToleranceMS || end > l.Words[i+1].StartMS+snapToleranceMS
	sliver = start > l.Words[i].StartMS+snapToleranceMS || end < truthEnd(l, i, silences)-snapToleranceMS
	s.cuts++
	if clips {
		s.clips++
	}
	if sliver {
		s.slivers++
	}
	if !clips && !sliver {
		s.clean++
	}
	return clips, sliver
}

// measureSnap cuts every word that the transcript and the labels agree on and that has a labelled neighbour on each
// side, at the whisper times (plain) and snapped.
func measureSnap(t *testing.T, lang string) (plain, snapped snapScore) {
	t.Helper()
	l, words, _ := loadFixture(t, lang)
	silences := loadSilences(t, lang)
	fps := timeval.FPS{Num: fixtureFPS, Den: 1}
	// The frames of the service (transcriptFrames): start at the boundary, never before the previous end.
	frames := make([]Word, len(words))
	cursor := int64(0)
	for j, w := range words {
		start := max(fps.FrameAtMillis(w.StartMS), cursor)
		end := max(fps.FrameAtMillis(w.EndMS), start+1)
		cursor = end
		frames[j] = Word{Text: w.Text, Start: int(start), End: int(end)}
	}
	truth := make([]string, len(l.Words))
	for i, w := range l.Words {
		truth[i] = Normalize(w.Text)
	}
	got := make([]string, len(words))
	for j, w := range words {
		got[j] = Normalize(w.Text)
	}
	for _, p := range lcs(truth, got) {
		i, j := p[0], p[1]
		if i == 0 || i+1 >= len(l.Words) || j == 0 || j+1 >= len(frames) {
			continue
		}
		pc, ps := plain.add(l, silences, frames[j].Start, frames[j].End, i)
		e := SnapEdges(silences, snapWindowFrame, frames, j, j)
		sc, ss := snapped.add(l, silences, e.Start.Frame, e.End.Frame, i)
		if e.Start.Frame != frames[j].Start || e.End.Frame != frames[j].End {
			plain.touched++
			snapped.touched++
			for _, c := range []struct {
				n *int
				v bool
			}{{&plain.touchedClips, pc}, {&plain.touchedSlivers, ps}, {&snapped.touchedClips, sc}, {&snapped.touchedSlivers, ss}} {
				if c.v {
					*c.n++
				}
			}
		}
	}
	return plain, snapped
}

// snapFloors hold the snap to what the measurement showed, ten points under it where a number is a share
// (docs/perf/2026-11-24-transcript-snap.md): the snap may never clip more cuts than the whisper times do, must leave
// fewer words half cut, and must change the cuts that have a pause beside them (at least minTouched of them).
var snapFloors = map[string]struct{ minTouched int }{
	"en": {minTouched: 5},
	"pt": {minTouched: 5},
}

func TestLabelledFixtureSnap(t *testing.T) {
	for _, lang := range []string{"en", "pt"} {
		plain, snapped := measureSnap(t, lang)
		t.Logf("%s: %d one-word cuts; the snap changed %d of them", lang, plain.cuts, snapped.touched)
		t.Logf("%s: unsnapped: clip a neighbour %.3f, leave part of the word %.3f, clean %.3f",
			lang, plain.share(plain.clips), plain.share(plain.slivers), plain.share(plain.clean))
		t.Logf("%s: snapped:   clip a neighbour %.3f, leave part of the word %.3f, clean %.3f",
			lang, snapped.share(snapped.clips), snapped.share(snapped.slivers), snapped.share(snapped.clean))
		t.Logf(
			"%s: the %d cuts the snap changed: clip %d -> %d, leave part of the word %d -> %d",
			lang,
			snapped.touched,
			plain.touchedClips,
			snapped.touchedClips,
			plain.touchedSlivers,
			snapped.touchedSlivers,
		)
		if snapped.clips > plain.clips {
			t.Errorf(
				"%s: the snap clips a neighbour in %d cuts, the whisper times in %d",
				lang,
				snapped.clips,
				plain.clips,
			)
		}
		if snapped.slivers >= plain.slivers {
			t.Errorf("%s: the snap leaves part of the word in %d cuts, the whisper times in %d", lang,
				snapped.slivers, plain.slivers)
		}
		if snapped.touched < snapFloors[lang].minTouched {
			t.Errorf(
				"%s: the snap changed %d cuts, below the floor %d",
				lang,
				snapped.touched,
				snapFloors[lang].minTouched,
			)
		}
	}
}
