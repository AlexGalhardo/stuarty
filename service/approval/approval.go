// Package approval is the approval queue of the service (ADR-0032 items 3 to 5, SDD-015): the
// records of the requests, the rules and the ceilings, the ledger of costs, the check of who may
// decide, and the evaluation that gathers what the pure rules of core/approval need. The service
// calls it; the store implements Store.
package approval

import (
	"context"
	"errors"
	"fmt"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The states of a request (SDD-015 section 3).
const (
	StatePending  = "pending"
	StateApproved = "approved"
	StateRejected = "rejected"
	StateExpired  = "expired"
)

// States lists every state of a request.
var States = []string{StatePending, StateApproved, StateRejected, StateExpired}

// The states of a ledger entry.
const (
	LedgerReserved = "reserved"
	LedgerSettled  = "settled"
	LedgerReleased = "released"
)

// ExpireAfter is how long a request waits for a person.
const ExpireAfter = 7 * 24 * time.Hour

// DecidedByRule is the prefix of decided_by for an automatic approval.
const DecidedByRule = "rule:"

// Errors of the store. The service maps them to the codes of SDD-015 section 8.
var (
	// ErrNotFound is a request or a rule that does not exist.
	ErrNotFound = errors.New("approval: not found")
	// ErrNotPending is a decision for a request that is no longer pending.
	ErrNotPending = errors.New("approval: the request is not pending")
)

// Request is what a surface sees of an approval request.
type Request struct {
	ID           string     `json:"id"`
	Actor        string     `json:"actor"`
	Kind         string     `json:"kind"`
	Summary      string     `json:"summary"          untrusted:"true"`
	ConnectionID *string    `json:"connection_id"`
	ProjectID    *string    `json:"project_id"`
	JobID        *string    `json:"job_id"`
	PayloadRef   string     `json:"payload_ref"`
	Estimate     core.Money `json:"estimate"`
	State        string     `json:"state"`
	DecidedBy    *string    `json:"decided_by"`
	RuleID       *string    `json:"rule_id"`
	Reason       string     `json:"reason,omitempty" untrusted:"true"`
	CreatedAt    time.Time  `json:"created_at"`
	DecidedAt    *time.Time `json:"decided_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

// Row is a request as the table holds it. Empty strings are NULL.
type Row struct {
	ID           string
	Actor        string
	OwnerUserID  string
	Kind         string
	Summary      string
	ConnectionID string
	ProjectID    string
	JobID        string
	PayloadRef   string
	Estimate     core.Money
	State        string
	DecidedBy    string
	RuleID       string
	Reason       string
	CreatedMs    int64
	DecidedMs    *int64
	ExpiresMs    int64
}

// View is the request as a surface sees it: no owner user, times as times.
func View(r *Row) Request {
	v := Request{
		ID:           r.ID,
		Actor:        r.Actor,
		Kind:         r.Kind,
		Summary:      r.Summary,
		ConnectionID: ptr(r.ConnectionID),
		ProjectID: ptr(
			r.ProjectID,
		),
		JobID:      ptr(r.JobID),
		PayloadRef: r.PayloadRef,
		Estimate:   r.Estimate,
		State:      r.State,
		DecidedBy: ptr(
			r.DecidedBy,
		),
		RuleID:    ptr(r.RuleID),
		Reason:    r.Reason,
		CreatedAt: time.UnixMilli(r.CreatedMs).UTC(),
		ExpiresAt: time.UnixMilli(r.ExpiresMs).UTC(),
	}

	if r.DecidedMs != nil {
		t := time.UnixMilli(*r.DecidedMs).UTC()
		v.DecidedAt = &t
	}

	return v
}

// Line is the request as one line of text for an agent: the summary is delimited as untrusted text,
// since an agent or a job kind wrote it (SDD-007 section 8).
func (r *Request) Line() string {
	cost := "no cost"
	if r.Estimate.Micros > 0 {
		cost = fmt.Sprintf(
			"%d.%06d %s",
			r.Estimate.Micros/core.MicrosPerUnit,
			r.Estimate.Micros%core.MicrosPerUnit,
			r.Estimate.Currency,
		)
	}

	dest := "no destination"
	if r.ConnectionID != nil {
		dest = "to " + *r.ConnectionID
	}

	return fmt.Sprintf("%s  %s  %s  %s  %s  by %s  %s", r.ID, r.State, r.Kind, dest, cost, r.Actor,
		untrusted.Inline(untrusted.SourceProjectText, nil, r.Summary, core.MaxSummary))
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// Rule is an approval rule with what the table adds.
type Rule struct {
	core.Rule
	CreatedBy string
	CreatedMs int64
}

// RuleView is a rule as a surface sees it.
type RuleView struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	ConnectionID *string   `json:"connection_id"`
	ProjectID    *string   `json:"project_id"`
	Currency     string    `json:"currency,omitempty"`
	PerItem      *int64    `json:"per_item_micros"`
	PerDay       *int64    `json:"per_day_micros"`
	Enabled      bool      `json:"enabled"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// ViewOf is the rule as a surface sees it.
func (r Rule) ViewOf() RuleView {
	return RuleView{
		ID: r.ID, Kind: r.Kind, ConnectionID: ptr(r.Connection), ProjectID: ptr(r.Project), Currency: r.Currency,
		PerItem: r.PerItem, PerDay: r.PerDay, Enabled: r.Enabled, CreatedBy: r.CreatedBy,
		CreatedAt: time.UnixMilli(r.CreatedMs).UTC(),
	}
}

// Ceiling is a cost ceiling with what the table adds.
type Ceiling struct {
	core.Ceiling
	UpdatedBy string
	UpdatedMs int64
}

// CeilingView is a ceiling as a surface sees it.
type CeilingView struct {
	ProjectID *string   `json:"project_id"`
	Currency  string    `json:"currency"`
	PerItem   *int64    `json:"per_item_micros"`
	PerDay    *int64    `json:"per_day_micros"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ViewOf is the ceiling as a surface sees it.
func (c Ceiling) ViewOf() CeilingView {
	return CeilingView{
		ProjectID: ptr(c.Project), Currency: c.Currency, PerItem: c.PerItem, PerDay: c.PerDay, UpdatedBy: c.UpdatedBy,
		UpdatedAt: time.UnixMilli(c.UpdatedMs).UTC(),
	}
}

// Entry is one line of the cost ledger (SDD-015 section 3).
type Entry struct {
	ID             string
	RequestID      string
	JobID          string
	ConnectionID   string
	ProjectID      string
	Kind           string
	Currency       string
	EstimateMicros int64
	ActualMicros   *int64
	State          string
	Day            string // the UTC date of the reservation
	CreatedMs      int64
	SettledMs      *int64
}

// Filter selects requests.
type Filter struct {
	State       string // empty means every state
	ProjectID   string // a key limited to one project
	OwnerUserID string // network mode, a caller who is not an administrator
	Actor       string // the requests one actor submitted
	Offset      int
	Limit       int
}

// SpentQuery selects the ledger entries that count for one day.
type SpentQuery struct {
	Day        string
	Currency   string
	Project    string // empty means every project
	Kind       string // empty means every kind
	Connection string // empty means every connection
}

// Total is one line of the cost summary: what was spent on a day.
type Total struct {
	Day          string `json:"day"`
	ConnectionID string `json:"connection_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	Currency     string `json:"currency"`
	Micros       int64  `json:"micros"`
}

// Store is where the records live (the tables of SDD-015 section 3). On a transaction it is bound to it.
type Store interface {
	// InsertRequest adds a request.
	InsertRequest(ctx context.Context, r *Row) error
	// GetRequest returns one request, or ErrNotFound.
	GetRequest(ctx context.Context, id string) (Row, error)
	// RequestOfJob returns the request of a job, or ErrNotFound.
	RequestOfJob(ctx context.Context, jobID string) (Row, error)
	// ListRequests returns the requests newest first; total is the number that match, before Offset and Limit.
	ListRequests(ctx context.Context, f Filter) (rows []Row, total int, err error)
	// Decide moves a pending request to state, once: it reports false when the request is not pending.
	Decide(ctx context.Context, id, state, decidedBy, ruleID, reason string, atMs int64) (bool, error)
	// Due returns the pending requests that expire at or before nowMs.
	Due(ctx context.Context, nowMs int64) ([]Row, error)

	ListRules(ctx context.Context) ([]Rule, error)
	GetRule(ctx context.Context, id string) (Rule, error)
	// PutRule inserts a rule, or replaces the one with the same id.
	PutRule(ctx context.Context, r *Rule) error
	// DeleteRule removes a rule, or ErrNotFound.
	DeleteRule(ctx context.Context, id string) error
	CountRules(ctx context.Context) (int, error)

	ListCeilings(ctx context.Context) ([]Ceiling, error)
	// PutCeiling sets the ceiling of a project and a currency.
	PutCeiling(ctx context.Context, c *Ceiling) error
	// DeleteCeiling removes one; a ceiling that is not there is not an error.
	DeleteCeiling(ctx context.Context, project, currency string) error

	// Reserve adds a ledger entry in the state reserved.
	Reserve(ctx context.Context, e *Entry) error
	// Record adds a ledger entry already settled at e.ActualMicros: what a run spent before it was
	// interrupted, while the reservation of the job stays for the run that follows.
	Record(ctx context.Context, e *Entry) error
	// Settle records the real cost of a reserved entry of a job; false when there is none.
	Settle(ctx context.Context, jobID string, actualMicros, atMs int64) (bool, error)
	// Release gives a reserved entry of a job back; false when there is none.
	Release(ctx context.Context, jobID string, atMs int64) (bool, error)
	// Spent is what counts for a day: the real cost of settled entries and the estimate of reserved ones.
	Spent(ctx context.Context, q SpentQuery) (int64, error)
	// Totals is the spend per day, connection and project, from fromDay (inclusive) on.
	Totals(ctx context.Context, fromDay string) ([]Total, error)
	// Reserved reports whether a job has a ledger entry that still counts as reserved.
	Reserved(ctx context.Context, jobID string) (bool, error)
}

// Day is the UTC date a ledger entry belongs to.
func Day(t time.Time) string { return t.UTC().Format(time.DateOnly) }

// Evaluate gathers what Decide needs from the store and asks it (SDD-015 section 2). today is the
// time the day of the spending is read at.
func Evaluate(ctx context.Context, st Store, a core.Action, today time.Time) (core.Outcome, error) {
	rules, err := st.ListRules(ctx)
	if err != nil {
		return core.Outcome{}, err
	}

	ceilings, err := st.ListCeilings(ctx)
	if err != nil {
		return core.Outcome{}, err
	}

	day := Day(today)
	spent := core.Spent{ByRule: map[string]int64{}}

	if a.Cost.Micros > 0 {
		q := SpentQuery{Day: day, Currency: a.Cost.Currency}

		if spent.Workspace, err = st.Spent(ctx, q); err != nil {
			return core.Outcome{}, err
		}

		if a.Project != "" {
			q.Project = a.Project
			if spent.Project, err = st.Spent(ctx, q); err != nil {
				return core.Outcome{}, err
			}
		}

		for i := range rules {
			r := &rules[i]
			if r.PerDay == nil || r.Kind != a.Kind {
				continue
			}

			rq := SpentQuery{Day: day, Currency: r.Currency, Project: r.Project, Kind: r.Kind, Connection: r.Connection}
			if spent.ByRule[r.ID], err = st.Spent(ctx, rq); err != nil {
				return core.Outcome{}, err
			}
		}
	}

	plain := make([]core.Rule, len(rules))
	for i := range rules {
		plain[i] = rules[i].Rule
	}

	plainCeilings := make([]core.Ceiling, len(ceilings))
	for i := range ceilings {
		plainCeilings[i] = ceilings[i].Ceiling
	}

	return core.Decide(a, plain, plainCeilings, spent), nil
}

// Caller is what the check of the privileged session needs of a caller.
type Caller struct {
	Actor       string
	Browser     bool // a privileged browser session (SDD-007 section 5.3)
	Admin       bool // holds the admin scope
	OwnerUserID string
}

// ErrNotPrivileged is a caller that is not a person in a privileged browser session.
var ErrNotPrivileged = errors.New("approval: only a person in a privileged browser session can do this")

// ErrNotAllowed is a person who may not decide this request: network mode, not an administrator and not its owner.
var ErrNotAllowed = errors.New("approval: the person may not decide this request")

// CheckPrivileged is the rule of SDD-015 section 4: only a browser session is a person. The local
// token, an API key, the in-process CLI and the stdio MCP server never are, whatever scope they hold.
func CheckPrivileged(c Caller) error {
	if !c.Browser {
		return ErrNotPrivileged
	}

	return nil
}

// CheckMayDecide adds the network-mode rule for a decision: an administrator, or the owner of the request.
func CheckMayDecide(c Caller, ownerUserID string) error {
	if err := CheckPrivileged(c); err != nil {
		return err
	}

	if c.OwnerUserID != "" && !c.Admin && c.OwnerUserID != ownerUserID {
		return ErrNotAllowed
	}

	return nil
}
