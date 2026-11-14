package service

import (
	"context"
	"errors"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Service is the single application boundary (ADR-0008): the UI, MCP, CLI and
// HTTP adapters call it. This is the part of the interface of SDD-003 section
// 2.1 for projects, edits, history and lint; the media, job, template and
// system methods join it with their tasks. Every method returns a *Error on
// failure and never a partial result with a non-nil error.
type Service interface {
	ProjectList(ctx context.Context, req ProjectListRequest) (*ProjectListResult, error)
	ProjectCreate(ctx context.Context, req ProjectCreateRequest) (*ProjectCreateResult, error)
	ProjectGet(ctx context.Context, req ProjectGetRequest) (*ProjectGetResult, error)
	ProjectTrash(ctx context.Context, req ProjectTrashRequest) (*ProjectStateResult, error)
	ProjectRestore(ctx context.Context, req ProjectTrashRequest) (*ProjectStateResult, error)

	Edit(ctx context.Context, req EditRequest) (*EditResult, error)
	CommandHelp(ctx context.Context, req CommandHelpRequest) (*CommandHelpResult, error)

	HistoryList(ctx context.Context, req HistoryListRequest) (*HistoryListResult, error)
	Undo(ctx context.Context, req UndoRequest) (*UndoResult, error)
	Redo(ctx context.Context, req UndoRequest) (*UndoResult, error)

	Lint(ctx context.Context, req LintRequest) (*LintResult, error)
}

// Event is what a commit publishes after it is durable (SDD-006 section 3.2).
// The hub that carries it is the business of the events task; the service
// only calls Options.Publish.
type Event struct {
	Type        string // "batch.committed" or "project.updated"
	ProjectID   string
	Revision    int64
	Kind        string // edit, undo or redo, for batch.committed
	Actor       string
	Description string
	At          time.Time
}

// Options configure New. Store is required.
type Options struct {
	Store Store
	// Registry is the command registry of the build; default command.Default().
	Registry *command.Registry
	// Now is the service clock (SDD-005 section 2 item 2); default time.Now.
	Now func() time.Time
	// NewIDs makes identifiers; default timeline.NewID (cryptographic).
	NewIDs func(prefix string) string
	// Publish receives events after the commit, in revision order per project.
	Publish func(Event)
	// Lint is the environment of the lint rules. SafeArea defaults to
	// plan.SafeArea; TextSize stays nil until a rasterizer supplies it, which
	// skips L_SAFE_AREA.
	Lint lint.Env
}

type svc struct {
	store   Store
	reg     *command.Registry
	now     func() time.Time
	newIDs  func(string) string
	publish func(Event)
	lintEnv lint.Env
}

// New returns the in-process Service over a store.
func New(o Options) (Service, error) {
	if o.Store == nil {
		return nil, errors.New("service: Options.Store is required")
	}

	s := &svc{store: o.Store, reg: o.Registry, now: o.Now, newIDs: o.NewIDs, publish: o.Publish, lintEnv: o.Lint}

	if s.reg == nil {
		s.reg = command.Default()
	}

	if s.now == nil {
		s.now = time.Now
	}

	if s.newIDs == nil {
		s.newIDs = timeline.NewID
	}

	if s.lintEnv.SafeArea == nil {
		s.lintEnv.SafeArea = func(w, h int) (x, y, sw, sh int) {
			r := plan.SafeArea(w, h)
			return int(r.X), int(r.Y), int(r.W), int(r.H)
		}
	}

	return s, nil
}

func (s *svc) nowMs() int64 { return s.now().UnixMilli() }

// emit publishes after the commit; a nil Publish is no event.
func (s *svc) emit(e Event) {
	if s.publish != nil {
		s.publish(e)
	}
}
