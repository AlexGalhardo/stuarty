package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// PreviewVersion is the version written into every key of a processed preview: raise it when the output of a
// processor changes, so that no file rendered by the old code is played again (SDD-004 section 7.1).
const PreviewVersion = 1

// ProcessedAudio is what an audio entry of a plan window says about its processed sound (SDD-004 section 7.1): the
// key of the file, the timeline frame of the file's first sample and the file's length in frames.
type ProcessedAudio struct {
	Key    string `json:"key"`
	Start  int    `json:"start"`
	Frames int    `json:"frames"`
}

// NeedsPreview reports whether the timeline preview plays the item from a rendered file: the item has an enabled
// processor or a noise reduction (`denoise`). The chain of its track is not in the file.
func NeedsPreview(it *timeline.Item) bool {
	return len(processorSpecs(it.Processors)) > 0 || round3(it.Denoise) > 0
}

// processedOf is the ProcessedAudio of an item that sounds, or nil when it has nothing to process.
func processedOf(doc *timeline.Document, it *timeline.Item) *ProcessedAudio {
	if !NeedsPreview(it) || it.Duration <= 0 {
		return nil
	}

	src, ok := PreviewSource(doc, it.ID, it.Start, it.Start+it.Duration)
	if !ok {
		return nil
	}

	return &ProcessedAudio{Key: PreviewKey(doc, &src), Start: it.Start, Frames: it.Duration}
}

// PreviewSource is the source that the file of an item holds over the timeline frames [from, to) clipped to the item:
// the decoded range with its speed, reverse, noise reduction and item chain, and nothing of the envelope (volume,
// fades and the volume of the track), of the track and of the position, which is 0. It is pure.
func PreviewSource(doc *timeline.Document, itemID string, from, to int) (AudioSource, bool) {
	it, ok := doc.Items[itemID]
	if !ok || (it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio) {
		return AudioSource{}, false
	}

	cs, ce := max(from, it.Start), min(to, it.Start+it.Duration)
	if cs >= ce {
		return AudioSource{}, false
	}

	src := audioSource(doc, &it, cs, ce)
	src.StartSample, src.Gain, src.Voice = 0, nil, ""
	src.Track, src.BusChain = "", nil

	return src, true
}

// previewKeyed is the canonical form that PreviewKey hashes: the fields of a source that change the bytes of its file.
type previewKeyed struct {
	Version     int               `json:"v"`
	Asset       string            `json:"asset"`
	SourceStart int64             `json:"source_start"`
	Samples     int64             `json:"samples"`
	Speed       timeline.Rational `json:"speed"`
	Curve       *SpeedRamp        `json:"speed_curve,omitempty"`
	Reverse     bool              `json:"reverse,omitempty"`
	Denoise     float64           `json:"denoise,omitempty"`
	Chain       []ProcessorSpec   `json:"chain,omitempty"`
}

// PreviewKey is the key of the file of a source made by PreviewSource: the sha256, in lower-case hex, of its canonical
// JSON with the content hash of its asset. The same input gives the same key (a map is written in key order).
func PreviewKey(doc *timeline.Document, src *AudioSource) string {
	b, err := json.Marshal(previewKeyed{
		Version: PreviewVersion, Asset: doc.Assets[src.AssetID].Hash, SourceStart: src.SourceStart,
		Samples: src.Samples, Speed: src.Speed, Curve: src.Curve, Reverse: src.Reverse, Denoise: src.Denoise,
		Chain: src.Chain,
	})
	if err != nil {
		// The input is numbers and strings: it cannot fail.
		panic("plan: the preview key cannot be written: " + err.Error())
	}

	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

// PreviewSubject is a document that holds only what rendering the preview of one item needs (its settings, its asset,
// its track with no volume, solo or mute, and the item), so that the service keeps no whole project to render later.
func PreviewSubject(doc *timeline.Document, itemID string) *timeline.Document {
	it := doc.Items[itemID]

	sub := &timeline.Document{
		SchemaVersion: doc.SchemaVersion, ID: doc.ID, Name: doc.Name, Settings: doc.Settings,
		Assets: map[string]timeline.Asset{},
		Tracks: map[string]timeline.Track{},
		Items:  map[string]timeline.Item{it.ID: it},
	}

	if a, ok := doc.Assets[it.Asset]; ok {
		sub.Assets[a.ID] = a
	}

	if t, ok := doc.Tracks[it.Track]; ok {
		t.Muted, t.Solo, t.Volume, t.Processors = false, false, nil, nil
		sub.Tracks[t.ID] = t
	}

	return sub
}
