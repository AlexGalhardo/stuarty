package service

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/lint"
)

// The image operations of SDD-021 section 11: a document of layers edited with the commands of the image
// registry, in batches that have a base revision, an idempotency key, a dry run, a log and undo.

// ImageRow is an image document as stored (SDD-005 section 5.13).
type ImageRow struct {
	ImageHead
	Document  json.RawMessage
	DocSHA256 string
}

// ImageHead is an image row without its document.
type ImageHead struct {
	ID            string
	Name          string
	Template      bool
	Revision      int64
	SchemaVersion int
	Width, Height int
	Layers        int
	CreatedMs     int64
	UpdatedMs     int64
	CreatedBy     string
	OwnerUserID   *string // network mode only
}

// ImageUpdate is what a commit writes into the row.
type ImageUpdate struct {
	Name          string
	Template      bool
	SchemaVersion int
	Document      json.RawMessage
	DocSHA256     string
	Width, Height int
	Layers        int
	UpdatedMs     int64
}

// ImageOpRow is one committed batch of an image, the log row of SDD-005 section 5.13.
type ImageOpRow struct {
	ImageID        string
	Revision       int64
	Kind           OperationKind
	Actor          string
	BaseRevision   int64
	TargetRevision *int64
	AtMs           int64
	Ops            json.RawMessage
	Inverse        json.RawMessage
	Diff           json.RawMessage
	Skipped        json.RawMessage
	DocSHA256      string
	ClosedBy       *int64
	IdempotencyKey *string
	RequestHash    string
	Result         json.RawMessage
}

// ImageQuery lists images: newest first, from Offset, at most Limit (and one more to tell there is a next page).
type ImageQuery struct {
	OwnerUserID string // network mode: only this owner's images; empty means all
	Offset      int
	Limit       int
	Templates   string // "" every image, "only" the templates, "none" the documents that are not
}

// ImageRepo is the store of image documents. Every method that writes runs inside Store.Write, whose one
// transaction serializes the writers of an image.
type ImageRepo interface {
	// Create inserts a row; ErrExists when the id is taken.
	Create(ctx context.Context, row ImageRow) error
	// Get returns the row; ErrNotFound when there is none.
	Get(ctx context.Context, id string) (ImageRow, error)
	// List returns up to q.Limit+1 heads.
	List(ctx context.Context, q ImageQuery) ([]ImageHead, error)
	// Commit sets the document and the revision to fromRevision+1; ErrStale when the row is no longer at fromRevision.
	Commit(ctx context.Context, id string, fromRevision int64, u ImageUpdate) error
	AppendOp(ctx context.Context, op ImageOpRow) error
	// OpsSince returns the batches after a revision, oldest first.
	OpsSince(ctx context.Context, id string, revision int64) ([]ImageOpRow, error)
	// OpsPage is OpsSince with a limit: the replay of the image stream reads the log in pages (SDD-006 section 12.2).
	OpsPage(ctx context.Context, id string, revision int64, limit int) ([]ImageOpRow, error)
	// LastOpen returns the latest batch of the actor, after the first revision, of one of the kinds, that no undo
	// or redo has consumed; ErrNotFound when there is none.
	LastOpen(ctx context.Context, id, actor string, kinds []OperationKind) (ImageOpRow, error)
	// Close marks a batch as consumed by the revision that undid or redid it.
	Close(ctx context.Context, id string, revision, by int64) error
	// CloseOpenRedo ends the redo chain of the actor: the undos still open are closed by the new revision.
	CloseOpenRedo(ctx context.Context, id, actor string, by int64) error
	// ByKey returns the batch the actor made with an idempotency key; ErrNotFound when there is none.
	ByKey(ctx context.Context, id, actor, key string) (ImageOpRow, error)
}

// ImageCreateRequest makes a document: blank (a preset or a size), or from a template with values.
type ImageCreateRequest struct {
	Name       string                         `json:"name,omitempty"`
	Preset     string                         `json:"preset,omitempty"`
	Width      int                            `json:"width,omitempty"`
	Height     int                            `json:"height,omitempty"`
	Background string                         `json:"background,omitempty"`
	Template   bool                           `json:"template,omitempty"`    // the new document is a template
	TemplateID string                         `json:"template_id,omitempty"` // make it from this template
	Values     map[string]coreimage.SlotValue `json:"values,omitempty"`
	ID         string                         `json:"id,omitempty"`
}

// ImageCreateResult answers ImageCreate.
type ImageCreateResult struct {
	Envelope
	ImageID  string `json:"image_id"`
	Revision int64  `json:"revision"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Text     string `json:"text"     untrusted:"true"`
}

// ImageListRequest lists the documents.
type ImageListRequest struct {
	Templates string `json:"templates,omitempty"` // "", "only" or "none"
	Page
}

// ImageSummary is one line of the list.
type ImageSummary struct {
	ImageID   string `json:"image_id"`
	Name      string `json:"name"       untrusted:"true"`
	Template  bool   `json:"template"`
	Revision  int64  `json:"revision"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Layers    int    `json:"layers"`
	UpdatedMs int64  `json:"updated_ms"`
}

// ImageListResult answers ImageList.
type ImageListResult struct {
	Envelope
	Images     []ImageSummary `json:"images"`
	NextCursor *string        `json:"next_cursor"`
	Text       string         `json:"text"        untrusted:"true"`
}

// ImageDescribeRequest reads one document.
type ImageDescribeRequest struct {
	ImageID string `json:"image_id"`
}

// ImageDescribeResult is the text view and the structure of a document.
type ImageDescribeResult struct {
	Envelope
	ImageID    string            `json:"image_id"`
	Revision   int64             `json:"revision"`
	Name       string            `json:"name"       untrusted:"true"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	Background string            `json:"background"`
	Template   bool              `json:"template"`
	Layers     []coreimage.Layer `json:"layers"     untrusted:"true"`
	Findings   []lint.Finding    `json:"findings"`
	Lint       LintTotals        `json:"lint"`
	Text       string            `json:"text"       untrusted:"true"`
}

// ImageEditRequest is a batch of image operations.
type ImageEditRequest struct {
	ImageID        string              `json:"image_id"`
	BaseRevision   int64               `json:"base_revision"`
	Operations     []command.Operation `json:"operations"` // 1 to 200
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	DryRun         bool                `json:"dry_run,omitempty"`
}

// ImageEditResult is the outcome of a batch.
type ImageEditResult struct {
	Envelope
	ImageID      string           `json:"image_id"`
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
	Description  string           `json:"description"   untrusted:"true"`
}

// ImageImportRequest brings an image file into an image document (SDD-021 section 10.1): the file of Path or
// the library file Hash, as a new document, or as one more layer of ImageID.
type ImageImportRequest struct {
	Path           string `json:"path,omitempty"`            // an absolute path on the machine of the server
	Hash           string `json:"hash,omitempty"`            // an image already in the workspace
	ImageID        string `json:"image_id,omitempty"`        // put the file in this document
	Name           string `json:"name,omitempty"`            // the document, or the layer when ImageID is given
	Width          int    `json:"width,omitempty"`           // the canvas of a new document
	Height         int    `json:"height,omitempty"`          // the canvas of a new document
	Preset         string `json:"preset,omitempty"`          // the canvas of a new document
	WaitMS         int    `json:"wait_ms,omitempty"`         // how long to wait for the ingest; 0 is 10,000
	BaseRevision   int64  `json:"base_revision,omitempty"`   // with ImageID
	IdempotencyKey string `json:"idempotency_key,omitempty"` // with ImageID
	DryRun         bool   `json:"dry_run,omitempty"`
}

// The states of an ImageImportResult.
const (
	ImageImportReady   = "ready"   // the file is an image of the workspace; the document or the layer was made
	ImageImportPending = "pending" // the ingest of the file outlived wait_ms: nothing was made, import the Hash again
)

// ImageImportResult answers ImageImport.
type ImageImportResult struct {
	Envelope
	State    string         `json:"state"`
	ImageID  string         `json:"image_id,omitempty"`
	Revision int64          `json:"revision,omitempty"`
	Created  bool           `json:"created"`            // a new document
	LayerID  string         `json:"layer_id,omitempty"` // the layer the file became
	Hash     string         `json:"hash,omitempty"`
	Name     string         `json:"name"               untrusted:"true"` // the name the file is known by
	Width    int            `json:"width,omitempty"`
	Height   int            `json:"height,omitempty"`
	JobID    string         `json:"job_id,omitempty"` // the ingest of a pending file
	Applied  bool           `json:"applied"`
	DryRun   bool           `json:"dry_run"`
	Replayed bool           `json:"replayed"`
	Findings []lint.Finding `json:"findings"`
	Lint     LintTotals     `json:"lint"`
	Text     string         `json:"text"               untrusted:"true"`
}

// ImageStepRequest undoes or redoes the last step of the caller.
type ImageStepRequest struct {
	ImageID string `json:"image_id"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// ImageStepResult is an edit result plus the step undone or redone and what was skipped.
type ImageStepResult struct {
	ImageEditResult
	Kind    string            `json:"kind"` // "undo" | "redo"
	Step    int64             `json:"step"` // the revision undone or redone
	Skipped []command.Skipped `json:"skipped"`
}

// ImageExportRequest renders a document to a file.
type ImageExportRequest struct {
	ImageID string `json:"image_id"`
	Format  string `json:"format,omitempty"` // png (default), jpeg or webp
	Quality int    `json:"quality,omitempty"`
	Preset  string `json:"preset,omitempty"` // resample to a named size
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

// ImageExportResult describes the file written.
type ImageExportResult struct {
	ImageID string `json:"image_id"`
	// JobID is the export job of kind image.export that records the file; a publication takes it as its
	// thumbnail (SDD-020 section 4).
	JobID    string `json:"job_id,omitempty"`
	Revision int64  `json:"revision"`
	Format   string `json:"format"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Text     string `json:"text"`
}
