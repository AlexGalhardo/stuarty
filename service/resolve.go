package service

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// resolveOps fills the `resolved` argument of the commands whose data comes
// from media analysis (SDD-002 section A.9): the silences of
// clip.remove_silences and the words of caption.from_transcript and
// caption.retime_from_clip, the speech of audio.duck and the beats of
// audio.mark_beats. It reads artifacts and starts nothing: when the
// artifact is missing the batch fails with E_INVALID_STATE and a hint that
// names the job to run. The stored operation carries `resolved`, so replaying
// the log gives the same document. A caller may supply `resolved` itself, as
// the golden tests do; it is left alone.
func (s *svc) resolveOps(
	ctx context.Context,
	tx Tx,
	doc *timeline.Document,
	ops []command.Operation,
) ([]command.Operation, error) {
	var out []command.Operation

	var fonts fontScope

	for i := range ops {
		var (
			resolved json.RawMessage
			rerr     error
		)

		switch ops[i].Op {
		case "clip.remove_silences":
			resolved, rerr = s.resolveSilences(ctx, tx, doc, &ops[i], i)
		case "caption.from_transcript", "caption.retime_from_clip":
			resolved, rerr = s.resolveWords(ctx, tx, doc, &ops[i], i)
		case "audio.duck":
			resolved, rerr = s.resolveSpeech(ctx, tx, doc, &ops[i], i)
		case "audio.mark_beats":
			resolved, rerr = s.resolveBeats(ctx, tx, doc, &ops[i], i)
		case "font.add":
			// Not analysis: the core cannot tell whether the font store holds the file.
			if e := s.checkFontAdd(ctx, tx, &fonts, &ops[i], i); e != nil {
				return nil, e
			}
		}

		if rerr != nil {
			return nil, rerr
		}

		if resolved == nil {
			continue
		}

		if out == nil {
			out = slices.Clone(ops)
		}

		args := map[string]json.RawMessage{}
		if err := json.Unmarshal(ops[i].Args, &args); err != nil {
			continue // the command reports the malformed arguments itself
		}

		args["resolved"] = resolved

		raw, err := json.Marshal(args)
		if err != nil {
			return nil, internalError()
		}

		out[i].Args = raw
	}

	if out == nil {
		return ops, nil
	}

	return out, nil
}

// opArgs reads the members the resolver needs; a malformed or incomplete
// operation is left to the command, which answers with its own error.
type opArgs struct {
	ItemID      string          `json:"item_id"`
	ClipItemID  string          `json:"clip_item_id"`
	ThresholdDB *int            `json:"threshold_db"`
	Resolved    json.RawMessage `json:"resolved"`
}

// unresolvedArgs reads the arguments of an operation that still needs its
// `resolved` member; ok is false when the arguments are malformed (the command
// reports that itself) or the caller already supplied it.
func unresolvedArgs(op *command.Operation) (a opArgs, ok bool) {
	if err := json.Unmarshal(op.Args, &a); err != nil {
		return opArgs{}, false
	}

	return a, len(a.Resolved) == 0
}

func clipAsset(doc *timeline.Document, itemID string) (timeline.Asset, bool) {
	it, ok := doc.Items[itemID]
	if !ok {
		return timeline.Asset{}, false
	}

	a, ok := doc.Assets[it.Asset]

	return a, ok
}

func (s *svc) resolveSilences(
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

	raw := s.readArtifact(arts, "silence", "")
	if raw == nil {
		return nil, notAnalysed(index, asset.ID, "the silences",
			"wait until the asset is ready (media_inspect shows its state); an asset without audio has none")
	}

	threshold := -35
	if a.ThresholdDB != nil {
		threshold = *a.ThresholdDB
	}

	ranges, ok := silenceFrames(raw, threshold, doc.Settings.FPS)
	if !ok {
		return nil, notAnalysed(index, asset.ID, "the silences",
			"wait until the asset is ready (media_inspect shows its state); an asset without audio has none")
	}

	out, merr := json.Marshal(command.ResolvedSilences{Silences: ranges})
	if merr != nil {
		return nil, internalError()
	}

	return out, nil
}

// silenceFrames turns the sample ranges of the silence artifact at one
// threshold into frames of the project rate, half-open: the start rounds up
// and the end rounds down, so a frame that is not silent throughout is not cut.
func silenceFrames(raw []byte, threshold int, fps timeline.Rational) ([]command.SilenceRange, bool) {
	var doc struct {
		SampleRate int `json:"sample_rate"`
		Sets       []struct {
			ThresholdDB int `json:"threshold_db"`
			Ranges      []struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"ranges"`
		} `json:"sets"`
	}

	if json.Unmarshal(raw, &doc) != nil || doc.SampleRate < 1 || fps.Num < 1 || fps.Den < 1 {
		return nil, false
	}

	rate := int64(doc.SampleRate)
	out := []command.SilenceRange{}
	found := false

	for _, set := range doc.Sets {
		if set.ThresholdDB != threshold {
			continue
		}

		found = true

		for _, r := range set.Ranges {
			start := ceilDiv(r.Start*int64(fps.Num), rate*int64(fps.Den))
			end := r.End * int64(fps.Num) / (rate * int64(fps.Den))

			if end > start && (len(out) == 0 || int(start) >= out[len(out)-1].End) {
				out = append(out, command.SilenceRange{Start: int(start), End: int(end)})
			}
		}
	}

	return out, found
}

func (s *svc) resolveWords(
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

	asset, ok := clipAsset(doc, a.ClipItemID)
	if !ok {
		return nil, nil //nolint:nilnil // the command answers E_NOT_FOUND
	}

	arts, err := tx.Assets().Artifacts(ctx, asset.Hash)
	if err != nil {
		return nil, asError(err)
	}

	raw := s.newestTranscript(arts)
	if raw == nil {
		return nil, notAnalysed(
			index,
			asset.ID,
			"a transcript",
			"start the transcription with job_start (kind \"transcribe\") on this asset, wait for it to succeed, then repeat the batch",
		)
	}

	words, ok := transcriptFrames(raw, doc.Settings.FPS)
	if !ok {
		return nil, notAnalysed(index, asset.ID, "a usable transcript",
			"start the transcription again with job_start (kind \"transcribe\") on this asset")
	}

	out, merr := json.Marshal(command.ResolvedWords{Words: words})
	if merr != nil {
		return nil, internalError()
	}

	return out, nil
}

// newestTranscript reads the most recent transcript artifact of an asset,
// whatever provider and model made it.
func (s *svc) newestTranscript(arts []ArtifactRow) []byte {
	var newest *ArtifactRow

	for i := range arts {
		if arts[i].Kind == "transcript" && arts[i].Ext == "json" &&
			(newest == nil || arts[i].CreatedMs > newest.CreatedMs) {
			newest = &arts[i]
		}
	}

	if newest == nil {
		return nil
	}

	return s.readArtifact([]ArtifactRow{*newest}, "transcript", newest.Variant)
}

// transcriptFrames maps the milliseconds of the transcript artifact (SDD-011
// section 10.5) to frames of the project rate with the rule of section 10.6:
// the boundary nearest to the time, no overlap, at least one frame per word.
func transcriptFrames(raw []byte, fps timeline.Rational) ([]command.ResolvedWord, bool) {
	var doc struct {
		Words []struct {
			Text    string `json:"text"`
			StartMS int64  `json:"start_ms"`
			EndMS   int64  `json:"end_ms"`
		} `json:"words"`
	}

	if json.Unmarshal(raw, &doc) != nil || fps.Num < 1 || fps.Den < 1 || len(doc.Words) == 0 {
		return nil, false
	}

	num, den := int64(fps.Num), int64(fps.Den)
	boundary := func(t int64) int64 { return (2*t*num + 1000*den) / (2000 * den) }

	out := make([]command.ResolvedWord, 0, len(doc.Words))

	var cursor int64

	for _, w := range doc.Words {
		start := max(boundary(w.StartMS), cursor)
		end := max(boundary(w.EndMS), start+1)
		cursor = end

		out = append(out, command.ResolvedWord{Text: w.Text, Start: int(start), End: int(end)})
	}

	return out, true
}

// notAnalysed is the refusal of an edit that needs analysis nobody ran: it
// names the call to make and never makes it (an edit starts no job).
func notAnalysed(index int, assetID, what, hint string) *Error {
	e := newError(command.CodeInvalidState, "the edit needs %s of asset %s, and there is none yet.", what, assetID).
		withHint("%s", hint).withPath("operations").withDetail("asset_id", assetID)
	e.OpIndex = &index

	return e
}
