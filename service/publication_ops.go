package service

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"slices"
	"strings"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/approval"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// KindPublishScheduled is the kind of the approval request of a scheduled post (SDD-020 section 26). A rule
// the person wrote for this kind approves it without asking.
const KindPublishScheduled = "publish.scheduled"

// EventPublicationUpdated is the event of a publication that changed (SDD-020 section 29).
const EventPublicationUpdated = "publication.updated"

// publicationRefPrefix starts the payload_ref of the request of a target.
const publicationRefPrefix = "publication:"

// Publication is a publication with its targets (SDD-020 section 23).
type Publication = pub.Publication

// PublicationCreateRequest plans a publication; every target starts as a draft.
type PublicationCreateRequest = pub.CreateInput

// PublicationUpdateRequest changes a publication; what it leaves out stays as it is.
type PublicationUpdateRequest = pub.UpdateInput

// PublicationGetRequest names a publication.
type PublicationGetRequest struct {
	ID string `json:"id"`
}

// PublicationListRequest selects publications: by project, by the state or the network of a target, or by a
// time range (from inclusive, to exclusive, RFC 3339).
type PublicationListRequest struct {
	ProjectID string `json:"project_id,omitempty"`
	State     string `json:"state,omitempty"`
	Network   string `json:"network,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	// Zone is the IANA zone a date or a local time of From and To is read in; empty is UTC.
	Zone   string `json:"zone,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// PublicationListResult is a page of publications.
type PublicationListResult struct {
	Publications []Publication `json:"publications"`
	Total        int           `json:"total"`
}

// PublicationScheduleRequest gives a target its time and asks for approval.
type PublicationScheduleRequest struct {
	ID                string `json:"id"`
	TargetID          string `json:"target_id,omitempty"`
	At                string `json:"at,omitempty"`
	Zone              string `json:"zone,omitempty"`
	LateWindowMinutes *int   `json:"late_window_minutes,omitempty"`
}

// PublicationTargetRequest names a target of a publication.
type PublicationTargetRequest struct {
	ID       string `json:"id"`
	TargetID string `json:"target_id,omitempty"`
}

// PublicationSendNowRequest asks for approval to send a target as soon as it is approved.
type PublicationSendNowRequest PublicationTargetRequest

// PublicationRetryRequest asks for approval to send a failed target again.
type PublicationRetryRequest PublicationTargetRequest

// PublicationCancelRequest cancels a target, or every one that has not gone out.
type PublicationCancelRequest PublicationTargetRequest

// publicationError maps what the calendar refused to the codes of SDD-000 section 8.
func publicationError(err error) error {
	if err == nil {
		return nil
	}

	if pe, ok := errors.AsType[*pub.Error](err); ok {
		code := command.CodeValidation
		if pe.Kind == pub.KindInvalidState {
			code = command.CodeInvalidState
		}

		e := newError(code, "%s", pe.Message).withPath(pe.Path)
		if pe.Hint != "" {
			e = e.withHint("%s", pe.Hint)
		}

		if len(pe.Issues) > 0 {
			e = e.withDetail("issues", pe.Issues)
		}

		return e
	}

	if errors.Is(err, pub.ErrNotFound) {
		return newError(command.CodeNotFound, "the publication or the target does not exist.").
			withHint("list the publications with publication:list")
	}

	return approvalError(err)
}

// publicationOf reads a publication the caller may see. A missing one, one of another project than the key's
// and one of a project the caller may not see look the same.
func (s *svc) publicationOf(ctx context.Context, op, id string) (Caller, *Publication, error) {
	c, err := authorize(ctx, op, "")
	if err != nil {
		return Caller{}, nil, err
	}

	p, err := s.pubs.Get(ctx, id)
	if err != nil {
		return Caller{}, nil, publicationError(err)
	}

	if c.ProjectID != "" && c.ProjectID != p.ProjectID {
		return Caller{}, nil, publicationError(pub.ErrNotFound)
	}

	row, lerr := s.loadProjectScope(ctx, c, p.ProjectID)
	if lerr != nil {
		return Caller{}, nil, publicationError(pub.ErrNotFound)
	}

	if row.State == ProjectTrashed && op != "PublicationGet" && op != "PublicationList" {
		return Caller{}, nil, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	return c, p, nil
}

// PublicationCreate plans a publication.
func (s *svc) PublicationCreate(ctx context.Context, req PublicationCreateRequest) (*Publication, error) {
	c, err := authorize(ctx, "PublicationCreate", req.ProjectID)
	if err != nil {
		return nil, err
	}

	// A publication belongs to the project, which may have no timeline yet (SDD-005 section 5.12.2).
	row, lerr := s.loadProjectScope(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if row.State == ProjectTrashed {
		return nil, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	req.ProjectID = row.ID

	p, err := s.pubs.Create(ctx, req)
	if err != nil {
		return nil, callerError(c, publicationError(err))
	}

	return p, nil
}

// PublicationGet reads one publication.
func (s *svc) PublicationGet(ctx context.Context, req PublicationGetRequest) (*Publication, error) {
	_, p, err := s.publicationOf(ctx, "PublicationGet", req.ID)

	return p, err
}

// PublicationList lists publications.
func (s *svc) PublicationList(ctx context.Context, req PublicationListRequest) (*PublicationListResult, error) {
	c, err := authorize(ctx, "PublicationList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	f := pub.Filter{
		ProjectID: req.ProjectID, State: corepub.TargetState(req.State), Network: req.Network, Limit: req.Limit,
		Offset: req.Offset,
	}

	if c.ProjectID != "" {
		f.ProjectID = c.ProjectID
	}

	if c.OwnerUserID != "" && !hasScope(c, ScopeAdmin) {
		f.OwnerUserID = c.OwnerUserID
	}

	for _, t := range []struct {
		raw  string
		dst  **time.Time
		path string
	}{{req.From, &f.From, "from"}, {req.To, &f.To, "to"}} {
		if t.raw == "" {
			continue
		}

		v, perr := corepub.ParseBound(t.raw, req.Zone)
		if perr != nil {
			return nil, validationAt(
				t.path,
				"must be a date such as 2026-11-01, or a time such as 2026-11-01T00:00:00Z.",
			)
		}

		*t.dst = &v
	}

	list, total, lerr := s.pubs.List(ctx, f)
	if lerr != nil {
		return nil, publicationError(lerr)
	}

	if list == nil {
		list = []Publication{}
	}

	return &PublicationListResult{Publications: list, Total: total}, nil
}

func hasScope(c Caller, scope string) bool { return slices.Contains(c.Scopes, scope) }

// PublicationUpdate changes a publication.
func (s *svc) PublicationUpdate(ctx context.Context, req PublicationUpdateRequest) (*Publication, error) {
	c, _, err := s.publicationOf(ctx, "PublicationUpdate", req.ID)
	if err != nil {
		return nil, err
	}

	p, uerr := s.pubs.Update(ctx, req)
	if uerr != nil {
		return nil, callerError(c, publicationError(uerr))
	}

	return p, nil
}

// PublicationSchedule gives a target its time and asks for approval.
func (s *svc) PublicationSchedule(ctx context.Context, req PublicationScheduleRequest) (*Publication, error) {
	c, _, err := s.publicationOf(ctx, "PublicationSchedule", req.ID)
	if err != nil {
		return nil, err
	}

	p, serr := s.pubs.Schedule(ctx, pub.ScheduleInput(req))
	if serr != nil {
		return nil, callerError(c, publicationError(serr))
	}

	return p, nil
}

// PublicationSendNow asks for approval to send a target as soon as it is approved.
func (s *svc) PublicationSendNow(ctx context.Context, req PublicationSendNowRequest) (*Publication, error) {
	c, _, err := s.publicationOf(ctx, "PublicationSendNow", req.ID)
	if err != nil {
		return nil, err
	}

	p, serr := s.pubs.SendNow(ctx, pub.TargetRef(req))
	if serr != nil {
		return nil, callerError(c, publicationError(serr))
	}

	return p, nil
}

// PublicationRetry asks for approval to send a failed target again.
func (s *svc) PublicationRetry(ctx context.Context, req PublicationRetryRequest) (*Publication, error) {
	c, _, err := s.publicationOf(ctx, "PublicationRetry", req.ID)
	if err != nil {
		return nil, err
	}

	p, serr := s.pubs.Retry(ctx, pub.TargetRef(req))
	if serr != nil {
		return nil, callerError(c, publicationError(serr))
	}

	return p, nil
}

// PublicationCancel cancels a target, or every one that has not gone out.
func (s *svc) PublicationCancel(ctx context.Context, req PublicationCancelRequest) (*Publication, error) {
	c, _, err := s.publicationOf(ctx, "PublicationCancel", req.ID)
	if err != nil {
		return nil, err
	}

	p, cerr := s.pubs.Cancel(ctx, pub.TargetRef(req))
	if cerr != nil {
		return nil, callerError(c, publicationError(cerr))
	}

	return p, nil
}

// SchedulerOf returns the publication scheduler of a Service made by New. The server runs it next to the
// queue workers (SDD-020 section 27); a command that is not a server never does.
func SchedulerOf(s Service) (*pub.Scheduler, bool) {
	v, ok := s.(*svc)
	if !ok || v.sched == nil {
		return nil, false
	}

	return v.sched, true
}

// svcClock is the clock of the scheduler: the service clock to read, the system timer to wait.
type svcClock struct{ now func() time.Time }

func (c svcClock) Now() time.Time                         { return c.now() }
func (c svcClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// pubEnv is the service as the calendar sees it.
type pubEnv struct{ s *svc }

func (e pubEnv) Store() pub.Store { return e.s.store.Publications() }

// Now is the instant in the form the store keeps it, UTC to the millisecond, so that what a create answers is what
// every later read answers (VV-285).
func (e pubEnv) Now() time.Time             { return e.s.now().UTC().Truncate(time.Millisecond) }
func (e pubEnv) NewID(prefix string) string { return e.s.newIDs(prefix) }

func (e pubEnv) Actor(ctx context.Context) pub.Actor {
	c, _ := CallerFrom(ctx)

	return pub.Actor{ID: c.Actor, OwnerUserID: c.OwnerUserID}
}

func (e pubEnv) Write(ctx context.Context, fn func(ctx context.Context, tx pub.Tx) error) error {
	return e.s.store.Write(ctx, func(ctx context.Context, tx Tx) error { return fn(ctx, e.s.pubTx(tx)) })
}

// pubTx is a write transaction of the service, as the calendar sees it.
type pubTx struct {
	s  *svc
	tx Tx
}

func (s *svc) pubTx(tx Tx) *pubTx { return &pubTx{s: s, tx: tx} }

func (t *pubTx) Store() pub.Store { return t.tx.Publications() }

func (t *pubTx) Emit(p pub.Publication) {
	owner := p.OwnerUserID

	t.tx.AfterCommit(func() {
		t.s.emit(
			Event{Type: EventPublicationUpdated, ProjectID: p.ProjectID, Publication: &p, Owner: owner, At: t.s.now()},
		)

		// An approval or a change may have made a target due: the loop looks again now, not at its next poll.
		if t.s.sched != nil {
			t.s.sched.Wake()
		}
	})
}

// publicationRef reads the payload_ref of the request of a target.
func publicationRef(ref string) (targetID string, ok bool) {
	rest, found := strings.CutPrefix(ref, publicationRefPrefix)
	if !found {
		return "", false
	}

	publicationID, targetID, ok := strings.Cut(rest, "/")

	return targetID, ok && publicationID != "" && targetID != ""
}

// ledgerKey is what the cost ledger keeps a reservation under: the job of the request, or the target of a
// scheduled post.
func ledgerKey(r *approval.Row) string {
	if r.JobID != "" {
		return r.JobID
	}

	if target, ok := publicationRef(r.PayloadRef); ok {
		return target
	}

	return ""
}

// Ask makes the approval request of a target: the same decision a job of a kind that needs approval goes
// through (SDD-015 section 5), with no job. A ceiling refuses it, a rule may approve it at once, and
// otherwise it waits for a person.
func (t *pubTx) Ask(ctx context.Context, a pub.Ask) error {
	s := t.s

	c, ok := CallerFrom(ctx)
	if !ok {
		return newError(CodeUnauthorized, "no caller.")
	}

	if terr := checkText("summary", a.Summary, core.ValidSummary); terr != nil {
		return newError(command.CodeValidation,
			"the post cannot be shown to a person: its summary is empty, longer than %d characters, "+
				"or has a control or invisible character.", core.MaxSummary).
			withPath("fields").
			withHint("remove control and invisible characters from the text of the post, and shorten it")
	}

	st, now := t.tx.Approvals(), s.nowMs()

	if _, err := t.tx.Connections().Get(ctx, a.ConnectionID); err != nil {
		return err
	}

	owner, oerr := costProject(ctx, t.tx, a.ProjectID)
	if oerr != nil {
		return oerr
	}

	estimate := core.Money{Micros: a.CostMicros, Currency: a.Currency}
	action := core.Action{Kind: KindPublishScheduled, Connection: a.ConnectionID, Project: owner, Cost: estimate}

	out, err := approval.Evaluate(ctx, st, action, time.UnixMilli(now))
	if err != nil {
		return err
	}

	if out.Verdict == core.Refused {
		return costCeilingError(out.Refusal)
	}

	if err := s.checkQueueShare(ctx, st, &c); err != nil {
		return err
	}

	row := approval.Row{
		ID:           a.ApprovalID,
		Actor:        c.Actor,
		OwnerUserID:  c.OwnerUserID,
		Kind:         KindPublishScheduled,
		Summary:      a.Summary,
		ConnectionID: a.ConnectionID,
		ProjectID:    a.ProjectID,
		PayloadRef:   publicationRefPrefix + a.PublicationID + "/" + a.TargetID,
		Estimate:     estimate,
		State:        approval.StatePending,
		CreatedMs:    now,
		ExpiresMs:    now + approval.ExpireAfter.Milliseconds(),
	}

	if err := st.InsertRequest(ctx, &row); err != nil {
		return err
	}

	requested := approval.View(&row)
	t.tx.AfterCommit(func() {
		s.emit(Event{Type: EventApprovalRequested, Approval: &requested, Owner: row.OwnerUserID, At: s.now()})
	})

	details := map[string]any{"kind": row.Kind, "connection_id": a.ConnectionID, "estimate": estimate}

	if out.Verdict == core.NeedsPerson {
		return s.auditApproval(ctx, t.tx, now, c.Actor, "approval.request", &row, details)
	}

	if err := s.approve(ctx, t.tx, &row, "", out.RuleID, now); err != nil {
		return err
	}

	decided := approval.View(&row)
	t.tx.AfterCommit(func() {
		s.emit(Event{Type: EventApprovalDecided, Approval: &decided, Owner: row.OwnerUserID, At: s.now()})
	})

	details["rule"] = out.RuleID

	return s.auditApproval(ctx, t.tx, now, c.Actor, "approval.auto", &row, details)
}

// Withdraw rejects the request of a target that was changed or cancelled while it waited.
func (t *pubTx) Withdraw(ctx context.Context, approvalID, reason string) error {
	s := t.s

	row, err := t.tx.Approvals().GetRequest(ctx, approvalID)
	if err != nil {
		if errors.Is(err, approval.ErrNotFound) {
			return nil
		}

		return err
	}

	if row.State != approval.StatePending {
		return nil
	}

	now := s.nowMs()
	actor := "system"

	if c, ok := CallerFrom(ctx); ok {
		actor = c.Actor
	}

	ok, err := t.tx.Approvals().Decide(ctx, row.ID, approval.StateRejected, actor, "", reason, now)
	if err != nil || !ok {
		return err
	}

	row.State, row.DecidedBy, row.Reason, row.DecidedMs = approval.StateRejected, actor, reason, &now
	view := approval.View(&row)

	if err := s.rejectJob(ctx, t.tx, &row, newError(CodeCancelled, "%s", reason), now); err != nil {
		return err
	}

	t.tx.AfterCommit(func() {
		s.emit(Event{Type: EventApprovalDecided, Approval: &view, Owner: row.OwnerUserID, At: s.now()})
	})

	return s.auditApproval(ctx, t.tx, now, actor, "approval.cancel", &row, map[string]any{"reason": reason})
}

// Release gives back the cost reserved for a target.
func (t *pubTx) Release(ctx context.Context, targetID string) error {
	_, err := t.tx.Approvals().Release(ctx, targetID, t.s.nowMs())

	return err
}

// Settle records that a target went out at its estimate.
func (t *pubTx) Settle(ctx context.Context, targetID string) error {
	p, err := t.tx.Publications().OfTarget(ctx, targetID)
	if err != nil {
		return err
	}

	tg := p.Target(targetID)
	if tg == nil || tg.ApprovalID == "" {
		return nil
	}

	req, err := t.tx.Approvals().GetRequest(ctx, tg.ApprovalID)
	if err != nil {
		if errors.Is(err, approval.ErrNotFound) {
			return nil
		}

		return err
	}

	_, err = t.tx.Approvals().Settle(ctx, targetID, req.Estimate.Micros, t.s.nowMs())

	return err
}

// targetApproved is what approving the request of a target does to the target (SDD-020 section 26).
func (s *svc) targetApproved(ctx context.Context, tx Tx, row *approval.Row) error {
	if row.JobID != "" {
		return nil
	}

	if target, ok := publicationRef(row.PayloadRef); ok {
		return s.pubs.Approved(ctx, s.pubTx(tx), target, row.ID)
	}

	return nil
}

// targetDeclined is what rejecting, cancelling or the expiry of the request of a target does to it.
func (s *svc) targetDeclined(ctx context.Context, tx Tx, row *approval.Row, why *Error) error {
	target, ok := publicationRef(row.PayloadRef)
	if !ok {
		return nil
	}

	code := pub.CodeRejected
	if why.Code == CodeApprovalExpired {
		code = pub.CodeExpired
	}

	return s.pubs.Declined(ctx, s.pubTx(tx), target, row.ID, code, why.Message)
}

// pubGateway is what the calendar needs from outside the plan: the connection, the files and the network.
type pubGateway struct{ s *svc }

// Resolve checks a post and, when strict, validates it with its network. It makes no request.
func (g pubGateway) Resolve(ctx context.Context, p pub.Post) (*pub.Resolved, error) {
	c, ok := CallerFrom(ctx)
	if !ok {
		return nil, newError(CodeUnauthorized, "no caller.")
	}

	if out, handled, berr := g.s.resolveBlogTarget(ctx, c, &p); handled {
		return out, berr
	}

	a := PublishArgs{
		ConnectionID: p.ConnectionID, Export: p.Export, Thumbnail: p.Thumbnail, Title: p.Fields.Title,
		Description: p.Fields.Description, Tags: p.Fields.Tags, Visibility: p.Fields.Visibility,
		MadeForKids: p.Fields.MadeForKids, AIDisclosure: p.Fields.AIDisclosure,
	}

	t, err := g.s.resolvePublish(ctx, c, p.ProjectID, a)
	if err != nil {
		return nil, err
	}

	cost := t.cost()

	out := &pub.Resolved{
		Network: t.desc.Network, NetworkName: t.desc.Name, Account: t.account.Title,
		FileName: path.Base(t.input.Output.Rel), FileBytes: t.input.Output.Bytes,
		CostMicros: cost.Micros, Currency: cost.Currency, CostNote: t.costNote(), Disclosure: disclosureText(t),
		Draft: t.desc.Draft,
	}

	if !p.Strict {
		return out, nil
	}

	out.Issues = t.issues

	if out.Snapshot, err = json.Marshal(t.input); err != nil {
		return nil, internalError()
	}

	return out, nil
}

// Send sends the snapshot of an approved target.
func (g pubGateway) Send(ctx context.Context, t pub.Target, report func(float64, string)) (pub.Sent, error) {
	if snap, isPost := blogSnapshotOf(t.Snapshot); isPost {
		sent, err := g.s.sendBlog(ctx, &t, snap, report)
		if err != nil {
			return pub.Sent{}, sendError(err)
		}

		return sent, nil
	}

	var in publishInput
	if err := json.Unmarshal(t.Snapshot, &in); err != nil {
		return pub.Sent{}, &pub.SendError{
			Kind: "integrity", Code: "payload_unreadable", Message: "the approved post cannot be read.",
			Hint: "schedule the post again and approve it", Err: err,
		}
	}

	res, err := g.s.sendPost(ctx, &in, t.ID, report)
	if err != nil {
		return pub.Sent{}, sendError(err)
	}

	return pub.Sent{RemoteID: res.RemoteID, URL: res.URL, RemoteState: string(res.State)}, nil
}

// sendError turns a service error of a send into what a target shows.
func sendError(err error) *pub.SendError {
	e, ok := errors.AsType[*Error](err)
	if !ok {
		return &pub.SendError{
			Kind: "internal", Code: "internal", Message: "the post could not be sent.",
			Hint: "try again; if it fails again, read the log of the server", Err: err,
		}
	}

	out := &pub.SendError{Kind: "internal", Code: e.Code, Message: e.Message, Hint: e.Hint, Err: err}

	switch e.Code {
	case CodePublish:
		if kind, ok := e.Details["kind"].(string); ok {
			out.Kind = kind
		}
	case command.CodeInvalidState:
		out.Kind = "integrity"
	case CodeConnectionNotFound, CodeConnectionDestination, CodeVaultLocked:
		out.Kind = "connection"
	}

	return out
}
