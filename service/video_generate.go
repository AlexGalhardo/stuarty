package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// JobVideoGenerate is the job kind of a video generation (SDD-016 section 6.4).
const JobVideoGenerate = "video.generate"

// Limits of a video generation (SDD-016 section 6.4).
const (
	// videoMaxWait is how long a run waits for the provider to finish a clip before it gives up.
	videoMaxWait = 30 * time.Minute
	// videoCallTimeout bounds one request to the provider: a submit, a poll or a cancel.
	videoCallTimeout = 60 * time.Second
	// videoFetchTimeout bounds the download of the clip.
	videoFetchTimeout = 10 * time.Minute
	// maxGeneratedVideo and maxFirstFrame bound a clip a provider returns and an image sent to it; the import
	// limit of the workspace bounds the clip too.
	maxGeneratedVideo = 1 << 30
	maxFirstFrame     = 20 << 20
	// The backoff of the polling: the first wait, doubled up to the largest (a provider may say its own).
	videoPollFirst = 2 * time.Second
	videoPollMax   = 30 * time.Second
	// videoPollFloor and videoPollCeiling bound the wait a provider asks for: a provider that asks for no wait
	// is polled once a second at most, never in a tight loop for the 30 minutes of a run.
	videoPollFloor   = time.Second
	videoPollCeiling = 60 * time.Second
	// videoPollFailures is how many polls in a row may fail for a reason that may pass before the job fails.
	videoPollFailures = 5
)

// VideoArgs are the arguments of a video.generate job: the connection, and the request of SDD-016 section 5.4.
type VideoArgs struct {
	ConnectionID string `json:"connection_id"`
	provider.VideoRequest
}

// VideoGenerateRequest starts a video generation in a project: the arguments of the job kind video.generate
// (SDD-016 section 6.4) and the wait and the dry run of JobStart.
type VideoGenerateRequest struct {
	ProjectID    string `json:"project_id"`
	ConnectionID string `json:"connection_id"`
	provider.VideoRequest
	WaitMS int  `json:"wait_ms,omitempty"`
	DryRun bool `json:"dry_run,omitempty"`
}

// VideoGenerate starts a video.generate job in a project and returns it. The estimate is on the job from the
// start, and the job waits for a person unless a rule the person wrote covers it (SDD-015 section 5).
func (s *svc) VideoGenerate(ctx context.Context, req VideoGenerateRequest) (*Job, error) {
	args, err := json.Marshal(VideoArgs{ConnectionID: req.ConnectionID, VideoRequest: req.VideoRequest})
	if err != nil {
		return nil, newError(command.CodeInternal, "the request cannot be encoded.")
	}

	return s.JobStart(ctx, JobStartRequest{
		ProjectID: req.ProjectID, Kind: JobVideoGenerate, Args: args, WaitMS: req.WaitMS, DryRun: req.DryRun,
	})
}

func (s *svc) videoGenerateKind() ApprovableKind {
	return ApprovableKind{
		Name: JobVideoGenerate, Scope: ScopeEdit, ProjectWide: true, Prepare: s.prepareVideo, Run: s.runVideo,
		Abandon: s.abandonVideo,
	}
}

// abandonVideo is the cancel of a video.generate job queued again after a shutdown with the task of its first run
// in its input: the task goes on at the provider, so the provider is asked to stop it, and the clip is owed unless
// it confirms that nothing is charged (SDD-016 section 6.4).
func (s *svc) abandonVideo(ctx context.Context, job *JobRow) *core.Money {
	var in generateInput
	if json.Unmarshal(job.Input, &in) != nil || in.TaskID == "" || in.Video == nil {
		return nil
	}

	spent := moneyOf(provider.VideoCost(in.Video.Seconds, in.Price))

	gen, ok := s.providers.Video(in.Provider)
	if !ok {
		return &spent
	}

	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), videoCallTimeout)
	defer cancel()

	if s.cancelVideoTaskIn(bg, &in, gen) {
		return nil
	}

	return &spent
}

func (s *svc) prepareVideo(ctx context.Context, req JobStartRequest) (Prepared, error) {
	const members = "model, prompt and optionally seconds, aspect and first_frame"

	args, aerr := decodeGenerateArgs[VideoArgs](req.Args, "connection_id, "+members)
	if aerr != nil {
		return Prepared{}, aerr
	}

	conn, err := s.mediaConnection(ctx, args.ConnectionID, provider.CapabilityVideo, members)
	if err != nil {
		return Prepared{}, err
	}

	gen, _ := s.providers.Video(conn.Kind)
	r := provider.NormalizeVideo(args.VideoRequest)

	if ferr := provider.ValidateVideo(r, gen.VideoLimits(r.Model)); ferr != nil {
		return Prepared{}, newError(command.CodeValidation, "%s", ferr.Message).withPath("args." + ferr.Path)
	}

	if r.FirstFrame != "" {
		if ferr := s.checkFirstFrame(ctx, req.ProjectID, r.FirstFrame); ferr != nil {
			return Prepared{}, ferr
		}
	}

	price, source, perr := s.generationPrice(&conn, r.Model, provider.UnitSecond)
	if perr != nil {
		return Prepared{}, perr
	}

	cost := provider.EstimateVideo(r, price)
	estimate := core.Money{Micros: cost.Micros, Currency: cost.Currency}

	input, merr := json.Marshal(generateInput{
		ConnectionID: conn.ID, Provider: conn.Kind, Video: &r, Price: price, PriceSource: source, Estimate: estimate,
	})
	if merr != nil {
		return Prepared{}, merr
	}

	head := fmt.Sprintf("Generate a video of %d seconds, aspect %s, with %s, model %s%s.",
		r.Seconds, r.Aspect, gen.Descriptor().Name, r.Model, withFrame(r.FirstFrame))

	return Prepared{
		Summary: summaryWithExcerpt(head, r.Prompt), ConnectionID: conn.ID, Estimate: estimate, Input: input,
	}, nil
}

func withFrame(frame string) string {
	if frame == "" {
		return ""
	}

	return ", starting from the image " + frame
}

// firstFrameAsset finds the image asset of the project the first frame names, ready to be read.
func (s *svc) firstFrameAsset(ctx context.Context, projectID, assetID string) (AssetRow, *Error) {
	pa, err := s.store.Assets().GetProjectAsset(ctx, projectID, assetID)
	// A pending asset is one whose ingest has not yet been linked into the document; the ingest state below is
	// what says whether the file can be used.
	if err != nil || pa.State == ProjectAssetTrashed {
		return AssetRow{}, newError(command.CodeNotFound, "the project has no asset %s.", assetID).
			withPath("args.first_frame").
			withHint("list the assets with media_inspect and no asset_id")
	}

	ar, err := s.store.Assets().Get(ctx, pa.AssetHash)
	if err != nil {
		return AssetRow{}, asErr(err)
	}

	switch {
	case ar.IngestState != IngestReady:
		return AssetRow{}, newError("E_ASSET_NOT_READY", "the asset %s is not ready.", assetID).
			withPath("args.first_frame").withHint("wait for its ingest to end")
	case ar.Kind != "image":
		return AssetRow{}, newError(command.CodeTypeMismatch, "the asset %s is not an image.", assetID).
			withPath("args.first_frame").withHint("the first frame of a clip must be an image asset")
	case ar.SizeBytes > maxFirstFrame:
		// Said before a person approves, not after, when the run reads the file.
		return AssetRow{}, newError(command.CodeValidation,
			"the image %s is larger than %d MB, too large to be a first frame.", assetID, maxFirstFrame>>20).
			withPath("args.first_frame").withHint("pick a smaller image, or export a smaller copy of it")
	}

	return ar, nil
}

func (s *svc) checkFirstFrame(ctx context.Context, projectID, assetID string) error {
	if _, err := s.firstFrameAsset(ctx, projectID, assetID); err != nil {
		return err
	}

	return nil
}

// readFirstFrame reads the image a clip starts from out of the media store and checks it by its signature.
func (s *svc) readFirstFrame(ctx context.Context, projectID, assetID string) (*providers.VideoFrame, error) {
	ar, err := s.firstFrameAsset(ctx, projectID, assetID)
	if err != nil {
		return nil, err
	}

	if s.media.Content == nil {
		return nil, internalError()
	}

	f, oerr := s.media.Content.Open(strings.TrimPrefix(ar.Hash, "sha256:"))
	if oerr != nil {
		return nil, newError(command.CodeNotFound, "the file of the asset %s is gone.", assetID).
			withPath("args.first_frame")
	}

	defer f.Close()

	data, rerr := io.ReadAll(io.LimitReader(f, maxFirstFrame+1))
	if rerr != nil || len(data) > maxFirstFrame {
		return nil, newError(command.CodeValidation, "the image %s is too large to be a first frame.", assetID).
			withPath("args.first_frame")
	}

	format, ok := provider.CheckImage(data)
	if !ok {
		return nil, newError(command.CodeTypeMismatch, "the asset %s is not a PNG, a JPEG or a WebP.", assetID).
			withPath("args.first_frame")
	}

	return &providers.VideoFrame{Data: data, Format: format}, nil
}

// runVideo is the job once approved (SDD-016 section 6.4): it submits the generation once, saves the task id of
// the provider before it waits, polls until the clip is done, fetches it, and stores it as an asset with the
// origin generated. A run that is queued again finds the task id in its input and resumes at the polling.
func (s *svc) runVideo(ctx context.Context, run JobRun) (JobOutcome, error) {
	in, err := s.readGenerateInput(&run)
	if err != nil {
		return JobOutcome{}, err
	}

	gen, ok := s.providers.Video(in.Provider)
	if !ok || in.Video == nil {
		return JobOutcome{}, providerGone(in.Provider)
	}

	req, limit := *in.Video, s.generationLimit(maxGeneratedVideo)
	spent := moneyOf(provider.VideoCost(req.Seconds, in.Price))

	if in.TaskID == "" {
		if err := s.submitVideo(ctx, &run, gen, &in, limit); err != nil {
			return JobOutcome{}, err
		}
	} else {
		run.Report(ctx, 0.1, "resuming the generation the provider already started")
	}

	if err := s.waitVideo(ctx, &run, gen, &in); err != nil {
		return paidOutcome(err, &spent)
	}

	run.Report(ctx, 0.9, "fetching the clip")

	var file providers.GeneratedFile

	err = s.conns.WithTimeout(ctx, in.ConnectionID, videoFetchTimeout, func(c connections.Credential) error {
		var ferr error

		file, ferr = gen.Fetch(ctx, c, in.TaskID, limit)

		return ferr
	})
	if err != nil {
		// The provider finished the clip, so it billed it, whatever became of the download.
		return JobOutcome{Cost: &spent}, providerError(err)
	}

	out := &GeneratedResult{
		Provider: in.Provider, Model: req.Model, Requested: 1, Delivered: 1, Cost: spent, Items: []GeneratedItem{},
	}

	container, valid := provider.CheckVideo(file.Data)
	if !valid {
		out.Notes = append(out.Notes, "the file is not an MP4, a QuickTime or a WebM clip and was not kept")

		return s.finishGenerated(ctx, run.ProjectID, out, &spent)
	}

	name := fmt.Sprintf("video-%s-1.%s", shortJob(run.ID), container)

	item, serr := s.storeGenerated(ctx, &run, "video", name, file.Data, &asset.Origin{
		Kind: asset.Generated, Provider: cut(in.Provider, asset.MaxProvider), Model: cut(req.Model, asset.MaxModel),
		Prompt: cut(req.Prompt, asset.MaxPrompt), CostMicros: originCost(spentCost(&spent)), Source: "job " + run.ID,
	})
	if serr != nil {
		out.Notes = append(out.Notes, "the clip was not stored: "+serr.Message)

		return s.finishGenerated(ctx, run.ProjectID, out, &spent)
	}

	out.Items = append(out.Items, item)

	return s.finishGenerated(ctx, run.ProjectID, out, &spent)
}

// paidOutcome pairs the error of the wait with the cost that is owed for it: a task the provider reported as
// failed costs nothing, and the other ways the wait ends say in the error whether the provider may have charged.
func paidOutcome(err error, spent *core.Money) (JobOutcome, error) {
	if oe, ok := errors.AsType[*owedError](err); ok {
		if oe.owed {
			return JobOutcome{Cost: spent}, oe.err
		}

		return JobOutcome{}, oe.err
	}

	return JobOutcome{}, err
}

// owedError is an error of the wait that says whether the provider may have charged for the task.
type owedError struct {
	err  error
	owed bool
}

func (e *owedError) Error() string { return e.err.Error() }
func (e *owedError) Unwrap() error { return e.err }

// submitVideo is the one call that spends. The task id goes into the input of the job before anything else, so a
// crash after the charge leaves the id behind and the next run does not submit again.
func (s *svc) submitVideo(
	ctx context.Context, run *JobRun, gen providers.VideoGenerator, in *generateInput, limit int64,
) error {
	if lerr := s.checkFreeSpace(limit); lerr != nil {
		return lerr
	}

	var frame *providers.VideoFrame

	if in.Video.FirstFrame != "" {
		var ferr error

		if frame, ferr = s.readFirstFrame(ctx, run.ProjectID, in.Video.FirstFrame); ferr != nil {
			return ferr
		}
	}

	run.Report(ctx, 0.05, "asking the provider")

	var id string

	err := s.conns.WithTimeout(ctx, in.ConnectionID, videoCallTimeout, func(c connections.Credential) error {
		var serr error

		if id, serr = gen.Submit(ctx, c, *in.Video, frame); serr != nil {
			return serr
		}

		// A server that speaks the protocol may repeat the key it was sent (T-55).
		if !provider.ValidTaskID(id) || c.Echoes(id) {
			return &providers.Error{Kind: providers.KindUnreadable}
		}

		return nil
	})
	if err != nil {
		return providerError(err)
	}

	in.TaskID = id

	raw, merr := json.Marshal(in)
	if merr != nil {
		return merr
	}

	// Saved with a context a shutdown does not end, since the provider has the task already.
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()

	if serr := run.SaveInput(sctx, raw); serr != nil {
		s.cancelVideoTask(in, gen)

		return serr
	}

	return nil
}

// waitVideo polls until the task is done. A nil error is a clip ready to fetch.
func (s *svc) waitVideo(
	ctx context.Context, run *JobRun, gen providers.VideoGenerator, in *generateInput,
) error {
	deadline := time.Now().Add(videoMaxWait)
	wait, failures := videoPollFirst, 0

	for {
		var st providers.VideoStatus

		err := s.conns.WithTimeout(ctx, in.ConnectionID, videoCallTimeout, func(c connections.Credential) error {
			var perr error

			st, perr = gen.Poll(ctx, c, in.TaskID)

			return perr
		})

		if ctx.Err() != nil {
			return s.videoInterrupted(ctx, run, gen, in)
		}

		switch {
		case err != nil:
			failures++

			if !pollMayPass(err) || failures >= videoPollFailures {
				return &owedError{err: providerError(err), owed: true}
			}
		case st.State == providers.VideoDone:
			return nil
		case st.State == providers.VideoFailed:
			// The provider says the task ended with no clip; it is owed only when the provider does not say that
			// such a task is free.
			if st.Err == nil {
				st.Err = &providers.Error{Kind: providers.KindUnavailable}
			}

			return &owedError{err: providerError(st.Err), owed: st.Charged}
		case st.State == providers.VideoPending:
			failures = 0

			run.Report(ctx, 0.1+0.8*min(max(st.Progress, 0), 1), "the provider is making the clip")
		default:
			return &owedError{err: providerError(&providers.Error{Kind: providers.KindUnreadable}), owed: true}
		}

		if time.Now().After(deadline) {
			return &owedError{
				err:  providerError(&providers.Error{Kind: providers.KindTimeout}),
				owed: true,
			}
		}

		var next time.Duration

		next, wait = nextPoll(st.PollAfter, wait)

		select {
		case <-ctx.Done():
			return s.videoInterrupted(ctx, run, gen, in)
		case <-time.After(next):
		}
	}
}

// nextPoll is the wait before the next poll and the backoff after it: the wait the provider asked for, kept between
// videoPollFloor and videoPollCeiling, or else the backoff, which doubles up to videoPollMax.
func nextPoll(asked, backoff time.Duration) (next, after time.Duration) {
	if asked > 0 {
		return min(max(asked, videoPollFloor), videoPollCeiling), backoff
	}

	return backoff, min(backoff*2, videoPollMax)
}

// pollMayPass says whether a failed poll may be tried again: a rate limit, an unavailable provider or a request
// that timed out may pass; a refused key or request will not.
func pollMayPass(err error) bool {
	if pe, ok := providers.AsError(err); ok {
		switch pe.Kind {
		case providers.KindRateLimited, providers.KindUnavailable, providers.KindTimeout:
			return true
		}

		return false
	}

	return true
}

// videoInterrupted ends the wait of a run whose context ended. A shutdown is not a cancellation: the job is
// queued again and the task goes on at the provider. A person's cancel asks the provider to stop the task; the
// cost already incurred is the estimate unless the provider confirms that nothing will be charged.
func (s *svc) videoInterrupted(
	ctx context.Context, run *JobRun, gen providers.VideoGenerator, in *generateInput,
) error {
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), videoCallTimeout)
	defer cancel()

	if run.CancelRequested == nil || !run.CancelRequested(bg) {
		return ctx.Err()
	}

	return &owedError{err: ctx.Err(), owed: !s.cancelVideoTaskIn(bg, in, gen)}
}

// cancelVideoTask asks the provider to stop a task whose id could not be saved, on a context of its own.
func (s *svc) cancelVideoTask(in *generateInput, gen providers.VideoGenerator) {
	bg, cancel := context.WithTimeout(context.Background(), videoCallTimeout)
	defer cancel()

	s.cancelVideoTaskIn(bg, in, gen)
}

// cancelVideoTaskIn asks the provider to stop the task and says whether it confirmed that nothing is charged.
func (s *svc) cancelVideoTaskIn(ctx context.Context, in *generateInput, gen providers.VideoGenerator) bool {
	confirmed := false

	err := s.conns.WithTimeout(ctx, in.ConnectionID, videoCallTimeout, func(c connections.Credential) error {
		var cerr error

		confirmed, cerr = gen.Cancel(ctx, c, in.TaskID)

		return cerr
	})

	return err == nil && confirmed
}
