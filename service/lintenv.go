package service

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
)

// maxPeaksFile bounds the peaks artifact read for a lint: an hour of audio is
// about 7.2 MB (SDD-011 section 4.6).
const maxPeaksFile = 64 << 20

// lintEnvFor is the lint environment for a Full lint of doc: the owner's
// Measurer and the rest of Options.Lint, plus the stored analysis artifacts of
// the assets of the document when the owner supplied no reader of its own
// (SDD-003 section 3.6). The Fast lint of an edit reads no artifact.
func (s *svc) lintEnvFor(ctx context.Context, doc *timeline.Document) lint.Env {
	env := s.lintEnv

	if len(timeline.CompositionTimelines(doc)) > 0 {
		nested, _ := s.nestedFor(ctx, s.store, doc)
		env.Nested = nestedLookup(nested)
	}

	if env.Artifacts != nil || s.workspace == "" {
		return env
	}

	r := &artifactReader{s: s, fps: doc.Settings.FPS, rows: make(map[string][]ArtifactRow, len(doc.Assets))}

	for id := range doc.Assets {
		if rows, err := s.store.Assets().Artifacts(ctx, doc.Assets[id].Hash); err == nil {
			r.rows[id] = rows
		}
	}

	env.Artifacts = r

	if env.AlphaOpaque == nil {
		env.AlphaOpaque = r.alphaOpaque
	}

	return env
}

// artifactReader is lint.ArtifactReader over the artifacts registered for the
// assets of one document, read from the workspace when a rule asks.
type artifactReader struct {
	s    *svc
	fps  timeline.Rational
	rows map[string][]ArtifactRow // by asset id
}

var _ lint.ArtifactReader = (*artifactReader)(nil)

// alphaOpaque is the alpha_opaque fact of the normalized artifact of an asset (SDD-011 section 4.3.6): the ingest
// measured that a file which declares an alpha channel has only opaque pictures. known is false when the asset has
// no normalized artifact or the artifact predates the fact, which reads as "not opaque", never as a finding.
func (r *artifactReader) alphaOpaque(asset string) (opaque, known bool) {
	i := slices.IndexFunc(r.rows[asset], func(a ArtifactRow) bool { return a.Kind == "normalized" && len(a.Meta) > 0 })
	if i < 0 {
		return false, false
	}

	var facts struct {
		HasAlpha    bool `json:"has_alpha"`
		AlphaOpaque bool `json:"alpha_opaque"`
	}

	if json.Unmarshal(r.rows[asset][i].Meta, &facts) != nil || !facts.HasAlpha {
		return false, false
	}

	return facts.AlphaOpaque, true
}

func (r *artifactReader) Peaks(asset string) (lint.Peaks, bool) {
	i := slices.IndexFunc(r.rows[asset], func(a ArtifactRow) bool { return a.Kind == "peaks" && a.Ext == "bin" })
	if i < 0 {
		return nil, false
	}

	p, ok := r.s.artifactPath(r.rows[asset][i].RelPath)
	if !ok {
		return nil, false
	}

	if st, err := os.Stat(p); err != nil || st.Size() > maxPeaksFile {
		return nil, false
	}

	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}

	pk, err := ingest.ParsePeaks(raw)
	if err != nil || pk.SampleRate < 1 {
		return nil, false
	}

	group := ingest.PeakLevels[0]
	perWindow := max(pk.SampleRate*lint.Window/1000, 1)
	out := make(lint.Peaks, (pk.SampleCount+int64(perWindow)-1)/int64(perWindow))

	for i, pr := range pk.Levels[0] {
		w := min(i*group/perWindow, len(out)-1)
		level := math.Max(math.Abs(float64(pr.Min)), math.Abs(float64(pr.Max))) / 32768
		out[w] = math.Max(out[w], math.Min(level, 1))
	}

	return out, len(out) > 0
}

func (r *artifactReader) Loudness(asset string) (lint.Loudness, bool) {
	var l struct {
		IntegratedLUFS *float64 `json:"integrated_lufs"`
	}

	raw := r.s.readArtifact(r.rows[asset], "loudness", "")
	if raw == nil || json.Unmarshal(raw, &l) != nil || l.IntegratedLUFS == nil {
		return lint.Loudness{}, false
	}

	return lint.Loudness{LUFS: *l.IntegratedLUFS}, true
}

func (r *artifactReader) Transcript(asset string) ([]lint.TranscriptWord, bool) {
	raw := r.s.newestTranscript(r.rows[asset])
	if raw == nil {
		return nil, false
	}

	words, ok := transcriptFrames(raw, r.fps)
	if !ok {
		return nil, false
	}

	out := make([]lint.TranscriptWord, len(words))
	for i, w := range words {
		out[i] = lint.TranscriptWord{Text: w.Text, Start: w.Start, End: w.End}
	}

	return out, true
}
