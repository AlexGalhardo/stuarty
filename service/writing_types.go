package service

import (
	"context"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/writing"
)

// WritingService is the part of the service that works on the texts of a production (SDD-019): the scripts, the
// copy and the captions, their revisions, the checks against a network and the draft made with a model. Service
// embeds it.
type WritingService interface {
	TextList(ctx context.Context, req TextListRequest) (*TextListResult, error)
	TextGet(ctx context.Context, req TextGetRequest) (*TextGetResult, error)
	TextWrite(ctx context.Context, req TextWriteRequest) (*TextWriteResult, error)
	TextRevisions(ctx context.Context, req TextRevisionsRequest) (*TextRevisionsResult, error)
	TextRevert(ctx context.Context, req TextRevertRequest) (*TextWriteResult, error)
	TextDraft(ctx context.Context, req TextDraftRequest) (*Job, error)
	CaptionValidate(ctx context.Context, req CaptionValidateRequest) (*CaptionValidateResult, error)
	ScriptEstimate(ctx context.Context, req ScriptEstimateRequest) (*ScriptEstimateResult, error)
}

// TextItem is a text of a production without its body.
type TextItem struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	ProjectName string    `json:"project_name,omitempty" untrusted:"true"`
	Title       string    `json:"title"                  untrusted:"true"`
	TextKind    string    `json:"text_kind"` // "script" | "copy" | "caption"
	State       string    `json:"state"`     // "active" | "trashed"
	Words       int       `json:"words"`
	Revision    int       `json:"revision"` // the latest revision
	UpdatedBy   string    `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TextRevision is one revision of a text, without its body.
type TextRevision struct {
	Revision   int         `json:"revision"`
	Actor      string      `json:"actor"`
	Source     string      `json:"source"` // "created" | "write" | "generated" | "revert"
	RevertedTo *int        `json:"reverted_to,omitempty"`
	Provider   string      `json:"provider,omitempty"`
	Model      string      `json:"model,omitempty"       untrusted:"true"`
	Cost       *core.Money `json:"cost,omitempty"`
	JobID      string      `json:"job_id,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

// TextListRequest lists texts. With no project it lists those of every project the caller may see.
type TextListRequest struct {
	ProjectID string `json:"project_id,omitempty"`
	TextKind  string `json:"text_kind,omitempty"` // "script", "copy" or "caption"; default: every kind
	State     string `json:"state,omitempty"`     // "active" (default) or "trashed"
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"` // 1 to 100; default 50
}

// TextListResult is a page of texts, newest first.
type TextListResult struct {
	Envelope
	// ProjectID echoes the project the list was asked for; empty for a list across projects.
	ProjectID  string     `json:"project_id,omitempty"`
	Texts      []TextItem `json:"texts"`
	NextCursor string     `json:"next_cursor,omitempty"`
	Text       string     `json:"text"`
}

// TextGetRequest reads a text, or one of its revisions.
type TextGetRequest struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	Revision  int    `json:"revision,omitempty"` // default: the latest
}

// TextGetResult is a text with its body.
type TextGetResult struct {
	Envelope
	ProjectID string       `json:"project_id"`
	Item      TextItem     `json:"item"`
	Revision  TextRevision `json:"revision"`
	// Body is the Markdown of the revision: untrusted text, a model may have written it.
	Body string `json:"body" untrusted:"true"`
	Text string `json:"text"`
}

// TextWriteRequest makes a text, or saves a new revision of one.
type TextWriteRequest struct {
	ProjectID string `json:"project_id"`
	// ItemID names the text to write; empty makes a new text (Title and TextKind).
	ItemID   string `json:"item_id,omitempty"`
	Title    string `json:"title,omitempty"`     // 1 to 120 characters; needed to make a text
	TextKind string `json:"text_kind,omitempty"` // making a text: script, copy (default) or caption
	Text     string `json:"text"`                // the Markdown, at most 256 KiB
	// BaseRevision, when set, must be the latest revision of the text: the write is refused when someone wrote
	// after the revision the writer read.
	BaseRevision *int `json:"base_revision,omitempty"`
	DryRun       bool `json:"dry_run,omitempty"`
}

// TextWriteResult is the text after a write, a revert or a draft.
type TextWriteResult struct {
	Envelope
	ProjectID string       `json:"project_id"`
	Item      TextItem     `json:"item"`
	Revision  TextRevision `json:"revision"`
	// Created is true when the call made the text; Changed is false when the body was the latest one already and no
	// revision was made.
	Created bool   `json:"created"`
	Changed bool   `json:"changed"`
	Text    string `json:"text"`
}

// TextRevisionsRequest lists the revisions of a text, and optionally compares two.
type TextRevisionsRequest struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	// Before starts the list under this revision (the next_before of the previous page); 0 starts at the latest.
	Before int `json:"before,omitempty"`
	Limit  int `json:"limit,omitempty"` // 1 to 100; default 50
	// From and To ask for the difference between two revisions at paragraph level; To defaults to the latest.
	From int `json:"from,omitempty"`
	To   int `json:"to,omitempty"`
}

// TextRevisionsResult is a page of revisions, newest first, and the difference when asked.
type TextRevisionsResult struct {
	Envelope
	ProjectID  string            `json:"project_id"`
	ItemID     string            `json:"item_id"`
	Revisions  []TextRevision    `json:"revisions"`
	NextBefore int               `json:"next_before,omitempty"`
	Diff       *writing.TextDiff `json:"diff,omitempty"`
	DiffFrom   int               `json:"diff_from,omitempty"`
	DiffTo     int               `json:"diff_to,omitempty"`
	Text       string            `json:"text"`
}

// TextRevertRequest brings an older revision back as a new one.
type TextRevertRequest struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	Revision  int    `json:"revision"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// TextDraftRequest asks a model for a draft of a text (SDD-019 section 6): the arguments of the job kind
// text.draft, and the wait and the dry run of JobStart.
type TextDraftRequest struct {
	ProjectID    string   `json:"project_id"`
	ItemID       string   `json:"item_id"`
	ConnectionID string   `json:"connection_id"`
	Model        string   `json:"model"`
	Instruction  string   `json:"instruction"`
	Network      string   `json:"network,omitempty"` // a caption: draft the variant of this network
	MaxTokens    int      `json:"max_tokens,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	WaitMS       int      `json:"wait_ms,omitempty"`
	DryRun       bool     `json:"dry_run,omitempty"`
}

// CaptionValidateRequest measures a caption against the limits of its networks (SDD-019 section 5.2). Exactly one of
// ItemID (a stored caption), Body (a whole caption, saved or not) and Text with Network (one variant) is given.
type CaptionValidateRequest struct {
	ProjectID string `json:"project_id,omitempty"`
	ItemID    string `json:"item_id,omitempty"`
	Body      string `json:"body,omitempty"`
	Text      string `json:"text,omitempty"`
	// Network is the network of Text; with ItemID it limits the answer to the variant of that network.
	Network string `json:"network,omitempty"`
}

// CaptionVariantReport is the report of one variant.
type CaptionVariantReport struct {
	writing.CaptionReport
	// Words is the number of words of the variant text, for the person who reads the counters.
	Words int `json:"words"`
}

// CaptionValidateResult is the report of every variant, in the order of the caption.
type CaptionValidateResult struct {
	Envelope
	ProjectID string                 `json:"project_id,omitempty"`
	Variants  []CaptionVariantReport `json:"variants"`
	// OK is true when no variant has a problem.
	OK   bool   `json:"ok"`
	Text string `json:"text"`
}

// ScriptEstimateRequest estimates the duration of a script when read aloud. Give ItemID (a stored script) or Text.
type ScriptEstimateRequest struct {
	ProjectID      string `json:"project_id,omitempty"`
	ItemID         string `json:"item_id,omitempty"`
	Text           string `json:"text,omitempty"`
	WordsPerMinute int    `json:"words_per_minute,omitempty"` // 60 to 300; default 150
}

// ScriptEstimateResult is the estimate of a script.
type ScriptEstimateResult struct {
	Envelope
	ProjectID string `json:"project_id,omitempty"`
	writing.Estimate
	Text string `json:"text"`
}
