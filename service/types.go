package service

import (
	"encoding/json"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// Page selects a page of a list (SDD-003 section 1.5).
type Page struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"` // default: per operation, SDD-003 section 3.10
}

// Envelope is embedded in every result that has a field tagged untrusted:"true".
type Envelope struct {
	UntrustedNotice string `json:"untrusted_notice,omitempty"`
}

// UntrustedNotice is the sentence of SDD-003 section 1.3.
const UntrustedNotice = untrusted.Notice

// TimeValue is a time in project terms (SDD-002 section A.4).
type TimeValue struct {
	Frame   int64   `json:"frame"`
	Seconds float64 `json:"seconds"` // frame * fps.den / fps.num, rounded to 3 decimals
}

// ProjectSummary is an item of the project listing.
type ProjectSummary struct {
	ID string `json:"id"`
	// ProjectID is ID under the name every other answer gives it (VV-107); ID stays for compatibility.
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"       untrusted:"true"`
	Revision  int64     `json:"revision"`
	State     string    `json:"state"` // "active" | "trashed"
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	FPS       string    `json:"fps"`      // "30" or "30000/1001"
	Duration  TimeValue `json:"duration"` // end of the last item
	Tracks    int       `json:"tracks"`
	Items     int       `json:"items"`
	Assets    int       `json:"assets"`
	// Timelines is how many timelines the project holds; the other members are those of the first of them.
	Timelines int       `json:"timelines"`
	UpdatedAt time.Time `json:"updated_at"` // UTC
}

// ProjectListRequest lists projects.
type ProjectListRequest struct {
	State string `json:"state,omitempty"` // default "active"; "trashed" | "all"
	Page
}

// ProjectListResult is a page of projects.
type ProjectListResult struct {
	Envelope
	Projects   []ProjectSummary `json:"projects"`
	NextCursor *string          `json:"next_cursor"`
	Text       string           `json:"text"`
}

// ProjectCreateRequest creates an empty project.
type ProjectCreateRequest struct {
	Name   string `json:"name"`             // 1 to 120 characters
	Preset string `json:"preset,omitempty"` // default "9:16"; "1:1" | "16:9" | "4:5"
	FPS    string `json:"fps,omitempty"`    // default "30"; "<n>" or "<n>/1001"
	ID     string `json:"id,omitempty"`     // optional caller-chosen id; E_ID_EXISTS if taken
	DryRun bool   `json:"dry_run,omitempty"`
}

// ProjectCreateResult is the created project.
type ProjectCreateResult struct {
	Envelope
	ProjectID string `json:"project_id"`
	Name      string `json:"name"       untrusted:"true"`
	Revision  int64  `json:"revision"` // 0: a new project has an empty document and no batch yet
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	FPS       string `json:"fps"`
	Created   bool   `json:"created"` // false on a dry run
	Text      string `json:"text"`
}

// ProjectGetRequest reads a project.
type ProjectGetRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	Detail     string `json:"detail,omitempty"`      // default "concise"; "full"; "document"
	Page
}

// ProjectCounts counts what a project holds.
type ProjectCounts struct {
	Tracks       int `json:"tracks"`
	Items        int `json:"items"`
	Assets       int `json:"assets"`
	CaptionWords int `json:"caption_words"`
}

// ProjectGetResult is the head of a project and, by detail, its text or document.
type ProjectGetResult struct {
	Envelope
	ProjectID      string        `json:"project_id"`
	Name           string        `json:"name"            untrusted:"true"`
	Revision       int64         `json:"revision"`
	State          string        `json:"state"`
	Width          int           `json:"width"`
	Height         int           `json:"height"`
	FPS            string        `json:"fps"`
	DurationFrames int64         `json:"duration_frames"`
	Duration       TimeValue     `json:"duration"`
	Counts         ProjectCounts `json:"counts"`
	// TimelineID is the timeline the head and the text are about.
	TimelineID string             `json:"timeline_id,omitempty"`
	Items      []ProjectItem      `json:"items"`
	Production ProductionState    `json:"production"`
	NextCursor *string            `json:"next_cursor"`
	Document   *timeline.Document `json:"document,omitempty"    bulk:"true"`
	Text       string             `json:"text,omitempty"`
}

// ProjectTrashRequest trashes or restores a project.
type ProjectTrashRequest struct {
	ProjectID string `json:"project_id"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ProjectStateResult is the state a project ended in.
type ProjectStateResult struct {
	ProjectID string `json:"project_id"`
	State     string `json:"state"` // "active" | "trashed"
	Text      string `json:"text"`
}

// EditRequest is a batch of operations (SDD-003 section 2.4).
type EditRequest struct {
	ProjectID      string              `json:"project_id"`
	TimelineID     string              `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	BaseRevision   int64               `json:"base_revision"`
	Operations     []command.Operation `json:"operations"` // 1 to 200
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	DryRun         bool                `json:"dry_run,omitempty"`
}

// OpEffect is what one operation of a batch did.
type OpEffect struct {
	Index     int            `json:"index"`
	Op        string         `json:"op"`
	Effective map[string]any `json:"effective"`
	Created   []string       `json:"created,omitempty"`
}

// LintScopeFast marks totals computed by the fast rules only.
const LintScopeFast = "fast"

// LintTotals counts findings. In an edit result LintScope is "fast": only the
// doc and item rules ran, so the totals can differ from lint of the project,
// which runs every rule (VV-082).
type LintTotals struct {
	Errors    int    `json:"errors"`
	Warnings  int    `json:"warnings"`
	LintScope string `json:"lint_scope,omitempty"`
}

// EditResult is the outcome of a batch.
type EditResult struct {
	Envelope
	ProjectID    string           `json:"project_id"`
	Revision     int64            `json:"revision"`
	BaseRevision int64            `json:"base_revision"`
	Applied      bool             `json:"applied"`
	DryRun       bool             `json:"dry_run"`
	Replayed     bool             `json:"replayed"`
	Actor        string           `json:"actor"`
	Created      []string         `json:"created"`
	Effective    []OpEffect       `json:"effective"`
	Diff         []command.Change `json:"diff"`
	Findings     []lint.Finding   `json:"findings"`
	Lint         LintTotals       `json:"lint"`
	Description  string           `json:"description"` // summaries of the commands, one line (SDD-012 section 15)
}

// HistoryListRequest pages the history, newest first.
type HistoryListRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	Actor      string `json:"actor,omitempty"`
	Page
}

// HistoryEntry is one step of the history.
type HistoryEntry struct {
	Revision     int64     `json:"revision"`
	Kind         string    `json:"kind"`
	Actor        string    `json:"actor"`
	At           time.Time `json:"at"`
	Ops          []string  `json:"ops"`
	Changes      int       `json:"changes"`
	Description  string    `json:"description"`    // summaries of the commands, one line (SDD-012 section 15)
	UndoableByMe bool      `json:"undoable_by_me"` // the step Undo takes for the caller now
	RedoableByMe bool      `json:"redoable_by_me"` // the step Redo takes for the caller now
	UndoOf       *int64    `json:"undo_of,omitempty"`
	UndoneBy     *int64    `json:"undone_by,omitempty"`
}

// HistoryListResult is a page of the history.
type HistoryListResult struct {
	ProjectID  string         `json:"project_id"`
	Revision   int64          `json:"revision"`
	Entries    []HistoryEntry `json:"entries"`
	NextCursor *string        `json:"next_cursor"`
	Text       string         `json:"text"`
}

// UndoRequest undoes or redoes one step.
type UndoRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	DryRun     bool   `json:"dry_run,omitempty"`
}

// HistoryRef names a step of the history.
type HistoryRef struct {
	Revision int64     `json:"revision"`
	Kind     string    `json:"kind"`
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
}

// SkippedOp is a primitive an undo or redo did not apply (SDD-005 section 12.3).
type SkippedOp struct {
	PrimitiveIndex int             `json:"primitive_index"`
	Primitive      json.RawMessage `json:"primitive"`
	Fields         []string        `json:"fields,omitempty"`
	Reason         string          `json:"reason"`
	ByRevision     *int64          `json:"by_revision,omitempty"`
	ByActor        string          `json:"by_actor,omitempty"`
	Details        map[string]any  `json:"details,omitempty"`
}

// UndoResult is an EditResult plus the step undone or redone and what was skipped.
type UndoResult struct {
	EditResult
	Kind    string      `json:"kind"` // "undo" | "redo"
	Undone  *HistoryRef `json:"undone,omitempty"`
	Redone  *HistoryRef `json:"redone,omitempty"`
	Skipped []SkippedOp `json:"skipped"`
}

// LintRequest runs the whole lint on the current revision.
type LintRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	Severity   string `json:"severity,omitempty"`    // minimum to list; default "warning" (everything); "error"
	Page
}

// LintResult is a page of findings with the totals over all of them.
type LintResult struct {
	ProjectID  string         `json:"project_id"`
	Revision   int64          `json:"revision"`
	Errors     int            `json:"errors"`
	Warnings   int            `json:"warnings"`
	Findings   []lint.Finding `json:"findings"`
	NextCursor *string        `json:"next_cursor"`
	Text       string         `json:"text"`
}
