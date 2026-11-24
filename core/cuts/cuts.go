// Package cuts proposes vertical cuts of a long talking video (SDD-024 section 8): ranges that start and end
// at sentence boundaries, long enough to publish and short enough to hold attention, in which the speaker's
// face is on screen. It is pure: it reads the words of a transcript and a face track and returns proposals; it
// changes no timeline. The batch of commands a person accepts is built here too, so no client builds one.
package cuts

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/facetrack"
)

// Word is a transcript word on the asset's time axis, in milliseconds.
type Word struct {
	Text    string
	StartMS int
	EndMS   int
}

// Options are the knobs of the proposal (SDD-024 section 8). Every value is an integer.
type Options struct {
	// Count is the most proposals returned.
	Count int `json:"count,omitempty"`
	// MinMS and MaxMS bound the length of a proposal.
	MinMS int `json:"min_ms,omitempty"`
	MaxMS int `json:"max_ms,omitempty"`
	// PauseMS is the silence between two words that ends a sentence even without punctuation.
	PauseMS int `json:"pause_ms,omitempty"`
	// MinFaceShare is the least share of a range, in thousandths, in which a face is on screen.
	MinFaceShare int `json:"min_face_share,omitempty"`
}

// DefaultOptions are the defaults of SDD-024 section 8.
func DefaultOptions() Options {
	return Options{Count: 5, MinMS: 20_000, MaxMS: 60_000, PauseMS: 700, MinFaceShare: 700}
}

// WithDefaults fills every member that is zero.
func (o Options) WithDefaults() Options {
	d := DefaultOptions()

	if o.Count == 0 {
		o.Count = d.Count
	}

	if o.MinMS == 0 {
		o.MinMS = d.MinMS
	}

	if o.MaxMS == 0 {
		o.MaxMS = d.MaxMS
	}

	if o.PauseMS == 0 {
		o.PauseMS = d.PauseMS
	}

	if o.MinFaceShare == 0 {
		o.MinFaceShare = d.MinFaceShare
	}

	return o
}

// Validate checks the ranges of options that already have their defaults.
func (o Options) Validate() error {
	switch {
	case o.Count < 1 || o.Count > 20:
		return fmt.Errorf("count %d is outside 1 to 20", o.Count)
	case o.MinMS < 3_000 || o.MinMS > o.MaxMS || o.MaxMS > 180_000:
		return fmt.Errorf("min_ms %d and max_ms %d must satisfy 3000 <= min <= max <= 180000", o.MinMS, o.MaxMS)
	case o.PauseMS < 100 || o.PauseMS > 5_000:
		return fmt.Errorf("pause_ms %d is outside 100 to 5000", o.PauseMS)
	case o.MinFaceShare < 0 || o.MinFaceShare > 1000:
		return fmt.Errorf("min_face_share %d is outside 0 to 1000", o.MinFaceShare)
	}

	return nil
}

// Proposal is one candidate cut.
type Proposal struct {
	// N is the rank, from 1.
	N       int `json:"n"`
	StartMS int `json:"start_ms"`
	EndMS   int `json:"end_ms"`
	Words   int `json:"words"`
	// FaceShare is the share of the range with a face on screen, in thousandths.
	FaceShare int `json:"face_share"`
	// Score is the words per second of the range, times 1000, plus the face share; higher is better.
	Score int `json:"score"`
	// Title is the first words of the range; it is text of the video, untrusted.
	Title string `json:"title"`
}

type sentence struct{ first, last int }

func endsSentence(w string) bool {
	w = strings.TrimRightFunc(w, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(`"')]}`, r) })

	return strings.HasSuffix(w, ".") || strings.HasSuffix(w, "!") || strings.HasSuffix(w, "?") ||
		strings.HasSuffix(w, "…")
}

func sentences(words []Word, pause int) []sentence {
	var out []sentence

	first := 0

	for i := range words {
		last := i == len(words)-1
		if last || endsSentence(words[i].Text) || words[i+1].StartMS-words[i].EndMS >= pause {
			out = append(out, sentence{first, i})
			first = i + 1
		}
	}

	return out
}

// faceShare is the share, in thousandths, of the samples of [start, end) that show a face.
func faceShare(t *facetrack.Track, start, end int) int {
	if t == nil {
		return 0
	}

	i := sort.Search(len(t.Samples), func(i int) bool { return t.Samples[i].T >= start })
	seen, total := 0, 0

	for ; i < len(t.Samples) && t.Samples[i].T < end; i++ {
		total++

		if len(t.Samples[i].Faces) > 0 {
			seen++
		}
	}

	if total == 0 {
		return 0
	}

	return seen * 1000 / total
}

func title(words []Word) string {
	parts := make([]string, 0, 8)

	for _, w := range words[:min(len(words), 8)] {
		parts = append(parts, strings.TrimSpace(w.Text))
	}

	return strings.Join(parts, " ")
}

// Propose returns up to o.Count non-overlapping proposals, best first. o must have its defaults and be valid.
// The words must be in order; a track with no samples in a range gives that range no face.
func Propose(words []Word, t *facetrack.Track, o Options) []Proposal {
	sents := sentences(words, o.PauseMS)

	var cands []Proposal

	for i := range sents {
		start := words[sents[i].first].StartMS

		// The shortest run of sentences from here that is long enough; a longer one overlaps the next candidate.
		for j := i; j < len(sents); j++ {
			end := words[sents[j].last].EndMS
			dur := end - start

			if dur > o.MaxMS {
				break
			}

			if dur < o.MinMS {
				continue
			}

			share := faceShare(t, start, end)
			if share >= o.MinFaceShare {
				n := sents[j].last - sents[i].first + 1
				cands = append(cands, Proposal{
					StartMS: start, EndMS: end, Words: n, FaceShare: share,
					Score: n*1_000_000/dur + share, Title: title(words[sents[i].first : sents[i].last+1]),
				})
			}

			break
		}
	}

	sort.SliceStable(cands, func(a, b int) bool {
		if cands[a].Score != cands[b].Score {
			return cands[a].Score > cands[b].Score
		}

		return cands[a].StartMS < cands[b].StartMS
	})

	var out []Proposal

	for _, c := range cands {
		if len(out) >= o.Count {
			break
		}

		if overlaps(out, &c) {
			continue
		}

		c.N = len(out) + 1
		out = append(out, c)
	}

	return out
}

func overlaps(taken []Proposal, c *Proposal) bool {
	for i := range taken {
		if c.StartMS < taken[i].EndMS && taken[i].StartMS < c.EndMS {
			return true
		}
	}

	return false
}

func seconds(ms int) string { return fmt.Sprintf("%d.%03ds", ms/1000, ms%1000) }

// Ids the batch gives to the track and the clip it makes: fixed, so the same proposal gives the same batch.
const (
	TrackID = "trk_cq00000001"
	ClipID  = "itm_cq00000001"
)

// Batch is the list of commands that makes the cut on a new timeline of the project once the asset is in it: a
// visual track, the clip of the range and its face framing (SDD-024 section 8 item 4). The asset is not part of
// the batch, because `asset.add` is issued only by the import flow (SDD-002 section A.9): the person first places
// the asset in the new timeline under the same id through MediaImport by hash. The timeline is the vertical one a
// project item of kind timeline makes by default, so no reframe is needed. A person sends it as it is.
func Batch(p *Proposal, assetID string) []command.Operation {
	dur := p.EndMS - p.StartMS

	return []command.Operation{
		{Op: "track.add", Args: []byte(fmt.Sprintf(`{"kind":"visual","id":%q,"name":"Cut %d"}`, TrackID, p.N))},
		{Op: "clip.add", Args: []byte(fmt.Sprintf(
			`{"track_id":%q,"asset_id":%q,"start":"0s","source_in":%q,"duration":%q,"id":%q}`,
			TrackID, assetID, seconds(p.StartMS), seconds(dur), ClipID))},
		{Op: "clip.face_frame", Args: []byte(fmt.Sprintf(`{"item_id":%q}`, ClipID))},
	}
}
