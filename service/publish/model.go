// Package publish is the calendar of publications and the scheduler that sends an approved post at its
// time (SDD-020 sections 15 to 21, ADR-0046). The rules are pure and live in core/publish; this package
// holds the plan (a publication and its targets), the operations over it, the approval of a scheduled post
// and the loop that claims a due target with a lease, sends it and records the result. It reaches the
// network only through a Gateway the service supplies, which goes through connections.Manager.With.
package publish

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

// TargetState is the state of a target (core/publish), named here so the store needs no import of core/.
type TargetState = core.TargetState

// Fields is the post of one target: what a network receives. An empty Title or Description is the one of
// the publication.
type Fields struct {
	Title        string   `json:"title,omitempty"         untrusted:"true"`
	Description  string   `json:"description,omitempty"   untrusted:"true"`
	Tags         []string `json:"tags,omitempty"          untrusted:"true"`
	Visibility   string   `json:"visibility,omitempty"`
	MadeForKids  *bool    `json:"made_for_kids,omitempty"`
	AIDisclosure bool     `json:"ai_disclosure,omitempty"`
}

// TargetError is why a target did not go out: written by the service from a kind and a reason code, never
// copied from the answer of a network (SDD-020 section 7.3).
type TargetError struct {
	Code      string `json:"code"`
	Kind      string `json:"kind,omitempty"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	Retryable bool   `json:"retryable"`
}

// Target is one account a publication goes to, at one time (SDD-020 section 15).
type Target struct {
	ID            string           `json:"id"`
	PublicationID string           `json:"publication_id"`
	ConnectionID  string           `json:"connection_id"`
	Network       string           `json:"network"`
	Fields        Fields           `json:"fields"`
	State         core.TargetState `json:"state"`
	// ScheduledAt is the instant, in UTC; Zone the IANA zone the person chose; LocalTime the instant on the
	// wall clock of that zone, for a person to read.
	ScheduledAt       *time.Time   `json:"scheduled_at"`
	Zone              string       `json:"zone"`
	LocalTime         string       `json:"local_time,omitempty"`
	LateWindowMinutes int          `json:"late_window_minutes"`
	SendNow           bool         `json:"send_now,omitempty"`
	ApprovalID        string       `json:"approval_id,omitempty"`
	Attempts          int          `json:"attempts"`
	NextAttemptAt     *time.Time   `json:"next_attempt_at,omitempty"`
	RemoteID          string       `json:"remote_id,omitempty"       untrusted:"true"`
	URL               string       `json:"url,omitempty"             untrusted:"true"`
	RemoteState       string       `json:"remote_state,omitempty"`
	Error             *TargetError `json:"error,omitempty"`
	PublishedAt       *time.Time   `json:"published_at,omitempty"`
	UpdatedAt         time.Time    `json:"updated_at"`

	// The rest is kept and never shown.
	Position      int             `json:"-"`
	Snapshot      json.RawMessage `json:"-"`
	PayloadSHA256 string          `json:"-"`
	LeaseOwner    string          `json:"-"`
	LeaseUntil    *time.Time      `json:"-"`
	CreatedAt     time.Time       `json:"-"`
}

// Candidate is the part of the target the schedule rules of core/publish read.
func (t *Target) Candidate() core.Candidate {
	c := core.Candidate{
		ID:                t.ID,
		State:             t.State,
		LateWindowMinutes: core.EffectiveLate(t.LateWindowMinutes, t.SendNow),
		Attempts:          t.Attempts,
	}

	if t.ScheduledAt != nil {
		c.At = *t.ScheduledAt
	}

	if t.NextAttemptAt != nil {
		c.NextAttempt = *t.NextAttemptAt
	}

	if t.LeaseUntil != nil {
		c.LeaseUntil = *t.LeaseUntil
	}

	return c
}

// Publication is what is published: shared by its targets (SDD-020 section 15).
type Publication struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"project_id"`
	Title     string   `json:"title"               untrusted:"true"`
	Text      string   `json:"text,omitempty"      untrusted:"true"`
	Export    string   `json:"export"`
	Thumbnail string   `json:"thumbnail,omitempty"`
	Targets   []Target `json:"targets"`
	// Notes say what the service did to a time it was given, for example that the local time did not exist.
	Notes     []string  `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	CreatedBy   string `json:"-"`
	OwnerUserID string `json:"-"`
}

// Target returns the target with an id.
func (p *Publication) Target(id string) *Target {
	for i := range p.Targets {
		if p.Targets[i].ID == id {
			return &p.Targets[i]
		}
	}

	return nil
}

// Filter selects publications. State, Network and the time range apply to the targets: a publication is
// listed when one of its targets matches, and it is listed with all of them.
type Filter struct {
	ProjectID   string
	State       core.TargetState
	Network     string
	OwnerUserID string
	// From and To bound the time of a target, From inclusive and To exclusive.
	From, To *time.Time
	Offset   int
	Limit    int
}

// ErrNotFound is a publication or a target that does not exist.
var ErrNotFound = errors.New("publish: not found")

// Store is where publications live (the tables of SDD-005 section 5.14). On a transaction it is bound to it.
type Store interface {
	// Insert adds a publication with its targets.
	Insert(ctx context.Context, p *Publication) error
	// Get returns a publication with its targets, or ErrNotFound.
	Get(ctx context.Context, id string) (Publication, error)
	// OfTarget returns the publication a target belongs to, or ErrNotFound.
	OfTarget(ctx context.Context, targetID string) (Publication, error)
	// Save writes the publication and replaces its targets with the ones given.
	Save(ctx context.Context, p *Publication) error
	// List returns the publications newest first, or in the order of their first matching time when the
	// filter has a time range; total is the number that match before Offset and Limit.
	List(ctx context.Context, f Filter) (pubs []Publication, total int, err error)
	// OfProject returns every publication of a project.
	OfProject(ctx context.Context, projectID string) ([]Publication, error)
	// Open returns the targets the scheduler looks at: approved, scheduled and publishing, of projects that
	// are not in the trash.
	Open(ctx context.Context) ([]Target, error)
	// Claim takes the lease of a target, once: false when it is not due, or someone holds the lease. It sets
	// the state publishing, the owner and the end of the lease, and counts one attempt.
	Claim(ctx context.Context, id, owner string, now, until time.Time) (bool, error)
	// Renew extends the lease the owner holds; false when it no longer holds it.
	Renew(ctx context.Context, id, owner string, until time.Time) (bool, error)
}
