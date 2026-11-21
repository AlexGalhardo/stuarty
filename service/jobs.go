package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

const (
	defJobLimit       = 20
	maxJobLimit       = 100
	recentFinalJobs   = 10
	maxLintFindings   = 20
	jobStateAll       = "all"
	jobStateActive    = "active"
	maxJobListOffset  = 1000
	codeTranscribeOff = "E_TRANSCRIBE_UNAVAILABLE"
)

var jobStates = []string{JobQueued, JobWaitingApproval, JobRunning, JobSucceeded, JobFailed, JobCancelled}

// JobFinal reports whether a job state is final: succeeded, failed or cancelled.
func JobFinal(state string) bool {
	return state == JobSucceeded || state == JobFailed || state == JobCancelled
}

// authorizeStart checks the scope JobStart needs for the kind and that a key limited to one project covers the
// project or timeline the request names.
func (s *svc) authorizeStart(ctx context.Context, req *JobStartRequest, approvable *ApprovableKind) (Caller, error) {
	var (
		c   Caller
		err error
	)

	switch {
	case approvable != nil:
		c, err = authorizeScope(ctx, approvable.Scope, "")
	case req.Kind == JobTranscribe:
		c, err = authorizeScope(ctx, ScopeMedia, "")
	default:
		c, err = authorize(ctx, "JobStart", "")
	}

	if err != nil {
		return Caller{}, err
	}

	return c, s.requireCovers(ctx, c, req.ProjectID)
}

// useStartTimeline resolves the timeline a job is kept under: the one the request names, or for a kind of work that
// belongs to the project (ProjectWide) the first active one.
func (s *svc) useStartTimeline(ctx context.Context, c Caller, req *JobStartRequest, k *ApprovableKind) error {
	if k != nil && k.ProjectWide && req.TimelineID == "" {
		return s.useAnchorTimeline(ctx, c, &req.ProjectID)
	}

	return s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID)
}

// useAnchorTimeline is useTimeline for work that belongs to the project (a generation, VV-330): the job is kept
// under the first active timeline, since a job row still names a timeline, and the caller is not asked which of
// several. A project with no active timeline cannot hold the job yet.
func (s *svc) useAnchorTimeline(ctx context.Context, c Caller, id *string) error {
	if *id == "" {
		return nil
	}

	head, err := s.loadProduction(ctx, s.store.Productions(), c, *id)
	if err != nil {
		return err
	}

	tls, err := s.store.Productions().ActiveTimelines(ctx, head.ID)
	if err != nil {
		return asError(err)
	}

	if len(tls) == 0 {
		return newError(command.CodeInvalidState, "the project %s has no timeline to hold the job.", head.ID).
			withHint("add a video to the project first, or restore a timeline")
	}

	*id = tls[0].TimelineID

	return nil
}

func (s *svc) JobStart(ctx context.Context, req JobStartRequest) (*Job, error) {
	// A kind that needs approval waits for a person or a rule (SDD-015 section 5); it has its own scope.
	approvable := s.kinds[req.Kind]

	c, err := s.authorizeStart(ctx, &req, approvable)
	if err != nil {
		return nil, err
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	if err := s.useStartTimeline(ctx, c, &req, approvable); err != nil {
		return nil, err
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	if approvable != nil {
		return s.startApprovable(ctx, c, req, approvable)
	}

	switch req.Kind {
	case JobExport:
	case JobTranscribe:
		return s.startTranscribe(ctx, c, req)
	default:
		return nil, newError(command.CodeValidation, "the kind %q cannot be started.", req.Kind).
			withPath("kind").withHint("use export or transcribe")
	}

	args, aerr := decodeExportArgs(req.Args)
	if aerr != nil {
		return nil, aerr
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if row.State == ProjectTrashed {
		return nil, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	if err := s.exporterMissing(); err != nil {
		return nil, err
	}

	// Starting an export writes a row and a file; below the free-space floor it
	// is refused (SDD-007 section 9.2).
	if derr := s.checkDisk(); derr != nil && !req.DryRun {
		return nil, forCaller(c, derr)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	totals, gerr := s.exportGate(ctx, doc, args.Format)
	if gerr != nil {
		return nil, gerr
	}

	start, end, terr := jobExportRange(doc, args)
	if terr != nil {
		return nil, terr
	}

	format, ferr := checkExportFormat(args.Format)
	if ferr != nil {
		return nil, ferr
	}

	if lerr := checkExportSettings(&doc.Settings); lerr != nil {
		return nil, lerr
	}

	if perr := checkPresetArgs(doc, format, args, end-start); perr != nil {
		return nil, perr
	}

	nested, _ := s.nestedFor(ctx, s.store, doc)

	input := marshalOrNull(
		exportInput{
			Revision: row.Revision, Start: start, End: end, Lint: &totals, Document: row.Document, Format: format,
			Nested:   nestedSnapshot(nested),
			Warnings: exportWarnings(doc, format, end-start), Preset: args.Preset, Codec: args.Codec,
		},
	)
	projectID := row.ID

	job := JobRow{
		ID: s.newIDs("job"), Kind: JobExport, ProjectID: &projectID, Actor: c.Actor, State: JobQueued, Input: input,
	}

	if req.DryRun {
		return s.dryRunView(c, job), nil
	}

	lim := &JobLimit{
		Kind: JobExport, Max: s.limits.MaxExports, Actor: c.Actor,
		MaxPerActor: s.limits.MaxExportsPerActor + queuedPerActor,
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		job.CreatedMs, job.UpdatedMs = now, now

		if err := tx.Jobs().CreateLimited(ctx, job, lim); err != nil {
			return err
		}

		if err := tx.Audit().Append(ctx, AuditRow{
			AtMs: now, Actor: c.Actor, Action: "job.start", ProjectID: &projectID, SubjectID: &job.ID,
		}); err != nil {
			return err
		}

		// Registered before the enqueue: an inline queue runs the job in its own
		// after-commit hook, and the queued event must come first.
		queued := job
		tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

		if s.enqueue != nil {
			return s.enqueue.Enqueue(ctx, tx, queueKindFor(job.Kind), job.ID)
		}

		return nil
	})
	if err != nil {
		return nil, callerError(c, limitError(err, s.limits))
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, job.ID, time.Duration(req.WaitMS)*time.Millisecond)
	}

	cur, gerr2 := s.store.Jobs().Get(ctx, job.ID)
	if gerr2 != nil {
		return nil, asError(gerr2)
	}

	return s.jobViewCtx(ctx, c, cur, req.WaitMS > 0), nil
}

// stamp gives a job that is never stored the times of a new one.
func (s *svc) stamp(j JobRow) JobRow {
	now := s.nowMs()
	j.CreatedMs, j.UpdatedMs = now, now

	return j
}

// exportRetryAfter is the Retry-After, in seconds, of an export refused because the exports are all busy.
const exportRetryAfter = 10

// limitError adds the details of SDD-003 section 2.8 rule 2 to a queue-full error.
func limitError(err error, l Limits) error {
	out := asError(err)

	if e, ok := errors.AsType[*Error](out); ok && e.Code == CodeQueueFull {
		// An export lasts seconds to minutes: ten seconds is a fair first wait before the caller asks again (VV-116).
		return e.withDetail("retry_after", exportRetryAfter).
			withHint("at most %d exports run at the same time: wait for one to finish with job_get or stop one "+
				"with job_cancel; the person running the server can raise STUARTY_MAX_EXPORTS", l.MaxExports)
	}

	return out
}

// exporterMissing refuses an export when this build has no exporter. A missing
// ffmpeg or ffprobe keeps its own code; anything else is an internal error.
func (s *svc) exporterMissing() error {
	if s.exporter != nil {
		return nil
	}

	if s.media.FFmpegMissing != nil {
		return fromEngine(s.media.FFmpegMissing)
	}

	return newError(command.CodeInternal, "exporting is not available in this build.")
}

func decodeExportArgs(raw json.RawMessage) (ExportArgs, *Error) {
	var a ExportArgs

	if len(bytes.TrimSpace(raw)) == 0 {
		return a, nil
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return a, newError(command.CodeValidation, "args must be one JSON object.").withPath("args")
	}

	for name := range probe {
		if name != "start" && name != "end" && name != "format" && name != "at" && name != "preset" && name != "codec" {
			return a, newError(command.CodeValidation, "the export does not take the argument %q.", name).
				withPath("args." + name).
				withHint("use start, end, format, at, preset and codec, all optional; at is for format png; " +
					"preset and codec are for format mp4")
		}
	}

	if err := json.Unmarshal(raw, &a); err != nil {
		return a, newError(command.CodeValidation, "start, end, format, at, preset and codec must be strings.").
			withPath("args")
	}

	return a, nil
}

// gifWarnBytes is the typical GIF size above which the start of an export warns (VV-056).
const gifWarnBytes = 25 << 20

// exportWarnings are the notes about an export that is accepted but probably not what the person
// wants: a GIF made smaller than the project (it is capped at 480 pixels wide, VV-131) and a GIF
// whose typical size is over 25 MB.
func exportWarnings(doc *timeline.Document, format string, frames int64) []string {
	if format != FormatGIF {
		return nil
	}

	est := export.EstimateGIF(doc.Settings, int(frames))

	var out []string
	if est.Width != doc.Settings.Width {
		out = append(out, fmt.Sprintf(
			"The GIF is scaled to %dx%d from the project's %dx%d: a GIF is capped at 480 pixels wide and 15 fps.",
			est.Width, est.Height, doc.Settings.Width, doc.Settings.Height))
	}

	if est.TypicalBytes > gifWarnBytes {
		out = append(out, fmt.Sprintf(
			"The GIF will be about %s (up to %s) at %dx%d, %d frames; a GIF this size is slow to share. "+
				"Export mp4 instead, or shorten the range.",
			textview.Size(est.TypicalBytes), textview.Size(est.WorstBytes), est.Width, est.Height, est.Frames))
	}

	return out
}

// exportGate is the lint of rule 1: an error finding refuses the start.
func (s *svc) exportGate(ctx context.Context, doc *timeline.Document, format string) (LintTotals, *Error) {
	all := silentAudioIsError(lint.Lint(doc, s.lintEnvFor(ctx, doc), lint.Full), format)
	errs, warns := lint.Totals(all)

	if errs == 0 {
		return LintTotals{Errors: 0, Warnings: warns}, nil
	}

	first := make([]lint.Finding, 0, maxLintFindings)

	for _, f := range all {
		if f.Severity == lint.SeverityError && len(first) < maxLintFindings {
			first = append(first, f)
		}
	}

	return LintTotals{}, newError(CodeLintBlocking, "the project has %d problem(s) that stop an export.", errs).
		withHint("fix the findings, or read them with lint, then start the export again").
		withDetail("findings", first).withDetail("total", errs)
}

// silentAudioIsError makes L_SILENT_EXPORT an error for an audio-only export: a
// silent video is a choice, but a wav or aac file of silence is never wanted.
func silentAudioIsError(all []lint.Finding, format string) []lint.Finding {
	if format != FormatWAV && format != FormatAAC {
		return all
	}

	out := slices.Clone(all)
	for i := range out {
		if out[i].Code == lint.CodeSilentExport {
			out[i].Severity = lint.SeverityError
		}
	}

	return out
}

// exportRange resolves start and end to frames; end 0 is the end of the timeline.
func exportRange(doc *timeline.Document, a ExportArgs) (start, end int64, err *Error) {
	fps := timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}
	last := endFrame(doc)
	end = last

	parse := func(field, v string) (int64, *Error) {
		f, perr := timeval.Parse(v, fps)
		if perr != nil {
			return 0, newError(command.CodeInvalidTime, "%s is not a time.", field).withPath("args." + field).
				withHint("write a time like 1.5s or 45f")
		}

		return f, nil
	}

	if a.Start != "" {
		if start, err = parse("start", a.Start); err != nil {
			return 0, 0, err
		}
	}

	if a.End != "" {
		if end, err = parse("end", a.End); err != nil {
			return 0, 0, err
		}
	}

	if start < 0 || end > last || start >= end {
		return 0, 0, newError(
			command.CodeValidation,
			"the range %d to %d is not inside the timeline (0 to %d).",
			start,
			end,
			last,
		).
			withPath("args")
	}

	return start, end, nil
}

// refuseJob is the answer for a job the caller cannot use. A key limited to one
// project is told E_FORBIDDEN only when the job belongs to a project of its own
// owner, which it may already know about; for any other job, including that of
// another user, the answer is the one of a job that does not exist, so no job
// is disclosed.
func (s *svc) refuseJob(ctx context.Context, c Caller, j *JobRow, id string) *Error {
	if c.ProjectID != "" && j != nil && j.ProjectID != nil && !s.covers(ctx, c, *j.ProjectID) {
		if h, err := s.store.Projects().Head(ctx, *j.ProjectID); err == nil && visible(c, h.OwnerUserID) {
			return limitedElsewhere()
		}
	}

	return jobNotFound(id)
}

// keyCovers reports whether a key limited to one project reaches the timeline document id (SDD-003 section 1.7
// rule 4): the document belongs to that project, or, for an id that is no timeline of any project, it is the
// project's own id. A caller with no project limit reaches every id. The answer for a timeline of another
// project and for an id that does not exist is the same, so it tells nothing about either.
func keyCovers(ctx context.Context, r interface{ Productions() ProductionRepo }, c Caller, id string) bool {
	if c.ProjectID == "" {
		return true
	}

	if owner, err := r.Productions().ProjectOfTimeline(ctx, id); err == nil {
		return owner == c.ProjectID
	}

	return id == c.ProjectID
}

func (s *svc) covers(ctx context.Context, c Caller, id string) bool {
	return keyCovers(ctx, s.store, c, id)
}

// requireCovers refuses a key limited to one project that names a project or timeline outside it, before
// anything of that project is read. An empty id names none and is left to the operation.
func (s *svc) requireCovers(ctx context.Context, c Caller, id string) error {
	if id == "" || s.covers(ctx, c, id) {
		return nil
	}

	return limitedElsewhere()
}

func limitedElsewhere() *Error {
	return newError(CodeForbidden, "the key is limited to another project.").
		withHint("use a key that covers this project")
}

// canSee reports whether the caller may read a job: a job of a project follows
// the project; an ingest job follows any project that holds its file (SDD-003
// section 2.8 rule 7).
func (s *svc) canSee(ctx context.Context, c Caller, j *JobRow) bool {
	if j.Kind == JobImageExport {
		return s.canSeeImageJob(ctx, c, j)
	}

	if j.ProjectID != nil {
		if !s.covers(ctx, c, *j.ProjectID) {
			return false
		}

		row, err := s.store.Projects().Get(ctx, *j.ProjectID)

		return err == nil && visible(c, row.OwnerUserID)
	}

	if j.AssetHash == nil {
		return false
	}

	refs := s.referrers(ctx, *j.AssetHash)
	for i := range refs {
		if !s.covers(ctx, c, refs[i].ProjectID) {
			continue
		}

		if row, err := s.store.Projects().Get(ctx, refs[i].ProjectID); err == nil && visible(c, row.OwnerUserID) {
			return true
		}
	}

	return false
}

func jobNotFound(id string) *Error {
	return newError(
		command.CodeNotFound,
		"there is no job %s.",
		id,
	).withHint("list the jobs of the project with job_get")
}

func (s *svc) JobGet(ctx context.Context, req JobGetRequest) (*Job, error) {
	c, err := authorize(ctx, "JobGet", "")
	if err != nil {
		return nil, err
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	row, gerr := s.store.Jobs().Get(ctx, req.JobID)
	if gerr != nil {
		return nil, jobNotFound(req.JobID)
	}

	if !s.canSee(ctx, c, &row) {
		return nil, s.refuseJob(ctx, c, &row, req.JobID)
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, row.ID, time.Duration(req.WaitMS)*time.Millisecond)

		if row, gerr = s.store.Jobs().Get(ctx, req.JobID); gerr != nil {
			return nil, asError(gerr)
		}
	}

	return s.jobViewCtx(ctx, c, row, req.WaitMS > 0), nil
}

// waitJob holds until the job is final or the time passes.
func (s *svc) waitJob(ctx context.Context, id string, d time.Duration) {
	deadline := time.Now().Add(d)

	for {
		row, err := s.store.Jobs().Get(ctx, id)
		if err != nil || JobFinal(row.State) || time.Now().After(deadline) {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitPollInterval):
		}
	}
}

func (s *svc) JobList(ctx context.Context, req JobListRequest) (*JobListResult, error) {
	c, err := authorize(ctx, "JobList", "")
	if err != nil {
		return nil, err
	}

	if err := s.requireCovers(ctx, c, req.ProjectID); err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if req.State != "" && req.State != jobStateAll && req.State != jobStateActive &&
		!slices.Contains(jobStates, req.State) {
		return nil, newError(command.CodeValidation, "the state %q is not known.", req.State).
			withPath("state").withHint("use queued, running, succeeded, failed, cancelled, active or all")
	}

	off := 0

	if req.Cursor != "" {
		var derr *Error
		if off, derr = decodeOffset(req.Cursor, "jobs", row.ID); derr != nil {
			return nil, derr
		}
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	limit := clampLimit(req.Limit, defJobLimit, maxJobLimit)

	rows, total, jerr := s.listJobs(ctx, row.ID, req.State, off, limit)
	if jerr != nil {
		return nil, asError(jerr)
	}

	res := &JobListResult{ProjectID: row.ID, Jobs: make([]Job, 0, len(rows))}
	lines := make([]string, 0, len(rows))

	for i := range rows {
		j := s.jobView(c, rows[i], false)
		res.Jobs = append(res.Jobs, *j)
		lines = append(lines, j.Text)
	}

	res.Text = textview.JobList(row.ID, lines, total)

	if next := off + len(rows); next < total {
		cur := offsetCursor("jobs", row.ID, next)
		res.NextCursor = &cur
		res.Text += "\nnext_cursor: " + cur
	}

	return res, nil
}

// listJobs pages the jobs of a project, newest first. The store lists with a
// limit only, so a page asks for the rows up to its end and drops the ones
// before it; the lookback is bounded.
func (s *svc) listJobs(ctx context.Context, projectID, state string, off, limit int) ([]JobRow, int, error) {
	if off > maxJobListOffset {
		off = maxJobListOffset
	}

	pick := func(states []string, n int) ([]JobRow, error) { return s.store.Jobs().List(ctx, projectID, states, n) }

	var all []JobRow

	switch state {
	case "":
		active, err := pick([]string{JobQueued, JobWaitingApproval, JobRunning}, 0)
		if err != nil {
			return nil, 0, err
		}

		final, err := pick([]string{JobSucceeded, JobFailed, JobCancelled}, recentFinalJobs)
		if err != nil {
			return nil, 0, err
		}

		all = slices.Concat(active, final)
	case jobStateAll:
		var err error
		if all, err = pick(nil, off+limit+1); err != nil {
			return nil, 0, err
		}
	case jobStateActive:
		var err error
		if all, err = pick([]string{JobQueued, JobWaitingApproval, JobRunning}, 0); err != nil {
			return nil, 0, err
		}
	default:
		var err error
		if all, err = pick([]string{state}, off+limit+1); err != nil {
			return nil, 0, err
		}
	}

	slices.SortStableFunc(all, func(a, b JobRow) int {
		switch {
		case a.CreatedMs != b.CreatedMs:
			return int(b.CreatedMs - a.CreatedMs)
		default:
			return strings.Compare(b.ID, a.ID)
		}
	})

	total := len(all)
	if off >= total {
		return nil, total, nil
	}

	return all[off:min(off+limit, total)], total, nil
}

// cancelWait is how long JobCancel holds for a running job to reach its final
// state; the cancellation itself is already requested when it starts waiting.
const cancelWait = 2 * time.Second

// JobCancel asks for the cancellation of a job and answers with the row as it
// is afterwards (SDD-003 section 2.8 item 5): a job that finished first is
// returned in its final state, and a second call changes nothing. The scope is
// render; media is enough for the caller's own ingest job (SDD-007 section 5.2).
func (s *svc) JobCancel(ctx context.Context, req JobCancelRequest) (*Job, error) {
	c, ok := CallerFrom(ctx)
	if !ok {
		return nil, newError(CodeUnauthorized, "the request carries no credentials.").
			withHint("send the local token or an API key")
	}

	// The scope comes first: a caller with neither render nor media learns
	// nothing about the job, whoever owns it.
	if !slices.Contains(c.Scopes, ScopeRender) && !slices.Contains(c.Scopes, ScopeMedia) {
		return nil, newError(CodeForbidden, "the caller lacks the %q scope.", ScopeRender).
			withHint("use a key with the %s scope", ScopeRender)
	}

	row, err := s.store.Jobs().Get(ctx, req.JobID)
	if err != nil {
		return nil, jobNotFound(req.JobID)
	}

	if !s.canSee(ctx, c, &row) {
		return nil, s.refuseJob(ctx, c, &row, req.JobID)
	}

	ownMediaJob := slices.Contains(c.Scopes, ScopeMedia) && (row.Kind == JobIngest || row.Kind == JobTranscribe) &&
		row.Actor == c.Actor
	if !slices.Contains(c.Scopes, ScopeRender) && !ownMediaJob {
		return nil, newError(CodeForbidden, "the caller lacks the %q scope.", ScopeRender).
			withHint("use a key with the %s scope", ScopeRender)
	}

	if row.ProjectID != nil && !s.covers(ctx, c, *row.ProjectID) {
		return nil, newError(CodeForbidden, "the key is limited to another project.").
			withHint("use a key that covers this project")
	}

	if row.Kind == JobIngest && row.AssetHash != nil && !JobFinal(row.State) {
		done, cerr := s.cancelSharedIngest(ctx, c, &row)
		if cerr != nil {
			return nil, asError(cerr)
		}

		if !done {
			// Other projects wait on the same ingest: it keeps running.
			after, gerr := s.store.Jobs().Get(ctx, row.ID)
			if gerr != nil {
				return nil, asError(gerr)
			}

			return s.jobViewCtx(ctx, c, after, false), nil
		}
	} else if row.State == JobWaitingApproval {
		if err := s.cancelWaitingJob(ctx, c.Actor, &row); err != nil {
			return nil, asError(err)
		}
	} else if err := s.cancelJob(ctx, row.ID); err != nil {
		return nil, asError(err)
	}

	if !JobFinal(row.State) && row.State != JobWaitingApproval {
		s.waitJob(ctx, row.ID, cancelWait)
	}

	after, gerr := s.store.Jobs().Get(ctx, row.ID)
	if gerr != nil {
		return nil, asError(gerr)
	}

	return s.jobViewCtx(ctx, c, after, false), nil
}

// cancelSharedIngest decides and applies the cancellation of an ingest, which
// is shared by content hash (SDD-005 section 5.6). The references are read and
// the cancellation recorded in one write transaction, so a project that
// imports the same file meanwhile is counted. When every live reference is a
// project the caller may see, the ingest is cancelled and cancelled is true.
// Otherwise cancelling would kill an ingest that a project the caller cannot
// see waits on: the caller's own pending references are trashed instead and
// the job runs on. Any failure to read the references is an error, never a
// yes.
func (s *svc) cancelSharedIngest(ctx context.Context, c Caller, job *JobRow) (cancelled bool, err error) {
	var after JobRow

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		var decided bool

		after, decided, err = s.decideSharedIngest(ctx, tx, c, job)
		cancelled = decided

		return err
	})
	if err != nil || !cancelled {
		return false, err
	}

	if after.State == JobRunning {
		s.running.cancel(job.ID)
	}

	if job.State == JobQueued && after.State == JobCancelled {
		s.afterQueuedCancel(ctx, &after)
	}

	return true, nil
}

// decideSharedIngest is the body of cancelSharedIngest, inside its transaction.
func (s *svc) decideSharedIngest(ctx context.Context, tx Tx, c Caller, job *JobRow) (JobRow, bool, error) {
	ids, err := tx.Assets().ProjectsReferencing(ctx, *job.AssetHash)
	if err != nil {
		return JobRow{}, false, err
	}

	var mine []ProjectAssetRow

	live := 0

	for _, id := range ids {
		ref, ferr := tx.Assets().FindProjectAssetByHash(ctx, id, *job.AssetHash)
		if ferr != nil {
			return JobRow{}, false, ferr
		}

		if ref.State == ProjectAssetTrashed {
			continue // it no longer waits on the ingest
		}

		live++

		owner, gerr := tx.Projects().Get(ctx, id)
		if gerr != nil {
			return JobRow{}, false, gerr
		}

		if visible(c, owner.OwnerUserID) && keyCovers(ctx, tx, c, id) {
			mine = append(mine, ref)
		}
	}

	now := s.nowMs()

	if len(mine) == live {
		after, rerr := tx.Jobs().RequestCancel(ctx, job.ID, now)

		return after, rerr == nil, rerr
	}

	for i := range mine {
		if mine[i].State != ProjectAssetPending {
			continue
		}

		if err := tx.Assets().
			SetProjectAssetState(ctx, mine[i].ProjectID, mine[i].AssetID, ProjectAssetTrashed, c.Actor, now, nil); err != nil {
			return JobRow{}, false, err
		}

		if err := appendAudit(ctx, tx, now, c.Actor, "media.trash", mine[i].ProjectID, mine[i].AssetID); err != nil {
			return JobRow{}, false, err
		}
	}

	return JobRow{}, false, nil
}

// Cancel is the cancellation of SDD-005 section 14.4 for the trash of a
// project and the tests, which drive it through JobRunner.
func (s *svc) Cancel(ctx context.Context, jobID string) error {
	return s.cancelJob(ctx, jobID)
}

// cancelJob is SDD-005 section 14.4: set the flag, finish a queued job at
// once, and stop a running one through its context. A final job is returned as
// it is.
func (s *svc) cancelJob(ctx context.Context, id string) error {
	before, err := s.store.Jobs().Get(ctx, id)
	if err != nil {
		return err
	}

	if JobFinal(before.State) {
		return nil
	}

	if before.State == JobWaitingApproval {
		if err := s.cancelWaiting(ctx, "system", &before); !errors.Is(err, errNoLongerWaiting) {
			return err
		}

		// Approved since it was read: it is queued or running now, and is cancelled as such.
		if before, err = s.store.Jobs().Get(ctx, id); err != nil || JobFinal(before.State) {
			return err
		}
	}

	if before.Kind == JobExportBatch {
		s.cancelBatchChildren(ctx, &before)
	}

	after, err := s.store.Jobs().RequestCancel(ctx, id, s.nowMs())
	if err != nil {
		return err
	}

	if after.State == JobRunning {
		s.running.cancel(id)
	}

	if before.State != JobRunning && after.State == JobCancelled {
		s.afterQueuedCancel(ctx, &after)
	}

	return nil
}

// afterQueuedCancel closes what a job cancelled before it ran leaves open.
func (s *svc) afterQueuedCancel(ctx context.Context, j *JobRow) {
	if j.Kind == JobIngest && j.AssetHash != nil {
		e := newError(CodeCancelled, "the job was cancelled.")
		if err := s.store.Assets().SetIngest(ctx, *j.AssetHash, IngestFailed, 0, errorJSON(e), s.nowMs()); err == nil {
			s.emitAsset(ctx, *j.AssetHash, IngestFailed, 0, e)
		}
	}

	// An approved job cancelled before it ran spent nothing, and RunJob never reaches finishCost for
	// it: its reservation goes back here, or it would hold the ceiling of the day (SDD-015 section 6).
	// One queued again after a shutdown may have left work at a provider, which its kind abandons.
	if k := s.kindOf(j.Kind); k != nil {
		s.settleQueuedCancel(ctx, k, j)
	}

	s.emitJob(ctx, j)
}

// cancelProjectJobs asks for the cancellation of the export and frame jobs of
// a project that went to the trash (SDD-005 section 14.4 item 5).
func (s *svc) cancelProjectJobs(ctx context.Context, projectID string) {
	rows, err := s.store.Jobs().List(ctx, projectID, []string{JobQueued, JobWaitingApproval, JobRunning}, 0)
	if err != nil {
		return
	}

	for i := range rows {
		_ = s.cancelJob(ctx, rows[i].ID)
	}
}

// imageExportView is the result of an image.export job for the caller: the file by its absolute path for a local
// caller and with no path in network mode, as an export is.
func (s *svc) imageExportView(c Caller, r *JobRow) *JobResult {
	if r.Kind != JobImageExport || len(r.Result) == 0 {
		return nil
	}

	var res JobResult
	if json.Unmarshal(r.Result, &res) != nil || res.Image == nil {
		return nil
	}

	res.Image.JobID = r.ID

	if c.OwnerUserID != "" {
		res.Image.Path = ""
	} else {
		res.Image.Path = filepath.Join(s.workspace, filepath.FromSlash(res.Image.Path))
	}

	return &res
}

// jobView is the Job object of a row. The path of an export is for local
// callers only.
func (s *svc) jobView(c Caller, r JobRow, waited bool) *Job {
	j := &Job{
		JobID:     r.ID,
		Kind:      r.Kind,
		ProjectID: r.ProjectID,
		State:     r.State,
		Progress:  r.Progress,
		Message:   r.Message,
		Error: forCaller(
			c,
			parseError(r.Error),
		),
		Actor:      r.Actor,
		CreatedAt:  msTime(r.CreatedMs),
		ApprovalID: r.ApprovalID,
	}

	// The last progress message of a finished job ("Rendering (97%)") contradicts its state (VV-102).
	if r.State == JobSucceeded {
		j.Message = ""
	}

	if r.StartedMs != nil {
		t := msTime(*r.StartedMs)
		j.StartedAt = &t
	}

	if r.FinishedMs != nil {
		t := msTime(*r.FinishedMs)
		j.FinishedAt = &t
	}

	if r.Kind == JobExport {
		var in exportInput
		if json.Unmarshal(r.Input, &in) == nil {
			j.Lint = in.Lint
			j.Warnings = in.Warnings
		}

		var res JobResult
		if len(r.Result) > 0 && json.Unmarshal(r.Result, &res) == nil && res.Export != nil {
			if c.OwnerUserID != "" {
				res.Export.Path = ""
			}

			j.Result = &res
		}
	}

	if res := s.imageExportView(c, &r); res != nil {
		j.Result = res
	}

	if r.Kind == JobPublishUpload && len(r.Result) > 0 {
		var res JobResult
		if json.Unmarshal(r.Result, &res) == nil && res.Publish != nil {
			j.Result = &res
		}
	}

	if r.Kind == JobTranscribe && len(r.Result) > 0 {
		var res JobResult
		if json.Unmarshal(r.Result, &res) == nil && res.Transcribe != nil {
			j.Result = &res
		}
	}

	viewGeneration(j, &r)

	j.Text = jobText(j, waited)

	return j
}

// dryRunView is the answer to a dry run of a job kind: what the job would be, with no id, because none was
// created and job_get would not find it (VV-303).
func (s *svc) dryRunView(c Caller, r JobRow) *Job {
	j := s.jobView(c, s.stamp(r), false)
	j.JobID, j.DryRun = "", true

	next := "start at once"
	if j.State == JobWaitingApproval {
		next = "wait for the approval of a person"
	}

	j.Text = fmt.Sprintf("dry run: a %s job would %s", j.Kind, next)
	if j.Estimate != nil {
		j.Text += "; estimate " + money(j.Estimate.Micros, j.Estimate.Currency)
	}

	j.Text += ". Nothing was created: there is no job to read; repeat without the dry run to start it."

	return j
}

func jobText(j *Job, waited bool) string {
	v := textview.JobView{
		ID: j.JobID, Kind: j.Kind, State: j.State, Percent: int(j.Progress * 100), Message: j.Message, Waited: waited,
	}

	if j.Result != nil && j.Result.Export != nil {
		e := j.Result.Export
		v.File, v.URL, v.Width, v.Height, v.SizeText = e.Path, e.DownloadURL, e.Width, e.Height, textview.Size(e.Bytes)
		v.Audio = e.Format == "wav" || e.Format == "aac"

		if fps, ok := parseFPS(e.FPS); ok {
			v.Duration = textview.Cell(timeval.FPS{Num: int64(fps.Num), Den: int64(fps.Den)}, e.Duration.Frame)
		}
	}

	if j.Estimate != nil {
		v.Estimate = money(j.Estimate.Micros, j.Estimate.Currency)
	}

	if j.Result != nil && j.Result.Text != nil {
		t := j.Result.Text
		v.Cost = money(t.Cost.Micros, t.Cost.Currency)
		// The text of a model is network text: delimited, cleaned and cut like every other (SDD-007 section 8).
		v.Output = untrusted.Block(untrusted.SourceNetworkText,
			untrusted.Attrs{"provider": t.Provider, "model": t.Model}, t.Text, untrusted.TranscriptFull)
	}

	if j.Result != nil && j.Result.Generated != nil {
		g := j.Result.Generated
		v.Cost = money(g.Cost.Micros, g.Cost.Currency)

		for i := range g.Items {
			v.Files = append(
				v.Files,
				fmt.Sprintf("%s %s %s", g.Items[i].AssetID, g.Items[i].Kind, textview.Size(g.Items[i].Bytes)),
			)
		}

		for _, n := range g.Notes {
			v.Warnings = append(v.Warnings, "warning: "+n)
		}
	}

	for _, w := range j.Warnings {
		v.Warnings = append(v.Warnings, "warning: "+w)
	}

	if j.Result != nil && j.Result.Publish != nil {
		p := j.Result.Publish
		v.Warnings = append(v.Warnings, fmt.Sprintf("publication %s: %s",
			p.State, untrusted.Inline(untrusted.SourceNetworkText, nil, p.URL, 200)))
	}

	if j.Error != nil && j.State == JobFailed {
		v.Error = "error " + j.Error.Code + ": " + j.Error.Message
	}

	if j.State == JobWaitingApproval {
		// The agent that reads this must know it waits and cannot decide (RF-135).
		v.Warnings = append(v.Warnings, fmt.Sprintf(
			"waiting for approval %s: a person approves or rejects it in the app, and you cannot; "+
				"do not submit it again, read it with job_get later", j.ApprovalID))
	}

	return textview.Job(v)
}
