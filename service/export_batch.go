package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// The limits of a batch (SDD-004 section 8.4).
const (
	// MaxBatchItems is the most exports one batch holds.
	MaxBatchItems = 12
	// maxActiveBatches and maxActiveBatchesPerActor bound the parents that wait or run; a parent holds no
	// encoder, so the bound is only there to keep the list of jobs readable.
	maxActiveBatches         = 8
	maxActiveBatchesPerActor = 3
	// batchRetryEvery is how long a parent waits before it asks again for a slot when the exports of the
	// server are all busy, and how often it reads the progress of the child that runs.
	batchRetryEvery = 500 * time.Millisecond
	// batchChildAlphabet is the alphabet of identifiers (SDD-000 section 2).
	batchChildAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"
)

// ExportBatchItem is one export of a batch: a preset, or the format, codec and range of a plain export.
type ExportBatchItem struct {
	Preset string `json:"preset,omitempty"`
	Format string `json:"format,omitempty"`
	Codec  string `json:"codec,omitempty"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

// ExportBatchRequest starts the exports of one project, one after the other, as one job (RF-088).
type ExportBatchRequest struct {
	ProjectID  string            `json:"project_id"`
	TimelineID string            `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	Items      []ExportBatchItem `json:"items"`
	WaitMS     int               `json:"wait_ms,omitempty"`
}

// batchItem is an item as it is stored: the arguments and the frames they resolved to at the start.
type batchItem struct {
	ExportBatchItem
	StartFrame int64    `json:"start_frame"`
	EndFrame   int64    `json:"end_frame"`
	Warnings   []string `json:"warnings,omitempty"`
}

// batchInput is the input column of an export_batch job. The document is the one of Revision, pinned when the
// batch starts, so every item exports the same project whatever edits land while it waits.
type batchInput struct {
	Revision int64           `json:"revision"`
	Document json.RawMessage `json:"document"`
	Lint     *LintTotals     `json:"lint,omitempty"`
	Items    []batchItem     `json:"items"`
}

// BatchItem is one export of a batch as it stands now, read from its child job.
type BatchItem struct {
	Index    int           `json:"index"`
	Preset   string        `json:"preset,omitempty"`
	Format   string        `json:"format"`
	Codec    string        `json:"codec,omitempty"`
	JobID    string        `json:"job_id,omitempty"`
	State    string        `json:"state"`
	Progress float64       `json:"progress"`
	Result   *ExportResult `json:"result,omitempty"`
	Error    *Error        `json:"error,omitempty"`
}

// BatchResult is the state of the children of a batch.
type BatchResult struct {
	Total     int         `json:"total"`
	Succeeded int         `json:"succeeded"`
	Failed    int         `json:"failed"`
	Cancelled int         `json:"cancelled"`
	Items     []BatchItem `json:"items"`
}

// batchChildID is the identifier of the child of item i of a parent: a function of both, so the parent finds a
// child after a restart, and the view finds it, without listing the jobs of the project.
func batchChildID(parentID string, i int) string {
	sum := sha256.Sum256([]byte(parentID + "/" + strconv.Itoa(i)))

	var b strings.Builder

	b.WriteString("job_")

	for _, c := range sum[:10] {
		b.WriteByte(batchChildAlphabet[c&31])
	}

	return b.String()
}

// ExportBatch checks every item, pins the project, and creates the parent job (SDD-004 section 8.4).
func (s *svc) ExportBatch(ctx context.Context, req ExportBatchRequest) (*Job, error) {
	c, err := s.authorizeProject(ctx, "ExportBatch", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	// A batch spends no heavy-call token: it makes one job, and its children pass through the limit of the
	// exports (MaxExports), while the parents that wait or run are bounded below.
	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	if n := len(req.Items); n < 1 || n > MaxBatchItems {
		return nil, newError(command.CodeValidation, "a batch has 1 to %d items; this one has %d.", MaxBatchItems, n).
			withPath("items").withHint("split it into batches of at most %d exports", MaxBatchItems)
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

	if derr := s.checkDisk(); derr != nil {
		return nil, forCaller(c, derr)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	if lerr := checkExportSettings(&doc.Settings); lerr != nil {
		return nil, lerr
	}

	items, totals, ierr := s.checkBatchItems(ctx, doc, req.Items)
	if ierr != nil {
		return nil, ierr
	}

	projectID := row.ID
	job := JobRow{
		ID: s.newIDs("job"), Kind: JobExportBatch, ProjectID: &projectID, Actor: c.Actor, State: JobQueued,
		Input: marshalOrNull(batchInput{Revision: row.Revision, Document: row.Document, Lint: &totals, Items: items}),
	}

	lim := &JobLimit{
		Kind: JobExportBatch, Max: maxActiveBatches, Actor: c.Actor, MaxPerActor: maxActiveBatchesPerActor,
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

		queued := job
		tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

		if s.enqueue != nil {
			return s.enqueue.Enqueue(ctx, tx, QueueKindExportBatch, job.ID)
		}

		return nil
	})
	if err != nil {
		return nil, callerError(c, batchLimitError(err))
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, job.ID, time.Duration(req.WaitMS)*time.Millisecond)
	}

	cur, gerr := s.store.Jobs().Get(ctx, job.ID)
	if gerr != nil {
		return nil, asError(gerr)
	}

	return s.jobViewCtx(ctx, c, cur, req.WaitMS > 0), nil
}

// batchLimitError is the E_QUEUE_FULL of too many batches at once.
func batchLimitError(err error) error {
	out := asError(err)

	if e, ok := errors.AsType[*Error](out); ok && e.Code == CodeQueueFull {
		return e.withDetail("retry_after", exportRetryAfter).
			withHint("wait for a batch to finish with job_get, or stop one with job_cancel")
	}

	return out
}

// checkBatchItems validates every item as an export of the project would be, with the paths of the item.
func (s *svc) checkBatchItems(
	ctx context.Context,
	doc *timeline.Document,
	in []ExportBatchItem,
) ([]batchItem, LintTotals, *Error) {
	var totals LintTotals

	out := make([]batchItem, 0, len(in))
	gated := map[string]bool{}

	for i, it := range in {
		at := func(e *Error) *Error {
			if e.Path == "" || e.Path == "args" {
				return e.withPath(fmt.Sprintf("items[%d]", i))
			}

			return e.withPath(fmt.Sprintf("items[%d].%s", i, strings.TrimPrefix(e.Path, "args.")))
		}

		format, ferr := checkExportFormat(it.Format)
		if ferr != nil {
			return nil, totals, at(ferr)
		}

		if format == FormatPNG {
			return nil, totals, at(newError(command.CodeValidation, "a png is one frame, not an export of a batch.").
				withPath("format").withHint("export the frame with job_start, format png"))
		}

		if !gated[format] {
			t, gerr := s.exportGate(ctx, doc, format)
			if gerr != nil {
				return nil, totals, gerr
			}

			gated[format], totals = true, t
		}

		args := ExportArgs{Start: it.Start, End: it.End, Format: format, Preset: it.Preset, Codec: it.Codec}

		start, end, rerr := exportRange(doc, args)
		if rerr != nil {
			return nil, totals, at(rerr)
		}

		if perr := checkPresetShape(args, format); perr != nil {
			return nil, totals, at(perr)
		}

		bi := batchItem{StartFrame: start, EndFrame: end, Warnings: exportWarnings(doc, format, end-start)}
		bi.Preset, bi.Format, bi.Codec, bi.Start, bi.End = it.Preset, format, it.Codec, it.Start, it.End
		out = append(out, bi)

	}

	return out, totals, nil
}

// checkPresetArgs refuses what an export with a preset or a codec cannot be: another format, an unknown name, or a
// project that does not fit the preset (SDD-004 section 8.2). It is nil for an export with neither.
func checkPresetArgs(doc *timeline.Document, format string, a ExportArgs, frames int64) *Error {
	if err := checkPresetShape(a, format); err != nil {
		return err
	}

	if a.Preset == "" {
		return nil
	}

	p, _ := exportcore.Lookup(a.Preset)

	return presetFit(doc, &p, frames, exportcore.Codec(a.Codec))
}

// checkPresetShape is the part of checkPresetArgs that needs no document.
func checkPresetShape(a ExportArgs, format string) *Error {
	if a.Preset == "" && a.Codec == "" {
		return nil
	}

	if format != FormatMP4 {
		path := "preset"
		if a.Preset == "" {
			path = "codec"
		}

		return newError(command.CodeValidation, "a preset or a codec is for an mp4 export, not %s.", format).
			withPath("args." + path).withHint("set format to mp4, or leave the preset and the codec out")
	}

	if _, err := checkExportCodec(a.Codec, "codec"); err != nil {
		return err
	}

	if a.Preset != "" {
		if _, err := lookupPreset(a.Preset, "preset"); err != nil {
			return err
		}
	}

	return nil
}

// runExportBatch is the parent of a batch: it takes the items one at a time, creates and runs the child of each, and
// ends with the outcome of all of them (SDD-004 section 8.4). A failed child does not stop the others.
func (s *svc) runExportBatch(ctx context.Context, job *JobRow, pw *progress) (json.RawMessage, error) {
	var in batchInput
	if err := json.Unmarshal(job.Input, &in); err != nil || job.ProjectID == nil {
		return nil, internalError()
	}

	n := len(in.Items)

	for i := range in.Items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		id := batchChildID(job.ID, i)

		pw.report(ctx, float64(i)/float64(n), fmt.Sprintf("Exporting %d of %d", i+1, n))

		if err := s.runBatchChild(ctx, job, &in, i, id, pw); err != nil {
			return nil, err
		}
	}

	return s.finishBatch(ctx, job, &in)
}

// runBatchChild makes the child of item i when it does not exist, runs it, and waits until it is final.
func (s *svc) runBatchChild(
	ctx context.Context,
	parent *JobRow,
	in *batchInput,
	i int,
	id string,
	pw *progress,
) error {
	child, err := s.store.Jobs().Get(ctx, id)
	if err != nil {
		if !isNotFound(err) {
			return err
		}

		if err := s.createBatchChild(ctx, parent, in, i, id); err != nil {
			return err
		}
	} else if JobFinal(child.State) {
		return nil
	}

	if ctx.Err() != nil {
		return s.parentStopped(ctx, parent.ID, id)
	}

	// RunJob claims the child when it is queued and runs it here, under the semaphore of the exports. When another
	// process already runs it (a restart handed it to the queue) the claim fails and RunJob returns at once; the
	// loop below then waits for that run.
	done := make(chan error, 1)

	go func() { done <- s.RunJob(ctx, id) }()

	tick := time.NewTicker(batchRetryEvery)
	defer tick.Stop()

	running := true
	label := fmt.Sprintf("Exporting %d of %d", i+1, len(in.Items))

	for {
		select {
		case rerr := <-done:
			if rerr != nil {
				return rerr
			}

			running = false
		case <-tick.C:
		}

		row, gerr := s.store.Jobs().Get(context.WithoutCancel(ctx), id)
		if gerr != nil {
			return gerr
		}

		if JobFinal(row.State) {
			return nil
		}

		if !running && ctx.Err() != nil {
			return s.parentStopped(ctx, parent.ID, id)
		}

		pw.report(ctx, (float64(i)+row.Progress)/float64(len(in.Items)), label)
	}
}

// parentStopped is what the parent returns when its context ended: a cancelled parent cancels the child it made
// last; a stopping server leaves the child queued, for the restart to run.
func (s *svc) parentStopped(ctx context.Context, parentID, childID string) error {
	bg := context.WithoutCancel(ctx)

	if row, err := s.store.Jobs().Get(bg, parentID); err == nil && row.CancelRequested {
		_ = s.cancelJob(bg, childID)
	}

	return ctx.Err()
}

// createBatchChild writes the child export job of item i, through the same limit as a JobStart. When the exports
// of the server are all busy it waits and asks again: a batch queues behind them (VV-220).
func (s *svc) createBatchChild(ctx context.Context, parent *JobRow, in *batchInput, i int, id string) error {
	it := in.Items[i]

	row := JobRow{
		ID: id, Kind: JobExport, ProjectID: parent.ProjectID, Actor: parent.Actor, State: JobQueued,
		Input: marshalOrNull(exportInput{
			Revision: in.Revision, Start: it.StartFrame, End: it.EndFrame, Lint: in.Lint, Document: in.Document,
			Format: it.Format, Warnings: it.Warnings, Preset: it.Preset, Codec: it.Codec,
		}),
	}

	lim := &JobLimit{
		Kind: JobExport, Max: s.limits.MaxExports, Actor: parent.Actor,
		MaxPerActor: s.limits.MaxExportsPerActor + queuedPerActor,
	}

	for {
		err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
			now := s.nowMs()
			row.CreatedMs, row.UpdatedMs = now, now

			if err := tx.Jobs().CreateLimited(ctx, row, lim); err != nil {
				return err
			}

			if err := tx.Audit().Append(ctx, AuditRow{
				AtMs: now, Actor: parent.Actor, Action: "job.start", ProjectID: parent.ProjectID, SubjectID: &row.ID,
			}); err != nil {
				return err
			}

			queued := row
			tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

			return nil
		})

		var full *QueueFullError

		switch {
		case err == nil, errors.Is(err, ErrExists):
			return nil
		case !errors.As(err, &full):
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(batchRetryEvery):
		}
	}
}

// finishBatch reads the children and turns them into the outcome of the parent: nothing to return when all
// succeeded, else the error of the first child that did not.
func (s *svc) finishBatch(ctx context.Context, job *JobRow, in *batchInput) (json.RawMessage, error) {
	res := s.batchState(ctx, Caller{}, job, in)

	raw, merr := json.Marshal(JobResult{Batch: res})
	if merr != nil {
		return nil, internalError()
	}

	if res.Failed == 0 && res.Cancelled == 0 {
		return raw, nil
	}

	var first *BatchItem

	for i := range res.Items {
		if it := &res.Items[i]; it.State == JobFailed || it.State == JobCancelled {
			first = it

			break
		}
	}

	e := newError(CodeExportFailed, "the batch ended with %d of %d exports done.", res.Succeeded, res.Total)
	if first != nil && first.Error != nil {
		cp := *first.Error
		cp.Details = maps.Clone(first.Error.Details)
		e = &cp
		e.Message = fmt.Sprintf(
			"Export %d of %d (%s) failed: %s",
			first.Index+1,
			res.Total,
			batchLabel(first),
			cp.Message,
		)
	}

	if e.Details == nil {
		e.Details = map[string]any{}
	}

	summary := map[string]any{
		"total": res.Total, "succeeded": res.Succeeded, "failed": res.Failed, "cancelled": res.Cancelled,
	}
	if first != nil {
		summary["item"] = first.Index
	}

	e.Details["batch"] = summary

	return nil, e
}

func batchLabel(it *BatchItem) string {
	if it.Preset != "" {
		return it.Preset
	}

	return it.Format
}

// batchState reads the children of a parent. A child that does not exist yet is queued while the parent lives,
// and cancelled once the parent is over (it never will).
func (s *svc) batchState(ctx context.Context, c Caller, job *JobRow, in *batchInput) *BatchResult {
	res := &BatchResult{Total: len(in.Items), Items: make([]BatchItem, 0, len(in.Items))}
	bg := context.WithoutCancel(ctx)

	for i := range in.Items {
		it := in.Items[i]
		item := BatchItem{
			Index: i, Preset: it.Preset, Format: formatName(it.Format), Codec: it.Codec, State: JobQueued,
		}

		id := batchChildID(job.ID, i)

		if child, err := s.store.Jobs().Get(bg, id); err == nil {
			item.JobID, item.State, item.Progress = child.ID, child.State, child.Progress
			item.Error = forCaller(c, parseError(child.Error))

			var r JobResult
			if len(child.Result) > 0 && json.Unmarshal(child.Result, &r) == nil && r.Export != nil {
				if c.OwnerUserID != "" {
					r.Export.Path = ""
				}

				item.Result = r.Export
			}
		} else if JobFinal(job.State) || job.CancelRequested {
			item.State = JobCancelled
		}

		switch item.State {
		case JobSucceeded:
			res.Succeeded++
		case JobFailed:
			res.Failed++
		case JobCancelled:
			res.Cancelled++
		}

		res.Items = append(res.Items, item)
	}

	return res
}

// attachBatch fills the children of an export_batch job into its view: the state of each, the progress of the
// whole, and the text.
func (s *svc) attachBatch(ctx context.Context, c Caller, j *Job, r *JobRow, waited bool) {
	var in batchInput
	if json.Unmarshal(r.Input, &in) != nil {
		return
	}

	res := s.batchState(ctx, c, r, &in)
	j.Result = &JobResult{Batch: res}
	j.Lint = in.Lint

	if !JobFinal(r.State) && res.Total > 0 {
		sum := 0.0

		for i := range res.Items {
			if JobFinal(res.Items[i].State) {
				sum++
			} else {
				sum += res.Items[i].Progress
			}
		}

		j.Progress = max(j.Progress, sum/float64(res.Total))
	}

	j.Text = batchText(j, res, waited)
}

func batchText(j *Job, res *BatchResult, waited bool) string {
	var b strings.Builder

	b.WriteString(jobText(j, waited))

	fmt.Fprintf(
		&b,
		"\nbatch: %d of %d done, %d failed, %d cancelled",
		res.Succeeded,
		res.Total,
		res.Failed,
		res.Cancelled,
	)

	for i := range res.Items {
		it := &res.Items[i]
		fmt.Fprintf(&b, "\n  %d. %s %s %s", it.Index+1, batchLabel(it), it.JobID, it.State)

		switch {
		case it.Result != nil:
			fmt.Fprintf(&b, " %s %dx%d %s", it.Result.VideoCodec, it.Result.Width, it.Result.Height,
				textview.Size(it.Result.Bytes))

			if it.Result.Path != "" {
				fmt.Fprintf(&b, " file %s", it.Result.Path)
			}

			for _, n := range it.Result.Notes {
				fmt.Fprintf(&b, " note: %s", n)
			}
		case it.Error != nil:
			fmt.Fprintf(&b, " error %s: %s", it.Error.Code, it.Error.Message)
		case it.State == JobRunning:
			fmt.Fprintf(&b, " %d%%", int(it.Progress*100))
		}
	}

	return b.String()
}

// jobViewCtx is jobView with the children of a batch read from the store.
func (s *svc) jobViewCtx(ctx context.Context, c Caller, r JobRow, waited bool) *Job {
	j := s.jobView(c, r, waited)

	if r.Kind == JobExportBatch {
		s.attachBatch(ctx, c, j, &r, waited)
	}

	return j
}

// cancelBatchChildren asks for the cancellation of every child of a batch that is not over, before the parent
// itself is cancelled: a child whose own cancel flag is set ends cancelled, and one whose context is cut by
// the parent would go back to the queue instead.
func (s *svc) cancelBatchChildren(ctx context.Context, parent *JobRow) {
	var in batchInput
	if json.Unmarshal(parent.Input, &in) != nil {
		return
	}

	for i := range in.Items {
		id := batchChildID(parent.ID, i)

		if child, err := s.store.Jobs().Get(ctx, id); err == nil && !JobFinal(child.State) {
			_ = s.cancelJob(ctx, id)
		}
	}
}
