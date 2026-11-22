package service

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/stuarty/engine/ingest"
)

// reasonProfileUpgrade is the reason of an ingest job that makes a missing artifact of a ready asset, not the
// asset itself (SDD-011 section 6.3).
const reasonProfileUpgrade = "profile_upgrade"

// jobInput is the part of the input of an ingest job that tells an import from a profile upgrade.
type jobInput struct {
	Reason string   `json:"reason"`
	Steps  []string `json:"steps"`
}

func inputOf(j *JobRow) jobInput {
	var in jobInput
	if len(j.Input) > 0 {
		_ = json.Unmarshal(j.Input, &in)
	}

	return in
}

// isProfileUpgrade is true for an ingest job that rebuilds an artifact of an asset that stays ready.
func isProfileUpgrade(j *JobRow) bool {
	return j.Kind == JobIngest && inputOf(j).Reason == reasonProfileUpgrade
}

// ingestsAsset is true for a job whose life is the life of the asset: its progress, failure, cancellation and
// requeue are the state of the asset (SDD-011 section 2.6). A profile upgrade never touches the asset.
func ingestsAsset(j *JobRow) bool {
	return j.Kind == JobIngest && j.AssetHash != nil && !isProfileUpgrade(j)
}

// peaksUpgradePending queues the peaks job of a long sound, or finds the one that is running, and returns what
// the read of the peaks answers meanwhile: E_ASSET_NOT_READY, retryable, with the job to follow. A file whose
// job was cancelled or failed in this process gets the limit error instead (SDD-011 section 6.3.1).
func (s *svc) peaksUpgradePending(ctx context.Context, ar AssetRow, size int64) error {
	if _, gaveUp := s.peaksGaveUp.Load(ar.Hash); gaveUp {
		return newError("E_LIMIT_EXCEEDED", "the audio is too long to compute its peaks in a request.").
			withHint("the file still plays and can be used; it has no waveform").
			withLimit("peaks_bytes", size, maxPeaksWAV)
	}

	actor := "system"
	if c, ok := CallerFrom(ctx); ok {
		actor = c.Actor
	}

	var id string

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		var jerr error

		id, jerr = s.ensureIngestJobFor(ctx, tx, ar.Hash, actor, s.nowMs(), map[string]any{
			"asset_hash": ar.Hash, "reason": reasonProfileUpgrade, "steps": []string{"peaks"},
		})

		return jerr
	})
	if err != nil {
		return asError(err)
	}

	e := newError("E_ASSET_NOT_READY", "the waveform of the asset is being computed in the background.").
		withHint("ask again in a few seconds; the job %s shows the progress", id).
		withDetail("reason", "peaks_pending").withDetail("job_id", id).withDetail("progress", 0.0)

	if j, gerr := s.store.Jobs().Get(ctx, id); gerr == nil {
		e.Details["progress"] = j.Progress
	}

	return e
}

// runPeaksUpgrade is the job of 6.3.1: the peaks of a long sound, read from its normalized WAV in blocks with
// progress, and registered. It never changes the state of the asset. A run that finds the artifact already
// registered (a restart after the commit) has nothing to do.
func (s *svc) runPeaksUpgrade(ctx context.Context, job *JobRow, pw *progress) (json.RawMessage, error) {
	if job.AssetHash == nil {
		return nil, internalError()
	}

	hash := *job.AssetHash

	arts, err := s.store.Assets().Artifacts(ctx, hash)
	if err != nil {
		return nil, err
	}

	if hasPeaks(arts) {
		return marshalOrNull(map[string]any{"steps": []string{"peaks"}, "skipped": true}), nil
	}

	i := slices.IndexFunc(arts, func(a ArtifactRow) bool { return a.Kind == "normalized" && a.Ext == "wav" })
	if i < 0 {
		return nil, newError(CodeIngestFailed, "the asset has no normalized audio to read its peaks from.").
			withDetail("reason", "decode_failed").withDetail("step", "peaks")
	}

	src, ok := s.artifactPath(arts[i].RelPath)
	if !ok {
		return nil, internalError()
	}

	if derr := s.checkDisk(); derr != nil {
		return nil, derr
	}

	pk, err := ingest.PeaksOfFileProgress(ctx, src, func(done, total int64) {
		if total > 0 {
			pw.report(ctx, float64(done)/float64(total), "peaks")
		}
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return nil, newError(CodeIngestFailed, "the peaks of the audio could not be computed.").
			withDetail("reason", "decode_failed").withDetail("step", "peaks")
	}

	// A header that declares no audio gives an empty result, which must not be registered as the waveform.
	if cerr := pk.Check(pk.Min, pk.Max); cerr != nil {
		return nil, newError(CodeIngestFailed, "the normalized audio holds no samples to read its peaks from.").
			withDetail("reason", "decode_failed").withDetail("step", "peaks")
	}

	if _, err := s.storePeaks(ctx, hash, pk); err != nil {
		return nil, err
	}

	return marshalOrNull(map[string]any{"steps": []string{"peaks"}}), nil
}

// noteUpgradeEnded remembers that a profile upgrade of a file ended cancelled or failed, so that a read does
// not queue it again in this process (SDD-011 section 6.3.1).
func (s *svc) noteUpgradeEnded(job *JobRow, state string) {
	if job.AssetHash != nil && state != JobSucceeded && isProfileUpgrade(job) {
		s.peaksGaveUp.Store(*job.AssetHash, struct{}{})
	}
}
