package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// Limits of the plan: a publication is a handful of posts, not a database.
const (
	MaxTitleRunes       = 200
	MaxTextBytes        = 20000
	MaxTags             = 30
	MaxTagRunes         = 100
	MaxTargets          = 10
	defaultListLimit    = 100
	maxListLimit        = 500
	summaryTitleRunes   = 80
	summaryAccountRunes = 60
	summaryTextRunes    = 100
	summaryFileRunes    = 60
)

// Actor is who calls: it is recorded on what it creates.
type Actor struct {
	ID          string
	OwnerUserID string
}

// Ask is the approval request of one target (SDD-020 section 26).
type Ask struct {
	// ApprovalID is the id the request is made with; the target already names it.
	ApprovalID    string
	PublicationID string
	TargetID      string
	ProjectID     string
	ConnectionID  string
	Summary       string
	CostMicros    int64
	Currency      string
}

// Env is what the service gives the manager: the store, the clock, identifiers and a transaction.
type Env interface {
	// Store reads outside a transaction.
	Store() Store
	// Write runs fn in one write transaction; hooks added with Tx.Emit run after the commit.
	Write(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
	Now() time.Time
	NewID(prefix string) string
	Actor(ctx context.Context) Actor
}

// Tx is a write transaction of the service, with what a publication needs besides its own store.
type Tx interface {
	Store() Store
	// Ask makes the approval request of a target. A rule the person wrote may approve it at once, in which
	// case Manager.Approved has run before Ask returns.
	Ask(ctx context.Context, a Ask) error
	// Withdraw rejects the pending request, giving the reason; a request that is no longer pending is not an
	// error.
	Withdraw(ctx context.Context, approvalID, reason string) error
	// Release gives back the cost reserved for a target; nothing reserved is not an error.
	Release(ctx context.Context, targetID string) error
	// Settle records the cost of a target that went out.
	Settle(ctx context.Context, targetID string) error
	// Emit sends publication.updated after the commit.
	Emit(p Publication)
}

// Post is what Gateway.Resolve is asked about: the files and the account of one target, with its effective
// fields.
type Post struct {
	ProjectID    string
	ConnectionID string
	Export       string
	Thumbnail    string
	Fields       Fields
	// Strict asks for the full check and the snapshot; otherwise the connection and the files are only
	// checked to exist, so a person can save a draft that is not finished.
	Strict bool
}

// Resolved is what the service learned about a post, without sending anything.
type Resolved struct {
	Network     string
	NetworkName string
	// Account is the title of the account on the network.
	Account   string
	FileName  string
	FileBytes int64
	// Snapshot is the post as the person will be shown it, with the digest of each file (Strict only).
	Snapshot json.RawMessage
	Issues   []core.Issue
	// CostMicros and Currency are what the network charges for one post; zero for none.
	CostMicros int64
	Currency   string
}

// Sent is the result of a send.
type Sent struct {
	RemoteID    string
	URL         string
	RemoteState string
}

// SendError is a failure of a send, written by the service: the text of a network is never in it.
type SendError struct {
	Kind    string
	Code    string
	Message string
	Hint    string
	Err     error
}

func (e *SendError) Error() string { return "publish: " + e.Code + ": " + e.Message }

// Unwrap returns the cause.
func (e *SendError) Unwrap() error { return e.Err }

// Gateway is what the calendar needs from the world outside the plan.
type Gateway interface {
	// Resolve checks the connection and the files of a post and, when Strict, validates it with its network.
	// It makes no request to the network. An error is the service error to show.
	Resolve(ctx context.Context, p Post) (*Resolved, error)
	// Send sends the snapshot of an approved target through its connection, resuming an upload that was
	// interrupted; an error that is a *SendError says how it failed.
	Send(ctx context.Context, t Target, report func(progress float64, message string)) (Sent, error)
}

// The kinds of Error.
const (
	KindValidation   = "validation"
	KindInvalidState = "invalid_state"
)

// Error is a refusal the service shows as E_VALIDATION or E_INVALID_STATE.
type Error struct {
	Kind    string
	Path    string
	Message string
	Hint    string
	Issues  []core.Issue
}

func (e *Error) Error() string { return "publish: " + e.Message }

func invalid(path, format string, args ...any) *Error {
	return &Error{Kind: KindValidation, Path: path, Message: fmt.Sprintf(format, args...)}
}

func state(format string, args ...any) *Error {
	return &Error{Kind: KindInvalidState, Message: fmt.Sprintf(format, args...)}
}

func (e *Error) hint(h string) *Error {
	e.Hint = h

	return e
}

// Manager is the operations over the plan and the transitions the approval and the scheduler cause.
type Manager struct {
	env  Env
	gw   Gateway
	wake func()
}

// NewManager makes the manager of a workspace.
func NewManager(env Env, gw Gateway) *Manager { return &Manager{env: env, gw: gw} }

// OnChange registers what to call after a change that may bring the next due time forward; the scheduler
// uses it to wake early.
func (m *Manager) OnChange(fn func()) { m.wake = fn }

func (m *Manager) changed() {
	if m.wake != nil {
		m.wake()
	}
}

// TargetInput is one target of a create or an update.
type TargetInput struct {
	// ID names an existing target to change; empty adds one.
	ID           string `json:"id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	Fields       Fields `json:"fields,omitzero"`
	// At is an instant (RFC 3339) or a local date and time read in Zone; empty leaves the time as it is.
	At   string `json:"at,omitempty"`
	Zone string `json:"zone,omitempty"`
	// LateWindowMinutes is how late after its time the post may still go without asking, 0 to 1440.
	LateWindowMinutes *int `json:"late_window_minutes,omitempty"`
	// Remove deletes the target of an update.
	Remove bool `json:"remove,omitempty"`
}

// CreateInput is a new publication.
type CreateInput struct {
	ProjectID string        `json:"project_id"`
	Title     string        `json:"title"`
	Text      string        `json:"text,omitempty"`
	Export    string        `json:"export"`
	Thumbnail string        `json:"thumbnail,omitempty"`
	Targets   []TargetInput `json:"targets,omitempty"`
}

// UpdateInput changes a publication; a nil field is left as it is.
type UpdateInput struct {
	ID        string        `json:"id"`
	Title     *string       `json:"title,omitempty"`
	Text      *string       `json:"text,omitempty"`
	Export    *string       `json:"export,omitempty"`
	Thumbnail *string       `json:"thumbnail,omitempty"`
	Targets   []TargetInput `json:"targets,omitempty"`
}

// ScheduleInput gives a target its time and asks for approval.
type ScheduleInput struct {
	ID string `json:"id"`
	// TargetID names the target; empty is every draft target that has a time.
	TargetID          string `json:"target_id,omitempty"`
	At                string `json:"at,omitempty"`
	Zone              string `json:"zone,omitempty"`
	LateWindowMinutes *int   `json:"late_window_minutes,omitempty"`
}

// TargetRef names a target of a publication.
type TargetRef struct {
	ID       string `json:"id"`
	TargetID string `json:"target_id,omitempty"`
}

func cleanText(path, s string, minRunes, maxRunes int, multiline bool) error {
	n := utf8.RuneCountInString(s)
	if n < minRunes || n > maxRunes {
		return invalid(path, "must be %d to %d characters.", minRunes, maxRunes)
	}

	if !utf8.ValidString(s) {
		return invalid(path, "must be valid text.")
	}

	for _, r := range s {
		if core.Invisible(r) || unicode.IsControl(r) && (!multiline || r != '\n' && r != '\t') {
			return invalid(path, "has a control or invisible character.").
				hint("remove the control and invisible characters")
		}
	}

	return nil
}

func checkFields(path string, f *Fields) error {
	if err := cleanText(path+".title", f.Title, 0, MaxTitleRunes, false); err != nil {
		return err
	}

	if len(f.Description) > MaxTextBytes {
		return invalid(path+".description", "must be at most %d bytes.", MaxTextBytes)
	}

	if err := cleanText(path+".description", f.Description, 0, MaxTextBytes, true); err != nil {
		return err
	}

	if len(f.Tags) > MaxTags {
		return invalid(path+".tags", "at most %d tags.", MaxTags)
	}

	for i, tag := range f.Tags {
		if err := cleanText(fmt.Sprintf("%s.tags[%d]", path, i), tag, 1, MaxTagRunes, false); err != nil {
			return err
		}
	}

	return nil
}

func checkLate(path string, p *int) error {
	if p != nil && (*p < 0 || *p > core.MaxLateWindowMinutes) {
		return invalid(path, "must be 0 to %d minutes.", core.MaxLateWindowMinutes)
	}

	return nil
}

// view is a publication as a surface shows it: the local time of each target.
func view(p Publication) Publication {
	p.Targets = slices.Clone(p.Targets)

	for i := range p.Targets {
		t := &p.Targets[i]
		if t.ScheduledAt != nil {
			t.LocalTime = core.Slot{At: *t.ScheduledAt, Zone: t.Zone}.Local()
		}
	}

	if p.Targets == nil {
		p.Targets = []Target{}
	}

	return p
}

func (m *Manager) post(p *Publication, t *Target, strict bool) Post {
	f := t.Fields
	if f.Title == "" {
		f.Title = p.Title
	}

	if f.Description == "" {
		f.Description = p.Text
	}

	return Post{
		ProjectID: p.ProjectID, ConnectionID: t.ConnectionID, Export: p.Export, Thumbnail: p.Thumbnail,
		Fields: f, Strict: strict,
	}
}

func (m *Manager) applyTime(t *Target, in *TargetInput) (note string, err error) {
	if err := checkLate("late_window_minutes", in.LateWindowMinutes); err != nil {
		return "", err
	}

	if in.LateWindowMinutes != nil {
		t.LateWindowMinutes = *in.LateWindowMinutes
	}

	if in.At == "" && in.Zone != "" {
		if _, zerr := core.LoadZone(in.Zone); zerr != nil {
			return "", invalid("zone", "%s", strings.TrimPrefix(zerr.Error(), "publish: invalid time: "))
		}

		t.Zone = in.Zone
	}

	if in.At == "" {
		return "", nil
	}

	t.SendNow = false

	zone := in.Zone
	if zone == "" {
		zone = t.Zone
	}

	slot, perr := core.ParseSlot(in.At, zone, "UTC")
	if perr != nil {
		return "", invalid("at", "%s", strings.TrimPrefix(perr.Error(), "publish: invalid time: ")).
			hint("write the time as 2026-11-20T09:00 with a zone such as America/Sao_Paulo, or as an instant with an offset")
	}

	at := slot.At
	t.ScheduledAt, t.Zone = &at, slot.Zone

	switch slot.Adjustment {
	case core.AdjustGap:
		return fmt.Sprintf("%s does not exist in %s (the clock jumps over it): the post is set for %s.",
			strings.ReplaceAll(in.At, "T", " "), slot.Zone, slot.Local()), nil
	case core.AdjustAmbiguous:
		return fmt.Sprintf("%s happens twice in %s (the clock goes back): the first one was taken, %s UTC.",
			strings.ReplaceAll(in.At, "T", " "), slot.Zone, at.Format("2006-01-02 15:04")), nil
	case core.AdjustNone:
	}

	return "", nil
}

// Create plans a publication: every target is a draft.
func (m *Manager) Create(ctx context.Context, in CreateInput) (*Publication, error) {
	if err := cleanText("title", in.Title, 1, MaxTitleRunes, false); err != nil {
		return nil, err
	}

	if err := cleanText("text", in.Text, 0, MaxTextBytes, true); err != nil {
		return nil, err
	}

	if in.Export == "" {
		return nil, invalid(
			"export",
			"name the export job of the video.",
		).hint("export first, then plan its publication")
	}

	if len(in.Targets) > MaxTargets {
		return nil, invalid("targets", "at most %d targets.", MaxTargets)
	}

	now, actor := m.env.Now(), m.env.Actor(ctx)
	p := &Publication{
		ID: m.env.NewID("pub"), ProjectID: in.ProjectID, Title: in.Title, Text: in.Text, Export: in.Export,
		Thumbnail: in.Thumbnail, CreatedAt: now, UpdatedAt: now, CreatedBy: actor.ID, OwnerUserID: actor.OwnerUserID,
	}

	for i := range in.Targets {
		t, err := m.newTarget(ctx, p, &in.Targets[i], fmt.Sprintf("targets[%d]", i), now)
		if err != nil {
			return nil, err
		}

		t.Position = i
		p.Targets = append(p.Targets, *t)
	}

	err := m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Store().Insert(ctx, p); err != nil {
			return err
		}

		tx.Emit(view(*p))

		return nil
	})
	if err != nil {
		return nil, err
	}

	out := view(*p)

	return &out, nil
}

func (m *Manager) newTarget(
	ctx context.Context,
	p *Publication,
	in *TargetInput,
	path string,
	now time.Time,
) (*Target, error) {
	if in.ConnectionID == "" {
		return nil, invalid(path+".connection_id", "name the connection the post goes to.").
			hint("list them with social:account:list")
	}

	if err := checkFields(path+".fields", &in.Fields); err != nil {
		return nil, err
	}

	t := &Target{
		ID: m.env.NewID("tgt"), PublicationID: p.ID, ConnectionID: in.ConnectionID, Fields: in.Fields,
		State: core.TargetDraft, Zone: "UTC", CreatedAt: now, UpdatedAt: now,
	}

	note, err := m.applyTime(t, in)
	if err != nil {
		return nil, err
	}

	if note != "" {
		p.Notes = append(p.Notes, note)
	}

	res, err := m.gw.Resolve(ctx, m.post(p, t, false))
	if err != nil {
		return nil, err
	}

	t.Network = res.Network

	return t, nil
}

// Get reads a publication.
func (m *Manager) Get(ctx context.Context, id string) (*Publication, error) {
	p, err := m.env.Store().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	out := view(p)

	return &out, nil
}

// List reads the publications a filter selects.
func (m *Manager) List(ctx context.Context, f Filter) ([]Publication, int, error) {
	switch {
	case f.Limit <= 0:
		f.Limit = defaultListLimit
	case f.Limit > maxListLimit:
		f.Limit = maxListLimit
	}

	if f.Offset < 0 {
		f.Offset = 0
	}

	if f.State != "" && !slices.Contains(core.TargetStates, f.State) {
		return nil, 0, invalid("state", "is not a state of a target.")
	}

	list, total, err := m.env.Store().List(ctx, f)
	if err != nil {
		return nil, 0, err
	}

	for i := range list {
		list[i] = view(list[i])
	}

	return list, total, nil
}

// ask moves a target to waiting_approval with a new request (SDD-020 section 26). Whatever the target was
// asked about before is withdrawn and its reserved cost released. The caller saves nothing: ask does, and
// replaces p with what the store holds after a rule may have approved the request at once.
func (m *Manager) ask(ctx context.Context, tx Tx, p *Publication, id, reason string, now time.Time) error {
	t := p.Target(id)
	if t == nil {
		return ErrNotFound
	}

	if !core.CanTarget(t.State, core.TargetWaitingApproval) {
		return state("the post to %s is %s and cannot be scheduled.", t.Network, t.State)
	}

	if t.ScheduledAt == nil {
		return invalid("at", "give the post a time.").hint("pass the time with --at, or use send-now")
	}

	if t.ScheduledAt.Before(now.Add(-core.Grace)) {
		return invalid("at", "the time is in the past.").hint("choose a time that has not come, or use send-now")
	}

	res, err := m.gw.Resolve(ctx, m.post(p, t, true))
	if err != nil {
		return err
	}

	if len(res.Issues) > 0 {
		first := res.Issues[0]

		return (&Error{Kind: KindValidation, Path: first.Path, Message: first.Message, Issues: res.Issues}).
			hint("fix the fields named in the issues and schedule again")
	}

	if t.State.Waiting() && t.ApprovalID != "" {
		if err := tx.Withdraw(ctx, t.ApprovalID, reason); err != nil {
			return err
		}
	}

	if err := tx.Release(ctx, t.ID); err != nil {
		return err
	}

	sum := sha256.Sum256(res.Snapshot)
	t.State, t.ApprovalID = core.TargetWaitingApproval, m.env.NewID("apr")
	t.Snapshot, t.PayloadSHA256 = res.Snapshot, hex.EncodeToString(sum[:])
	t.Attempts, t.NextAttemptAt, t.LeaseOwner, t.LeaseUntil = 0, nil, "", nil
	t.RemoteID, t.URL, t.RemoteState, t.Error, t.PublishedAt = "", "", "", nil, nil
	t.Network, t.UpdatedAt, p.UpdatedAt = res.Network, now, now

	if err := tx.Store().Save(ctx, p); err != nil {
		return err
	}

	err = tx.Ask(ctx, Ask{
		ApprovalID:    t.ApprovalID,
		PublicationID: p.ID,
		TargetID:      t.ID,
		ProjectID:     p.ProjectID,
		ConnectionID:  t.ConnectionID,
		Summary:       m.summary(res, p, t),
		CostMicros:    res.CostMicros,
		Currency:      res.Currency,
	})
	if err != nil {
		return err
	}

	cur, err := tx.Store().Get(ctx, p.ID)
	if err != nil {
		return err
	}

	cur.Notes = p.Notes
	*p = cur

	return nil
}

// summary is what a person reads before approving, at most 500 characters: where, as whom, what, who may
// watch, the start of the text, the file and the time with its zone and its UTC time.
func (m *Manager) summary(res *Resolved, p *Publication, t *Target) string {
	clean := func(s string, n int) string {
		kept, _ := untrusted.Cut(untrusted.Clean(strings.ReplaceAll(s, "\n", " ")), n)

		return kept
	}

	f := m.post(p, t, false).Fields

	vis := f.Visibility
	if vis == "" {
		vis = string(core.Private)
	}

	slot := core.Slot{At: *t.ScheduledAt, Zone: t.Zone}
	sum := fmt.Sprintf(
		"Publish %q to %s, account %q (%s), visible to: %s; at %s %s (%s); file %s, %d bytes",
		clean(
			f.Title,
			summaryTitleRunes,
		),
		res.NetworkName,
		clean(res.Account, summaryAccountRunes),
		t.ConnectionID,
		vis,
		slot.Local(),
		t.Zone,
		slot.At.UTC().Format("2006-01-02T15:04:05Z"),
		clean(res.FileName, summaryFileRunes),
		res.FileBytes,
	)

	if text := clean(f.Description, summaryTextRunes); text != "" {
		sum += fmt.Sprintf("; text: %q", text)
	}

	return sum
}

// Schedule gives a target its time and asks for approval. A target that is already waiting or scheduled is
// moved to the new time with a new request.
func (m *Manager) Schedule(ctx context.Context, in ScheduleInput) (*Publication, error) {
	return m.change(ctx, in.ID, func(ctx context.Context, tx Tx, p *Publication, now time.Time) error {
		ids, err := m.pick(p, in.TargetID, true)
		if err != nil {
			return err
		}

		for _, id := range ids {
			t := p.Target(id)
			t.SendNow = false

			if note, err := m.applyTime(
				t,
				&TargetInput{At: in.At, Zone: in.Zone, LateWindowMinutes: in.LateWindowMinutes},
			); err != nil {
				return err
			} else if note != "" {
				p.Notes = append(p.Notes, note)
			}

			if err := m.ask(ctx, tx, p, id, "rescheduled", now); err != nil {
				return err
			}
		}

		return nil
	})
}

// SendNow asks for approval to send a target as soon as it is approved. The person who chooses it accepts
// a late approval up to a day, so a request answered in the afternoon still goes.
func (m *Manager) SendNow(ctx context.Context, in TargetRef) (*Publication, error) {
	return m.change(ctx, in.ID, func(ctx context.Context, tx Tx, p *Publication, now time.Time) error {
		if in.TargetID == "" {
			return invalid("target_id", "name the target to send.")
		}

		return m.sendNow(ctx, tx, p, in.TargetID, "", now)
	})
}

// Retry is SendNow for a target that failed, with its error cleared by the new request.
func (m *Manager) Retry(ctx context.Context, in TargetRef) (*Publication, error) {
	return m.change(ctx, in.ID, func(ctx context.Context, tx Tx, p *Publication, now time.Time) error {
		if in.TargetID == "" {
			return invalid("target_id", "name the target to retry.")
		}

		t := p.Target(in.TargetID)
		if t == nil {
			return ErrNotFound
		}

		if t.State != core.TargetFailed {
			return state("only a failed post is retried; this one is %s.", t.State)
		}

		return m.sendNow(ctx, tx, p, in.TargetID, "", now)
	})
}

func (m *Manager) sendNow(ctx context.Context, tx Tx, p *Publication, id, reason string, now time.Time) error {
	t := p.Target(id)
	if t == nil {
		return ErrNotFound
	}

	if t.State == core.TargetPublishing || t.State.Final() {
		return state("the post to %s is %s.", t.Network, t.State)
	}

	at := now
	t.ScheduledAt, t.SendNow = &at, true

	if t.Zone == "" {
		t.Zone = "UTC"
	}

	if reason == "" {
		reason = "sent now"
	}

	return m.ask(ctx, tx, p, id, reason, now)
}

// pick is the targets a schedule names: one, or every draft target that has a time.
func (m *Manager) pick(p *Publication, id string, needTime bool) ([]string, error) {
	if id != "" {
		if p.Target(id) == nil {
			return nil, ErrNotFound
		}

		return []string{id}, nil
	}

	var out []string

	for i := range p.Targets {
		if t := &p.Targets[i]; t.State == core.TargetDraft && (t.ScheduledAt != nil || !needTime) {
			out = append(out, t.ID)
		}
	}

	if len(out) == 0 {
		return nil, invalid(
			"target_id",
			"no target is waiting for a time.",
		).hint("name the target, or give a time to a draft")
	}

	return out, nil
}

// Cancel cancels one target, or every one that has not gone out. Its request is rejected and its cost
// released.
func (m *Manager) Cancel(ctx context.Context, in TargetRef) (*Publication, error) {
	return m.change(ctx, in.ID, func(ctx context.Context, tx Tx, p *Publication, now time.Time) error {
		var ids []string

		if in.TargetID != "" {
			if p.Target(in.TargetID) == nil {
				return ErrNotFound
			}

			ids = []string{in.TargetID}
		} else {
			for i := range p.Targets {
				if !p.Targets[i].State.Final() {
					ids = append(ids, p.Targets[i].ID)
				}
			}
		}

		for _, id := range ids {
			if err := m.cancel(ctx, tx, p, id, now); err != nil {
				return err
			}
		}

		return tx.Store().Save(ctx, p)
	})
}

func (m *Manager) cancel(ctx context.Context, tx Tx, p *Publication, id string, now time.Time) error {
	t := p.Target(id)

	if t.State == core.TargetPublishing {
		return state("the post to %s is being sent and cannot be cancelled.", t.Network).
			hint("wait for it to end, then read its result")
	}

	if t.State == core.TargetPublished {
		return state("the post to %s was published; cancel it on the network.", t.Network)
	}

	if t.State == core.TargetCancelled {
		return nil
	}

	if t.State == core.TargetWaitingApproval && t.ApprovalID != "" {
		if err := tx.Withdraw(ctx, t.ApprovalID, "cancelled by "+m.env.Actor(ctx).ID); err != nil {
			return err
		}
	}

	if err := tx.Release(ctx, t.ID); err != nil {
		return err
	}

	t.State, t.UpdatedAt, p.UpdatedAt = core.TargetCancelled, now, now
	t.LeaseOwner, t.LeaseUntil, t.NextAttemptAt = "", nil, nil

	return nil
}

// change runs one operation on a publication in one transaction and emits the result.
func (m *Manager) change(
	ctx context.Context, id string, fn func(ctx context.Context, tx Tx, p *Publication, now time.Time) error,
) (*Publication, error) {
	var out Publication

	err := m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		p, err := tx.Store().Get(ctx, id)
		if err != nil {
			return err
		}

		p.Notes = nil

		if err := fn(ctx, tx, &p, m.env.Now()); err != nil {
			return err
		}

		cur, err := tx.Store().Get(ctx, id)
		if err != nil {
			return err
		}

		cur.Notes = p.Notes
		out = view(cur)
		tx.Emit(out)

		return nil
	})
	if err != nil {
		return nil, err
	}

	m.changed()

	return &out, nil
}

// Update changes a publication. A target that was waiting for approval, approved or scheduled and whose
// post changed returns to waiting_approval with a new request: what a person approved is exactly what is
// sent (SDD-020 section 24).
func (m *Manager) Update(ctx context.Context, in UpdateInput) (*Publication, error) {
	return m.change(ctx, in.ID, func(ctx context.Context, tx Tx, p *Publication, now time.Time) error {
		shared, err := m.applyShared(p, &in)
		if err != nil {
			return err
		}

		changed := map[string]bool{}

		if shared {
			for i := range p.Targets {
				if st := p.Targets[i].State; st == core.TargetPublishing {
					return state("a post is being sent: change the publication when it ends.")
				} else if st != core.TargetPublished && st != core.TargetCancelled {
					changed[p.Targets[i].ID] = true
				}
			}
		}

		if len(in.Targets) > MaxTargets {
			return invalid("targets", "at most %d targets at a time.", MaxTargets)
		}

		for i := range in.Targets {
			if err := m.patchTarget(ctx, tx, p, &in.Targets[i], i, changed, now); err != nil {
				return err
			}
		}

		if len(p.Targets) > MaxTargets {
			return invalid("targets", "at most %d targets.", MaxTargets)
		}

		p.UpdatedAt = now

		if err := tx.Store().Save(ctx, p); err != nil {
			return err
		}

		return m.askChanged(ctx, tx, p, changed, now)
	})
}

// askChanged asks again for the targets whose post changed while they waited for a person or a time.
func (m *Manager) askChanged(ctx context.Context, tx Tx, p *Publication, changed map[string]bool, now time.Time) error {
	ids := make([]string, 0, len(changed))
	for id := range changed {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	for _, id := range ids {
		t := p.Target(id)
		if t == nil || !t.State.Waiting() {
			continue
		}

		if err := m.ask(ctx, tx, p, id, "edited", now); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) applyShared(p *Publication, in *UpdateInput) (bool, error) {
	changed := false
	set := func(dst *string, src *string, path string, lo, hi int, multi bool) error {
		if src == nil || *src == *dst {
			return nil
		}

		if err := cleanText(path, *src, lo, hi, multi); err != nil {
			return err
		}

		*dst, changed = *src, true

		return nil
	}

	if err := set(&p.Title, in.Title, "title", 1, MaxTitleRunes, false); err != nil {
		return false, err
	}

	if err := set(&p.Text, in.Text, "text", 0, MaxTextBytes, true); err != nil {
		return false, err
	}

	for _, f := range []struct {
		dst, src *string
		path     string
	}{{&p.Export, in.Export, "export"}, {&p.Thumbnail, in.Thumbnail, "thumbnail"}} {
		if f.src == nil || *f.src == *f.dst {
			continue
		}

		if f.path == "export" && *f.src == "" {
			return false, invalid("export", "a publication needs its video.")
		}

		*f.dst, changed = *f.src, true
	}

	return changed, nil
}

func (m *Manager) patchTarget(
	ctx context.Context, tx Tx, p *Publication, in *TargetInput, i int, changed map[string]bool, now time.Time,
) error {
	path := fmt.Sprintf("targets[%d]", i)

	if in.ID == "" {
		if in.Remove {
			return invalid(path+".id", "name the target to remove.")
		}

		t, err := m.newTarget(ctx, p, in, path, now)
		if err != nil {
			return err
		}

		t.Position = len(p.Targets)
		p.Targets = append(p.Targets, *t)

		return nil
	}

	t := p.Target(in.ID)
	if t == nil {
		return invalid(path+".id", "is not a target of this publication.")
	}

	if t.State == core.TargetPublishing || t.State == core.TargetPublished || t.State == core.TargetCancelled {
		return state("the post to %s is %s and cannot change.", t.Network, t.State)
	}

	if in.Remove {
		return m.removeTarget(ctx, tx, p, in.ID)
	}

	before := *t

	if in.ConnectionID != "" && in.ConnectionID != t.ConnectionID {
		t.ConnectionID = in.ConnectionID

		res, err := m.gw.Resolve(ctx, m.post(p, t, false))
		if err != nil {
			return err
		}

		t.Network = res.Network
	}

	// The fields of an update replace the target's own as a whole, so a person can clear one; an update that
	// names none leaves them.
	if !fieldsEmpty(&in.Fields) {
		if err := checkFields(path+".fields", &in.Fields); err != nil {
			return err
		}

		t.Fields = in.Fields
	}

	note, err := m.applyTime(t, in)
	if err != nil {
		return err
	}

	if note != "" {
		p.Notes = append(p.Notes, note)
	}

	if !sameTarget(&before, t) {
		changed[t.ID] = true
		t.UpdatedAt = now
	}

	return nil
}

func (m *Manager) removeTarget(ctx context.Context, tx Tx, p *Publication, id string) error {
	t := p.Target(id)

	if t.State == core.TargetWaitingApproval && t.ApprovalID != "" {
		if err := tx.Withdraw(ctx, t.ApprovalID, "removed by "+m.env.Actor(ctx).ID); err != nil {
			return err
		}
	}

	if err := tx.Release(ctx, id); err != nil {
		return err
	}

	p.Targets = slices.DeleteFunc(p.Targets, func(x Target) bool { return x.ID == id })

	return nil
}

func fieldsEmpty(f *Fields) bool {
	return f.Title == "" && f.Description == "" && len(f.Tags) == 0 && f.Visibility == "" && f.MadeForKids == nil &&
		!f.AIDisclosure
}

// sameTarget says whether two versions of a target would send the same post at the same time.
func sameTarget(a, b *Target) bool {
	return a.ConnectionID == b.ConnectionID && sameFields(&a.Fields, &b.Fields) && a.Zone == b.Zone &&
		a.LateWindowMinutes == b.LateWindowMinutes && ptrEqual(a.ScheduledAt, b.ScheduledAt)
}

func sameFields(a, b *Fields) bool {
	kidsA, kidsB := a.MadeForKids, b.MadeForKids

	return a.Title == b.Title && a.Description == b.Description && slices.Equal(a.Tags, b.Tags) &&
		a.Visibility == b.Visibility && a.AIDisclosure == b.AIDisclosure &&
		(kidsA == nil) == (kidsB == nil) && (kidsA == nil || *kidsA == *kidsB)
}

func ptrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Equal(*b)
}

// Approved is what approving the request of a target does, inside the transaction of the approval
// (SDD-020 section 26): the target is scheduled while its time is ahead, approved when it has come, and
// missed when the person answered after its window.
func (m *Manager) Approved(ctx context.Context, tx Tx, targetID, approvalID string) error {
	p, err := tx.Store().OfTarget(ctx, targetID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}

		return err
	}

	t := p.Target(targetID)
	if t.State != core.TargetWaitingApproval || t.ApprovalID != approvalID {
		return nil
	}

	now := m.env.Now()

	switch {
	case t.ScheduledAt == nil || !core.OnTime(*t.ScheduledAt, core.EffectiveLate(t.LateWindowMinutes, t.SendNow), now):
		t.State = core.TargetMissed

		if err := tx.Release(ctx, t.ID); err != nil {
			return err
		}
	case t.ScheduledAt.After(now):
		t.State = core.TargetScheduled
	default:
		t.State = core.TargetApproved
	}

	t.UpdatedAt, p.UpdatedAt = now, now

	if err := tx.Store().Save(ctx, &p); err != nil {
		return err
	}

	tx.Emit(view(p))

	return nil
}

// The codes of the reasons a request did not become a send.
const (
	CodeRejected = "approval_rejected"
	CodeExpired  = "approval_expired"
)

// Declined is what a rejected or expired request does to its target: back to a draft, with the reason, so
// the person can change it and ask again. A request that was replaced does nothing.
func (m *Manager) Declined(ctx context.Context, tx Tx, targetID, approvalID, code, message string) error {
	p, err := tx.Store().OfTarget(ctx, targetID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}

		return err
	}

	t := p.Target(targetID)
	if t.State != core.TargetWaitingApproval || t.ApprovalID != approvalID {
		return nil
	}

	now := m.env.Now()
	t.State, t.ApprovalID = core.TargetDraft, ""
	t.Error = &TargetError{Code: code, Message: message, Hint: "change the post if you want, then schedule it again"}
	t.UpdatedAt, p.UpdatedAt = now, now

	if err := tx.Store().Save(ctx, &p); err != nil {
		return err
	}

	tx.Emit(view(p))

	return nil
}

// Open returns the targets the scheduler looks at.
func (m *Manager) Open(ctx context.Context) ([]Target, error) { return m.env.Store().Open(ctx) }

// Claim takes the lease of a target for a scheduler.
func (m *Manager) Claim(ctx context.Context, id, owner string, now, until time.Time) (bool, error) {
	return m.env.Store().Claim(ctx, id, owner, now, until)
}

// claimed reads a target this owner just claimed, as the store holds it after the claim.
func (m *Manager) claimed(ctx context.Context, id, owner string) (Target, error) {
	p, err := m.env.Store().OfTarget(ctx, id)
	if err != nil {
		return Target{}, err
	}

	t := p.Target(id)
	if t == nil || t.State != core.TargetPublishing || t.LeaseOwner != owner {
		return Target{}, ErrLeaseLost
	}

	return *t, nil
}

// Renew extends a lease.
func (m *Manager) Renew(ctx context.Context, id, owner string, until time.Time) (bool, error) {
	return m.env.Store().Renew(ctx, id, owner, until)
}

// Send sends a claimed target through the gateway.
func (m *Manager) Send(ctx context.Context, t Target, report func(float64, string)) (Sent, error) {
	// The digest was taken when the request was made: a snapshot that does not match it is not what the
	// person was shown and is never sent.
	sum := sha256.Sum256(t.Snapshot)
	if len(t.Snapshot) == 0 || hex.EncodeToString(sum[:]) != t.PayloadSHA256 {
		return Sent{}, &SendError{
			Kind: "integrity", Code: "payload_changed",
			Message: "the post is not the one that was approved.",
			Hint:    "schedule the post again and approve it",
		}
	}

	return m.gw.Send(ctx, t, report)
}

// ErrLeaseLost is a result that arrived after the lease of the target ended.
var ErrLeaseLost = errors.New("publish: the lease was lost")

// Finish records the end of a send in one transaction: the result and the settlement of the cost, or the
// error and either a later attempt (a transient error with attempts left) or the end (SDD-020 section 27).
func (m *Manager) Finish(ctx context.Context, owner, targetID string, sent *Sent, serr *SendError) error {
	err := m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		p, err := tx.Store().OfTarget(ctx, targetID)
		if err != nil {
			return err
		}

		t := p.Target(targetID)
		if t.State != core.TargetPublishing || t.LeaseOwner != owner {
			return ErrLeaseLost
		}

		now := m.env.Now()
		t.LeaseOwner, t.LeaseUntil, t.UpdatedAt, p.UpdatedAt = "", nil, now, now

		if serr == nil {
			t.State, t.Error, t.NextAttemptAt = core.TargetPublished, nil, nil
			t.RemoteID, t.URL, t.RemoteState = sent.RemoteID, sent.URL, sent.RemoteState
			published := now
			t.PublishedAt = &published

			if err := tx.Settle(ctx, t.ID); err != nil {
				return err
			}
		} else if err := m.failed(ctx, tx, t, serr, now); err != nil {
			return err
		}

		if err := tx.Store().Save(ctx, &p); err != nil {
			return err
		}

		tx.Emit(view(p))

		return nil
	})
	if err == nil {
		m.changed()
	}

	return err
}

func (m *Manager) failed(ctx context.Context, tx Tx, t *Target, serr *SendError, now time.Time) error {
	delay, again := core.RetryDelay(t.Attempts)
	transient := core.Transient(serr.Kind) && again

	t.Error = &TargetError{
		Code: serr.Code, Kind: serr.Kind, Message: serr.Message, Hint: serr.Hint, Retryable: core.Transient(serr.Kind),
	}

	if transient {
		next := now.Add(delay)
		t.State, t.NextAttemptAt = core.TargetScheduled, &next

		return nil
	}

	t.State, t.NextAttemptAt = core.TargetFailed, nil

	return tx.Release(ctx, t.ID)
}

// Miss marks a target missed when, at this moment, its time passed beyond its window; it reports whether it
// did. The target is read again in the transaction, so a send that began meanwhile is not undone.
func (m *Manager) Miss(ctx context.Context, targetID string) (bool, error) {
	done := false

	err := m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		p, err := tx.Store().OfTarget(ctx, targetID)
		if err != nil {
			return err
		}

		t := p.Target(targetID)
		now := m.env.Now()

		if core.Decide(t.Candidate(), now) != core.ActionMiss {
			return nil
		}

		t.State, t.UpdatedAt, p.UpdatedAt = core.TargetMissed, now, now
		t.Error = &TargetError{
			Code:    "missed",
			Message: "the time passed while nothing was sending; the post was not sent.",
			Hint:    "send it now, or give it a new time",
		}

		if err := tx.Release(ctx, t.ID); err != nil {
			return err
		}

		if err := tx.Store().Save(ctx, &p); err != nil {
			return err
		}

		tx.Emit(view(p))

		done = true

		return nil
	})

	return done, err
}

// Interrupt ends a target that was being sent and whose lease ran out when its attempts are used.
func (m *Manager) Interrupt(ctx context.Context, targetID string) (bool, error) {
	done := false

	err := m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		p, err := tx.Store().OfTarget(ctx, targetID)
		if err != nil {
			return err
		}

		t := p.Target(targetID)
		now := m.env.Now()

		if core.Decide(t.Candidate(), now) != core.ActionResume {
			return nil
		}

		t.State, t.LeaseOwner, t.LeaseUntil, t.NextAttemptAt = core.TargetFailed, "", nil, nil
		t.UpdatedAt, p.UpdatedAt = now, now
		t.Error = &TargetError{
			Code:    "interrupted",
			Message: "the app stopped while the post was being sent, too many times.",
			Hint:    "look at the account to see whether the post is there, then retry",
		}

		if err := tx.Release(ctx, t.ID); err != nil {
			return err
		}

		if err := tx.Store().Save(ctx, &p); err != nil {
			return err
		}

		tx.Emit(view(p))

		done = true

		return nil
	})

	return done, err
}
