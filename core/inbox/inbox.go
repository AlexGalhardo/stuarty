// Package inbox holds the pure rules of the one list of what waits for a person (SDD-015 section
// 4.1, author decision D-137): how a permission request of the hosted agent and an approval request
// become the same row, which rows come first, and what a row says about undo. It reads no clock
// and does no I/O: the service gathers the requests and asks for the rows. It has no write path; a
// row is decided where its source is decided.
package inbox

import (
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/chat"
)

// MaxRecent is how many answered, expired or cancelled rows follow the ones that wait.
const MaxRecent = 20

// Source is the flow a request belongs to.
type Source string

// The sources.
const (
	SourceChat     Source = "chat"
	SourceApproval Source = "approval"
)

// State is where a request stands.
type State string

// The states of a row.
const (
	StateWaiting   State = "waiting"
	StateApproved  State = "approved"
	StateRejected  State = "rejected"
	StateExpired   State = "expired"
	StateCancelled State = "cancelled"
)

// Undo is what a row says about undoing the action, in one of three words (author decision D-140).
type Undo string

// The three answers. A row says UndoCan only when Stuarty can prove it, never from what an agent
// claims; no source proves it today, so no row says it.
const (
	UndoCan         Undo = "can"
	UndoCannot      Undo = "cannot"
	UndoNotVerified Undo = "not_verified"
)

// Row is one request, whichever flow it came from. agent, skill, tool, summary, item and
// destination are text an agent wrote and are untrusted: they are tagged, so the HTTP schema marks
// them x-untrusted and the result carries the notice of SDD-007 section 8. A row never carries a
// credential.
type Row struct {
	ID          string          `json:"id"`
	Source      Source          `json:"source"`
	SourceID    string          `json:"source_id"`
	State       State           `json:"state"`
	Agent       string          `json:"agent"       untrusted:"true"`
	Skill       string          `json:"skill"       untrusted:"true"`
	Tool        string          `json:"tool"        untrusted:"true"`
	Summary     string          `json:"summary"     untrusted:"true"`
	ActionKey   string          `json:"action_key"`
	Item        string          `json:"item"        untrusted:"true"`
	Destination string          `json:"destination" untrusted:"true"`
	Cost        *approval.Money `json:"cost"`
	Undo        Undo            `json:"undo"`
	ProjectID   string          `json:"project_id"`
	SessionID   string          `json:"session_id"`
	CreatedAt   time.Time       `json:"created_at"`
	AnsweredAt  *time.Time      `json:"answered_at"`
}

// RowID is the id of a row: the source and the id the source gave the request.
func RowID(s Source, sourceID string) string { return string(s) + ":" + sourceID }

// Permission is a permission request of a hosted agent session, as the host reports it.
type Permission struct {
	SessionID  string
	ProjectID  string
	Profile    string
	RequestID  string
	Title      string
	ToolKind   string
	Detail     *chat.PermissionDetail
	Skill      string
	Answer     chat.Answer // empty while the request waits
	AskedAt    time.Time
	AnsweredAt *time.Time
}

// FromPermission turns a permission request into a row. The skill, the item and the destination
// are what the host kept; for a tool that is not Stuarty's own they are empty, and so is the
// description key: the person reads "Not reported", never a guess.
//
// Undo is always UndoNotVerified (D-140). The tool is known only by the title the agent gave the
// call, and the call has not run when the permission is asked, so nothing Stuarty holds proves
// what it will do: an agent that titles any call "edit" and names commands of the editor is
// making a claim, and a claim never makes a row say it can be undone.
func FromPermission(p *Permission) Row {
	r := Row{
		ID: RowID(SourceChat, p.RequestID), Source: SourceChat, SourceID: p.RequestID, State: chatState(p.Answer),
		Agent: p.Profile, Skill: p.Skill, Tool: p.Title, ProjectID: p.ProjectID, SessionID: p.SessionID,
		CreatedAt: p.AskedAt, AnsweredAt: p.AnsweredAt, Undo: UndoNotVerified,
	}

	name, ok := chat.StuartyToolName(p.Title)
	if !ok {
		return r
	}

	var ops []string

	if p.Detail != nil {
		r.Item, r.Destination, ops = p.Detail.Item, p.Detail.Project, p.Detail.Operations
	}

	r.ActionKey = chat.DescriptionKey(name, firstAction(ops))

	return r
}

// firstAction is the action a description is chosen by: the catalogue lists the actions of a tool
// by name, so only an operation that is one of them selects an action text.
func firstAction(ops []string) string {
	if len(ops) == 0 {
		return ""
	}

	return ops[0]
}

func chatState(a chat.Answer) State {
	switch a {
	case chat.AnswerAllow:
		return StateApproved
	case chat.AnswerReject:
		return StateRejected
	case chat.AnswerCancel:
		return StateCancelled
	}

	return StateWaiting
}

// Approval is an approval request as the queue holds it.
type Approval struct {
	ID           string
	Actor        string
	Kind         string
	Summary      string
	ConnectionID string
	ProjectID    string
	Estimate     approval.Money
	State        string
	CreatedAt    time.Time
	DecidedAt    *time.Time
}

// FromApproval turns an approval request into a row. What will happen is the kind of the job and
// the summary the requester wrote; where is the connection and the project; the cost is the estimate;
// an approval is for a publication or a costly generation, which Stuarty cannot undo, so its undo is
// UndoCannot.
func FromApproval(a *Approval) Row {
	r := Row{
		ID: RowID(SourceApproval, a.ID), Source: SourceApproval, SourceID: a.ID, State: approvalState(a.State),
		Agent: a.Actor, Tool: a.Kind, Summary: a.Summary, Destination: destinationOf(a),
		ProjectID: a.ProjectID, CreatedAt: a.CreatedAt, AnsweredAt: a.DecidedAt, Undo: UndoCannot,
	}

	if a.Estimate.Micros > 0 {
		m := a.Estimate
		r.Cost = &m
	}

	return r
}

// destinationOf is the connection and the project the action goes to, in that order.
func destinationOf(a *Approval) string {
	parts := make([]string, 0, 2)
	if a.ConnectionID != "" {
		parts = append(parts, a.ConnectionID)
	}

	if a.ProjectID != "" {
		parts = append(parts, a.ProjectID)
	}

	return strings.Join(parts, " / ")
}

func approvalState(s string) State {
	switch s {
	case "approved":
		return StateApproved
	case "rejected":
		return StateRejected
	case "expired":
		return StateExpired
	}

	return StateWaiting
}

// Order puts every row that waits first, newest first, then the MaxRecent rows settled last, newest
// first by the time they were answered. A tie is broken by the id, so the order is the same on every
// read. It returns a new slice and leaves rows as it was.
func Order(rows []Row) []Row {
	waiting := make([]Row, 0, len(rows))
	settled := make([]Row, 0, len(rows))

	for i := range rows {
		if rows[i].State == StateWaiting {
			waiting = append(waiting, rows[i])
		} else {
			settled = append(settled, rows[i])
		}
	}

	slices.SortFunc(waiting, func(a, b Row) int { return newest(a.CreatedAt, b.CreatedAt, a.ID, b.ID) })
	slices.SortFunc(settled, func(a, b Row) int { return newest(settledAt(&a), settledAt(&b), a.ID, b.ID) })

	if len(settled) > MaxRecent {
		settled = settled[:MaxRecent]
	}

	return append(waiting, settled...)
}

// settledAt is when a row stopped waiting; a request that expired with no recorded time counts from
// its creation.
func settledAt(r *Row) time.Time {
	if r.AnsweredAt != nil {
		return *r.AnsweredAt
	}

	return r.CreatedAt
}

func newest(a, b time.Time, idA, idB string) int {
	if c := b.Compare(a); c != 0 {
		return c
	}

	return strings.Compare(idA, idB)
}
