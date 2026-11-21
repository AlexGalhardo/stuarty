package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

// JobWaitingApproval is the state of a job of a kind that needs approval, until a person or a rule
// approves it (SDD-015 section 5). The job is not running and has made no call.
const JobWaitingApproval = "waiting_approval"

// The most requests that may wait for a person at once (SDD-015 section 5). A flood of requests by
// one agent must not bury the ones that matter or block everyone else, so each submitter has a share
// of its own: the user in network mode (every key of a user counts together), else the actor. The
// workspace as a whole has a much larger bound.
const (
	MaxPendingPerSubmitter = 50
	MaxPendingApprovals    = 1000
)

// Prepared is what a job kind says about an action before it is approved (SDD-015 section 5).
type Prepared struct {
	// Summary tells the person what will be sent or generated, at most 500 characters of plain text.
	Summary string
	// ConnectionID is the destination connection of the action; empty when it has none.
	ConnectionID string
	// Estimate is the estimated cost; zero micros for an action that costs nothing.
	Estimate core.Money
	// Input is what the job needs to run; it is stored in the job and given to Run.
	Input json.RawMessage
}

// JobRun is the job as Run sees it.
type JobRun struct {
	ID        string
	Kind      string
	Actor     string
	ProjectID string
	Input     json.RawMessage
	// Report records the progress of the job, 0 to 1, and a short message.
	Report func(ctx context.Context, progress float64, message string)
	// SaveInput replaces the input the job is given, durably: a run that is interrupted and queued again starts
	// with the saved input, so a job that submits work to a provider saves its task id here before it waits.
	SaveInput func(ctx context.Context, input json.RawMessage) error
	// CancelRequested reports whether a person asked for this job to be cancelled (a shutdown is not that).
	CancelRequested func(ctx context.Context) bool
}

// JobOutcome is what a finished job reports.
type JobOutcome struct {
	// Result is stored on the job row.
	Result json.RawMessage
	// Cost is the real cost, when the job spent. A job that succeeds and reports none is settled at its
	// estimate: the daily ceiling must never count less than what may have been spent.
	Cost *core.Money
}

// ApprovableKind is a job kind that needs approval. A feature epic registers its kinds in
// Options.JobKinds (publishing, generation); the product registers none of its own here.
type ApprovableKind struct {
	// Name is the job kind: 1 to 80 characters of [a-z0-9_.:-], not one of the built-in kinds.
	Name string
	// Scope is the scope needed to submit the kind; render when empty.
	Scope string
	// ProjectWide marks a kind whose work belongs to the project and not to one of its timelines (a generation, a
	// draft): with no timeline_id the job is kept under the first active timeline, whatever the number of
	// timelines, and the caller is never asked which (SDD-016 section 6, SDD-003 section 1.7).
	ProjectWide bool
	// Anchor is optional: the timeline the job must be kept under because its work is made from that timeline (a
	// post, from the timeline of its export), so that trashing that timeline and no other ends the job. An empty
	// answer leaves the job where ProjectWide or the request put it.
	Anchor func(ctx context.Context, c Caller, req *JobStartRequest) string
	// Prepare checks the arguments and describes the action. It makes no outbound call.
	Prepare func(ctx context.Context, req JobStartRequest) (Prepared, error)
	// Run is the job once approved. It reaches the network only through connections.Manager.With.
	Run func(ctx context.Context, run JobRun) (JobOutcome, error)
	// Abandon is optional. It is called when a job of the kind is cancelled while queued again after a shutdown
	// interrupted a run that had started work at a provider (the task of a video, kept in the saved input): it
	// asks the provider to stop that work and returns the cost that is owed for it, or nil when nothing is owed.
	Abandon func(ctx context.Context, job *JobRow) *core.Money
}

var builtinKinds = []string{JobIngest, JobTranscribe, JobExport, JobFrame}

// registerKinds checks and indexes the approvable kinds of the options.
func registerKinds(list []ApprovableKind) (map[string]*ApprovableKind, error) {
	out := make(map[string]*ApprovableKind, len(list))

	for i := range list {
		k := list[i]

		switch {
		case !core.ValidKind(k.Name) || slices.Contains(builtinKinds, k.Name):
			return nil, fmt.Errorf("service: %q is not a name for an approvable job kind", k.Name)
		case out[k.Name] != nil:
			return nil, fmt.Errorf("service: the job kind %q is registered twice", k.Name)
		case k.Prepare == nil || k.Run == nil:
			return nil, fmt.Errorf("service: the job kind %q needs Prepare and Run", k.Name)
		case k.Scope != "" && !slices.Contains(AllScopes, k.Scope):
			return nil, fmt.Errorf("service: the job kind %q names the unknown scope %q", k.Name, k.Scope)
		}

		if k.Scope == "" {
			k.Scope = ScopeRender
		}

		out[k.Name] = &k
	}

	return out, nil
}

// startApprovable is JobStart for a registered kind (SDD-015 section 5), after JobStart authorized the
// caller with the scope of the kind and applied the heavy-operation limit.
func (s *svc) startApprovable(ctx context.Context, c Caller, req JobStartRequest, k *ApprovableKind) (*Job, error) {
	row, lerr := s.loadProjectScope(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if row.State == ProjectTrashed {
		return nil, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	prep, perr := k.Prepare(ctx, req)
	if perr != nil {
		return nil, asError(perr)
	}

	// The summary is usually made of what the caller sent (the text of a post, a prompt), so a refusal
	// is the caller's to fix, not a bug to report (SDD-015 section 5).
	if terr := checkText("summary", prep.Summary, core.ValidSummary); terr != nil {
		return nil, newError(command.CodeValidation,
			"the action of %s cannot be shown to a person: its summary is empty, longer than %d characters, "+
				"or has a control or invisible character.", k.Name, core.MaxSummary).
			withPath("args").
			withHint("remove control and invisible characters from the text of the action, and shorten it")
	}

	projectID := row.ID
	action := core.Action{
		Kind: k.Name, Connection: prep.ConnectionID, Project: costProject(ctx, s.store, projectID), Cost: prep.Estimate,
	}

	job := JobRow{
		ID: s.newIDs(
			"job",
		),
		Kind:      k.Name,
		ProjectID: &projectID,
		Actor:     c.Actor,
		State:     JobWaitingApproval,
		Input:     prep.Input,
	}

	if req.DryRun {
		out, err := approval.Evaluate(ctx, s.store.Approvals(), action, s.now())
		if err != nil {
			return nil, asError(err)
		}

		if out.Verdict == core.Refused {
			return nil, costCeilingError(out.Refusal)
		}

		if out.Verdict == core.AutoApproved {
			job.State = JobQueued
		}

		return s.dryRunView(c, job), nil
	}

	sub := &submission{c: c, k: k, prep: prep, action: action, job: job}

	if werr := s.store.Write(
		ctx,
		func(ctx context.Context, tx Tx) error { return s.submit(ctx, tx, sub) },
	); werr != nil {
		return nil, callerError(c, approvalError(werr))
	}

	cur, gerr := s.store.Jobs().Get(ctx, job.ID)
	if gerr != nil {
		return nil, asError(gerr)
	}

	return s.jobView(c, cur, false), nil
}

// releaseJob lets the job of an approved request go (SDD-015 section 5): it becomes queued and is
// handed to the queue in the same transaction. A request with no job has nothing to release.
func (s *svc) releaseJob(ctx context.Context, tx Tx, row *approval.Row, now int64) error {
	if row.JobID == "" {
		return nil
	}

	ok, err := tx.Jobs().Release(ctx, row.JobID, now)
	if err != nil {
		return err
	}

	if !ok {
		return newError(command.CodeInvalidState, "the job of the request is no longer waiting.").
			withHint("it was cancelled; submit the action again")
	}

	job, err := tx.Jobs().Get(ctx, row.JobID)
	if err != nil {
		return err
	}

	// Registered before the enqueue: an inline queue runs the job in its own after-commit hook, and
	// the queued event must come first.
	tx.AfterCommit(func() { s.emitJob(ctx, &job) })

	if s.enqueue != nil {
		return s.enqueue.Enqueue(ctx, tx, queueKindFor(job.Kind), job.ID)
	}

	return nil
}

// rejectJob ends the job of a request that was rejected, cancelled or expired, with why as its error.
func (s *svc) rejectJob(ctx context.Context, tx Tx, row *approval.Row, why *Error, now int64) error {
	if row.JobID == "" {
		// A request with no job is the request of a scheduled post: its target hears of it (SDD-020 section 26).
		return s.targetDeclined(ctx, tx, row, why)
	}

	ok, err := tx.Jobs().FinishWaiting(ctx, row.JobID, JobCancelled, errorJSON(why), now)
	if err != nil || !ok {
		return err
	}

	job, err := tx.Jobs().Get(ctx, row.JobID)
	if err != nil {
		return err
	}

	tx.AfterCommit(func() { s.emitJob(ctx, &job) })

	return nil
}

// errNoLongerWaiting is a job that left waiting_approval between the read of the cancel and its
// write: a person or a rule approved it, so the caller cancels it as a queued job.
var errNoLongerWaiting = errors.New("service: the job is no longer waiting for approval")

// cancelWaiting is JobCancel for a job that waits for approval: the request is rejected and the job
// is cancelled, in one transaction (SDD-015 section 5). errNoLongerWaiting when it was approved first.
func (s *svc) cancelWaiting(ctx context.Context, actor string, job *JobRow) error {
	var view approval.Request

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		row, err := tx.Approvals().GetRequest(ctx, job.ApprovalID)
		if err != nil {
			if errors.Is(err, approval.ErrNotFound) {
				// A waiting job with no request cannot be decided by anyone: let it go.
				_, ferr := tx.Jobs().
					FinishWaiting(ctx, job.ID, JobCancelled, errorJSON(newError(CodeCancelled, "the job was cancelled.")), s.nowMs())

				return ferr
			}

			return err
		}

		now := s.nowMs()
		why := "cancelled by " + actor

		ok, err := tx.Approvals().Decide(ctx, row.ID, approval.StateRejected, actor, "", why, now)
		if err != nil {
			return err
		}

		if !ok {
			return errNoLongerWaiting
		}

		row.State, row.DecidedBy, row.Reason, row.DecidedMs = approval.StateRejected, actor, why, &now
		view = approval.View(&row)

		if err := s.rejectJob(
			ctx,
			tx,
			&row,
			newError(CodeCancelled, "the job was cancelled by %s before it was approved.", actor),
			now,
		); err != nil {
			return err
		}

		tx.AfterCommit(func() {
			s.emit(Event{Type: EventApprovalDecided, Approval: &view, Owner: row.OwnerUserID, At: s.now()})
		})

		return s.auditApproval(ctx, tx, now, actor, "approval.cancel", &row, nil)
	})

	return err
}

// cancelWaitingJob is JobCancel for a job read in waiting_approval: its request is rejected (SDD-015
// section 5). One approved since it was read is cancelled as a queued job, so a cancel never lets it run.
func (s *svc) cancelWaitingJob(ctx context.Context, actor string, job *JobRow) error {
	err := s.cancelWaiting(ctx, actor, job)
	if !errors.Is(err, errNoLongerWaiting) {
		return err
	}

	if err := s.cancelJob(ctx, job.ID); err != nil {
		return err
	}

	s.waitJob(ctx, job.ID, cancelWait)

	return nil
}

// runApprovable is the handler of an approved job of a registered kind: it makes sure the cost is
// reserved, runs the kind and returns the cost the kind reports. The ledger is settled by finishCost,
// once the outcome of the job is known (SDD-015 section 6).
func (s *svc) runApprovable(
	ctx context.Context,
	k *ApprovableKind,
	job *JobRow,
	pw *progress,
) (json.RawMessage, *core.Money, error) {
	if err := s.ensureReserved(ctx, job); err != nil {
		return nil, nil, err
	}

	project := ""
	if job.ProjectID != nil {
		project = *job.ProjectID
	}

	out, err := k.Run(ctx, JobRun{
		ID: job.ID, Kind: job.Kind, Actor: job.Actor, ProjectID: project, Input: job.Input,
		Report: func(ctx context.Context, p float64, msg string) { pw.report(ctx, p, msg) },
		SaveInput: func(ctx context.Context, input json.RawMessage) error {
			ok, err := s.store.Jobs().SaveInput(ctx, job.ID, input, s.nowMs())
			if err == nil && !ok {
				err = newError(command.CodeInvalidState, "the job is no longer running.")
			}

			return err
		},
		CancelRequested: func(ctx context.Context) bool {
			row, err := s.store.Jobs().Get(ctx, job.ID)

			return err == nil && row.CancelRequested
		},
	})

	return out.Result, out.Cost, err
}

// ensureReserved makes a job that is about to spend have its cost reserved. Approving reserves it and
// an interruption keeps it, so this finds it; when it is gone (released by an earlier end, or lost) the
// ceilings are checked again and it is reserved before anything is spent, and a job that no longer fits
// is refused without running.
func (s *svc) ensureReserved(ctx context.Context, job *JobRow) error {
	return s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		st := tx.Approvals()

		req, err := st.RequestOfJob(ctx, job.ID)
		if err != nil || req.Estimate.Micros == 0 {
			return nil //nolint:nilerr // a job with no request or no cost reserves nothing
		}

		if ok, err := st.Reserved(ctx, job.ID); err != nil || ok {
			return err
		}

		now := s.nowMs()
		action := core.Action{
			Kind: req.Kind, Connection: req.ConnectionID,
			Project: costProject(ctx, tx, req.ProjectID), Cost: req.Estimate,
		}

		out, err := approval.Evaluate(ctx, st, action, time.UnixMilli(now))
		if err != nil {
			return err
		}

		if out.Verdict == core.Refused {
			return costCeilingError(out.Refusal)
		}

		return st.Reserve(ctx, s.ledgerEntry(&req, req.Estimate.Micros, now))
	})
}

// costProject is the project a rule and a ceiling are matched against: they are written for a project, while a
// request and a ledger entry carry the id of the timeline the job is kept on, so a job on a second timeline does not
// leave the ceiling of its project (VV-315, SDD-015 section 3).
func costProject(ctx context.Context, r interface{ Productions() ProductionRepo }, id string) string {
	if owner, err := r.Productions().ProjectOfTimeline(ctx, id); err == nil {
		return owner
	}

	return id
}

func (s *svc) ledgerEntry(req *approval.Row, estimate, now int64) *approval.Entry {
	return &approval.Entry{
		ID: s.newIDs(
			"led",
		),
		RequestID:      req.ID,
		JobID:          req.JobID,
		ConnectionID:   req.ConnectionID,
		ProjectID:      req.ProjectID,
		Kind:           req.Kind,
		Currency:       req.Estimate.Currency,
		EstimateMicros: estimate,
		Day:            approval.Day(time.UnixMilli(now)),
		CreatedMs:      now,
	}
}

// finishCost settles the ledger once the outcome of a run is known, from the state the job reached
// (SDD-015 section 6). A job that succeeded is settled at the cost it reports, or at its estimate when
// it reports none; one that failed or was cancelled is settled at the cost it reports, and gives its
// reservation back only when it spent nothing; one that was requeued (a shutdown interrupted it) keeps
// its reservation, since it will run again and must stay counted against the ceilings, and what the
// interrupted run reports as spent is recorded besides.
func (s *svc) finishCost(ctx context.Context, job *JobRow, cost *core.Money) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()

	cur, err := s.store.Jobs().Get(ctx, job.ID)
	if err != nil {
		return
	}

	_ = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		st, now := tx.Approvals(), s.nowMs()

		switch {
		case cur.State == JobSucceeded || JobFinal(cur.State) && cost != nil:
			return s.recordActual(ctx, st, job.ID, cost, now)
		case JobFinal(cur.State):
			_, err := st.Release(ctx, job.ID, now)

			return err
		case cost != nil && cost.Micros > 0:
			// Requeued after it spent: what this run spent is recorded now, and the reservation stays
			// for the run that follows, so the day never counts less than was spent.
			return s.recordInterrupted(ctx, st, job.ID, cost.Micros, now)
		}

		return nil
	})
}

// recordInterrupted records what a run spent before a shutdown interrupted it, as a settled entry of
// its own, without touching the reservation of the job.
func (s *svc) recordInterrupted(ctx context.Context, st approval.Store, jobID string, spent, now int64) error {
	req, err := st.RequestOfJob(ctx, jobID)
	if err != nil {
		return nil //nolint:nilerr // a job with no request has no ledger
	}

	e := s.ledgerEntry(&req, 0, now)
	e.ActualMicros, e.SettledMs = &spent, &now

	return st.Record(ctx, e)
}

// recordActual settles the reserved entry of a job at its real cost; with none reserved it records a
// settled entry, so what was spent is never lost.
func (s *svc) recordActual(ctx context.Context, st approval.Store, jobID string, cost *core.Money, now int64) error {
	req, err := st.RequestOfJob(ctx, jobID)
	if err != nil {
		return nil //nolint:nilerr // a job with no request has no ledger
	}

	actual := req.Estimate.Micros
	if cost != nil {
		actual = max(cost.Micros, 0)
	}

	if ok, err := st.Settle(ctx, jobID, actual, now); err != nil || ok || actual == 0 {
		return err
	}

	if err := st.Reserve(ctx, s.ledgerEntry(&req, actual, now)); err != nil {
		return err
	}

	_, err = st.Settle(ctx, jobID, actual, now)

	return err
}

// releaseUnrun gives back the reservation of an approved job that ended before it ran.
func (s *svc) releaseUnrun(ctx context.Context, j *JobRow) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()

	if _, err := s.store.Approvals().Release(ctx, j.ID, s.nowMs()); err != nil {
		s.logJob(ctx, slog.LevelWarn, "the reservation of a cancelled job was not released", j,
			slog.String("error", err.Error()))
	}
}

// settleQueuedCancel settles an approved job cancelled while queued. It did not run, so its reservation goes back,
// unless an earlier run that a shutdown interrupted left work at a provider: then the kind abandons that work and
// what it says is owed is recorded (SDD-015 section 6).
func (s *svc) settleQueuedCancel(ctx context.Context, k *ApprovableKind, j *JobRow) {
	if k.Abandon != nil {
		if cost := k.Abandon(ctx, j); cost != nil {
			s.finishCost(ctx, j, cost)

			return
		}
	}

	s.releaseUnrun(ctx, j)
}

// kindOf is the registered approvable kind of a job kind, or nil.
func (s *svc) kindOf(kind string) *ApprovableKind { return s.kinds[kind] }

// submission is one request to run a job of a registered kind, on its way into the queue.
type submission struct {
	c      Caller
	k      *ApprovableKind
	prep   Prepared
	action core.Action
	job    JobRow
}

// submit is the transaction of a submission (SDD-015 section 5): the decision, the request and the
// waiting job are written together, and a rule that covers the action approves it in the same
// transaction.
func (s *svc) submit(ctx context.Context, tx Tx, sub *submission) error {
	now, st := s.nowMs(), tx.Approvals()
	prep, job := sub.prep, &sub.job

	if prep.ConnectionID != "" {
		if _, err := tx.Connections().Get(ctx, prep.ConnectionID); err != nil {
			return err
		}
	}

	out, err := approval.Evaluate(ctx, st, sub.action, time.UnixMilli(now))
	if err != nil {
		return err
	}

	if out.Verdict == core.Refused {
		return costCeilingError(out.Refusal)
	}

	if err := s.checkQueueShare(ctx, st, &sub.c); err != nil {
		return err
	}

	row := approval.Row{
		ID: s.newIDs(
			"apr",
		),
		Actor:        sub.c.Actor,
		OwnerUserID:  sub.c.OwnerUserID,
		Kind:         sub.k.Name,
		Summary:      prep.Summary,
		ConnectionID: prep.ConnectionID,
		ProjectID:    *job.ProjectID,
		JobID:        job.ID,
		PayloadRef:   "job:" + job.ID,
		Estimate:     prep.Estimate,
		State:        approval.StatePending,
		CreatedMs:    now,
		ExpiresMs:    now + approval.ExpireAfter.Milliseconds(),
	}

	job.ApprovalID, job.CreatedMs, job.UpdatedMs = row.ID, now, now

	if err := st.InsertRequest(ctx, &row); err != nil {
		return err
	}

	if err := tx.Jobs().CreateWaiting(ctx, *job); err != nil {
		return err
	}

	requested := approval.View(&row)
	tx.AfterCommit(func() {
		s.emit(Event{Type: EventApprovalRequested, Approval: &requested, Owner: row.OwnerUserID, At: s.now()})
	})

	details := map[string]any{"kind": row.Kind, "connection_id": prep.ConnectionID, "estimate": prep.Estimate}

	if out.Verdict == core.NeedsPerson {
		waiting := *job
		tx.AfterCommit(func() { s.emitJob(ctx, &waiting) })

		return s.auditApproval(ctx, tx, now, sub.c.Actor, "approval.request", &row, details)
	}

	if err := s.approve(ctx, tx, &row, "", out.RuleID, now); err != nil {
		return err
	}

	decided := approval.View(&row)
	tx.AfterCommit(func() {
		s.emit(Event{Type: EventApprovalDecided, Approval: &decided, Owner: row.OwnerUserID, At: s.now()})
	})

	details["rule"] = out.RuleID

	return s.auditApproval(ctx, tx, now, sub.c.Actor, "approval.auto", &row, details)
}

// checkQueueShare refuses a submission when the submitter already has its share of requests waiting,
// or the workspace as a whole is full.
func (s *svc) checkQueueShare(ctx context.Context, st approval.Store, c *Caller) error {
	mine := approval.Filter{State: approval.StatePending, Limit: 1, Actor: c.Actor}
	if c.OwnerUserID != "" {
		mine = approval.Filter{State: approval.StatePending, Limit: 1, OwnerUserID: c.OwnerUserID}
	}

	if _, n, err := st.ListRequests(ctx, mine); err != nil {
		return err
	} else if n >= MaxPendingPerSubmitter {
		return newError("E_LIMIT_EXCEEDED", "you already have %d requests waiting for a person.", n).
			withHint("wait for the person to answer them, or cancel the ones you no longer need with job_cancel")
	}

	if _, n, err := st.ListRequests(ctx, approval.Filter{State: approval.StatePending, Limit: 1}); err != nil {
		return err
	} else if n >= MaxPendingApprovals {
		return newError("E_LIMIT_EXCEEDED", "%d requests are already waiting for a person.", n).
			withHint("try again when the person has answered some of them")
	}

	return nil
}
