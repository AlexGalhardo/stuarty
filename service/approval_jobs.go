package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

// JobWaitingApproval is the state of a job of a kind that needs approval, until a person or a rule
// approves it (SDD-015 section 5). The job is not running and has made no call.
const JobWaitingApproval = "waiting_approval"

// MaxPendingApprovals is the most requests that may wait for a person at once: a flood of requests
// by an agent must not bury the ones that matter.
const MaxPendingApprovals = 200

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
	// Prepare checks the arguments and describes the action. It makes no outbound call.
	Prepare func(ctx context.Context, req JobStartRequest) (Prepared, error)
	// Run is the job once approved. It reaches the network only through connections.Manager.With.
	Run func(ctx context.Context, run JobRun) (JobOutcome, error)
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
	row, lerr := s.loadProject(ctx, c, req.ProjectID)
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

	if terr := checkText("summary", prep.Summary, core.ValidSummary); terr != nil {
		return nil, newError(
			command.CodeInternal,
			"the job kind %s described the action with text that cannot be shown.",
			k.Name,
		)
	}

	projectID := row.ID
	action := core.Action{Kind: k.Name, Connection: prep.ConnectionID, Project: projectID, Cost: prep.Estimate}

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

		return s.jobView(c, s.stamp(job), false), nil
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
		return nil
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

// cancelWaiting is JobCancel for a job that waits for approval: the request is rejected and the job
// is cancelled, in one transaction (SDD-015 section 5).
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
		if err != nil || !ok {
			return err
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

		tx.AfterCommit(func() { s.emit(Event{Type: EventApprovalDecided, Approval: &view, At: s.now()}) })

		return s.auditApproval(ctx, tx, now, actor, "approval.cancel", &row, nil)
	})

	return err
}

// runApprovable is the handler of an approved job of a registered kind: it runs the kind and settles
// the ledger with the real cost (SDD-015 section 6).
func (s *svc) runApprovable(
	ctx context.Context,
	k *ApprovableKind,
	job *JobRow,
	pw *progress,
) (json.RawMessage, error) {
	project := ""
	if job.ProjectID != nil {
		project = *job.ProjectID
	}

	out, err := k.Run(ctx, JobRun{
		ID: job.ID, Kind: job.Kind, Actor: job.Actor, ProjectID: project, Input: job.Input,
		Report: func(ctx context.Context, p float64, msg string) { pw.report(ctx, p, msg) },
	})

	s.settleCost(ctx, job, out.Cost, err == nil)

	return out.Result, err
}

// settleCost records the real cost of an ended job. A job that spent reports what it spent, also
// when it failed afterwards; a job that succeeded and reports none is settled at its estimate; a job
// that failed having spent nothing gives its reservation back.
func (s *svc) settleCost(ctx context.Context, job *JobRow, cost *core.Money, succeeded bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()

	st, now := s.store.Approvals(), s.nowMs()

	switch {
	case cost != nil:
		_, _ = st.Settle(ctx, job.ID, max(cost.Micros, 0), now)
	case succeeded:
		if req, err := st.RequestOfJob(ctx, job.ID); err == nil {
			_, _ = st.Settle(ctx, job.ID, req.Estimate.Micros, now)
		}
	default:
		_, _ = st.Release(ctx, job.ID, now)
	}
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

	if _, pending, err := st.ListRequests(ctx, approval.Filter{State: approval.StatePending, Limit: 1}); err != nil {
		return err
	} else if pending >= MaxPendingApprovals {
		return newError("E_LIMIT_EXCEEDED", "%d requests are already waiting for a person.", pending).
			withHint("wait for the person to answer the requests that are waiting")
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
		ProjectID:    sub.action.Project,
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
	tx.AfterCommit(func() { s.emit(Event{Type: EventApprovalRequested, Approval: &requested, At: s.now()}) })

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
	tx.AfterCommit(func() { s.emit(Event{Type: EventApprovalDecided, Approval: &decided, At: s.now()}) })

	details["rule"] = out.RuleID

	return s.auditApproval(ctx, tx, now, sub.c.Actor, "approval.auto", &row, details)
}
