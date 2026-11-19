package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/library"
	"github.com/AlexGalhardo/stuarty/service/publish"
)

// Sentinel errors returned by the store (SDD-005 section 2 item 8). The
// service maps them to the codes of SDD-000 section 8.
var (
	ErrNotFound = errors.New("service: not found")
	ErrExists   = errors.New("service: already exists")
	ErrStale    = errors.New("service: stale")
	// ErrBadCursor is a page cursor the store did not produce.
	ErrBadCursor = errors.New("service: invalid cursor")
)

// QueueFullError is returned by JobRepo.CreateLimited: the jobs that hold the slots (their ids), the
// kind that is full, the ceiling that was reached and whether it is the one of a single actor.
type QueueFullError struct {
	Running  []string
	Kind     string
	Ceiling  int
	PerActor bool
}

func (e *QueueFullError) Error() string { return "service: job limit reached" }

// Store is the only persistence dependency of Service.
// Reads on Repos use the reader pool and never wait for the writer.
// A single-statement write on Repos (a job progress update) is allowed
// outside a transaction; anything that must be atomic with another write
// goes through Write or WriteProject.
type Store interface {
	Repos

	// Write runs fn in one write transaction (BEGIN IMMEDIATE). fn returning
	// an error rolls back. Hooks added with AfterCommit run after a
	// successful commit, in order, on the calling goroutine, before Write returns.
	Write(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error

	// WriteProject is Write plus: it first acquires the in-process lock of
	// the project (waiting while ctx is live), then loads the project row
	// inside the transaction, and holds the lock until Write has returned
	// including the AfterCommit hooks. ErrNotFound when the project does not
	// exist (fn is not called). A trashed project is passed to fn.
	WriteProject(ctx context.Context, projectID string, fn func(ctx context.Context, tx ProjectTx) error) error

	Close() error
}

// Repos groups the repositories. On a Tx they are bound to the transaction.
type Repos interface {
	Projects() ProjectRepo
	Productions() ProductionRepo
	Operations() OperationRepo
	Idempotency() IdempotencyRepo
	Assets() AssetRepo
	Jobs() JobRepo
	Templates() TemplateRepo
	Backups() BackupRepo
	Audit() AuditRepo
	Checkpoints() CheckpointRepo
	Fonts() FontRepo
	BrandKit() BrandKitRepo
	Images() ImageRepo
	Connections() connections.Store
	Approvals() approval.Store
	Library() library.Index
	AssetLibrary() assets.Store
	Publications() publish.Store
}

// Tx is a write transaction.
type Tx interface {
	Repos
	AfterCommit(fn func())
}

// ProjectTx is a Tx of WriteProject.
type ProjectTx interface {
	Tx
	Project() ProjectRow // as loaded when the transaction started
}

// Projects

// ProjectState is the trash state of a project.
type ProjectState string

// The states of a project.
const (
	ProjectActive  ProjectState = "active"
	ProjectTrashed ProjectState = "trashed"
)

// DocSummary is computed by the service from the document and written by the
// commit, so that listing never parses a document (SDD-005 section 5.1). It is
// SDD-005's ProjectSummary, renamed because SDD-003 §2.3 gives that name to the
// listing item of the API.
type DocSummary struct {
	Width, Height         int
	FPSNum, FPSDen        int
	DurationFrames        int64 // end of the last item, 0 when empty
	Tracks, Items, Assets int
}

// ProjectHead is a project row without its document.
type ProjectHead struct {
	ID            string
	Name          string
	State         ProjectState
	Revision      int64
	SchemaVersion int
	Summary       DocSummary
	CreatedMs     int64
	UpdatedMs     int64
	CreatedBy     string
	OwnerUserID   *string // network mode only (SDD-007 4.1)
	TrashedMs     *int64
	TrashedBy     *string
	ResyncFloor   int64
}

// ProjectRow is a project with its document.
type ProjectRow struct {
	ProjectHead
	Document  []byte // canonical JSON as stored
	DocSHA256 string // lower-case hex of sha256(Document)
}

// ProjectQuery selects a page of the project listing.
type ProjectQuery struct {
	State          ProjectState // "" means active only
	IncludeTrashed bool         // both states; wins over State
	OwnerUserID    string       // network mode: only this user's projects, applied in SQL
	ProjectID      string       // a key limited to one project: the timelines of that one
	After          string       // opaque cursor from the previous page
	Limit          int          // 1 to 200; 0 means 50
}

// ProjectUpdate is what a commit writes to the project row.
type ProjectUpdate struct {
	Name          string
	SchemaVersion int
	Document      []byte
	DocSHA256     string
	Summary       DocSummary
	UpdatedMs     int64
	ResyncFloor   *int64 // set only by migration and import
}

// ProjectRepo stores projects.
type ProjectRepo interface {
	List(ctx context.Context, q ProjectQuery) (items []ProjectHead, next string, err error)
	// Count is the number of projects the query selects, ignoring its cursor and limit.
	Count(ctx context.Context, q ProjectQuery) (int, error)
	Head(ctx context.Context, id string) (ProjectHead, error)
	Get(ctx context.Context, id string) (ProjectRow, error) // with the document
	Create(ctx context.Context, row ProjectRow) error       // ErrExists
	// Commit writes the new document and moves revision from expected to
	// expected+1 in one guarded UPDATE ... WHERE id=? AND revision=?.
	// ErrStale when no row matched.
	Commit(ctx context.Context, id string, expected int64, u ProjectUpdate) error
	SetState(ctx context.Context, id string, st ProjectState, by string, atMs int64) error
	// Delete is a hard delete; it cascades, and also removes the
	// document_backups of the project. Only TrashEmpty calls it.
	Delete(ctx context.Context, id string) error
}

// Productions (SDD-005 section 5.12)

// ProductionHead is a project in the sense of ADR-0039: the production row, the number of its active timelines
// and the head of the first of them, which is what the list shows.
type ProductionHead struct {
	ID          string
	Name        string
	State       ProjectState
	CreatedMs   int64
	UpdatedMs   int64
	CreatedBy   string
	OwnerUserID *string // network mode only (SDD-007 4.1)
	TrashedMs   *int64
	TrashedBy   *string
	// Timelines counts the active timeline items.
	Timelines int
	// Timeline is the head of the first active timeline by position; its ID is empty when there is none.
	Timeline ProjectHead
}

// ProductionQuery selects a page of the list of projects; the members are those of ProjectQuery.
type ProductionQuery = ProjectQuery

// ItemRow is a row of project_items.
type ItemRow struct {
	ID        string
	ProjectID string
	Kind      string
	Title     string
	Position  int
	State     ProjectState
	Ref       string // timeline: the timeline id; output: the export job id
	Body      string // brief and text
	Format    string // output
	TextKind  string // text: script, copy or caption (SDD-019); empty for any other kind
	CreatedMs int64
	UpdatedMs int64
	CreatedBy string
	TrashedMs *int64
}

// TimelineRef is an active timeline item of a project.
type TimelineRef struct {
	ItemID     string
	TimelineID string
	Title      string
}

// ProductionRepo stores projects as productions and their items.
type ProductionRepo interface {
	List(ctx context.Context, q ProductionQuery) (items []ProductionHead, next string, err error)
	Count(ctx context.Context, q ProductionQuery) (int, error)
	Get(ctx context.Context, id string) (ProductionHead, error) // ErrNotFound
	Create(ctx context.Context, h ProductionHead) error         // ErrExists
	// SetState writes the state of the project and mirrors it on the rows of the timelines of its active timeline
	// items. ErrNotFound when there is no such project.
	SetState(ctx context.Context, id string, st ProjectState, by string, atMs int64) error
	// Touch sets updated_ms.
	Touch(ctx context.Context, id string, atMs int64) error
	// Delete is a hard delete that cascades the items; the timeline rows are the business of the caller. Only
	// TrashEmpty calls it.
	Delete(ctx context.Context, id string) error

	Items(ctx context.Context, projectID string) ([]ItemRow, error) // by position, then id
	Item(ctx context.Context, itemID string) (ItemRow, error)       // ErrNotFound
	CreateItem(
		ctx context.Context,
		it ItemRow,
	) error // ErrExists for a second brief or a timeline already in an item
	UpdateItem(ctx context.Context, it ItemRow) error // title, body, state, trashed_ms, updated_ms
	// SetPositions numbers the items 0 to n-1 in the order of ids.
	SetPositions(ctx context.Context, projectID string, ids []string) error
	// ProjectOfTimeline is the project whose timeline item has ref timelineID; ErrNotFound when none.
	ProjectOfTimeline(ctx context.Context, timelineID string) (string, error)
	// ActiveTimelines lists the active timeline items of a project by position.
	ActiveTimelines(ctx context.Context, projectID string) ([]TimelineRef, error)

	// The texts of a production and their revisions (SDD-019). CreateItem of a text item writes its revision 1
	// ('created') in the same call, so a text item never exists without a revision.
	//
	// TextItems lists text items with the facts of their latest revision, newest first, with the body of each.
	TextItems(ctx context.Context, q TextQuery) (items []TextHead, next string, err error)
	// TextRevisions lists the revisions of a text, newest first, without bodies; before is a revision number to
	// start under (0 for the latest); the answer has at most limit rows.
	TextRevisions(ctx context.Context, itemID string, before, limit int) ([]TextRevisionRow, error)
	// TextRevision reads one revision with its body; ErrNotFound when there is none.
	TextRevision(ctx context.Context, itemID string, revision int) (TextRevisionRow, error)
	// TextLatest is the latest revision of a text, with its body; ErrNotFound when the item has none.
	TextLatest(ctx context.Context, itemID string) (TextRevisionRow, error)
	// AddTextRevision appends a revision and drops those below pruneBefore, and sets the body and the
	// updated_ms of the item to those of the revision.
	AddTextRevision(ctx context.Context, rev TextRevisionRow, pruneBefore int) error
}

// TextRevisionRow is a row of text_revisions.
type TextRevisionRow struct {
	ItemID     string
	Revision   int
	Body       string // empty in a list
	Actor      string
	Source     string // created, write, generated or revert
	RevertedTo *int
	Provider   string
	Model      string
	CostMicros *int64
	Currency   string
	JobID      string
	CreatedMs  int64
}

// TextQuery selects a page of text items.
type TextQuery struct {
	ProjectID   string // one project; empty for every project the caller may see
	OwnerUserID string // network mode: only this user's projects
	TextKind    string // empty for every kind
	State       ProjectState
	After       string // opaque cursor from the previous page
	Limit       int    // 1 to 100; 0 means 50
}

// TextHead is a text item with the project it is in and the facts of its latest revision.
type TextHead struct {
	Item         ItemRow
	ProjectName  string
	Revision     int
	RevisionBy   string
	RevisionSrc  string
	ProjectOwner *string
}

// Operations

// OperationKind is the kind of a committed batch.
type OperationKind string

// The kinds of operation rows.
const (
	OpEdit    OperationKind = "edit"
	OpUndo    OperationKind = "undo"
	OpRedo    OperationKind = "redo"
	OpMigrate OperationKind = "migrate"
	OpImport  OperationKind = "import"
)

// OperationRow is one row of the append-only operation log.
type OperationRow struct {
	ProjectID      string
	Revision       int64
	Kind           OperationKind
	Actor          string
	BaseRevision   int64
	TargetRevision *int64 // undo and redo: the batch consumed
	AtMs           int64
	Ops            json.RawMessage // []command.Operation, ids filled in, resolved included
	Inverse        json.RawMessage // []command.Primitive in apply order
	Diff           json.RawMessage // []command.Change
	Skipped        json.RawMessage // []Skipped, undo and redo only; else nil
	SchemaVersion  int
	DocSHA256      string
	ClosedBy       *int64
	IdempotencyKey *string
}

// OperationFilter bounds OperationRepo.Since.
type OperationFilter struct {
	ExcludeActor string // when set, rows of this actor are left out
	Limit        int    // maximum rows; 0 means 1000
	MaxBytes     int    // stop after the row that makes the total of Ops+Inverse+Diff exceed this; 0 means no limit
}

// HistoryQuery selects a page of the history, newest first.
type HistoryQuery struct {
	Before int64  // revisions below this one; 0 means from the newest
	Actor  string // optional filter
	Limit  int    // 1 to 200
}

// OperationRepo stores the operation log.
type OperationRepo interface {
	Append(ctx context.Context, row OperationRow) error // ErrExists on a duplicate revision
	Get(ctx context.Context, projectID string, revision int64) (OperationRow, error)
	// Count is the number of rows of the project, of one actor when actor is set.
	Count(ctx context.Context, projectID, actor string) (int64, error)
	// Since returns rows with revision > after in ascending order.
	Since(ctx context.Context, projectID string, after int64, f OperationFilter) ([]OperationRow, error)
	// Page returns rows in descending order and whether older rows exist.
	Page(ctx context.Context, projectID string, q HistoryQuery) ([]OperationRow, bool, error)
	UndoCandidate(ctx context.Context, projectID, actor string) (OperationRow, error) // ErrNotFound: nothing to undo
	RedoCandidate(ctx context.Context, projectID, actor string) (OperationRow, error) // ErrNotFound: nothing to redo
	// Close sets closed_by on one row (it was consumed by that revision).
	// ErrStale when the row is missing or already closed.
	Close(ctx context.Context, projectID string, revision, by int64) error
	// CloseOpenRedo closes every open undo row of the actor (a new edit ends its redo chain).
	CloseOpenRedo(ctx context.Context, projectID, actor string, by int64) error
	// CloseAllOpen closes every open row of the project (a migration or import ends all undo chains).
	CloseAllOpen(ctx context.Context, projectID string, by int64) error
}

// CheckpointRow is a name and a note on a revision of a timeline (SDD-005 section 5.14).
type CheckpointRow struct {
	ProjectID string
	Name      string
	Note      string
	Revision  int64
	Actor     string
	Auto      bool
	AtMs      int64
}

// CheckpointRepo stores the checkpoints of a timeline. No method touches the log.
type CheckpointRepo interface {
	Create(ctx context.Context, row CheckpointRow) error // ErrExists on a duplicate name
	Get(ctx context.Context, projectID, name string) (CheckpointRow, error)
	// List returns the checkpoints of the project, newest revision first, then by name.
	List(ctx context.Context, projectID string) ([]CheckpointRow, error)
	// Rename changes the name, and the note when note is not nil, and makes the checkpoint a named one (Auto
	// false). ErrNotFound; ErrExists on a taken name.
	Rename(ctx context.Context, projectID, name, newName string, note *string) error
	Remove(ctx context.Context, projectID, name string) error // ErrNotFound
	// TrimAuto removes the oldest automatic checkpoints until at most keep remain.
	TrimAuto(ctx context.Context, projectID string, keep int) error
}

// Idempotency

// IdempotencyRow is a stored result for an idempotency key.
type IdempotencyRow struct {
	ProjectID   string
	Actor       string
	Key         string
	RequestHash string // lower-case hex sha256 of the canonical JSON of the request's operations
	Revision    int64
	Result      json.RawMessage // the result as first returned (SDD-003 shape), without "replayed"
	CreatedMs   int64
	ExpiresMs   int64
}

// IdempotencyRepo stores idempotency keys.
type IdempotencyRepo interface {
	// Get returns ErrNotFound for a missing or expired row (expires_ms <= nowMs).
	Get(ctx context.Context, projectID, actor, key string, nowMs int64) (IdempotencyRow, error)
	// Put replaces an expired row with the same key; ErrExists when a live row has it.
	Put(ctx context.Context, row IdempotencyRow) error
	PurgeExpired(ctx context.Context, nowMs int64) (int64, error)
	// PurgeProjectExpired deletes the expired keys of one project: the commit
	// does it in its own transaction (section 9.2 step 12) without touching
	// another project's rows.
	PurgeProjectExpired(ctx context.Context, projectID string, nowMs int64) (int64, error)
	// CountLive is how many unexpired keys one actor holds in one project, and when the first of
	// them expires (zero when there is none). The commit refuses a new key at the ceiling and
	// evicts nothing (SDD-005 section 10, SDD-007 section 9.1).
	CountLive(ctx context.Context, projectID, actor string, nowMs int64) (n int, firstExpiresMs int64, err error)
}

// Assets, project assets and artifacts

// IngestState is the ingest state of an asset: pending, ingesting, ready or failed.
type IngestState string

// The ingest states.
const (
	IngestPending   IngestState = "pending"
	IngestIngesting IngestState = "ingesting"
	IngestReady     IngestState = "ready"
	IngestFailed    IngestState = "failed"
)

// AssetRow is a catalogued original file, shared by every project.
type AssetRow struct {
	Hash         string // sha256:<64 hex>
	Kind         string // video, audio, image; empty until the probe classified the file
	SizeBytes    int64
	OriginalName string // untrusted
	IngestState  IngestState
	Progress     float64
	IngestError  json.RawMessage // typed error (SDD-002 A.8) when failed
	Facts        AssetFacts
	CreatedMs    int64
	UpdatedMs    int64
}

// AssetFacts are the probe facts of an asset, all nil until ingest is ready.
type AssetFacts struct {
	// Kind is the class the probe gave the file; SetFacts leaves the stored
	// kind alone when it is empty.
	Kind                       string
	NormalizedVersion          *int // profile version of the normalized file the facts were measured on
	DurationMs                 *int64
	Width, Height              *int
	HasAudio                   bool
	HasAlpha                   bool // a video whose normalized file keeps an alpha channel
	FrameRateNum, FrameRateDen *int
	FrameCount                 *int64
	SampleCount                *int64
	// FrameDelays is the display time in milliseconds of each frame of an animated image; empty for a still.
	FrameDelays []int
}

// ProjectAssetState is the state of a project's use of an asset: pending, active or trashed.
type ProjectAssetState string

// The project asset states.
const (
	ProjectAssetPending ProjectAssetState = "pending"
	ProjectAssetActive  ProjectAssetState = "active"
	ProjectAssetTrashed ProjectAssetState = "trashed"
)

// ProjectAssetRow maps a project's asset identifier to a hash and its trash state.
type ProjectAssetRow struct {
	ProjectID  string
	AssetID    string // ast_...
	AssetHash  string
	State      ProjectAssetState
	Name       string          // display name at import; untrusted
	Snapshot   json.RawMessage // the SDD-001 Asset object, kept for restore; nil until known
	ImportedBy string
	ImportedMs int64
	TrashedBy  *string
	TrashedMs  *int64
}

// ArtifactRow lists a derived file of an asset.
type ArtifactRow struct {
	AssetHash string
	Kind      string // probe, normalized, proxy, thumbnails, peaks, silence, loudness, transcript
	Profile   int    // profile version; part of the file name
	Variant   string // transcripts: the 12 hex key of provider, model and language; empty otherwise
	Ext       string // file extension without the dot
	RelPath   string // relative to the workspace, forward slashes
	SizeBytes int64
	SHA256    string // of the artifact file
	Meta      json.RawMessage
	CreatedMs int64
}

// AssetRepo stores assets, project assets and artifacts.
type AssetRepo interface {
	// Upsert inserts the row or returns the existing one; created reports which.
	Upsert(ctx context.Context, row AssetRow) (existing AssetRow, created bool, err error)
	Get(ctx context.Context, hash string) (AssetRow, error)
	SetIngest(
		ctx context.Context,
		hash string,
		st IngestState,
		progress float64,
		errJSON json.RawMessage,
		nowMs int64,
	) error
	SetFacts(ctx context.Context, hash string, f AssetFacts, nowMs int64) error
	// PutArtifact replaces the same (hash, kind, profile, variant, ext).
	PutArtifact(ctx context.Context, a ArtifactRow) error
	Artifacts(ctx context.Context, hash string) ([]ArtifactRow, error)

	// PutProjectAsset inserts or replaces the same (project, asset id);
	// ErrExists when the project already maps the hash to another asset id.
	PutProjectAsset(ctx context.Context, r ProjectAssetRow) error
	GetProjectAsset(ctx context.Context, projectID, assetID string) (ProjectAssetRow, error)
	FindProjectAssetByHash(ctx context.Context, projectID, hash string) (ProjectAssetRow, error)
	ListProjectAssets(ctx context.Context, projectID string, states ...ProjectAssetState) ([]ProjectAssetRow, error)
	SetProjectAssetState(
		ctx context.Context,
		projectID, assetID string,
		st ProjectAssetState,
		by string,
		atMs int64,
		snapshot json.RawMessage,
	) error
	ProjectsReferencing(ctx context.Context, hash string) ([]string, error) // project ids, any state
	PendingForHash(ctx context.Context, hash string) ([]ProjectAssetRow, error)

	// For TrashEmpty.
	DeleteTrashedProjectAssets(ctx context.Context, projectID string) (int64, error)
	Unreferenced(ctx context.Context) ([]AssetRow, error) // assets with no project_assets row
	Delete(ctx context.Context, hash string) error        // cascades artifacts
}

// Jobs, templates, backups

// JobRow is a job of the editor (ADR-0018), not a vukem queue row.
type JobRow struct {
	ID              string
	Kind            string  // ingest, transcribe, export, frame
	ProjectID       *string // nil for ingest
	AssetHash       *string // ingest and transcribe
	Actor           string
	State           string // queued, running, succeeded, failed, cancelled
	Progress        float64
	Message         string
	Input           json.RawMessage
	Result          json.RawMessage
	Error           json.RawMessage // typed error object (SDD-002 A.8)
	CancelRequested bool
	Attempts        int
	CreatedMs       int64
	UpdatedMs       int64
	StartedMs       *int64
	FinishedMs      *int64
	// ApprovalID is the approval request a job in state waiting_approval waits on, and the request
	// that released it afterwards (SDD-015 section 5); empty for a job that needs none.
	ApprovalID string
}

// JobLimit is the concurrency limit checked by CreateLimited.
type JobLimit struct {
	Kind        string
	Max         int    // concurrent queued+running jobs of this kind
	Actor       string // when set, MaxPerActor is applied to this actor's jobs of the kind
	MaxPerActor int
}

// JobRepo stores jobs. Claim, Progress, Finish and Requeue are single
// conditional updates; false means the row was not in the expected state.
type JobRepo interface {
	// CreateLimited inserts the job in state queued unless the limit is
	// reached, in which case it returns *QueueFullError. Call it inside Write.
	CreateLimited(ctx context.Context, row JobRow, lim *JobLimit) error
	Get(ctx context.Context, id string) (JobRow, error)
	List(ctx context.Context, projectID string, states []string, limit int) ([]JobRow, error)
	ActiveIngestForHash(ctx context.Context, hash string) (JobRow, error)

	Claim(ctx context.Context, id string, nowMs int64) (claimed bool, err error)
	Progress(ctx context.Context, id string, progress float64, message string, nowMs int64) (updated bool, err error)
	// Finish moves a running job to a terminal state; false when it was not running.
	Finish(ctx context.Context, id, state string, result, errJSON json.RawMessage, nowMs int64) (bool, error)
	// Requeue moves a running job back to queued (shutdown interruption or recovery).
	Requeue(ctx context.Context, id, message string, nowMs int64) (bool, error)
	// RequestCancel sets cancel_requested; a queued job becomes cancelled at once.
	// It returns the row as it is afterwards.
	RequestCancel(ctx context.Context, id string, nowMs int64) (JobRow, error)
	// Running returns every job in state running (used by recovery at start-up).
	Running(ctx context.Context) ([]JobRow, error)
	// QueuedBefore returns the jobs still queued that were created before
	// the instant, oldest first (recovery dispatches them again).
	QueuedBefore(ctx context.Context, createdBeforeMs int64) ([]JobRow, error)

	// CreateWaiting inserts a job in state waiting_approval, with the request it waits on in
	// row.ApprovalID (SDD-015 section 5). Call it inside Write.
	CreateWaiting(ctx context.Context, row JobRow) error
	// Release moves a waiting_approval job to queued; false when it was not waiting.
	Release(ctx context.Context, id string, nowMs int64) (bool, error)
	// FinishWaiting moves a waiting_approval job to a terminal state with its error; false when it
	// was not waiting.
	FinishWaiting(ctx context.Context, id, state string, errJSON json.RawMessage, nowMs int64) (bool, error)
}

// TemplateRow is a saved template document.
type TemplateRow struct {
	ID              string
	Name            string
	Description     string
	SchemaVersion   int
	Document        []byte
	DocSHA256       string
	Slots           json.RawMessage // [{name, kind, required, description}] for listing
	Summary         DocSummary
	SourceProjectID *string
	CreatedMs       int64
	CreatedBy       string
	OwnerUserID     *string // network mode: the user who saved it; nil in local mode
}

// TemplateRepo stores templates.
type TemplateRepo interface {
	Create(ctx context.Context, row TemplateRow) error
	Get(ctx context.Context, id string) (TemplateRow, error)
	// List pages the templates of ownerUserID, or every template when it is
	// empty (local mode); Document is left empty.
	List(ctx context.Context, limit int, after, ownerUserID string) ([]TemplateRow, string, error)
	UpdateDocument(
		ctx context.Context,
		id string,
		schemaVersion int,
		doc []byte,
		sha string,
		slots json.RawMessage,
		sum DocSummary,
	) error
}

// BackupRow is a document saved before a migration or import.
type BackupRow struct {
	ID            int64
	OwnerKind     string // "project" or "template"
	OwnerID       string
	Revision      int64 // 0 for a template
	SchemaVersion int
	Document      []byte
	Reason        string // e.g. "migrate v1 to v2", "import"
	CreatedMs     int64
}

// BackupRepo stores document backups.
type BackupRepo interface {
	Put(ctx context.Context, b BackupRow) error
	List(ctx context.Context, ownerKind, ownerID string) ([]BackupRow, error) // Document left empty
	Get(ctx context.Context, id int64) (BackupRow, error)
}

// Audit

// AuditRow is one entry of the audit table.
type AuditRow struct {
	ID        int64
	AtMs      int64
	Actor     string
	Action    string
	ProjectID *string
	SubjectID *string
	Details   json.RawMessage
}

// AuditRepo appends to and lists the audit table; it has no update or delete.
type AuditRepo interface {
	Append(ctx context.Context, row AuditRow) error
	List(ctx context.Context, projectID string, beforeID int64, limit int) ([]AuditRow, error) // projectID "" means all
}

// Fonts and the brand kit (SDD-005 section 5.11)

// FontRepo records which user imported or brought each stored font file. A hash is in the form of
// a document, "sha256:" and 64 hex digits.
type FontRepo interface {
	// Record adds owner to the owners of hash; recording twice changes nothing.
	Record(ctx context.Context, owner, hash string, atMs int64) error
	Owns(ctx context.Context, owner, hash string) (bool, error)
	// Owned lists the hashes owner imported or brought, oldest first.
	Owned(ctx context.Context, owner string) ([]string, error)
}

// BrandKitRow is the brand kit of one owner, "" in local mode.
type BrandKitRow struct {
	OwnerUserID string
	Document    json.RawMessage
	UpdatedMs   int64
	UpdatedBy   string
}

// BrandKitRepo stores the brand kit.
type BrandKitRepo interface {
	Get(ctx context.Context, owner string) (BrandKitRow, error) // ErrNotFound when there is none
	Put(ctx context.Context, row BrandKitRow) error
}
