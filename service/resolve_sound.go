package service

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

const (
	waitForAnalysis = "wait until the asset is ready (media_inspect shows its state); an asset without audio has none"
)

// resolveSpeech fills resolved.speech of audio.duck: one entry for each speech item, with the
// speech ranges of its asset in asset frames (SDD-002 A.9, C.51).
func (s *svc) resolveSpeech(
	ctx context.Context,
	tx Tx,
	doc *timeline.Document,
	op *command.Operation,
	index int,
) (json.RawMessage, error) {
	if _, ok := unresolvedArgs(op); !ok {
		return nil, nil //nolint:nilnil // nothing to resolve
	}

	var ids struct {
		SpeechItemIDs []string `json:"speech_item_ids"`
	}

	if json.Unmarshal(op.Args, &ids) != nil || len(ids.SpeechItemIDs) == 0 {
		return nil, nil //nolint:nilnil // the command answers E_VALIDATION
	}

	speech := make([]command.SpeechRanges, 0, len(ids.SpeechItemIDs))

	for _, id := range ids.SpeechItemIDs {
		asset, found := clipAsset(doc, id)
		if !found {
			return nil, nil //nolint:nilnil // the command answers E_NOT_FOUND
		}

		arts, err := tx.Assets().Artifacts(ctx, asset.Hash)
		if err != nil {
			return nil, asError(err)
		}

		raw := s.readArtifact(arts, "speech", "")
		if raw == nil {
			return nil, notAnalysed(index, asset.ID, "the speech", waitForAnalysis)
		}

		ranges, ok := speechFrames(raw, doc.Settings.FPS)
		if !ok {
			return nil, notAnalysed(index, asset.ID, "the speech", waitForAnalysis)
		}

		speech = append(speech, command.SpeechRanges{ItemID: id, Ranges: ranges})
	}

	out, merr := json.Marshal(command.ResolvedSpeech{Speech: speech})
	if merr != nil {
		return nil, internalError()
	}

	return out, nil
}

// speechFrames turns the sample ranges of the speech artifact into asset frames of the project
// rate, rounding outward: a frame that holds any speech is inside, so ducking never starts late
// or ends early (SDD-011 section 9.3).
func speechFrames(raw []byte, fps timeline.Rational) ([]command.SilenceRange, bool) {
	var doc struct {
		SampleRate int `json:"sample_rate"`
		Ranges     []struct {
			Start int64 `json:"start"`
			End   int64 `json:"end"`
		} `json:"ranges"`
	}

	if json.Unmarshal(raw, &doc) != nil || doc.SampleRate < 1 || fps.Num < 1 || fps.Den < 1 {
		return nil, false
	}

	rate := int64(doc.SampleRate)
	out := []command.SilenceRange{}

	for _, r := range doc.Ranges {
		start := r.Start * int64(fps.Num) / (rate * int64(fps.Den))
		end := ceilDiv(r.End*int64(fps.Num), rate*int64(fps.Den))

		if end <= start {
			continue
		}

		if n := len(out); n > 0 && int(start) <= out[n-1].End {
			out[n-1].End = max(out[n-1].End, int(end))

			continue
		}

		out = append(out, command.SilenceRange{Start: int(start), End: int(end)})
	}

	return out, true
}

// resolveBeats fills resolved.beats of audio.mark_beats from the beat grid of the asset of the
// item (SDD-002 A.9, C.52).
func (s *svc) resolveBeats(
	ctx context.Context,
	tx Tx,
	doc *timeline.Document,
	op *command.Operation,
	index int,
) (json.RawMessage, error) {
	a, ok := unresolvedArgs(op)
	if !ok {
		return nil, nil //nolint:nilnil // nothing to resolve
	}

	asset, ok := clipAsset(doc, a.ItemID)
	if !ok {
		return nil, nil //nolint:nilnil // the command answers E_NOT_FOUND
	}

	arts, err := tx.Assets().Artifacts(ctx, asset.Hash)
	if err != nil {
		return nil, asError(err)
	}

	raw := s.readArtifact(arts, "beats", "")
	if raw == nil {
		return nil, notAnalysed(index, asset.ID, "the beat grid", waitForAnalysis)
	}

	beats, ok := beatFrames(raw, doc.Settings.FPS)
	if !ok {
		return nil, notAnalysed(index, asset.ID, "the beat grid", waitForAnalysis)
	}

	out, merr := json.Marshal(command.ResolvedBeats{Beats: beats})
	if merr != nil {
		return nil, internalError()
	}

	return out, nil
}

// beatFrames places each beat of the grid (samples) on the nearest frame of the project rate,
// keeps the strictly increasing ones (two beats on one frame count once) and stops at the limit
// of the command.
func beatFrames(raw []byte, fps timeline.Rational) ([]int, bool) {
	var doc struct {
		SampleRate int     `json:"sample_rate"`
		Beats      []int64 `json:"beats"`
	}

	if json.Unmarshal(raw, &doc) != nil || doc.SampleRate < 1 || fps.Num < 1 || fps.Den < 1 {
		return nil, false
	}

	per := int64(doc.SampleRate) * int64(fps.Den)
	out := make([]int, 0, len(doc.Beats))

	for _, b := range doc.Beats {
		if b < 0 {
			continue
		}

		f := int((2*b*int64(fps.Num) + per) / (2 * per))
		if n := len(out); n > 0 && f <= out[n-1] {
			continue
		}

		if len(out) >= timeline.MaxFrames {
			break
		}

		out = append(out, f)
	}

	return out, true
}
