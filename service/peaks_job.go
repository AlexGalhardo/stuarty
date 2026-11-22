package service

import (
	"context"
	"encoding/json"
	"errors"
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

// maxActiveProfileUpgrades is how many profile upgrades may be queued or running in a workspace. Reads queue them,
// so a page of long sounds must not take the ingest queue from the imports (SDD-011 section 6.3.1).
const maxActiveProfileUpgrades = 1

// peaksPending is the answer of a read while the peaks are computed. id is empty for a file that waits for its
// turn: no job exists for it yet, and a later read queues it.
func peaksPending(id string, progress float64) *Error {
	hint := "ask again in a few seconds"
	if id != "" {
		hint += "; the job " + id + " shows the progress"
	}

	e := newError("E_ASSET_NOT_READY", "the waveform of the asset is being computed in the background.").
		withHint("%s", hint).withDetail("reason", "peaks_pending").withDetail("progress", progress)
	if id != "" {
		_ = e.withDetail("job_id", id)
	}

	return e
}

// peaksUpgradePending queues the peaks job of a long sound, or finds the one that is running, and returns what
// the read of the peaks answers meanwhile: E_ASSET_NOT_READY, retryable, with the job to follow. A read that finds
// the job takes no write transaction, because a page asks again every few seconds. A caller whose cancel stopped
// the job, and anyone after it failed, gets the limit error instead (SDD-011 section 6.3.1).
func (s *svc) peaksUpgradePending(ctx context.Context, ar AssetRow, size int64) error {
	actor, owner := "system", ""
	if c, ok := CallerFrom(ctx); ok {
		actor, owner = c.Actor, c.OwnerUserID
	}

	if s.peaksGivenUp(ar.Hash, owner) {
		return newError("E_LIMIT_EXCEEDED", "the audio is too long to compute its peaks in a request.").
			withHint("the file still plays and can be used; it has no waveform").
			withLimit("peaks_bytes", size, maxPeaksWAV)
	}

	if j, jerr := s.store.Jobs().ActiveIngestForHash(ctx, ar.Hash); jerr == nil {
		return peaksPending(j.ID, j.Progress)
	}

	var id string

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		var jerr error

		id, jerr = s.ensureIngestJobFor(ctx, tx, ar.Hash, actor, s.nowMs(), map[string]any{
			"asset_hash": ar.Hash, "reason": reasonProfileUpgrade, "steps": []string{"peaks"},
		})

		return jerr
	})
	if full, isFull := errors.AsType[*QueueFullError](err); isFull && full.PerReason {
		return peaksPending("", 0)
	}

	if err != nil {
		return asError(err)
	}

	return peaksPending(id, 0)
}

// gaveUpEveryone is the owner of the record of a job that failed: nobody gets it queued again.
const gaveUpEveryone = "\x00"

func gaveUpKey(hash, owner string) string { return hash + "\x00" + owner }

func (s *svc) peaksGivenUp(hash, owner string) bool {
	if _, ok := s.peaksGaveUp.Load(gaveUpKey(hash, gaveUpEveryone)); ok {
		return true
	}

	_, ok := s.peaksGaveUp.Load(gaveUpKey(hash, owner))

	return ok
}

// notePeaksCancelled remembers who stopped the peaks job of a file, so that their open page does not queue it
// again. The record is theirs alone: another user of the same file still gets its waveform.
func (s *svc) notePeaksCancelled(job *JobRow, owner string) {
	if job.AssetHash != nil && isProfileUpgrade(job) {
		s.peaksGaveUp.Store(gaveUpKey(*job.AssetHash, owner), struct{}{})
	}
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

// noteUpgradeEnded remembers that a profile upgrade of a file failed, so that no read queues it again in this
// process (SDD-011 section 6.3.1). A cancel is recorded for the one who made it, by notePeaksCancelled.
func (s *svc) noteUpgradeEnded(job *JobRow, state string) {
	if job.AssetHash != nil && state == JobFailed && isProfileUpgrade(job) {
		s.peaksGaveUp.Store(gaveUpKey(*job.AssetHash, gaveUpEveryone), struct{}{})
	}
}
