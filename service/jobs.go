package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
	"github.com/AlexGalhardo/vukem-video/service/textview"
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

var jobStates = []string{JobQueued, JobRunning, JobSucceeded, JobFailed, JobCancelled}

func isFinal(state string) bool {
	return state == JobSucceeded || state == JobFailed || state == JobCancelled
}

func (s *svc) JobStart(ctx context.Context, req JobStartRequest) (*Job, error) {
	c, err := authorize(ctx, "JobStart", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	switch req.Kind {
	case JobExport:
	case "transcribe":
		return nil, newError(codeTranscribeOff, "transcription is not available in this build.")
	default:
		return nil, newError(command.CodeValidation, "the kind %q cannot be started.", req.Kind).
			withPath("kind").withHint("use export")
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

	if s.exporter == nil {
		return nil, newError(command.CodeInternal, "exporting is not available in this build.")
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	totals, gerr := s.exportGate(doc)
	if gerr != nil {
		return nil, gerr
	}

	start, end, terr := exportRange(doc, args)
	if terr != nil {
		return nil, terr
	}

	input := marshalOrNull(
		exportInput{Revision: row.Revision, Start: start, End: end, Lint: &totals, Document: row.Document},
	)
	projectID := row.ID

	job := JobRow{
		ID: s.newIDs("job"), Kind: JobExport, ProjectID: &projectID, Actor: c.Actor, State: JobQueued, Input: input,
	}

	if req.DryRun {
		return s.jobView(c, s.stamp(job), false), nil
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
			return s.enqueue.Enqueue(ctx, tx, QueueKindExport, job.ID)
		}

		return nil
	})
	if err != nil {
		return nil, limitError(err, s.limits)
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, job.ID, time.Duration(req.WaitMS)*time.Millisecond)
	}

	cur, gerr2 := s.store.Jobs().Get(ctx, job.ID)
	if gerr2 != nil {
		return nil, asError(gerr2)
	}

	return s.jobView(c, cur, req.WaitMS > 0), nil
}

// stamp gives a job that is never stored the times of a new one.
func (s *svc) stamp(j JobRow) JobRow {
	now := s.nowMs()
	j.CreatedMs, j.UpdatedMs = now, now

	return j
}

// limitError adds the details of SDD-003 section 2.8 rule 2 to a queue-full error.
func limitError(err error, l Limits) error {
	out := asError(err)

	if e, ok := errors.AsType[*Error](out); ok && e.Code == CodeQueueFull {
		return e.withDetail("limit", "exports").withDetail("ceiling", l.MaxExports).
			withDetail("variable", "VUKEM_VIDEO_MAX_EXPORTS")
	}

	return out
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
		if name != "start" && name != "end" {
			return a, newError(command.CodeValidation, "the export does not take the argument %q.", name).
				withPath("args." + name).withHint("use start and end, both optional times")
		}
	}

	if err := json.Unmarshal(raw, &a); err != nil {
		return a, newError(command.CodeValidation, "start and end must be strings holding a time.").withPath("args")
	}

	return a, nil
}

// exportGate is the lint of rule 1: an error finding refuses the start.
func (s *svc) exportGate(doc *timeline.Document) (LintTotals, *Error) {
	all := lint.Lint(doc, s.lintEnv, lint.Full)
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

// canSee reports whether the caller may read a job: a job of a project follows
// the project; an ingest job follows any project that holds its file (SDD-003
// section 2.8 rule 7).
func (s *svc) canSee(ctx context.Context, c Caller, j *JobRow) bool {
	if j.ProjectID != nil {
		if c.ProjectID != "" && c.ProjectID != *j.ProjectID {
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
		if c.ProjectID != "" && c.ProjectID != refs[i].ProjectID {
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
	if gerr != nil || !s.canSee(ctx, c, &row) {
		return nil, jobNotFound(req.JobID)
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, row.ID, time.Duration(req.WaitMS)*time.Millisecond)

		if row, gerr = s.store.Jobs().Get(ctx, req.JobID); gerr != nil {
			return nil, asError(gerr)
		}
	}

	return s.jobView(c, row, req.WaitMS > 0), nil
}

// waitJob holds until the job is final or the time passes.
func (s *svc) waitJob(ctx context.Context, id string, d time.Duration) {
	deadline := time.Now().Add(d)

	for {
		row, err := s.store.Jobs().Get(ctx, id)
		if err != nil || isFinal(row.State) || time.Now().After(deadline) {
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
	c, err := authorize(ctx, "JobList", req.ProjectID)
	if err != nil {
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
		active, err := pick([]string{JobQueued, JobRunning}, 0)
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
		if all, err = pick([]string{JobQueued, JobRunning}, 0); err != nil {
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

// Cancel is the cancellation of SDD-005 section 14.4. It is not an operation of
// the Service interface (JobCancel arrives with ALPHA); the trash of a project
// and the tests drive it through JobRunner.
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

	if isFinal(before.State) {
		return nil
	}

	after, err := s.store.Jobs().RequestCancel(ctx, id, s.nowMs())
	if err != nil {
		return err
	}

	if after.State == JobRunning {
		s.running.cancel(id)
	}

	if before.State == JobQueued && after.State == JobCancelled {
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

	s.emitJob(ctx, j)
}

// cancelProjectJobs asks for the cancellation of the export and frame jobs of
// a project that went to the trash (SDD-005 section 14.4 item 5).
func (s *svc) cancelProjectJobs(ctx context.Context, projectID string) {
	rows, err := s.store.Jobs().List(ctx, projectID, []string{JobQueued, JobRunning}, 0)
	if err != nil {
		return
	}

	for i := range rows {
		_ = s.cancelJob(ctx, rows[i].ID)
	}
}

// jobView is the Job object of a row. The path of an export is for local
// callers only.
func (s *svc) jobView(c Caller, r JobRow, waited bool) *Job {
	j := &Job{
		JobID: r.ID, Kind: r.Kind, ProjectID: r.ProjectID, State: r.State, Progress: r.Progress, Message: r.Message,
		Error: parseError(r.Error), Actor: r.Actor, CreatedAt: msTime(r.CreatedMs),
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
		}

		var res JobResult
		if len(r.Result) > 0 && json.Unmarshal(r.Result, &res) == nil && res.Export != nil {
			if c.OwnerUserID != "" {
				res.Export.Path = ""
			}

			j.Result = &res
		}
	}

	j.Text = jobText(j, waited)

	return j
}

func jobText(j *Job, waited bool) string {
	v := textview.JobView{
		ID: j.JobID, Kind: j.Kind, State: j.State, Percent: int(j.Progress * 100), Message: j.Message, Waited: waited,
	}

	if j.Result != nil && j.Result.Export != nil {
		e := j.Result.Export
		v.File, v.Width, v.Height, v.SizeText = e.Path, e.Width, e.Height, textview.Size(e.Bytes)

		if fps, ok := parseFPS(e.FPS); ok {
			v.Duration = textview.Cell(timeval.FPS{Num: int64(fps.Num), Den: int64(fps.Den)}, e.Duration.Frame)
		}
	}

	if j.Error != nil && j.State == JobFailed {
		v.Error = "error " + j.Error.Code + ": " + j.Error.Message
	}

	return textview.Job(v)
}
