package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The events of the approval queue (SDD-015 section 7).
const (
	EventApprovalRequested = "approval.requested"
	EventApprovalDecided   = "approval.decided"
)

const (
	defApprovalLimit  = 50
	maxApprovalLimit  = 200
	costSummaryDays   = 30
	approveDecision   = "approve"
	rejectDecision    = "reject"
	approvalListState = "all"
)

// ApprovalListRequest lists the requests of the queue. State is pending (the default), approved,
// rejected, expired or all.
type ApprovalListRequest struct {
	State     string `json:"state,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Page
}

// ApprovalListResult is a page of requests, newest first.
type ApprovalListResult struct {
	Envelope
	Requests   []approval.Request `json:"requests"`
	NextCursor *string            `json:"next_cursor"`
	Total      int                `json:"total"`
	Text       string             `json:"text"`
}

// ApprovalRef names a request or a rule.
type ApprovalRef struct {
	ID string `json:"id"`
}

// ApprovalDecideRequest is the answer of a person: Decision is approve or reject.
type ApprovalDecideRequest struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// ApprovalRuleListRequest has no member.
type ApprovalRuleListRequest struct{}

// ApprovalRuleListResult is every rule, oldest first.
type ApprovalRuleListResult struct {
	Rules []approval.RuleView `json:"rules"`
}

// ApprovalRuleSetRequest creates a rule, or replaces the one named by ID. A limit is in micros, a
// millionth of the currency unit. Enabled defaults to true.
type ApprovalRuleSetRequest struct {
	ID            string `json:"id,omitempty"`
	Kind          string `json:"kind"`
	ConnectionID  string `json:"connection_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	Currency      string `json:"currency,omitempty"`
	PerItemMicros *int64 `json:"per_item_micros,omitempty"`
	PerDayMicros  *int64 `json:"per_day_micros,omitempty"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

// CostSummaryRequest has no member.
type CostSummaryRequest struct{}

// CostLine is what was spent today in one scope and currency.
type CostLine struct {
	Scope     string `json:"scope"` // workspace or project
	ProjectID string `json:"project_id,omitempty"`
	Currency  string `json:"currency"`
	Micros    int64  `json:"micros"`
}

// CostSummaryResult is SDD-015 section 6: the ceilings, today, and the last 30 days.
type CostSummaryResult struct {
	Ceilings []approval.CeilingView `json:"ceilings"`
	Today    []CostLine             `json:"today"`
	Day      string                 `json:"day"`
	Days     []approval.Total       `json:"days"`
}

// CostCeilingSetRequest sets the ceiling of the workspace (no ProjectID) or of a project, in one
// currency. With both limits empty it removes the ceiling.
type CostCeilingSetRequest struct {
	ProjectID     string `json:"project_id,omitempty"`
	Currency      string `json:"currency"`
	PerItemMicros *int64 `json:"per_item_micros,omitempty"`
	PerDayMicros  *int64 `json:"per_day_micros,omitempty"`
}

func approvalCaller(c Caller) approval.Caller {
	return approval.Caller{
		Actor: c.Actor, Browser: c.Browser, Admin: slices.Contains(c.Scopes, ScopeAdmin), OwnerUserID: c.OwnerUserID,
	}
}

// person is the check of the operations for a person (SDD-015 section 4). The order is fixed: a
// missing caller is E_UNAUTHORIZED; anyone who is not a privileged browser session is
// E_PRIVILEGED_SESSION, whatever scope it holds, so an agent learns one thing; then the scope.
func (s *svc) person(ctx context.Context, method string) (Caller, error) {
	c, ok := CallerFrom(ctx)
	if !ok {
		return Caller{}, newError(CodeUnauthorized, "the request carries no credentials.").
			withHint("send the local token or an API key")
	}

	if err := approval.CheckPrivileged(approvalCaller(c)); err != nil {
		return Caller{}, privilegedError()
	}

	return authorize(ctx, method, "")
}

func privilegedError() *Error {
	return newError(CodePrivilegedSession, "only a person in the app can do this.").
		withHint("a person decides in the Approvals page of the Stuarty desktop app, or of a server with login; the local " +
			"token, API keys and the agent surfaces cannot approve, write a rule or set a ceiling")
}

// approvalFilter narrows a list to what the caller may read: a key limited to one project reads
// that project's requests only, and in network mode a caller without admin reads the own user's.
func (s *svc) approvalFilter(c Caller, f approval.Filter) approval.Filter {
	if c.ProjectID != "" {
		f.ProjectID = c.ProjectID
	}

	if c.OwnerUserID != "" && !slices.Contains(c.Scopes, ScopeAdmin) {
		f.OwnerUserID = c.OwnerUserID
	}

	return f
}

func (s *svc) mayRead(c Caller, r *approval.Row) bool {
	if c.ProjectID != "" && r.ProjectID != c.ProjectID {
		return false
	}

	return c.OwnerUserID == "" || slices.Contains(c.Scopes, ScopeAdmin) || r.OwnerUserID == c.OwnerUserID
}

func approvalNotFound(id string) *Error {
	return newError(CodeApprovalNotFound, "there is no approval request or rule %s.", id).
		withHint("list the pending requests with approval:list")
}

func approvalError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, approval.ErrNotFound):
		return newError(CodeApprovalNotFound, "the approval request or rule does not exist.")
	case errors.Is(err, connections.ErrNotFound):
		return newError(CodeConnectionNotFound, "the connection does not exist or was removed.").
			withHint("list the connections with connection:list")
	}

	return asError(err)
}

func costCeilingError(r *core.Refusal) *Error {
	where := "the workspace"
	if r.Scope == core.ScopeProject {
		where = "the project"
	}

	var msg, hint string

	switch r.Reason {
	case core.ReasonPerItem:
		msg = fmt.Sprintf(
			"the estimate of %s is over the limit per item of %s (%s).",
			money(r.Estimate, r.Currency),
			where,
			money(r.Limit, r.Currency),
		)
		hint = "ask for something cheaper, or ask the person to raise the limit per item in the Approvals page"
	case core.ReasonPerDay:
		msg = fmt.Sprintf("the estimate of %s with %s already spent today is over the daily limit of %s (%s).",
			money(r.Estimate, r.Currency), money(r.Spent, r.Currency), where, money(r.Limit, r.Currency))
		hint = "wait for the next UTC day, or ask the person to raise the daily limit in the Approvals page"
	default:
		return newError(command.CodeValidation, "%s", r.Message).withHint("describe the action with a kind and a cost")
	}

	return newError(CodeCostCeiling, "%s", msg).withHint("%s", hint).
		withDetail("scope", string(r.Scope)).withDetail("reason", string(r.Reason)).
		withDetail("limit_micros", r.Limit).withDetail("spent_micros", r.Spent).
		withDetail("estimate_micros", r.Estimate).withDetail("currency", r.Currency)
}

func money(micros int64, currency string) string {
	return fmt.Sprintf("%d.%06d %s", micros/core.MicrosPerUnit, micros%core.MicrosPerUnit, currency)
}

// checkText refuses the text a person reads before deciding when it could look like something else.
// core/approval refuses every control and format character; the characters that service/untrusted
// drops from text an agent reads (the tag characters, the variation selectors) are refused too.
func checkText(path, s string, valid func(string) bool) *Error {
	if !valid(s) || strings.ContainsFunc(s, func(r rune) bool { return r != '\n' && untrusted.Removed(r) }) {
		return newError(command.CodeValidation, "%s is not valid text for a person to read.", path).withPath(path).
			withHint("use plain text of at most %d characters, with no control or invisible characters", core.MaxSummary)
	}

	return nil
}

// expireDue marks the pending requests whose time has passed as expired and rejects their jobs
// (SDD-015 section 3). It writes only when something is due.
func (s *svc) expireDue(ctx context.Context) {
	due, err := s.store.Approvals().Due(ctx, s.nowMs())
	if err != nil || len(due) == 0 {
		return
	}

	for i := range due {
		row := due[i]

		var view approval.Request

		err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
			now := s.nowMs()

			ok, err := tx.Approvals().Decide(ctx, row.ID, approval.StateExpired, "", "", "", now)
			if err != nil || !ok {
				return err
			}

			if err := s.rejectJob(
				ctx,
				tx,
				&row,
				newError(CodeApprovalExpired, "no one answered the approval request in time.").
					withHint("submit it again if it is still wanted"),
				now,
			); err != nil {
				return err
			}

			row.State, row.DecidedMs = approval.StateExpired, &now
			view = approval.View(&row)

			return s.auditApproval(ctx, tx, now, "system", "approval.expire", &row, nil)
		})
		if err == nil && view.ID != "" {
			s.emit(Event{Type: EventApprovalDecided, Approval: &view, Owner: row.OwnerUserID, At: s.now()})
		}
	}
}

func (s *svc) auditApproval(
	ctx context.Context,
	tx Tx,
	now int64,
	actor, action string,
	r *approval.Row,
	details map[string]any,
) error {
	row := AuditRow{AtMs: now, Actor: actor, Action: action, SubjectID: &r.ID}

	if r.ProjectID != "" {
		row.ProjectID = &r.ProjectID
	}

	if details != nil {
		raw, err := json.Marshal(details)
		if err != nil {
			return err
		}

		row.Details = raw
	}

	return tx.Audit().Append(ctx, row)
}

// ApprovalList returns the requests of the queue.
func (s *svc) ApprovalList(ctx context.Context, req ApprovalListRequest) (*ApprovalListResult, error) {
	c, err := s.authorizeProject(ctx, "ApprovalList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	state := req.State
	if state == "" {
		state = approval.StatePending
	}

	if state != approvalListState && !slices.Contains(approval.States, state) {
		return nil, newError(command.CodeValidation, "the state %q is not known.", state).withPath("state").
			withHint("use pending, approved, rejected, expired or all")
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	off := 0

	if req.Cursor != "" {
		var derr *Error
		if off, derr = decodeOffset(req.Cursor, "approvals", state); derr != nil {
			return nil, derr
		}
	}

	s.expireDue(ctx)

	f := approval.Filter{
		ProjectID: req.ProjectID,
		Offset:    off,
		Limit:     clampLimit(req.Limit, defApprovalLimit, maxApprovalLimit),
	}
	if state != approvalListState {
		f.State = state
	}

	rows, total, lerr := s.store.Approvals().ListRequests(ctx, s.approvalFilter(c, f))
	if lerr != nil {
		return nil, asError(lerr)
	}

	res := &ApprovalListResult{Requests: make([]approval.Request, 0, len(rows)), Total: total}
	lines := []string{fmt.Sprintf("%d approval request(s) %s", total, state)}

	for i := range rows {
		v := approval.View(&rows[i])
		res.Requests = append(res.Requests, v)
		lines = append(lines, v.Line())
	}

	if len(rows) > 0 {
		res.UntrustedNotice = UntrustedNotice
	}

	if next := off + len(rows); next < total {
		cur := offsetCursor("approvals", state, next)
		res.NextCursor = &cur
		lines = append(lines, "next_cursor: "+cur)
	}

	res.Text = strings.Join(lines, "\n")

	return res, nil
}

// ApprovalGet returns one request.
func (s *svc) ApprovalGet(ctx context.Context, req ApprovalRef) (*approval.Request, error) {
	c, err := authorize(ctx, "ApprovalGet", "")
	if err != nil {
		return nil, err
	}

	s.expireDue(ctx)

	row, gerr := s.store.Approvals().GetRequest(ctx, req.ID)
	if gerr != nil || !s.mayRead(c, &row) {
		return nil, approvalNotFound(req.ID)
	}

	v := approval.View(&row)

	return &v, nil
}

// ApprovalDecide is the answer of a person (SDD-015 section 4).
func (s *svc) ApprovalDecide(ctx context.Context, req ApprovalDecideRequest) (*approval.Request, error) {
	c, err := s.person(ctx, "ApprovalDecide")
	if err != nil {
		return nil, err
	}

	if req.Decision != approveDecision && req.Decision != rejectDecision {
		return nil, newError(command.CodeValidation, "the decision must be approve or reject.").withPath("decision")
	}

	if terr := checkText("reason", req.Reason, core.ValidReason); terr != nil {
		return nil, terr
	}

	s.expireDue(ctx)

	row, gerr := s.store.Approvals().GetRequest(ctx, req.ID)
	if gerr != nil || !s.mayRead(c, &row) {
		return nil, approvalNotFound(req.ID)
	}

	if err := approval.CheckMayDecide(approvalCaller(c), row.OwnerUserID); err != nil {
		return nil, newError(CodeForbidden, "only an administrator or the owner of the request can decide it.").
			withHint("ask an administrator")
	}

	var out approval.Request

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		cur, err := tx.Approvals().GetRequest(ctx, req.ID)
		if err != nil {
			return err
		}

		if cur.State != approval.StatePending {
			return newError(command.CodeInvalidState, "the request is %s, not pending.", cur.State).
				withHint("a request is decided once; submit the action again if it is still wanted")
		}

		if req.Decision == approveDecision {
			err = s.personApproves(ctx, tx, &cur, c.Actor)
		} else {
			err = s.personRejects(ctx, tx, &cur, c.Actor, req.Reason)
		}

		if err != nil {
			return err
		}

		out = approval.View(&cur)

		tx.AfterCommit(
			func() { s.emit(Event{Type: EventApprovalDecided, Approval: &out, Owner: cur.OwnerUserID, At: s.now()}) },
		)

		return nil
	})
	if werr != nil {
		return nil, approvalError(werr)
	}

	return &out, nil
}

func (s *svc) personApproves(ctx context.Context, tx Tx, row *approval.Row, actor string) error {
	now := s.nowMs()

	if err := s.approve(ctx, tx, row, actor, "", now); err != nil {
		return err
	}

	return s.auditApproval(ctx, tx, now, actor, "approval.approve", row, nil)
}

func (s *svc) personRejects(ctx context.Context, tx Tx, row *approval.Row, actor, reason string) error {
	now := s.nowMs()

	ok, err := tx.Approvals().Decide(ctx, row.ID, approval.StateRejected, actor, "", reason, now)
	if err != nil {
		return err
	}

	if !ok {
		return newError(command.CodeInvalidState, "the request is no longer pending.")
	}

	row.State, row.DecidedBy, row.Reason, row.DecidedMs = approval.StateRejected, actor, reason, &now

	why := "rejected by a person"
	if reason != "" {
		why += ": " + reason
	}

	if err := s.rejectJob(ctx, tx, row, newError(CodeCancelled, "%s", why), now); err != nil {
		return err
	}

	return s.auditApproval(ctx, tx, now, actor, "approval.reject", row, map[string]any{"reason": reason})
}

// approve is what approving does, for a person and for a rule, inside the write transaction: the
// connection must still exist, the ceilings are checked again (the spent amount changes while a
// request waits), the request becomes approved, the cost is reserved in the ledger and the job is
// let go (SDD-015 sections 4 and 6). A ceiling that is hit leaves the request pending.
func (s *svc) approve(ctx context.Context, tx Tx, row *approval.Row, decidedBy, ruleID string, now int64) error {
	if row.ConnectionID != "" {
		if _, err := tx.Connections().Get(ctx, row.ConnectionID); err != nil {
			return err
		}
	}

	action := core.Action{Kind: row.Kind, Connection: row.ConnectionID, Project: row.ProjectID, Cost: row.Estimate}

	out, err := approval.Evaluate(ctx, tx.Approvals(), action, time.UnixMilli(now))
	if err != nil {
		return err
	}

	if out.Verdict == core.Refused {
		return costCeilingError(out.Refusal)
	}

	by := decidedBy
	if ruleID != "" {
		by = approval.DecidedByRule + ruleID
	}

	ok, err := tx.Approvals().Decide(ctx, row.ID, approval.StateApproved, by, ruleID, "", now)
	if err != nil {
		return err
	}

	if !ok {
		return newError(command.CodeInvalidState, "the request is no longer pending.")
	}

	row.State, row.DecidedBy, row.RuleID, row.DecidedMs = approval.StateApproved, by, ruleID, &now

	if row.Estimate.Micros > 0 {
		e := approval.Entry{
			ID: s.newIDs(
				"led",
			),
			RequestID:      row.ID,
			JobID:          row.JobID,
			ConnectionID:   row.ConnectionID,
			ProjectID:      row.ProjectID,
			Kind:           row.Kind,
			Currency:       row.Estimate.Currency,
			EstimateMicros: row.Estimate.Micros,
			Day:            approval.Day(time.UnixMilli(now)),
			CreatedMs:      now,
		}

		if err := tx.Approvals().Reserve(ctx, &e); err != nil {
			return err
		}
	}

	return s.releaseJob(ctx, tx, row, now)
}

// ApprovalRuleList returns every rule.
func (s *svc) ApprovalRuleList(ctx context.Context, _ ApprovalRuleListRequest) (*ApprovalRuleListResult, error) {
	c, err := authorize(ctx, "ApprovalRuleList", "")
	if err != nil {
		return nil, err
	}

	if c.ProjectID != "" {
		return nil, limitedToProject()
	}

	rules, lerr := s.store.Approvals().ListRules(ctx)
	if lerr != nil {
		return nil, asError(lerr)
	}

	res := &ApprovalRuleListResult{Rules: make([]approval.RuleView, 0, len(rules))}
	for i := range rules {
		res.Rules = append(res.Rules, rules[i].ViewOf())
	}

	return res, nil
}

func limitedToProject() *Error {
	return newError(CodeForbidden, "the key is limited to one project and cannot use the approval rules or the costs.").
		withHint("use a key without a project limit")
}

// ApprovalRuleSet creates or replaces a rule (SDD-015 section 3). Only a person in the app writes one.
func (s *svc) ApprovalRuleSet(ctx context.Context, req ApprovalRuleSetRequest) (*approval.RuleView, error) {
	c, err := s.person(ctx, "ApprovalRuleSet")
	if err != nil {
		return nil, err
	}

	enabled := req.Enabled == nil || *req.Enabled

	rule, ferr := core.NormalizeRule(core.Rule{
		Kind: req.Kind, Connection: req.ConnectionID, Project: req.ProjectID, Currency: req.Currency,
		PerItem: req.PerItemMicros, PerDay: req.PerDayMicros, Enabled: enabled,
	})
	if ferr != nil {
		return nil, newError(command.CodeValidation, "%s", ferr.Message).withPath(ferr.Path)
	}

	var view approval.RuleView

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		st := tx.Approvals()
		row := approval.Rule{Rule: rule, CreatedBy: c.Actor, CreatedMs: now}

		if rule.Connection != "" {
			if _, err := tx.Connections().Get(ctx, rule.Connection); err != nil {
				return newError(command.CodeValidation, "the connection does not exist.").withPath("connection_id")
			}
		}

		if req.ID != "" {
			old, err := st.GetRule(ctx, req.ID)
			if err != nil {
				return err
			}

			row.ID, row.CreatedBy, row.CreatedMs = old.ID, old.CreatedBy, old.CreatedMs
		} else {
			n, err := st.CountRules(ctx)
			if err != nil {
				return err
			}

			if n >= core.MaxRules {
				return newError(
					"E_LIMIT_EXCEEDED",
					"the workspace has the most rules it may have (%d).",
					core.MaxRules,
				).
					withHint("remove a rule first")
			}

			row.ID = s.newIDs("rul")
		}

		if err := st.PutRule(ctx, &row); err != nil {
			return err
		}

		view = row.ViewOf()

		return auditRule(ctx, tx, now, c.Actor, "approval.rule.set", &row)
	})
	if werr != nil {
		return nil, approvalError(werr)
	}

	return &view, nil
}

// ApprovalRuleRemove removes a rule. Only a person in the app does.
func (s *svc) ApprovalRuleRemove(ctx context.Context, req ApprovalRef) (*approval.RuleView, error) {
	c, err := s.person(ctx, "ApprovalRuleRemove")
	if err != nil {
		return nil, err
	}

	var view approval.RuleView

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		old, err := tx.Approvals().GetRule(ctx, req.ID)
		if err != nil {
			return err
		}

		if err := tx.Approvals().DeleteRule(ctx, req.ID); err != nil {
			return err
		}

		view = old.ViewOf()

		return auditRule(ctx, tx, s.nowMs(), c.Actor, "approval.rule.remove", &old)
	})
	if werr != nil {
		return nil, approvalError(werr)
	}

	return &view, nil
}

// auditRule writes the audit row of a rule written or removed, with what the rule covered and its
// limits (SDD-015 section 9), so the log says what a removed rule used to approve.
func auditRule(ctx context.Context, tx Tx, now int64, actor, action string, r *approval.Rule) error {
	raw, err := json.Marshal(map[string]any{
		"kind": r.Kind, "connection_id": r.Connection, "project_id": r.Project, "currency": r.Currency,
		"per_item_micros": r.PerItem, "per_day_micros": r.PerDay, "enabled": r.Enabled,
	})
	if err != nil {
		return err
	}

	return tx.Audit().Append(ctx, AuditRow{AtMs: now, Actor: actor, Action: action, SubjectID: &r.ID, Details: raw})
}

// CostSummary returns the ceilings and what was spent.
func (s *svc) CostSummary(ctx context.Context, _ CostSummaryRequest) (*CostSummaryResult, error) {
	c, err := authorize(ctx, "CostSummary", "")
	if err != nil {
		return nil, err
	}

	if c.ProjectID != "" {
		return nil, limitedToProject()
	}

	now := s.now()
	today := approval.Day(now)

	ceilings, lerr := s.store.Approvals().ListCeilings(ctx)
	if lerr != nil {
		return nil, asError(lerr)
	}

	totals, terr := s.store.Approvals().Totals(ctx, approval.Day(now.AddDate(0, 0, 1-costSummaryDays)))
	if terr != nil {
		return nil, asError(terr)
	}

	res := &CostSummaryResult{
		Ceilings: make([]approval.CeilingView, 0, len(ceilings)), Today: []CostLine{}, Day: today, Days: totals,
	}

	if res.Days == nil {
		res.Days = []approval.Total{}
	}

	for i := range ceilings {
		res.Ceilings = append(res.Ceilings, ceilings[i].ViewOf())
	}

	res.Today = costToday(totals, today)

	return res, nil
}

// costToday adds the lines of today: the workspace per currency and each project per currency.
func costToday(totals []approval.Total, today string) []CostLine {
	type key struct{ project, currency string }

	sums := map[key]int64{}
	work := map[string]int64{}

	for i := range totals {
		t := &totals[i]
		if t.Day != today {
			continue
		}

		work[t.Currency] += t.Micros

		if t.ProjectID != "" {
			sums[key{t.ProjectID, t.Currency}] += t.Micros
		}
	}

	out := []CostLine{}

	for cur, m := range work {
		out = append(out, CostLine{Scope: "workspace", Currency: cur, Micros: m})
	}

	for k, m := range sums {
		out = append(out, CostLine{Scope: "project", ProjectID: k.project, Currency: k.currency, Micros: m})
	}

	slices.SortFunc(out, func(a, b CostLine) int {
		return strings.Compare(a.Scope+a.ProjectID+a.Currency, b.Scope+b.ProjectID+b.Currency)
	})

	return out
}

// CostCeilingSet sets or removes a ceiling. Only a person in the app does.
func (s *svc) CostCeilingSet(ctx context.Context, req CostCeilingSetRequest) (*CostSummaryResult, error) {
	c, err := s.person(ctx, "CostCeilingSet")
	if err != nil {
		return nil, err
	}

	ceiling, ferr := core.NormalizeCeiling(core.Ceiling{
		Project: req.ProjectID, Currency: req.Currency, PerItem: req.PerItemMicros, PerDay: req.PerDayMicros,
	})
	if ferr != nil {
		return nil, newError(command.CodeValidation, "%s", ferr.Message).withPath(ferr.Path)
	}

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()

		if ceiling.PerItem == nil && ceiling.PerDay == nil {
			if err := tx.Approvals().DeleteCeiling(ctx, ceiling.Project, ceiling.Currency); err != nil {
				return err
			}
		} else if err := tx.Approvals().PutCeiling(ctx, &approval.Ceiling{Ceiling: ceiling, UpdatedBy: c.Actor, UpdatedMs: now}); err != nil {
			return err
		}

		raw, err := json.Marshal(map[string]any{
			"currency": ceiling.Currency, "per_item_micros": ceiling.PerItem, "per_day_micros": ceiling.PerDay,
		})
		if err != nil {
			return err
		}

		row := AuditRow{AtMs: now, Actor: c.Actor, Action: "approval.ceiling.set", Details: raw}
		if ceiling.Project != "" {
			row.SubjectID = &ceiling.Project
		}

		return tx.Audit().Append(ctx, row)
	})
	if werr != nil {
		return nil, approvalError(werr)
	}

	return s.CostSummary(ctx, CostSummaryRequest{})
}
