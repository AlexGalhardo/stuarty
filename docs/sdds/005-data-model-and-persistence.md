# SDD-005: Data model and persistence

- Amended (2026-11-18) for Stuarty: a project becomes the container of a production and what this document calls a project becomes a timeline inside it (ADR-0039); third-party credentials live in a vault outside these tables (ADR-0033); jobs gain a scheduled start and an approval state (ADR-0046). The epics that build each change rewrite the sections they touch; until then this document describes the model as built.
- Status: Accepted (2026-11-14). Drafted by senior-c, reviewed and corrected by the architect; reconciled with SDD-000, SDD-006 and SDD-007.
- Depends on: ADR-0001, ADR-0006, ADR-0008, ADR-0009, ADR-0010, ADR-0017, ADR-0018, ADR-0019, ADR-0020, ADR-0022, ADR-0026, SDD-000, SDD-001, SDD-002, SDD-006
- Implements: RF-001, RF-005, RF-006, RF-013, RF-105, RF-107, RF-111, RF-115, RF-119, RF-131, RF-132, RF-134, RF-201, RF-208, NFR-001, NFR-012, NFR-013, NFR-024, NFR-025
- Owner area: senior-c (service, persistence, HTTP, events, jobs)

## Contents

1. Purpose and scope
2. Conventions
3. Persistence interfaces of `service/`
4. SQLite settings
5. Tables
6. Workspace ownership
7. Start-up order and migrations
8. Schema migration of stored documents
9. The commit transaction
10. Idempotency keys
11. The operation log
12. Undo and redo
13. Trash
14. Jobs
15. Templates
16. The project bundle
17. Decisions of this SDD
18. Acceptance

## 1. Purpose and scope

This document says how projects, the operation log, media records, jobs and templates are stored in SQLite; how a batch is committed; and how the process that owns a workspace is chosen. It is detailed enough to write the Go interfaces in `service/`, their implementation in `web/`, the migrations and the tests, without further questions.

Out of this document: the arguments and results of `Service` methods (SDD-003), the wire format of events (SDD-006), authentication (SDD-007), how media is probed, normalized and stored on disk (SDD-011), and how commands change a document (SDD-002).

Layering (ADR-0001, ADR-0008): `core/` and `service/` import no vukem package. `service/` declares the interfaces of section 3 and holds all logic: conflict checks, undo, idempotency, trash, job protocol. `web/` implements them over vukem's `database`, `orm`, `schema` and `queue` packages. The store implementation contains SQL and nothing else: it never decodes a document, never calls `core/`, and never publishes an event.

## 2. Conventions

1. **Time.** Every stored instant is an `INTEGER` of Unix milliseconds in UTC, in a column whose name ends in `_ms`. **Decision of this SDD.** vukem's ORM stores SQLite timestamps as text and compares them as text (`orm.mdx`, "Times and time zones"); integers compare and sort correctly with no dependency on a text layout. The wire format (events, HTTP) stays RFC 3339 UTC; the service converts. The old Studio did the same for its quota window (`created_unix`).
2. **Clock.** The service takes `now` from `app.Now()` (injectable in tests), never from `time.Now()`.
3. **Identifiers.** `TEXT`, in the form of SDD-000 §2. `assets.id` is the exception: it holds the content hash `sha256:<64 hex>`, because vukem's foreign key builder can only reference a column named `id` (`schema/table.go`, `Column.Constrained`).
4. **JSON.** Documents, operations, inverses, diffs, job inputs and results are stored as `TEXT` containing JSON. The store treats them as opaque `json.RawMessage` or `[]byte`; the service encodes with `timeline.Canonical` and `encoding/json`.
5. **Booleans** are `INTEGER` 0 or 1 in hand-written SQL (the schema builder's `Boolean` is the same on SQLite).
6. **Enumerations** are `TEXT` with a `CHECK (col IN (...))`, which is what `Table.Enum` emits on SQLite.
7. **Hand-written SQL.** Hot paths (commit, claim, progress, listing) use `database.Executor` with `database.Rebind` and `?` placeholders. The ORM is used for plain inserts of rows with a single-column key. Models of tables with a composite key implement `orm.CompositeKeyer`, and `Find` is not used on them (`orm.mdx`, "Composite primary keys").
8. **Sentinel errors.** The store returns these and the service maps them to the codes of SDD-000 §8 and SDD-002 §A.8:

| Sentinel | Returned when | Service mapping |
| --- | --- | --- |
| `ErrNotFound` | no row | `E_NOT_FOUND` |
| `ErrExists` | unique or primary key violated (`orm.ErrConflict` in the ORM) | `E_ID_EXISTS` |
| `ErrStale` | a guarded update matched no row | `E_INTERNAL` inside the commit (it means the lock discipline was broken); `E_INVALID_STATE` elsewhere |
| `*QueueFullError` | a job limit is reached | `E_QUEUE_FULL` with `details.running` |

## 3. Persistence interfaces of `service/`

File `service/store.go`. Package `service` imports the standard library and `core/` only. Types of rows are plain structs; JSON columns are `json.RawMessage` so that the store needs no knowledge of `core/`.

```go
package service

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrNotFound = errors.New("service: not found")
	ErrExists   = errors.New("service: already exists")
	ErrStale    = errors.New("service: stale")
)

// QueueFullError is returned by JobRepo.CreateLimited.
type QueueFullError struct{ Running []string } // job ids

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

type Repos interface {
	Projects() ProjectRepo
	Operations() OperationRepo
	Idempotency() IdempotencyRepo
	Assets() AssetRepo
	Jobs() JobRepo
	Templates() TemplateRepo
	Backups() BackupRepo
	Audit() AuditRepo // section 5.9
	Checkpoints() CheckpointRepo // section 5.14
}

type Tx interface {
	Repos // bound to the transaction
	AfterCommit(fn func())
}

type ProjectTx interface {
	Tx
	Project() ProjectRow // as loaded when the transaction started
}
```

### 3.1 Projects

```go
type ProjectState string

const (
	ProjectActive  ProjectState = "active"
	ProjectTrashed ProjectState = "trashed"
)

// DocSummary is computed by the service from the document and written by
// the commit, so that listing never parses a document (section 5.1).
type DocSummary struct {
	Width, Height      int
	FPSNum, FPSDen     int
	DurationFrames     int64 // end of the last item, 0 when empty
	Tracks, Items, Assets int
}

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

type ProjectRow struct {
	ProjectHead
	Document  []byte // canonical JSON as stored
	DocSHA256 string // lower-case hex of sha256(Document)
}

type ProjectQuery struct {
	State  ProjectState // "" means active only; "all" is not a state, use IncludeTrashed
	IncludeTrashed bool
	OwnerUserID string // network mode: only this user's projects, in SQL for List and Count
	ProjectID   string // a key limited to one project: only that project
	After  string // opaque cursor from the previous page
	Limit  int    // 1 to 200
}

type ProjectUpdate struct {
	Name          string
	SchemaVersion int
	Document      []byte
	DocSHA256     string
	Summary       DocSummary
	UpdatedMs     int64
	ResyncFloor   *int64 // set only by migration and import
}

type ProjectRepo interface {
	List(ctx context.Context, q ProjectQuery) (items []ProjectHead, next string, err error)
	Head(ctx context.Context, id string) (ProjectHead, error)
	Get(ctx context.Context, id string) (ProjectRow, error) // with the document
	Create(ctx context.Context, row ProjectRow) error       // ErrExists
	// Commit writes the new document and moves revision from expected to
	// expected+1 in one guarded UPDATE ... WHERE id=? AND revision=?.
	// ErrStale when no row matched.
	Commit(ctx context.Context, id string, expected int64, u ProjectUpdate) error
	SetState(ctx context.Context, id string, st ProjectState, by string, atMs int64) error
	Delete(ctx context.Context, id string) error // hard delete; cascades, and removes the project's document_backups; only TrashEmpty calls it
}
```

The repositories of this section operate on `timelines` (section 5.2); the Go names keep `Project` until the rename epic. The production has its own repository:

```go
type ProductionRepo interface {
	List(ctx context.Context, q ProductionQuery) (items []ProductionHead, next string, err error)
	Count(ctx context.Context, q ProductionQuery) (int, error)
	Get(ctx context.Context, id string) (ProductionHead, error)
	Create(ctx context.Context, h ProductionHead) error // ErrExists
	// SetState writes the state and mirrors it on the rows of the timelines of the active timeline items.
	SetState(ctx context.Context, id string, st ProjectState, by string, atMs int64) error
	Touch(ctx context.Context, id string, atMs int64) error
	Delete(ctx context.Context, id string) error // cascades the items; the caller deletes the timeline rows first; emptying only
	Items(ctx context.Context, projectID string) ([]ItemRow, error) // by position
	Item(ctx context.Context, itemID string) (ItemRow, error)
	CreateItem(ctx context.Context, it ItemRow) error // ErrExists for a second active brief
	UpdateItem(ctx context.Context, it ItemRow) error // title, body, state, times
	SetPositions(ctx context.Context, projectID string, ids []string) error // numbers the items 0 to n-1 in this order
	ProjectOfTimeline(ctx context.Context, timelineID string) (projectID string, err error)
	ActiveTimelines(ctx context.Context, projectID string) ([]TimelineRef, error) // by position
}
```

`ProductionHead` is the row of `projects` plus the summary of the first active timeline (its id, revision and `DocSummary`) that the list and the overview need, read by one join.

### 3.2 Operations

```go
type OperationKind string

const (
	OpEdit    OperationKind = "edit"
	OpUndo    OperationKind = "undo"
	OpRedo    OperationKind = "redo"
	OpMigrate OperationKind = "migrate"
	OpImport  OperationKind = "import"
)

type OperationRow struct {
	ProjectID      string
	Revision       int64
	Kind           OperationKind
	Actor          string
	BaseRevision   int64
	TargetRevision *int64          // undo and redo: the batch consumed
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

type OperationFilter struct {
	ExcludeActor string // when set, rows of this actor are left out
	Limit        int    // maximum rows; 0 means 1000
	MaxBytes     int    // stop after the row that makes the total of Ops+Inverse+Diff exceed this; 0 means no limit
}

type HistoryQuery struct {
	Before int64  // revisions below this one; 0 means from the newest
	Actor  string // optional filter
	Limit  int    // 1 to 200
}

type OperationRepo interface {
	Append(ctx context.Context, row OperationRow) error // ErrExists on a duplicate revision
	Get(ctx context.Context, projectID string, revision int64) (OperationRow, error)
	// Since returns rows with revision > after in ascending order.
	Since(ctx context.Context, projectID string, after int64, f OperationFilter) ([]OperationRow, error)
	// Page returns rows in descending order and whether older rows exist.
	Page(ctx context.Context, projectID string, q HistoryQuery) ([]OperationRow, bool, error)
	UndoCandidate(ctx context.Context, projectID, actor string) (OperationRow, error) // ErrNotFound: nothing to undo
	RedoCandidate(ctx context.Context, projectID, actor string) (OperationRow, error) // ErrNotFound: nothing to redo
	// Close sets closed_by on one row (it was consumed by that revision).
	Close(ctx context.Context, projectID string, revision, by int64) error
	// CloseOpenRedo closes every open undo row of the actor (a new edit ends its redo chain).
	CloseOpenRedo(ctx context.Context, projectID, actor string, by int64) error
	// CloseAllOpen closes every open row of the project (a migration or import ends all undo chains).
	CloseAllOpen(ctx context.Context, projectID string, by int64) error
}

// CheckpointRepo is the checkpoints of a timeline (section 5.14; epic 36). Repos.Checkpoints() returns it.
type CheckpointRow struct {
	ProjectID string
	Name      string
	Note      string
	Revision  int64
	Actor     string
	Auto      bool
	AtMs      int64
}

type CheckpointRepo interface {
	Create(ctx context.Context, row CheckpointRow) error // ErrExists on a duplicate name
	Get(ctx context.Context, projectID, name string) (CheckpointRow, error)
	List(ctx context.Context, projectID string) ([]CheckpointRow, error) // newest revision first, then name
	// Rename changes the name, and the note when note is not nil; ErrNotFound, ErrExists on a taken name.
	Rename(ctx context.Context, projectID, name, newName string, note *string) error
	Remove(ctx context.Context, projectID, name string) error // ErrNotFound; no revision is touched
	// TrimAuto removes the oldest automatic checkpoints until at most keep remain.
	TrimAuto(ctx context.Context, projectID string, keep int) error
}
```

### 3.3 Idempotency

```go
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

type IdempotencyRepo interface {
	// Get returns ErrNotFound for a missing or expired row (expires_ms <= nowMs).
	Get(ctx context.Context, projectID, actor, key string, nowMs int64) (IdempotencyRow, error)
	Put(ctx context.Context, row IdempotencyRow) error // replaces an expired row with the same key
	PurgeExpired(ctx context.Context, nowMs int64) (int64, error)
	PurgeProjectExpired(ctx context.Context, projectID string, nowMs int64) (int64, error) // the commit's own purge, step 12 of section 9.2
}
```

### 3.4 Assets, project assets and artifacts

```go
type IngestState string // "pending", "ingesting", "ready", "failed"

type AssetRow struct {
	Hash         string // sha256:<64 hex>
	Kind         string // video, audio, image
	SizeBytes    int64
	OriginalName string // untrusted
	IngestState  IngestState
	Progress     float64
	IngestError  json.RawMessage // typed error (SDD-002 A.8) when failed
	Facts        AssetFacts // NormalizedVersion is the ingest profile version (column normalized_version)
	CreatedMs    int64
	UpdatedMs    int64
}

type AssetFacts struct { // all nil until ingest is ready
	Kind               string // video, audio or image; SetFacts leaves the stored kind alone when empty
	NormalizedVersion  *int // profile version of the normalized file the facts were measured on
	DurationMs         *int64
	Width, Height      *int
	HasAudio           bool
	FrameRateNum, FrameRateDen *int
	FrameCount         *int64
	SampleCount        *int64
}

type ProjectAssetState string // "pending", "active", "trashed"

type ProjectAssetRow struct {
	ProjectID   string
	AssetID     string // ast_...
	AssetHash   string
	State       ProjectAssetState
	Name        string          // display name at import; untrusted
	Snapshot    json.RawMessage // the SDD-001 Asset object, kept for restore; nil until known
	ImportedBy  string
	ImportedMs  int64
	TrashedBy   *string
	TrashedMs   *int64
}

type ArtifactRow struct {
	AssetHash string
	Kind      string // probe, normalized, proxy, thumbnails, peaks, silence, loudness, speech, beats, frames, transcript
	Profile   int    // profile version; part of the file name
	Variant   string // transcripts: the 12 hex key of provider, model and language; empty otherwise
	Ext       string // file extension without the dot
	RelPath   string // relative to the workspace, forward slashes
	SizeBytes int64
	SHA256    string // of the artifact file
	Meta      json.RawMessage
	CreatedMs int64
}

type AssetRepo interface {
	// Upsert inserts the row or returns the existing one; created reports which.
	Upsert(ctx context.Context, row AssetRow) (existing AssetRow, created bool, err error)
	Get(ctx context.Context, hash string) (AssetRow, error)
	SetIngest(ctx context.Context, hash string, st IngestState, progress float64, errJSON json.RawMessage, nowMs int64) error
	SetFacts(ctx context.Context, hash string, f AssetFacts, nowMs int64) error
	PutArtifact(ctx context.Context, a ArtifactRow) error // replaces the same (hash, kind, profile)
	Artifacts(ctx context.Context, hash string) ([]ArtifactRow, error)

	PutProjectAsset(ctx context.Context, r ProjectAssetRow) error // insert or replace the same (project, asset id)
	GetProjectAsset(ctx context.Context, projectID, assetID string) (ProjectAssetRow, error)
	FindProjectAssetByHash(ctx context.Context, projectID, hash string) (ProjectAssetRow, error)
	ListProjectAssets(ctx context.Context, projectID string, states ...ProjectAssetState) ([]ProjectAssetRow, error)
	SetProjectAssetState(ctx context.Context, projectID, assetID string, st ProjectAssetState, by string, atMs int64, snapshot json.RawMessage) error
	ProjectsReferencing(ctx context.Context, hash string) ([]string, error) // project ids, any state
	PendingForHash(ctx context.Context, hash string) ([]ProjectAssetRow, error)

	// For TrashEmpty.
	DeleteTrashedProjectAssets(ctx context.Context, projectID string) (int64, error)
	Unreferenced(ctx context.Context) ([]AssetRow, error) // assets with no project_assets row
	Delete(ctx context.Context, hash string) error        // cascades artifacts
}
```

`Repos` also has `AssetLibrary() assets.Store` (package `service/assets`, as `Connections()` has `connections.Store`; `Library()` is the index of memory, skills and prompts of SDD-018): every method takes the owner (the row carries it in `Put`): `Get(owner, hash)`, `GetMany(owner, hashes)`, `Put(row)` (the `asset_info` row, its tags and its `asset_search` row in one call, inside the caller's transaction), `Reindex(owner, hash)` (the `asset_search` row alone) and `Search(owner, match)` (the hashes of the owner whose search row matches an expression built by `core/asset`, section 5.5.1). `AssetRepo.Delete` also removes the `asset_search` rows of the hash; `AssetRepo.Upsert` no longer writes one, since a search row belongs to an owner.

### 3.5 Jobs, templates, backups

```go
type JobRow struct {
	ID              string
	Kind            string          // ingest, transcribe, export, frame
	ProjectID       *string         // nil for ingest
	AssetHash       *string         // ingest and transcribe
	Actor           string
	State           string          // queued, running, succeeded, failed, cancelled
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
}

type JobLimit struct {
	Kind   string
	Max    int    // concurrent queued+running jobs of this kind
	Actor  string // when set, the same cap is applied to this actor's jobs
	MaxPerActor int
}

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
	// QueuedBefore returns the jobs still queued that were created before the
	// instant, oldest first (recovery dispatches them again, 14.5 item 3).
	QueuedBefore(ctx context.Context, createdBeforeMs int64) ([]JobRow, error)
}

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
}

type TemplateRepo interface {
	Create(ctx context.Context, row TemplateRow) error
	Get(ctx context.Context, id string) (TemplateRow, error)
	List(ctx context.Context, limit int, after string) ([]TemplateRow, string, error) // Document left empty
	UpdateDocument(ctx context.Context, id string, schemaVersion int, doc []byte, sha string, slots json.RawMessage, sum DocSummary) error
}

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

type BackupRepo interface {
	Put(ctx context.Context, b BackupRow) error
	List(ctx context.Context, ownerKind, ownerID string) ([]BackupRow, error) // Document left empty
	Get(ctx context.Context, id int64) (BackupRow, error)
}
```

### 3.6 What the service needs from `core/`

Two functions are not named in SDD-002 and are required by sections 9 and 12. Their signatures are fixed here so that SDD-002 Part C can adopt them; they are pure.

```go
package command

// Conflicts implements the rule of SDD-002 A.7 step 2. It reports the first
// object referenced by ops (by identifier, in args) that any of the intervening
// diffs added, updated or removed.
func Conflicts(ops []Operation, intervening []Change) (conflict *ConflictInfo)

// ApplyPrimitives applies primitives in the given order to doc, skipping those
// the guard rejects or whose result would violate timeline.Validate. It never
// fails because of a skipped primitive. Inverse undoes exactly what was applied.
func ApplyPrimitives(doc *timeline.Document, prims []Primitive, guard Guard) (Result, []Skipped)

// Guard says whether another actor changed what a primitive touches.
type Guard interface {
	Changed(collection, id, field string) (by Touch, changed bool) // field "" means any field, "*" means the whole object was added or removed
}
```

## 4. SQLite settings

vukem opens SQLite through `database.Open` and applies these settings to every connection (`database/database.go`, `sqlitePragmas`, `sqliteDSN`). This SDD relies on them and does not change them.

| Setting | Value | Why it matters here |
| --- | --- | --- |
| `journal_mode` | `WAL` | Readers (listing, history, event replay, the HTTP client path of the UI) never block the commit; a crash leaves a recoverable log. |
| `busy_timeout` | `5000` ms | A second connection that finds the database locked waits instead of failing. With one owner process this only matters for the reader pool during a checkpoint. |
| `foreign_keys` | `ON` | `ON DELETE CASCADE` in the tables below (trash emptying) and `RESTRICT` on asset references are enforced by the database. |
| `synchronous` | `NORMAL` | Safe with WAL against corruption; see the durability note below. |
| `temp_store` | `MEMORY` | No temp files for sorts. |
| Writer pool | exactly 1 connection, `_txlock=immediate` | Every `Write` is serialized and takes the write lock at `BEGIN`, so a transaction is never upgraded halfway (the classic `SQLITE_BUSY`). |
| Reader pool | `mode=ro`, sized by `DB_MAX_OPEN` (default 10) | Reads run concurrently with the writer. |

**Durability note. Decision of this SDD.** With `synchronous=NORMAL` in WAL mode a committed transaction is in the operating system's cache. It survives a crash of the process (kill, panic, `os.Exit`), which is the guarantee of NFR-013 ("a crash of the process"). It can be lost if the machine loses power or the OS crashes before the next checkpoint; the database is never corrupted and rolls back to an earlier commit. We accept this and document it; raising it to `FULL` needs a vukem change because the pragma is appended after any user value and cannot be overridden through the URL (VF candidate 1 in the report). Verified by the crash tests of section 18 (kill the process, not the machine). Re-confirmed by the crash suite (P2-SRV-022): the crash suite kills the process at every commit, undo, redo, trash-emptying and start-up migration point, and a `SIGKILL` in the middle of 200 concurrent HTTP commits loses no acknowledged revision; `PRAGMA integrity_check` is `ok` every time.

**Path rules.** The database file is `<workspace>/db.sqlite`, passed to vukem as `file:<path>` with forward slashes. vukem does not escape the path (`sqliteDSN` splits on the first `?`), so a workspace path containing `?`, `#` or `%` is refused at start-up with an error that names the offending character. Spaces, non-ASCII characters and a Windows drive letter are accepted; verified by a test that opens a workspace under `C:\Users\Test User\é\.stuarty` on the Windows CI job.

**Checkpoints.** SQLite's automatic checkpoint (1000 pages) is kept. On graceful shutdown the owner runs `PRAGMA wal_checkpoint(TRUNCATE)` so that `db.sqlite` is self-contained when copied by hand.

**Measured on a log of 100,000 rows (P4-SRV-002).** The same benchmarks of section 18 check 12 run on the project of 200 items after 100,000 empty edits were appended to its log (`BenchmarkCommit200ItemsLog100k`, `BenchmarkRoundTrip200ItemsLog100k`; the rows come from `padLog`, one recursive `INSERT`). Windows, Ryzen 7 5700X3D (16 threads), `-benchtime=300x`, with four other agents compiling and testing on the same machine (the p95 values are inflated by that load):

| Benchmark | p50 | p95 | Store share p95 | Limit |
| --- | --- | --- | --- | --- |
| `BenchmarkCommit200Items` (log of 2 rows) | 22.8 ms | 26.5 ms | 2.6 ms | 50 ms round trip, 25 ms store |
| `BenchmarkCommit200ItemsLog100k` | 22.5 ms | 26.2 ms | 2.8 ms | the same |
| `BenchmarkRoundTrip200Items` (HTTP) | 26.3 ms | 34.2 ms | | 50 ms |
| `BenchmarkRoundTrip200ItemsLog100k` (HTTP) | 26.0 ms | 36.9 ms | | 50 ms |

The log of 100,000 rows costs nothing measurable: the store reads the row of the project and appends one row, both by primary key. A `base_revision` of 0 on that log is refused from the two revision numbers alone in 0.5 ms (`TestConflictAnalysisIsBoundedOnALongLog`); one at the edge of the lookback (`current - 1000`) reads at most the 1,000 newest rows and commits in about 100 ms under load. The event from commit to a loopback client has p95 1.5 ms over 1,000 commits (NFR-002, limit 20 ms) and `BenchmarkCLIRead` has p95 7.7 ms against a server that owns the workspace (NFR-024, limit 300 ms).

**One process.** No second process opens `db.sqlite` while a server runs: the workspace lock (section 6) is taken before the database is opened, by every command, including vukem's built-in `migrate`, `queue:*` and `db:*` commands. vukem API to be confirmed in task: a per-command hook so that built-in commands go through the workspace provider (`vukem.Provider` has `Register` and `Booter` but only `KindFilter` for scoping).

## 5. Tables

### 5.1 Migration approach

Migrations are Go files in `web/database/migrations/`, one per change, registered in `registry.go`, as in the Studio (`web/database/migrations/registry.go`) and in vukem's documentation.

- File: `web/database/migrations/<UTC timestamp, 14 digits>_<snake_case_name>.go`. The ID is the file name without `.go` and decides the order.
- Function: `m<timestamp><PascalCaseName>() schema.Migration`.
- `registry.go` lists them in `All()`; `schema.NewMigrator` sorts by ID, so the list order is cosmetic.
- vukem's queue tables come from `queue.Migrations(queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"})`, concatenated before ours: `vukem.WithMigrations(slices.Concat(queue.Migrations(qopts), migrations.All()))`. Its ID `20261018000000_vukem_create_queue_tables` sorts first.
- The Studio's migrations (`posts`, `users`, `jobs` of the old render job, ...) are deleted by the repository-reset task of ADR-0001; the editor starts with an empty list. In particular, the old table `jobs` must not exist: this SDD owns `jobs`.
- A migration never edits a released migration. A new column is a new `s.Table` migration. SQLite cannot add a check or a foreign key to an existing table; such a change uses `s.Rebuild` with `NoTransaction: true` (`migrations.mdx`, "Rebuilding a table on SQLite").
- Partial indexes with an explicit name are created with `s.Raw`, because `schema.Index` has no naming option and two indexes over the same columns would get the same derived name (VF candidate 4).

The set of migrations of release 0.5.0, in order:

| ID | Creates |
| --- | --- |
| `20261114100000_create_projects_table` | `projects` |
| `20261114100100_create_operations_table` | `operations` and its indexes |
| `20261114100200_create_idempotency_keys_table` | `idempotency_keys` |
| `20261114100300_create_assets_tables` | `assets`, `project_assets`, `artifacts` |
| `20261114100400_create_jobs_table` | `jobs` |
| `20261114100500_create_templates_table` | `templates` |
| `20261114100600_create_document_backups_table` | `document_backups` |
| `20261114100700_create_audit_table` | `audit` |
| `20261114100800_create_api_keys_table` (with network mode) | `api_keys` |

**As built (P1-SRV-001/002).** The migration files hold the `CREATE TABLE` and `CREATE INDEX` statements of this section through `s.Raw`, word for word, instead of the schema builder: the builder has no way to say `REAL`, a partial index or a trigger, and the effective SQL below is the contract. `templates` is in the set (the interfaces need it); `api_keys` waits for network mode. The code is in `web/database/migrations/` and the store in `web/app/store/`. The example that follows is the builder form of the first two, kept as a reference for tables the builder can express (vukem `schema.Table` methods exist for every call shown; `Primary` with one column and with several is supported in `Create`):

```go
package migrations

import "github.com/AlexGalhardo/vukem/schema"

func m20261114100000CreateProjectsTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100000_create_projects_table",
		Up: func(s *schema.Schema) error {
			return s.Create("projects", func(t *schema.Table) {
				t.String("id", 14).Primary()
				t.String("name", 120)
				t.Enum("state", "active", "trashed").Default("active")
				t.BigInteger("revision").Default(0)
				t.Integer("schema_version")
				t.Text("document")
				t.String("doc_sha256", 64)
				t.Integer("width")
				t.Integer("height")
				t.Integer("fps_num")
				t.Integer("fps_den")
				t.BigInteger("duration_frames").Default(0)
				t.Integer("track_count").Default(0)
				t.Integer("item_count").Default(0)
				t.Integer("asset_count").Default(0)
				t.BigInteger("resync_floor").Default(0)
				t.BigInteger("created_ms")
				t.BigInteger("updated_ms")
				t.String("created_by", 80)
				t.String("owner_user_id", 40).Nullable()
				t.BigInteger("trashed_ms").Nullable()
				t.String("trashed_by", 80).Nullable()
				t.Index("state", "updated_ms")
			})
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("projects") },
	}
}

func m20261114100100CreateOperationsTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100100_create_operations_table",
		Up: func(s *schema.Schema) error {
			if err := s.Create("operations", func(t *schema.Table) {
				t.String("project_id", 14).Constrained("projects").CascadeOnDelete()
				t.BigInteger("revision")
				t.Enum("kind", "edit", "undo", "redo", "migrate", "import")
				t.String("actor", 80)
				t.BigInteger("base_revision")
				t.BigInteger("target_revision").Nullable()
				t.BigInteger("at_ms")
				t.Text("ops")
				t.Text("inverse")
				t.Text("diff")
				t.Text("skipped").Nullable()
				t.Integer("schema_version")
				t.String("doc_sha256", 64)
				t.BigInteger("closed_by").Nullable()
				t.String("idempotency_key", 64).Nullable()
				t.Primary("project_id", "revision")
				t.Check("operations_revision_check", "revision >= 1")
				t.Index("project_id", "actor", "revision")
			}); err != nil {
				return err
			}
			// Partial indexes with explicit names (section 12).
			if err := s.Raw("CREATE INDEX operations_undo_idx ON operations (project_id, actor, revision) " +
				"WHERE closed_by IS NULL AND kind IN ('edit','redo')"); err != nil {
				return err
			}
			return s.Raw("CREATE INDEX operations_redo_idx ON operations (project_id, actor, revision) " +
				"WHERE closed_by IS NULL AND kind = 'undo'")
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("operations") },
	}
}
```

```go
// registry.go
func All() []schema.Migration {
	return []schema.Migration{
		m20261114100000CreateProjectsTable(),
		m20261114100100CreateOperationsTable(),
		m20261114100200CreateIdempotencyKeysTable(),
		m20261114100300CreateAssetsTables(),
		m20261114100400CreateJobsTable(),
		m20261114100500CreateTemplatesTable(),
		m20261114100600CreateDocumentBackupsTable(),
		m20261114100700CreateAuditTable(),
		m20261114100800CreateAPIKeysTable(),
	}
}
```

The effective SQL of every table follows. `NOT NULL` is the default of the builder and is written explicitly. Types are SQLite's.

### 5.2 `timelines`

Since ADR-0039 a project is a production (section 5.12) and the table this section describes holds its timelines. Migration `20261119120000_project_items` renamed the former `projects` table to `timelines`; its columns, its checks and every foreign key that pointed at it (`operations`, `idempotency_keys`, `project_assets`, `jobs`, `api_keys`) are unchanged, and the column `project_id` of those tables now holds a **timeline id**. The word "project" in sections 3 and 8 to 15 means the timeline document unless the text says production; an id given as `project_id` is resolved to a timeline by the rule of SDD-003 section 1.7.

One row per timeline: the current document, its revision, its schema version, trash state, and columns denormalized from the document so that listing never parses JSON.

```sql
CREATE TABLE timelines (
  id              TEXT    NOT NULL,
  name            TEXT    NOT NULL,                 -- denormalized: document.name
  state           TEXT    NOT NULL DEFAULT 'active' CHECK (state IN ('active','trashed')),
  revision        INTEGER NOT NULL DEFAULT 0,
  schema_version  INTEGER NOT NULL,                 -- denormalized: document.schema_version
  document        TEXT    NOT NULL,                 -- canonical JSON (SDD-001 section 2, item 8)
  doc_sha256      TEXT    NOT NULL,                 -- hex sha256 of document
  width           INTEGER NOT NULL,                 -- denormalized: settings.width
  height          INTEGER NOT NULL,                 -- denormalized: settings.height
  fps_num         INTEGER NOT NULL,                 -- denormalized: settings.fps.num
  fps_den         INTEGER NOT NULL,                 -- denormalized: settings.fps.den
  duration_frames INTEGER NOT NULL DEFAULT 0,       -- denormalized: max(start+duration) over items
  track_count     INTEGER NOT NULL DEFAULT 0,       -- denormalized: len(tracks)
  item_count      INTEGER NOT NULL DEFAULT 0,       -- denormalized: len(items)
  asset_count     INTEGER NOT NULL DEFAULT 0,       -- denormalized: len(assets)
  resync_floor    INTEGER NOT NULL DEFAULT 0,       -- highest revision that a client cannot replay across
  created_ms      INTEGER NOT NULL,
  updated_ms      INTEGER NOT NULL,                 -- last commit, trash or restore
  created_by      TEXT    NOT NULL,                 -- actor
  owner_user_id   TEXT,                             -- network mode: id of the vukem user who owns it (SDD-007 4.1); NULL in local mode
  trashed_ms      INTEGER,                          -- NULL unless state = 'trashed'
  trashed_by      TEXT,
  PRIMARY KEY (id)
);
CREATE INDEX timelines_state_updated_ms_index ON timelines (state, updated_ms);
```

- **Revision.** A new project has `revision = 0` and no `operations` row. The first committed batch is revision 1. The rows of `operations` of a project are exactly revisions `1..revision`, contiguous.
- **Denormalized columns** are written by the same statement that writes `document` (section 9, step 10) and are never written by anything else. The service computes `DocSummary` from the new document with plain traversal; invariant: for every project, the summary columns equal the summary recomputed from `document` (checked by a test and by `doctor`).
- **Listing query:** `SELECT <head columns> FROM timelines WHERE state = 'active' AND (updated_ms, id) < (?, ?) ORDER BY updated_ms DESC, id DESC LIMIT ?`; the cursor is `base64url(updated_ms ":" id)`. `IncludeTrashed` drops the `state` condition. `EXPLAIN QUERY PLAN` shows `timelines_state_updated_ms_index`. The list of projects (section 5.12) is a different query, over `projects`.
- **`resync_floor`** is set to the revision of a `migrate` or `import` row (sections 8 and 16) and tells SDD-006 that a client behind that revision cannot replay and must reload.
- **Trash** touches `state`, `trashed_ms`, `trashed_by` and `updated_ms`; never `revision` or `document` (section 13).

### 5.3 `operations`

The append-only log, one row per committed batch (SDD-000 §7, ADR-0006). Section 11 explains the content.

```sql
CREATE TABLE operations (
  project_id      TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  revision        INTEGER NOT NULL CHECK (revision >= 1),
  kind            TEXT    NOT NULL CHECK (kind IN ('edit','undo','redo','migrate','import')),
  actor           TEXT    NOT NULL,
  base_revision   INTEGER NOT NULL,        -- revision the caller planned against; revision-1 for migrate and import
  target_revision INTEGER,                 -- undo and redo: revision of the batch consumed
  at_ms           INTEGER NOT NULL,
  ops             TEXT    NOT NULL,        -- JSON array of {op, args}, identifiers filled in, resolved included; [] for undo, redo, migrate, import
  inverse         TEXT    NOT NULL,        -- JSON array of primitives, in the order they were applied
  diff            TEXT    NOT NULL,        -- JSON array of changes (SDD-002 A.5)
  skipped         TEXT,                    -- JSON array of skipped primitives; undo and redo only
  schema_version  INTEGER NOT NULL,        -- schema version of the document after this batch
  doc_sha256      TEXT    NOT NULL,        -- hex sha256 of the canonical document after this batch
  closed_by       INTEGER,                 -- revision that consumed this row for undo or redo (section 12)
  idempotency_key TEXT,
  PRIMARY KEY (project_id, revision)
);
CREATE INDEX operations_project_id_actor_revision_index ON operations (project_id, actor, revision);
CREATE INDEX operations_undo_idx ON operations (project_id, actor, revision)
  WHERE closed_by IS NULL AND kind IN ('edit','redo');
CREATE INDEX operations_redo_idx ON operations (project_id, actor, revision)
  WHERE closed_by IS NULL AND kind = 'undo';
```

The log is never updated except for `closed_by`, which only moves from `NULL` to a revision. No code path deletes a row except the cascade of `Delete` on `projects` (trash emptying). RF-134 ("the record cannot be edited") is enforced by the store interface having no update method other than `Close*`, and by a test that greps the implementation for `UPDATE operations` and finds only those three statements. The migration also installs the trigger `operations_append_only` (SDD-007 T-28): an `UPDATE` that changes any column other than `closed_by`, or that touches a row whose `closed_by` is already set, aborts with `operations is append-only`. It does not guard `DELETE`, which the cascade of a hard project delete needs.

### 5.4 `idempotency_keys`

```sql
CREATE TABLE idempotency_keys (
  project_id   TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  actor        TEXT    NOT NULL,
  key          TEXT    NOT NULL,      -- 8 to 64 characters, as received
  request_hash TEXT    NOT NULL,      -- hex sha256 of the canonical JSON of the request's operations
  revision     INTEGER NOT NULL,      -- revision the batch produced
  result       TEXT    NOT NULL,      -- JSON of the result as first returned
  created_ms   INTEGER NOT NULL,
  expires_ms   INTEGER NOT NULL,      -- created_ms + 86400000
  PRIMARY KEY (project_id, actor, key)
);
CREATE INDEX idempotency_keys_expires_ms_index ON idempotency_keys (expires_ms);
```

Behaviour is in section 10.

### 5.5 `assets`, `project_assets`, `artifacts`

`assets` is the content store's catalogue: one row per original file, by hash, shared by every project (ADR-0010 item 2). `project_assets` maps a project's asset identifier (`ast_...`, SDD-001 §3.2, per project) to a hash and carries the trash state of that use (ADR-0020). `artifacts` lists derived files (ADR-0010 item 3). An asset of kind `lut` (a `.cube` colour table, SDD-011 section 7.1) has one artifact, the validated table kept as `normalized` (`artifacts/<hash>/normalized-1.cube`, a byte copy of the original that the export reads), and no picture facts; the migration `20261116130000_assets_lut_kind` rebuilds `assets` for the wider check, with foreign keys off, keeping every row.

```sql
CREATE TABLE assets (
  id              TEXT    NOT NULL,                 -- 'sha256:' + 64 hex
  kind            TEXT    CHECK (kind IS NULL OR kind IN ('video','audio','image','lut')), -- NULL until the probe classified the file; lut is a .cube colour table (SDD-011, TASK-0254)
  size_bytes      INTEGER NOT NULL,
  original_name   TEXT    NOT NULL,                 -- first name seen; untrusted text
  ingest_state    TEXT    NOT NULL DEFAULT 'pending' CHECK (ingest_state IN ('pending','ingesting','ready','failed')),
  ingest_progress REAL    NOT NULL DEFAULT 0 CHECK (ingest_progress >= 0 AND ingest_progress <= 1),
  ingest_error    TEXT,                             -- JSON error object when failed
  normalized_version INTEGER,                       -- profile version of the normalized file the facts below were measured on (SDD-011 §6.3); NULL until ready
  duration_ms     INTEGER,                          -- facts for the media library, NULL until ready
  width           INTEGER,
  height          INTEGER,
  has_audio       INTEGER NOT NULL DEFAULT 0,
  has_alpha       INTEGER NOT NULL DEFAULT 0,       -- video: the normalized file keeps an alpha channel (SDD-011 4.3.5)
  frame_rate_num  INTEGER,
  frame_rate_den  INTEGER,
  frame_count     INTEGER,
  frame_delays    TEXT,                   -- animated images: JSON array of the display time in ms of each frame, NULL for a still
  sample_count    INTEGER,
  created_ms      INTEGER NOT NULL,
  updated_ms      INTEGER NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE project_assets (
  project_id  TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  asset_id    TEXT    NOT NULL,                     -- ast_...
  asset_hash  TEXT    NOT NULL REFERENCES assets (id) ON DELETE RESTRICT,
  state       TEXT    NOT NULL CHECK (state IN ('pending','active','trashed')),
  name        TEXT    NOT NULL,                     -- display name at import; untrusted
  snapshot    TEXT,                                 -- JSON Asset object of SDD-001 3.2, for restore
  imported_by TEXT    NOT NULL,
  imported_ms INTEGER NOT NULL,
  trashed_by  TEXT,
  trashed_ms  INTEGER,
  PRIMARY KEY (project_id, asset_id)
);
CREATE UNIQUE INDEX project_assets_project_id_asset_hash_unique ON project_assets (project_id, asset_hash);
CREATE INDEX project_assets_asset_hash_index ON project_assets (asset_hash);

CREATE TABLE artifacts (
  asset_hash TEXT    NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('probe','normalized','proxy','thumbnails','peaks','silence','loudness','speech','beats','frames','chunks','transcript')),
  profile    INTEGER NOT NULL,                      -- profile version; part of the file name (SDD-000 section 9)
  variant    TEXT    NOT NULL DEFAULT '',           -- transcripts: the 12 hex key of provider, model and language (SDD-011 section 6.1); empty otherwise
  ext        TEXT    NOT NULL,                      -- file extension without the dot; one kind may have more than one file
  rel_path   TEXT    NOT NULL,                      -- relative to the workspace, forward slashes
  size_bytes INTEGER NOT NULL,
  sha256     TEXT    NOT NULL,                      -- of the artifact file
  meta       TEXT,                                  -- JSON: the measured facts of a normalized file, the layout of a thumbnail sheet, provider and language of a transcript, ...
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (asset_hash, kind, profile, variant, ext)
);
```

State of a `project_assets` row:

| State | Meaning | Entered by |
| --- | --- | --- |
| `pending` | imported, the identifier is allocated, the asset is not in the document yet (ingest running or failed) | `MediaImport` |
| `active` | the asset is in the project's document | the commit of an `asset.add` batch, and of any batch whose diff has `asset.added` (including undo and redo) |
| `trashed` | removed from the document; `snapshot` holds the object | the commit of a batch whose diff has `asset.removed`; `MediaTrash` of a `pending` row |

**Sync rule (section 9, step 11).** After applying a batch the commit walks its diff: `asset.added` sets the row to `active` (inserting it if missing, with `snapshot` from `object`); `asset.removed` sets it to `trashed` with `snapshot` from `object`. So undo and redo keep `project_assets` correct with no special case. The columns `snapshot`, `imported_by` and the pair of trash columns are the only data that survives a trash for a later restore.

**Importing a file already in the project.** `UNIQUE (project_id, asset_hash)` means the same file maps to one identifier per project. Importing a hash whose row is `trashed` behaves as `MediaRestore` and returns the old identifier; a row in any other state is returned unchanged.

**Facts.** The `assets` fact columns are the denormalized probe facts used by the media library list; they are written by `SetFacts` when ingest reaches `ready`, in the same transaction as the `ingest_state` change. The authoritative copy for editing is the Asset object in the document.

#### 5.5.1 The library side of an asset (`asset_info`, `asset_tags`, `asset_search`)

Migration `20261119150000_asset_library`. The bytes are one row per hash; the library side is one row per owner and hash, `owner_user_id` being the user in network mode and the empty string in local mode, so two users who hold the same bytes keep their own record (SDD-011 DS-22, SDD-007 T-06). `assets` is already one row per file by hash and shared by every project, so the library is workspace-wide without moving a file (ADR-0010, ADR-0039); what the library adds is the record of where a file came from, what the user may do with it and how to find it. It lives beside `assets`, keyed by the owner and the same hash, so that the ingest never writes it and a release that does not know it still opens the workspace.

```sql
CREATE TABLE asset_info (
  owner_user_id TEXT  NOT NULL DEFAULT '',          -- the user in network mode, '' in local mode
  asset_hash  TEXT    NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  origin_kind TEXT    NOT NULL DEFAULT 'imported' CHECK (origin_kind IN ('imported','generated','fetched_by_agent','rendered_externally')),
  source      TEXT    NOT NULL DEFAULT '',          -- a path, a URL or a tool name, as plain text; untrusted
  provider    TEXT    NOT NULL DEFAULT '',          -- generated: the provider of the connection
  model       TEXT    NOT NULL DEFAULT '',          -- generated: the model
  prompt      TEXT    NOT NULL DEFAULT '',          -- generated: the prompt; untrusted
  cost_micros INTEGER CHECK (cost_micros IS NULL OR cost_micros >= 0), -- millionths of a US dollar, NULL when unknown
  rights_note TEXT    NOT NULL DEFAULT '',          -- the user's own record of the licence or the permission; untrusted
  description TEXT    NOT NULL DEFAULT '',          -- untrusted
  reusable    INTEGER NOT NULL DEFAULT 0 CHECK (reusable IN (0,1)),
  created_by  TEXT    NOT NULL,                     -- the actor of the first import
  created_ms  INTEGER NOT NULL,
  updated_ms  INTEGER NOT NULL,
  PRIMARY KEY (owner_user_id, asset_hash)
);
CREATE INDEX asset_info_reusable_index ON asset_info (owner_user_id, asset_hash) WHERE reusable = 1;

CREATE TABLE asset_tags (
  owner_user_id TEXT NOT NULL DEFAULT '',
  asset_hash TEXT NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  tag        TEXT NOT NULL,                         -- normalised by core/asset (SDD-011 section 14.3)
  PRIMARY KEY (owner_user_id, asset_hash, tag)
);
CREATE INDEX asset_tags_tag_index ON asset_tags (owner_user_id, tag, asset_hash);

CREATE VIRTUAL TABLE asset_search USING fts5(
  owner_user_id UNINDEXED, asset_hash UNINDEXED, name, tags, description, origin, tokenize = 'unicode61 remove_diacritics 2'
);
```

- **A row is optional.** An asset with no `asset_info` row for the owner reads as `origin_kind = 'imported'`, no source, not reusable, no tags (the ingest never writes the library). `MediaImport` and `ProjectBundleImport` write the owner's row, and its search row, when they link the file to a project, whether or not the asset is new; the migration writes one for every owner whose projects use an asset.
- **Origin is written once.** The row of an owner and hash keeps the origin of its first import. `rights_note`, `description`, `reusable` and the tags are the user's and change through `AssetSetInfo` and `AssetTag`. A later import of the same bytes with an origin block is validated and does not replace the origin.
- **`asset_search`** has one row per owner and hash, written by the service in the same transaction as every change of `asset_info`, `asset_tags`, `assets.original_name` or a `project_assets.name`: `name` is the names the file is known by in the projects of that owner (the first name of the asset when no project of the owner holds it), `tags` the tags joined by spaces, `origin` the source, provider, model and prompt. A file that reaches a project with no import (an `asset.add` of a template fill, the copy of a project) gets the search row of the project's owner in the same transaction, so it is found by the name that project gives it even when that owner has no `asset_info` row. A virtual table holds no foreign key, so `Assets().Delete` (the emptying of the trash) deletes the rows of every owner too. Every query of the three tables filters by `owner_user_id`. A query is built by `core/asset` (every term quoted, a prefix match on the last) and never concatenated from the caller's text.
- **Migration.** For every owner whose projects hold an asset (the owner of the project, the empty string when it has none) and for the empty owner of an asset no project holds: an `asset_info` row with `origin_kind = 'imported'`, `created_by` the importer of that owner's oldest `project_assets` row (`migration` when there is none) and `created_ms` of the asset, and its `asset_search` row (the names that owner's projects give it). `Down` drops the three tables. No `project_assets` row and no file changes: a clip still resolves its media by the hash.
- **The render origin (epic 33), migration `20261119210000_asset_render_origin`.** Three columns of `asset_info`, all `TEXT NOT NULL DEFAULT ''`: `engine`, `engine_version` and `render_key`, written with the origin of kind `rendered_externally` (SDD-011 section 15.2) and empty for every other row. No index: a project holds at most 200 assets, so the earlier renders of a key are found by reading the caller's rows of the project's hashes (`GetMany` of the owner). `engine` and `engine_version` join the `origin` text of `asset_search`. `Down` drops the three columns. Existing rows keep the empty values.
- **Uses.** The projects that use an asset are computed, never stored: the `project_assets` rows of the hash in state `active` (`pending` is an import not yet added; `trashed` is removed), each timeline mapped to its production by `project_items` (section 5.12). The same hash held by two timelines of one project is one use.

### 5.6 `jobs`

```sql
CREATE TABLE jobs (
  id               TEXT    NOT NULL,                -- job_...
  kind             TEXT    NOT NULL CHECK (kind IN ('ingest','transcribe','export','export_batch','frame')),
  project_id       TEXT    REFERENCES projects (id) ON DELETE CASCADE,   -- NULL for ingest
  asset_hash       TEXT    REFERENCES assets (id) ON DELETE CASCADE,     -- ingest and transcribe
  actor            TEXT    NOT NULL,
  state            TEXT    NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','succeeded','failed','cancelled')),
  progress         REAL    NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 1),
  message          TEXT    NOT NULL DEFAULT '',     -- short, for humans and agents
  input            TEXT    NOT NULL,                -- JSON, kind specific
  result           TEXT,                            -- JSON, set when succeeded
  error            TEXT,                            -- JSON error object (SDD-002 A.8) when failed or cancelled
  cancel_requested INTEGER NOT NULL DEFAULT 0,
  attempts         INTEGER NOT NULL DEFAULT 0,      -- incremented by each claim
  created_ms       INTEGER NOT NULL,
  updated_ms       INTEGER NOT NULL,
  started_ms       INTEGER,
  finished_ms      INTEGER,
  PRIMARY KEY (id),
  CHECK ((kind = 'ingest' AND asset_hash IS NOT NULL) OR (kind <> 'ingest' AND project_id IS NOT NULL))
);
CREATE INDEX jobs_project_id_created_ms_index ON jobs (project_id, created_ms);
CREATE INDEX jobs_kind_state_index ON jobs (kind, state);
CREATE UNIQUE INDEX jobs_active_ingest_unique ON jobs (asset_hash) WHERE kind = 'ingest' AND state IN ('queued','running');
```

An ingest job belongs to the content, not to a project, so that trashing or deleting the project that started it does not kill an ingest another project waits for. The partial unique index makes "one active ingest per file" a database fact. Protocol in section 14. An `export_batch` job is the parent of the `export` jobs of one `ExportBatch` (SDD-004 section 8.4); migration `20261119120050_jobs_export_batch_kind` rebuilds the table to add the kind, keeping every row. The later `20261119140100_jobs_waiting_approval` (SDD-015) drops the closed list of kinds and adds `approval_id` and the state `waiting_approval`; its Down gives the list back with `export_batch`, and the export_batch rebuild, when it runs out of order on a table that already has `approval_id`, leaves the table alone.

### 5.7 `templates`

```sql
CREATE TABLE templates (
  id                TEXT    NOT NULL,               -- tpl_...
  name              TEXT    NOT NULL,
  description       TEXT    NOT NULL DEFAULT '',
  schema_version    INTEGER NOT NULL,
  document          TEXT    NOT NULL,               -- canonical JSON of a template document (SDD-001 3.10)
  doc_sha256        TEXT    NOT NULL,
  slots             TEXT    NOT NULL,               -- JSON [{name, kind, required, description}], denormalized for listing
  width             INTEGER NOT NULL,
  height            INTEGER NOT NULL,
  fps_num           INTEGER NOT NULL,
  fps_den           INTEGER NOT NULL,
  duration_frames   INTEGER NOT NULL DEFAULT 0,
  source_project_id TEXT,                           -- no foreign key: the project may be emptied from the trash
  created_ms        INTEGER NOT NULL,
  created_by        TEXT    NOT NULL,
  PRIMARY KEY (id)
);
CREATE INDEX templates_created_ms_index ON templates (created_ms);
```

In network mode a template belongs to the user who saved it, as a project does (section 5.10). Migration `20261115100000_add_templates_owner_user_id` adds `owner_user_id TEXT` (NULL in local mode) and `templates_owner_user_id_created_ms_index ON templates (owner_user_id, created_ms)`. `TemplateRepo.List` filters by owner, and every read path of the service checks `visible` and answers a template of another owner exactly as a missing one (`E_NOT_FOUND`). The three starter templates (ADR-0022 item 8) are the one exception: rows with the fixed ids `tpl_5tar7c0001` to `tpl_5tar7c0003`, no owner and `created_by` `system:starter`, which start-up step 8 inserts when missing (never overwritten). `TemplateRepo.List` and `visible` treat a row with no owner and that creator as shared, so every user of network mode sees them; a row with no owner and another creator (a workspace that was local before) stays hidden from users.

Section 15.

### 5.8 `document_backups`

Needed by ADR-0009 item 6 and SDD-001 §6 item 7; not listed in SDD-000 §7 (reported as an issue).

```sql
CREATE TABLE document_backups (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_kind     TEXT    NOT NULL CHECK (owner_kind IN ('project','template')),
  owner_id       TEXT    NOT NULL,                  -- no foreign key: polymorphic
  revision       INTEGER NOT NULL,                  -- the project's revision when saved; 0 for a template
  schema_version INTEGER NOT NULL,
  document       TEXT    NOT NULL,                  -- the document exactly as it was stored
  reason         TEXT    NOT NULL,
  created_ms     INTEGER NOT NULL
);
CREATE INDEX document_backups_owner_kind_owner_id_index ON document_backups (owner_kind, owner_id);
```

Rows are deleted only when their owner is deleted by `TrashEmpty`.

### 5.9 `audit`

RF-134 asks that every operation record its actor in a record that cannot be edited. Edits are in `operations`. Everything else that changes state is recorded here: one row per action, written in the same transaction as the action.

```sql
CREATE TABLE audit (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  at_ms      INTEGER NOT NULL,
  actor      TEXT    NOT NULL,                     -- SDD-000 section 3
  action     TEXT    NOT NULL,                     -- see the list below
  project_id TEXT,                                 -- no foreign key: the row outlives the project
  subject_id TEXT,                                 -- asset, job, template, key or user identifier, when there is one
  details    TEXT                                  -- small JSON object; never a secret, never media text
);
CREATE INDEX audit_project_id_at_ms_index ON audit (project_id, at_ms);
CREATE INDEX audit_at_ms_index ON audit (at_ms);
```

Actions: `project.create`, `project.trash`, `project.restore`, `media.import`, `media.trash`, `media.restore`, `job.start`, `job.cancel`, `template.save`, `template.import`, `template.instantiate`, `trash.empty` (with the counts), `key.create`, `key.revoke`, `user.create`, `bundle.import`, and for the production (section 5.12, SDD-003 §2.3.1) `project.item.add`, `project.item.move`, `project.item.rename`, `project.item.trash`, `project.item.restore`, `project.brief.set`, `project.duplicate` (written with a `project.create` for the copy). The store interface has `Append` and a paged `List` and no update or delete; the table is never purged, including by `TrashEmpty`. The migration `create_audit_append_only_triggers` enforces it: `UPDATE` and `DELETE` on `audit` abort with `audit is append-only`. `Repos` gains `Audit() AuditRepo`:

```go
type AuditRow struct {
	ID        int64
	AtMs      int64
	Actor     string
	Action    string
	ProjectID *string
	SubjectID *string
	Details   json.RawMessage
}

type AuditRepo interface {
	Append(ctx context.Context, row AuditRow) error
	List(ctx context.Context, projectID string, beforeID int64, limit int) ([]AuditRow, error) // projectID "" means all
}
```

`HistoryList` merges nothing from this table: it stays the list of revisions. `doctor --bundle` includes the last 500 audit rows.

### 5.10 `api_keys` and project ownership (network mode)

SDD-007 §4 needs two things from persistence. Both are unused in local mode.

- `projects.owner_user_id` (section 5.2): the vukem user who created the project. `ProjectList` filters by it and every operation on a project compares it with the caller (SDD-007 §5.2).
- `api_keys`, which maps the product's key identifier to vukem's token:

```sql
CREATE TABLE api_keys (
  id         TEXT    NOT NULL,                     -- key_...
  token_id   TEXT    NOT NULL,                     -- id of the vukem token (auth.Tokens generates it)
  user_id    TEXT    NOT NULL,                     -- owner
  project_id TEXT    REFERENCES projects (id) ON DELETE CASCADE,  -- optional project limit
  name       TEXT    NOT NULL,
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (id)
);
CREATE UNIQUE INDEX api_keys_token_id_unique ON api_keys (token_id);
CREATE INDEX api_keys_user_id_index ON api_keys (user_id);
```

Scopes, expiry, last use and revocation live in vukem's token row (SDD-007 §4.2); this table adds only what vukem's token cannot hold. `Repos` gains `Keys() KeyRepo` with `Create`, `Get`, `GetByTokenID`, `ListByUser`. vukem's own user, session and token tables are created by vukem's migrations and are not described here.

### 5.11 `brand_kits` and `font_owners` (RF-053)

Migration `20261115140000_create_brand_kits_and_font_owners`. Both tables are small and never joined to the document.

```sql
CREATE TABLE brand_kits (
  owner_user_id TEXT    NOT NULL,                  -- '' in local mode: one kit per workspace
  document      TEXT    NOT NULL,                  -- the kit as JSON, below
  updated_ms    INTEGER NOT NULL,
  updated_by    TEXT    NOT NULL,                  -- actor
  PRIMARY KEY (owner_user_id)
);
CREATE TABLE font_owners (
  hash          TEXT    NOT NULL,                  -- 64 hex digits of the file in fonts/
  owner_user_id TEXT    NOT NULL,                  -- the user who imported or brought the file
  created_ms    INTEGER NOT NULL,
  PRIMARY KEY (hash, owner_user_id)
);
```

**The brand kit** is one JSON object per workspace (per user in network mode, so that one user never sees another's kit): `{"logo": "sha256:...", "colors": {"accent": "#FF3366"}, "fonts": {"Brand Sans": {"files": [{"weight": 400, "hash": "sha256:..."}]}}}`. All three members are optional, but `BrandKitSet` needs at least one: a body with none of them is `E_VALIDATION` (it would silently empty the kit), and an explicit empty member such as `{"colors": {}}` clears it. An empty kit is the same as no row. `logo` names a media asset already in the content store (an image the caller can see: in network mode one linked to a project of the caller). `colors` has at most 32 entries; a name matches `[a-z][a-z0-9_]{0,39}` and a value is `#RRGGBB` or `#RRGGBBAA`, stored upper case. `fonts` has at most 16 families with the rules of SDD-001 §3.9 (not a bundled family, 1 to 9 files, one per weight, 100 to 900 in steps of 100), and every file must be in the font store and usable by the caller (SDD-007 §7.7 item 6). `BrandKitSet` replaces the whole kit in one transaction and `BrandKitGet` returns it (an empty kit when there is none). A bad name, colour, weight or family is `E_VALIDATION` with the `path` of the member; a font the caller may not use is `E_NOT_FOUND`, the refusal of `font.add`.

**Templates and the brand kit.** `TemplateInstantiateRequest` has two optional lists: `brand_colors`, names of the template's own `template.colors` that take their value from the kit, and `brand_fonts`, families of the kit that are added to the new project's `fonts`. The template keeps `"$name"` references: instantiation sets `template.colors.<name>` of the new project to the kit's colour, so every style that says `"$accent"` draws the brand colour. A name that is not a colour of the template, a colour the kit lacks, or a family the kit lacks is `E_VALIDATION` with `details.reason` `brand_color_missing`, `brand_color_unknown` or `brand_font_missing` and the `path` `brand_colors[i]` or `brand_fonts[i]`; nothing is created. A template that is instantiated without these lists keeps its own colours.

**`font_owners`** records, per stored font file, each user who imported it (`FontImport`) or brought it (a bundle or a template file that ships it). It exists in network mode only: in local mode the store is the caller's own and a stored file is enough. `font.add`, a bundle export, a bundle import that names a font it does not ship and an instantiation of a template with fonts all ask the same question, "may this caller use this hash": the caller is a recorded owner of it, or a project of the caller already uses it (SDD-007 §7.7 item 6). `Repos` gains `Fonts() FontRepo` with `Record(ctx, owner, hash, ms)` (idempotent) and `Owns(ctx, owner, hash)`, and `BrandKit() BrandKitRepo` with `Get(ctx, owner)` and `Put(ctx, row)`.

### 5.12 `projects`, `project_items` (ADR-0039, RF-300)

Migration `20261119120000_project_items`. A project is a production: the container of a brief, texts, timelines and output pieces. The pure rules (kinds, order, production state) are in `core/project`.

```sql
CREATE TABLE projects (
  id            TEXT    NOT NULL,                  -- prj_ (SDD-000 section 2)
  name          TEXT    NOT NULL,                  -- follows the name of the timeline that has the same id (below)
  state         TEXT    NOT NULL DEFAULT 'active' CHECK (state IN ('active','trashed')),
  created_ms    INTEGER NOT NULL,
  updated_ms    INTEGER NOT NULL,                  -- last change of the project, of an item, or a commit of one of its timelines
  created_by    TEXT    NOT NULL,                  -- actor
  owner_user_id TEXT,                              -- network mode (SDD-007 4.1); NULL in local mode
  trashed_ms    INTEGER,
  trashed_by    TEXT,
  PRIMARY KEY (id)
);
CREATE INDEX projects_state_updated_ms_index ON projects (state, updated_ms);
CREATE TABLE project_items (
  id         TEXT    NOT NULL,                     -- pit_
  project_id TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('brief','text','timeline','output')),
  title      TEXT    NOT NULL,
  position   INTEGER NOT NULL,                     -- 0 to n-1 over every item of the project, trashed ones included
  state      TEXT    NOT NULL DEFAULT 'active' CHECK (state IN ('active','trashed')),
  ref        TEXT,                                 -- timeline: the timelines.id; output: the id of the export job; otherwise NULL
  body       TEXT    NOT NULL DEFAULT '',          -- brief and text: Markdown
  format     TEXT,                                 -- output: the destination format of the file (mp4, gif, wav, aac, png)
  created_ms INTEGER NOT NULL,
  updated_ms INTEGER NOT NULL,
  created_by TEXT    NOT NULL,
  trashed_ms INTEGER,
  PRIMARY KEY (id)
);
CREATE INDEX project_items_project_id_position_index ON project_items (project_id, position);
CREATE UNIQUE INDEX project_items_timeline_ref_unique ON project_items (ref) WHERE kind = 'timeline';
CREATE UNIQUE INDEX project_items_brief_unique ON project_items (project_id) WHERE kind = 'brief' AND state = 'active';
```

- **Kinds.** `brief`, `text`, `timeline`, `output`. `image` (epic 27) and `publication` (epic 20) are reserved: the migration that adds them rebuilds the table with a longer check, as section 5.5 does for artifact kinds.
- **A timeline item** has `ref` = the id of its row in `timelines`. The first timeline of a project has the id of the project (`timelines.id = projects.id`); a later one has a new `prj_` id (the document keeps the `prj_` prefix, SDD-001). The operation log, the revisions, the undo chain and the assets of a timeline stay keyed by the timeline id, so nothing of the timeline document or of `operations` changes. The item title is a label of the production and is independent of the document name.
- **Name and time.** A commit of a timeline sets `projects.updated_ms`, and sets `projects.name` when the timeline has the same id as the project, in the same transaction as the commit (the store does it; the service writes no second row). The list of projects orders by `projects.updated_ms`, so a project rises in it when any of its timelines changes.
- **Trash.** Trashing a project writes `projects.state` and mirrors it on every row of `timelines` of its active timeline items, so that the rules of section 13.1 (a trashed timeline rejects edits) stay one check on the row the commit loads. Trashing a timeline item mirrors the same on its row; the last active timeline of a project cannot be trashed (`E_INVALID_STATE`). Emptying (section 13.3) deletes the project, which cascades its items, and deletes the `timelines` rows its items referenced.
- **Brief and texts.** `BriefSet` writes the `body` of the one active `brief` item, creating it at position 0 when the project has none; the text is at most 256 KiB (`E_LIMIT_EXCEEDED` above). A `text` item holds a script or any other Markdown, same limit. They have no revision history here: writing tools and their history belong to epic 17. The brief is cleared, never trashed. A project holds at most 256 active items and 64 active timelines (`core/project`), the most a bundle of the whole project carries (section 16.1.1), so that every project can be exported and imported again.
- **Outputs.** An `output` item is added with the id of a succeeded export job of the same project and records its destination format; the file is read through the job, so an item never holds a path.
- **Migration.** Backup first: `Open` copies the database with `VACUUM INTO` before a migration runs (section 7 step 3). Then, in one transaction: `ALTER TABLE projects RENAME TO timelines` (SQLite rewrites the foreign keys that name it), rename the index, create the two tables, and for every row of `timelines` insert one `projects` row with the same `id`, `name`, `state`, times, actor, owner and trash columns, and one `project_items` row (kind `timeline`, title = name, position 0, `ref` = the id, the `pit_` id with the characters of the project id, state `active`: the trash of a former project is that of the project, and its restore brings the timeline back through the mirror of the trash rule of this section). Every id a surface exposed is kept. **Down** deletes the items and productions, drops them, and renames `timelines` back to `projects`; a timeline that is not the first of its project (its id differs from its project's) is deleted with its log, which is why the backup comes first. Tests run both directions over fixtures of the workspaces of releases 0.2.0, 0.17.0 and 0.29.0.
- **Document backups.** `document_backups.owner_kind` stays `project`; `owner_id` is a timeline id.

### 5.12.1 The guided briefing and the brief schema (RF-301, RF-302)

The brief is still one item with one Markdown body (section 5.12); the guided briefing is a way to fill it, and it adds no table and no column. The questions and the rules are data and pure functions of `core/project` (`briefing.go`), so a form, the CLI, the HTTP API and an agent in a chat ask and store the same thing.

| Id | Heading in the brief | Answer | Required |
| --- | --- | --- | --- |
| `audience` | Audience | text, at most 400 characters | yes |
| `goal` | Goal | text, at most 400 | yes |
| `platform` | Platform | one of `youtube-shorts`, `tiktok`, `instagram-reels`, `facebook-reels`, `other` | yes |
| `format` | Format | one of `vertical-9x16`, `square-1x1`, `horizontal-16x9` | yes |
| `duration` | Duration (seconds) | a whole number from 1 to 180 | yes |
| `tone` | Tone | text, at most 200 | no |
| `references` | References | text, at most 2000 | no |
| `must_include` | Must appear | text, at most 1000 | no |
| `must_avoid` | Must not appear | text, at most 1000 | no |
| `deadline` | Deadline | a date, `YYYY-MM-DD` | no |

**The schema of the brief the briefing writes** is Markdown: the line `# Brief`, then for each answered question, in the order above, a blank line, `## <heading>`, a blank line and the answer. An unanswered optional question has no section. A line of an answer that starts with `#` is written with a backslash in front, so an answer never makes a heading, and is read back without it. `ParseBriefing` reads the answers back and answers `ok` only for a text of this shape (the title, only known headings, each at most once) whose answers pass `CheckAnswers` and that `BriefingMarkdown` writes again as it is (trailing line ends aside): a brief written by hand, even one that uses the same headings, is never taken for one.

**`BriefingAnswer`** validates every answer (`CheckAnswers`: white space trimmed, line ends as LF, an empty answer left out, no control or text direction character, an id that is not a question refused, a required question left unanswered refused; `E_VALIDATION` with `path` `answers.<id>`), writes the Markdown with `BriefSet` (so the audit row is `project.brief.set`, the dry run and the item rules are those of `BriefSet`), and replaces the whole brief. A brief that has text and is not a guided one is not replaced unless `replace` is true (`CheckBriefingReplace`: `E_INVALID_STATE`, `path` `replace`), so a briefing never destroys text a person wrote; the check runs in the transaction of the write, so no other write of the brief lands between them, and it needs no scope beyond `edit`. **`BriefingQuestions`** returns the questions and, given a project, whether it `has_brief`, whether it is `guided` and the answers it holds, for a form to start from what is there. The text of an answer is the user's and is untrusted (SDD-007 section 8).

### 5.13 `image_documents` and `image_operations` (SDD-021)

Migration `20261119110000_create_image_documents_and_operations`. They follow `projects` and `operations` with the columns an image needs and no more.

```sql
CREATE TABLE image_documents (
  id             TEXT    NOT NULL,                 -- img_...
  name           TEXT    NOT NULL,
  template       INTEGER NOT NULL DEFAULT 0 CHECK (template IN (0,1)),
  revision       INTEGER NOT NULL DEFAULT 0,
  schema_version INTEGER NOT NULL,
  document       TEXT    NOT NULL,                 -- canonical JSON of the document
  doc_sha256     TEXT    NOT NULL,
  width          INTEGER NOT NULL,
  height         INTEGER NOT NULL,
  layers         INTEGER NOT NULL DEFAULT 0,       -- count, so that listing never parses a document
  created_ms     INTEGER NOT NULL,
  updated_ms     INTEGER NOT NULL,
  created_by     TEXT    NOT NULL,                 -- actor
  owner_user_id  TEXT,                             -- network mode only
  PRIMARY KEY (id)
);
CREATE INDEX image_documents_updated_ms_index ON image_documents (updated_ms);
CREATE TABLE image_operations (
  image_id        TEXT    NOT NULL REFERENCES image_documents (id) ON DELETE CASCADE,
  revision        INTEGER NOT NULL CHECK (revision >= 1),
  kind            TEXT    NOT NULL CHECK (kind IN ('edit','undo','redo')),
  actor           TEXT    NOT NULL,
  base_revision   INTEGER NOT NULL,
  target_revision INTEGER,
  at_ms           INTEGER NOT NULL,
  ops             TEXT    NOT NULL,                -- JSON array of {op, args}, identifiers filled in
  inverse         TEXT    NOT NULL,                -- JSON array of primitives
  diff            TEXT    NOT NULL,                -- JSON array of changes
  skipped         TEXT,
  doc_sha256      TEXT    NOT NULL,
  closed_by       INTEGER,
  idempotency_key TEXT,
  request_hash    TEXT,
  result          TEXT,                            -- the stored result of a keyed edit
  PRIMARY KEY (image_id, revision)
);
CREATE INDEX image_operations_actor_index ON image_operations (image_id, actor, revision);
CREATE UNIQUE INDEX image_operations_key_index ON image_operations (image_id, actor, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
```

The log is append-only except `closed_by`, which moves from `NULL` to a revision, as in section 5.3; undo and redo follow section 12 with the rules of SDD-021 section 4. `Repos` gains `Images() ImageRepo` with `Create`, `Get`, `List`, `Commit` (a compare-and-set on the revision, `ErrStale` otherwise), `AppendOperation`, `Operation`, `LastOpen` (the actor's last step open to undo or redo), `Close`, `CloseOpenRedo` and `ByKey`. The writer is the one write transaction of `Store.Write`.

### 5.14 `checkpoints` (RF-121, epic 36)

A name and a note on a revision of a timeline, so that a person or an agent can say "this is the state to come back to" and go back by that name. A checkpoint is not a copy of the document: it points at a revision, whose document the log rebuilds (section 12.4).

```sql
CREATE TABLE checkpoints (
  project_id TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE, -- the id of the timeline document (section 5.12)
  name       TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 240),
  note       TEXT    NOT NULL DEFAULT '',
  revision   INTEGER NOT NULL CHECK (revision >= 1),
  actor      TEXT    NOT NULL,
  auto       INTEGER NOT NULL DEFAULT 0 CHECK (auto IN (0,1)),
  at_ms      INTEGER NOT NULL,
  PRIMARY KEY (project_id, name)
);
CREATE INDEX checkpoints_revision_index ON checkpoints (project_id, revision DESC);
```

The name is unique inside a timeline (`ErrExists` becomes `E_ID_EXISTS`), 1 to 60 characters after `untrusted.Clean` and trimming, with no `/`; the note is cut at 300 characters. A checkpoint is removed with its project (the cascade of `Delete`, section 13.3) and never otherwise except by `CheckpointRemove` and by the ceiling of the automatic ones below; it holds no foreign key to `operations`, which is append-only. `Repos` gains `Checkpoints() CheckpointRepo` (section 3.2).

**Automatic checkpoint.** In the transaction of `Edit`, after the checks that can refuse the batch and before the commit, when the batch is not a dry run and changes the document (not a no-op), and the actor is an agent (SDD-003 §2.5.1), the service makes the checkpoint `auto <actor> <YYYY-MM-DD>` (UTC date of the service clock) at the revision the batch started from, unless that name exists in the timeline. The name is the whole "once a day per actor" rule: the first edit of the day creates it, the later ones find it. At most 30 automatic checkpoints are kept per timeline: after creating one, `TrimAuto(project, 30)` removes the oldest automatic ones. A checkpoint made by a person or an agent by name is never removed by the ceiling.

## 6. Workspace ownership

ADR-0008 decides that one process owns a workspace. This section fixes how.

### 6.1 Files

| File | Written by | Content |
| --- | --- | --- |
| `<workspace>/lock` | the process that takes the lock | empty; created if missing, never deleted (SDD-000 §9) |
| `<workspace>/runtime.json` | an owner process: `serve`, or `mcp` when it becomes the owner (6.4) | owner details, below |
| `<workspace>/token` | an owner process at first start | local token (SDD-007 §3.1) |
| `<workspace>/runtime.secret` | a network-mode owner, at every start, before `runtime.json`; removed with it | per-run handshake secret, mode 0600 (SDD-007 §3.8) |

The token is in its own `token` file (ADR-0019, SDD-007 §3.2). `runtime.json` holds no secret (the per-run secret of network mode is its own file):

```json
{"schema": 1, "pid": 18344, "addr": "127.0.0.1:7878", "mode": "local", "started_at": "2026-11-14T12:00:00Z", "version": "0.5.0"}
```

### 6.2 The lock

Package `service/workspace` (standard library plus `golang.org/x/sys`, a direct dependency approved by the author, pinned at v0.49.0; the fallback at the end of this section is not used). Beyond the listing below the package has `Dir.EnsureLayout`, `Dir.Path`, `ProcessAlive`, the token functions of SDD-007 §3.2 (`EnsureToken`, `LoadToken`, `RotateToken`) and `Dir.LockedError`, which builds the `E_WORKSPACE_LOCKED` error naming the owner pid.

```go
package workspace

type Dir string

func Resolve(getenv func(string) string) (Dir, error) // STUARTY_HOME or ~/.stuarty; creates it (0700 on Unix); refuses ? # % in the path

var ErrLocked = errors.New("workspace: locked by another process")

// TryLock takes the exclusive lock without waiting. ErrLocked when held.
func (d Dir) TryLock() (*Lock, error)
func (l *Lock) Release() error // also called by the OS if the process dies

type Runtime struct {
	Schema    int       `json:"schema"`
	PID       int       `json:"pid"`
	Addr      string    `json:"addr"`
	Mode      string    `json:"mode"` // local or network
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
}

func (d Dir) ReadRuntime() (Runtime, error)   // os.ErrNotExist when absent; retries a parse error for 200 ms
func (d Dir) WriteRuntime(r Runtime) error    // temp file in the same directory, then rename
func (d Dir) RemoveRuntime() error
```

| Platform | Implementation | Release |
| --- | --- | --- |
| Linux, macOS | `unix.Flock(fd, LOCK_EX\|LOCK_NB)` on `lock`, opened `O_RDWR\|O_CREATE\|O_CLOEXEC` | when the descriptor closes or the process exits |
| Windows | `windows.LockFileEx(h, LOCKFILE_EXCLUSIVE_LOCK\|LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &Overlapped{Offset: 0x7fffff00})` on a handle from `CreateFile` with share mode read and write (what `os.OpenFile` uses) | when the handle closes or the process terminates |

Windows specifics, which are why the design differs from a naive `os.OpenFile`:

1. Windows locks are mandatory for the region they cover, and any other handle's read or write of that region fails. The locked region is one byte at offset `0x7fffff00`, beyond the end of an empty file, so no reader ever touches it. The lock file itself can still be opened, read and listed by other tools (antivirus, backup, indexers) without producing a false "locked" answer. This is the reason for a lock on a byte range and not a share-mode-0 open: an antivirus scanner that has the file open at that moment would make `CreateFile` fail with `ERROR_SHARING_VIOLATION` on a workspace nobody owns.
2. `ERROR_LOCK_VIOLATION` (33) from `LockFileEx` means locked; anything else is returned as an error, not as `ErrLocked`.
3. `runtime.json` is not locked and is written by rename. On Windows `os.Rename` fails with `ERROR_ACCESS_DENIED` or `ERROR_SHARING_VIOLATION` when another process has the destination open without delete sharing, so `WriteRuntime` and `RemoveRuntime` retry up to 10 times over 1 second.
4. The lock handle is created non-inheritable (Go's default), so an FFmpeg child never keeps the workspace locked after the server dies. On Unix `O_CLOEXEC` does the same.
5. The workspace must be on a local file system. Locks on network shares (SMB, NFS) and on some Docker volume drivers are not reliable; `doctor` reports the file system type where the OS exposes it and warns for known network types (verified by a unit test of the classification, not by a network mount).

Fallback if the direct dependency on `golang.org/x/sys` is refused: Unix uses `syscall.Flock`; Windows uses `syscall.CreateFile` with share mode 0 on `lock`, retrying the sharing violation five times over 250 ms before concluding it is locked. Behaviour is the same except for the scanner case.

**Stale lock detection. Decision of this SDD.** ADR-0008 mentions detection by process id and a heartbeat. The kernel releases the lock when the owning process dies, so a lock that can be taken is by definition free and no heartbeat is needed. `runtime.json` carries the pid for diagnostics and for the liveness check of 6.4, not as the source of truth.

### 6.3 Owner start-up order

1. `Resolve` the workspace; create the directories of SDD-000 §9 that are missing.
2. `TryLock`. Failure: exit 2 with `E_WORKSPACE_LOCKED` (the message names the pid from `runtime.json` when it is readable).
3. `RemoveRuntime` (any file present is stale: only the lock holder writes it).
4. Section 7 (database, migrations, recovery).
5. Bind the listener. Only after a successful bind, `WriteRuntime` with the bound address.
6. Serve. On graceful shutdown: stop accepting, let jobs finish up to `SHUTDOWN_TIMEOUT` (15 s), checkpoint the database, `RemoveRuntime`, release the lock.

vukem API to be confirmed in task: how the application learns the address the built-in HTTP runner bound to. `serve.go` has `HTTPServer.Ready chan<- string`, but `App.Run` builds the runner privately (`httpRunner`). If it cannot be reached, the application binds its own listener, passes it to `vukem.NewHTTPServer(...).Listener`, registers it with `vukem.WithoutHTTP()` plus `AddRunner`, and writes `runtime.json` after `Listen` returns.

### 6.4 How a command chooses in-process or HTTP

Implemented in `web/app/workspace/open.go`: `func Open(ctx context.Context, o Options) (service.Service, io.Closer, error)`. `Options.Force` may be `Auto` (default), `Server` (never fall back; `E_WORKSPACE_LOCKED` when no owner) or `InProcess` (never use a server; fail with `E_WORKSPACE_LOCKED` when one exists).

```text
Open (Auto)                                  (the order of SDD-003 §5.2)
  1. STUARTY_URL set -> remote client (service/remote) with STUARTY_KEY or --key-file.
     No local file is read. Return it.
  2. d := Resolve(env)
     rt, err := d.ReadRuntime()
     if err is nil and processAlive(rt.PID):
        GET http://<host(rt.Addr)>/up with a 500 ms timeout
           200 -> run the handshake of SDD-007 3.8, then return the HTTP client Service
                  (token from <workspace>/token)
  3. lock, err := d.TryLock()
     ok -> NO OWNER. RemoveRuntime() (stale). Build the in-process Service
            (section 7 steps 3 to 8). Hold the lock until Close. Return it.
            (a one-shot CLI command runs in process under this lock; `mcp` becomes the owner, below)
     ErrLocked -> an owner exists or is starting. Go to 4.
  4. deadline := now + 10 s            (constant; verified by the test of 6.5)
     loop:
        wait 100 ms
        TryLock again (ok: as step 3)
        rt, err := d.ReadRuntime()
        if err is os.ErrNotExist: retry (owner starting, or another in-process command running)
        if !processAlive(rt.PID):  retry (stale file about to be removed by a new owner)
        GET /up as in step 2; 200 -> handshake, return the HTTP client Service; else retry
     deadline passed -> E_WORKSPACE_LOCKED (retryable, exit code 2)
```

- **A slow owner is not an absent owner (VV-252).** Only the lock says whether an owner exists: a command takes the in-process path solely when `TryLock` succeeds. While the lock is held, a probe that times out or fails only means the owner is slow or starting, so the command keeps retrying (step 4) and, at the deadline, fails with `E_WORKSPACE_LOCKED`; it never opens the workspace itself and never answers from it. A test double of an owner therefore holds the lock, as a real owner does; one that does not is a workspace with no owner, and the in-process path is then correct. `TestOpenSlowOwnerHoldingTheLockIsNeverReplacedByInProcess` and `TestASlowOwnerIsNeverReplacedByLocalAccess` (`web/app/cli`) cover it.
- `host(addr)`: an unspecified address (`0.0.0.0`, `::`) is replaced by `127.0.0.1` or `::1`.
- `processAlive`: `os.FindProcess` then, on Unix, `Signal(0)`; on Windows `os.FindProcess` already fails when `OpenProcess` fails. A `PermissionDenied` answer counts as alive.
- The probe is `GET /up`, which vukem registers by default (`http.go`, `r.Get("/up", ...)`), answers 200 with `{"status":"ok"}` and is outside `/api/v1`. The token is sent only after `/up` succeeds on a loopback address of a live pid and the server has proved that it knows the token (the handshake of SDD-007 §3.8), so it is never sent to a stranger that took the port of a dead owner. When `runtime.json` says `mode` is `network`, no local token exists: the command needs `STUARTY_KEY` (SDD-007 §4.6) and fails with `E_UNAUTHORIZED` and that hint when it is unset.
- In-process Service built here uses `queue.Sync{}` (`vukem.WithQueueDriver`) so jobs run inline (ADR-0008 item 6); section 14. This is the path of a one-shot CLI command.
- `stuarty mcp` over stdio does not use this path when no owner exists: it becomes the owner. It runs the owner start-up order of 6.3 (lock for its lifetime, listener on loopback, queue worker, event hub, `runtime.json`) and adds the stdio adapter, so jobs run on the worker, `wait_ms` works, and the UI and the CLI reach it over HTTP. When standard input closes it shuts down like `serve`. With an owner present it is an HTTP client like the CLI (SDD-003 §4.5).
- Credentials for the HTTP client in network mode are chosen by SDD-007; this SDD decides only the transport.
- NFR-024 (300 ms for a read when a server owns the workspace): `TryLock` fails in microseconds; reading `runtime.json`, `/up` and one request on loopback add a few milliseconds. Verified by an acceptance benchmark.
- A contention case: two CLI commands started together with no server. One takes the lock and runs in process; the other sees `ErrLocked`, finds no `runtime.json` and waits up to 10 s, then runs in its turn (it re-runs step 3 on each loop pass; **Decision of this SDD**: the loop in step 4 retries `TryLock` before reading `runtime.json`, so the waiting command becomes the owner as soon as the first one exits).

### 6.5 Tests of ownership

Listed in section 18: two processes racing for the lock, a killed owner (including `taskkill /F` on Windows) releasing it, a stale `runtime.json`, a CLI started while the owner is still starting, and the 10-second limit.

### 6.6 Adopting the workspace of the former name

Until release 0.29.0 the default workspace was `~/.vukem-video`. `Resolve` (section 6.2) adopts it at the first start of Stuarty, only when `STUARTY_HOME` is unset (a folder the person chose is never replaced), `~/.stuarty` does not exist and `~/.vukem-video` does and holds no `MOVED-TO-STUARTY.txt`. `workspace.AdoptFormer` does this, in order, and a failure at any step leaves the former folder complete:

1. It takes the lock of the former workspace and releases it; a process that still owns it (an old server, an old desktop application) stops the adoption with `ErrFormerInUse`, which asks the person to close it. It is a `workspace.Error` with `E_WORKSPACE_LOCKED` (retryable) whose message makes that request and names no folder, so every surface answers it as such, never as `E_INTERNAL`.
2. An optional `Check` may refuse the folder (the caller names the reason; `ErrAdoptionRefused` wraps it). The newer-workspace rule of section 7 step 2 still decides, at the database, whether this build can open what it adopted: it refuses with both version numbers and writes nothing, and the adopted folder can be renamed back.
3. **Backup.** `db.sqlite` and its `-wal` and `-shm` files are copied to `backups/adopt-<UTC yyyymmddThhmmssZ>/` inside the former folder, so the backup travels with the workspace.
4. **Move.** The folder is renamed to `~/.stuarty` (the same volume). When the rename reports another volume (`EXDEV` on Unix, `ERROR_NOT_SAME_DEVICE` on Windows) the folder is copied to `~/.stuarty.adopting`, every file is checked (size, and SHA-256 for the database files), the copy is renamed into place, and `MOVED-TO-STUARTY.txt` is left in the former folder, whose files stay. A symbolic link or another special file in the workspace stops the copy; `runtime.json` and `runtime.secret` are not copied. A failed copy removes `~/.stuarty.adopting` and nothing else.
5. **Record.** `adopted.json` in the adopted workspace holds `from`, `to`, `method` (`rename` or `copy`), `backup` and `adopted_at`.

Any failure of these steps other than the one in step 1 is a `workspace.Error` with `E_INVALID_STATE` (not retryable) whose message gives the reason, says that nothing of the former workspace was moved or deleted and that at most a backup of its database was written inside it, and names `STUARTY_HOME` as the way out, so the CLI, the HTTP API and `serve` all show it instead of `E_INTERNAL`. A failure once the folder is in place (the note `MOVED-TO-STUARTY.txt` after a verified copy, or the record of step 5) says that the workspace was adopted and only the note or the record failed.

The former folder is never deleted. `Resolve` prints one line on the standard error when it adopts (`stuarty: the workspace of Vukem Video, <folder>, was moved to <folder>; a backup of its database is in <folder>.`); the next start finds `~/.stuarty` and prints nothing. The line starts with `workspace.AdoptedNoticePrefix`; the desktop application, which has no standard error to read, captures `workspace.Notice` and shows that line once in a dialog, after a sentence in the language of the system (`shell.AdoptionMessage`). When both folders exist and the former one holds a database, the new one is used and one line names the former one and `STUARTY_HOME`. The former environment variables are separate (SDD-000 section 13.1).

Tests: `TestAdoptFormerWorkspace` (the same volume, another volume, both folders, a refused folder, an owned folder, a failed or damaged copy, no database), `TestResolveAdoptsTheFormerWorkspace` and the workspace of a release 0.29.0 binary under `service/testdata/former-workspace/`.

## 7. Start-up order and migrations

The same sequence runs for `serve` and for an in-process command, after the workspace lock is held:

| Step | Action | On failure |
| --- | --- | --- |
| 1 | Open the database through `app.DB()` with `DATABASE_URL=file:<workspace>/db.sqlite`. Assert `PRAGMA journal_mode` is `wal` and `PRAGMA foreign_keys` is 1. | exit 1 |
| 2 | **Newer-workspace check.** `Migrator.Status`: any entry with `Missing` true (applied but unknown to this binary) refuses to start: "this workspace was written by a newer version (migration `<id>` is unknown); install version `>=` ...". Also `SELECT max(schema_version)` over `projects` and `templates`; above the binary's current schema version refuses with both numbers (SDD-001 §6 item 5; RF-208). Nothing is written. | exit 1 (no `Service` exists yet). Every surface prints the same reason: `serve` as text, a command or the stdio MCP server as an `E_INVALID_STATE` error whose message is the sentence above (`owner.StartupError`); a loose token file is refused the same way with a fixed sentence that names no path |
| 3 | **Database backup.** If `Migrator.Pending` is not empty and at least one migration was applied before (not a fresh workspace): `VACUUM INTO '<workspace>/backups/db-<UTC yyyymmddThhmmss>-before-<first pending id>.sqlite'`. Keep the 5 newest, delete older ones. `backups/` is a workspace directory of SDD-000 §9. | exit 1: no migration runs if the backup cannot be written |
| 4 | `Migrator.Up(ctx)`. vukem runs each migration in its own transaction and records it; SQLite takes no migration lock (`migrations.mdx`), which is safe because the workspace lock is held. | exit 1; earlier migrations stay applied, the failing one is rolled back |
| 5 | **Document migration** (section 8) of every project and template below the current `schema_version`. | exit 1 naming the project; its transaction was rolled back, its backup row was not written |
| 6 | **Job recovery** (section 14.5). | log and continue |
| 7 | **Pending asset reconciliation:** for each `project_assets` row in `pending` whose asset is `ready` and whose project is active, issue the internal `asset.add` batch (the same code path as ingest completion). | log and continue |
| 8 | **Housekeeping:** `Idempotency.PurgeExpired(now)`; orphan sweep of section 13.4; the starter templates that the library lacks are inserted (`service.SeedStarterTemplates`). | log and continue |
| 9 | An owner process only (`serve`, or `mcp` when it becomes the owner): bind, write `runtime.json`, start serving and the queue worker (section 6.3). | |

Steps 2 to 4 run in a fresh in-process command in a few milliseconds (one `SELECT` on `migrations`, one `SELECT max(...)`, one `Pending` query). vukem creates its queue tables at boot **before** application migrations when `QUEUE_CONNECTION=database` and the driver is not supplied by `WithQueueDriverFunc` (VF-001); this SDD supplies the driver with `WithQueueDriverFunc` (section 14.1) and registers `queue.Migrations`, so the tables are created by migration 0 and the clash does not occur.

## 8. Schema migration of stored documents

ADR-0009 items 3, 4 and 6; SDD-001 §6. Step 5 of section 7.

For each project (and each template) whose `schema_version` is lower than `timeline.CurrentSchemaVersion`, **in its own `Write` transaction, holding the project lock**:

1. Read the row. Decode the stored bytes as a raw document; `timeline.Migrate` chains the pure functions `vN to vN+1` (SDD-001 §6 item 3). Validate the result with `timeline.Validate`; any violation aborts with the list.
2. `Backups.Put`: the document exactly as stored, `reason = "migrate v1 to v2"`, the project's `revision`, its old `schema_version`.
3. For a project: append an `operations` row with `kind = 'migrate'`, `revision = old revision + 1`, `actor = 'local:migrate'` (a valid actor of SDD-000 §3), `base_revision = old revision`, `ops = []`, `inverse = []`, `diff = [{"kind":"project.updated","id":"<prj_>","changes":{"schema_version":[1,2]}}]`, the new `schema_version` and `doc_sha256`, `closed_by` equal to its own revision.
4. `Operations.CloseAllOpen(project, by = new revision)`: no undo or redo chain crosses a migration, because the primitives of older rows describe the old shape.
5. `Projects.Commit` with the migrated document, `schema_version`, `resync_floor = new revision`, the new revision.
6. Commit. A template is the same without steps 3 and 4 (`TemplateRepo.UpdateDocument`).

Properties:

- **All or nothing per project.** A failure rolls back, backup row included. The process refuses to start (section 7 step 5), because a half-migrated workspace would violate NFR-012. A deliberately failing migration is part of the acceptance tests.
- **No clients are connected** when this runs at start-up, so there is no event. A client that reconnects with a `Last-Event-ID` below `resync_floor` receives `resync.required` (SDD-006).
- **Rewinding through the log** (applying `inverse` backwards, ADR-0006) stops at a `migrate` row: older history is read through `document_backups`.
- A bundle import that finds an older schema runs the same steps 1 to 5 inside the import transaction (section 16).

## 9. The commit transaction

ADR-0009 item 5, ADR-0006 items 1 to 5, SDD-002 §A.3 and §A.7. All logic lives in `service/`; the store supplies `WriteProject`.

### 9.1 Single writer per project inside one process

Four layers, each necessary:

| Layer | Guarantees |
| --- | --- |
| Workspace lock (section 6) | one process writes the database |
| Project lock inside `WriteProject`: a map `projectID -> chan struct{}` of capacity 1, acquired with `select` on the channel and `ctx.Done()`, released after the transaction **and its after-commit hooks** | commits of one project are strictly ordered, and so are the events published by the hooks |
| The one writer connection with `BEGIN IMMEDIATE` (vukem) | a write transaction never runs concurrently with another, and never waits for a lock upgrade |
| `UPDATE projects ... WHERE id = ? AND revision = ?` | the revision moves from R to R+1 or the transaction aborts with `ErrStale` (`E_INTERNAL`); a bug in the lock discipline cannot silently fork history |

Why the project lock is not redundant: `db.Tx` releases the writer connection at `COMMIT`, then runs the after-commit hooks. Without the project lock, a second commit could begin, commit and publish its event between the first commit and the first's hook, so events would be published out of revision order. The project lock is held until the hooks have run. The map entry for a project is created on first use and removed when the project is deleted; the map is bounded by the number of projects.

The project lock is always taken before the writer connection and never while holding it. Operations that touch two projects do not exist.

### 9.2 Steps, in order

Inputs: `projectID`, `actor`, `base_revision`, the operations as sent, optional `idempotency_key`, `dry_run`.

| # | Step | Writes | If the process dies right after this step |
| --- | --- | --- | --- |
| 0 | **Outside lock and transaction.** Shape checks: at most 200 operations and 512 KiB, else `E_PAYLOAD_TOO_LARGE`; key of 8 to 64 characters from `[A-Za-z0-9._~-]`, else `E_VALIDATION`. Run the analysis of SDD-002 A.9 (silences, transcript words) and fill `resolved`. It reads artifacts, can take seconds, and depends on the asset, not on the revision. | nothing | nothing happened |
| 1 | Acquire the project lock (`WriteProject`). | nothing | nothing happened |
| 2 | `BEGIN IMMEDIATE`; load the project row (`revision` R, `schema_version`, `state`, `document`). Missing: `E_NOT_FOUND`. `state = 'trashed'`: `E_INVALID_STATE` with the hint to restore it. | nothing | nothing happened |
| 3 | **Idempotency.** With a key: `Idempotency.Get`. Found and `request_hash` equal: return the stored result with `replayed: true`, writing nothing (transaction rolls back). Found and different: `E_IDEMPOTENCY_MISMATCH`. | nothing | nothing happened |
| 4 | **Base revision** (SDD-002 A.7). `base > R`: `E_VALIDATION`. `base = R`: continue. `base < R - 1000`: `E_REVISION_CONFLICT` with the hint to read the project again, without reading the log (SDD-007 T-24). `base < R`: `Operations.Since(project, base, {ExcludeActor: actor})`; decode the diffs; `command.Conflicts(ops, diffs)`. A conflict: `E_REVISION_CONFLICT` with the current revision R and the text diff of **all** batches in `(base, R]` (`DescribeDiff`); `retryable: true`. No conflict: continue and apply to the current document. | nothing | nothing happened |
| 5 | **Fill identifiers.** For every created object whose id was not supplied, `Env.NewIDs(prefix)` (cryptographic, SDD-001 §2 item 6) and write it into the stored copy of `args`. For the two commands that create a number of objects known only after they run, the stored `new_ids` is taken from `effective.new_ids` after step 6 (SDD-002 §A.3 rule 4). | nothing | nothing happened |
| 6 | **Apply.** Decode the document, fold `command.Apply` over the operations (SDD-002 A.3 rule 5), collecting `Diff` and `Inverse`. Any error: return it with `op_index`; the transaction rolls back. `dry_run`: roll back and return the result with `applied: false` and revision R. | nothing | nothing happened |
| 7 | Encode `timeline.Canonical(newDoc)`; `doc_sha256`; compute `DocSummary`; N = R + 1. | nothing | nothing happened |
| 8 | **Append the log row.** `Operations.Append` with `kind = 'edit'`, `revision = N`, `base_revision`, `at_ms`, the operations with identifiers filled in and `resolved` included, `inverse`, `diff`, `schema_version`, `doc_sha256`, `idempotency_key`. | `operations` (uncommitted) | rolled back by SQLite on restart: revision R is the head |
| 9 | **Close the actor's redo chain.** `Operations.CloseOpenRedo(project, actor, N)`: a new edit makes the actor's undone batches non-redoable (section 12). | `operations.closed_by` (uncommitted) | rolled back |
| 10 | **Write the document.** `Projects.Commit(project, expected R, {name, schema_version, document, doc_sha256, summary, updated_ms})`. `ErrStale` here is a bug: `E_INTERNAL`, rollback. | `projects` (uncommitted) | rolled back |
| 11 | **Sync `project_assets`** from the diff (section 5.5). | `project_assets` (uncommitted) | rolled back |
| 12 | **Store the idempotency result** if a key was sent. `Idempotency.Put` with `expires_ms = now + 24 h`. Delete this project's expired keys in the same transaction. | `idempotency_keys` (uncommitted) | rolled back |
| 13 | `tx.AfterCommit(publish)` where `publish` sends `batch.committed` for revision N on the hub topic `project:<id>` (SDD-006). | nothing | not reached |
| 14 | **COMMIT.** One SQLite transaction: revision N, the document, the log row and everything above become durable together, or none does. | all | see below |
| 15 | The hooks run: `publish`, then cache invalidation. The project lock is released after the hooks return. | the event | see below |
| 16 | Return the result: `revision N`, `diff`, text description, `effective`, new lint findings. | | |

**Crash outcomes.**

- Before step 14 completes (steps 1 to 14): SQLite discards the uncommitted WAL frames when the file is next opened. The database is exactly at revision R: no log row, old document, old summary. The caller received nothing. A caller that used an idempotency key retries safely and gets either a fresh commit (the first never happened) or the stored result (section 10).
- After step 14, before the response is sent (step 15 or 16): revision N is durable and consistent (log row, document, summary and `project_assets` agree). The caller saw a connection error. With an idempotency key, the retry returns the stored result with `replayed: true`. Without one, the retry carries the old `base_revision`; if the batch referred to objects it itself changed, step 4 answers `E_REVISION_CONFLICT` only for **other** actors' changes, so an exact repeat of an additive command (for example `item.split` with a caller-chosen `new_id`) fails with `E_ID_EXISTS`, which is the safe outcome. Callers that must not guess send a key (SDD-002 A.7).
- The event of revision N is lost if the process dies between step 14 and the hook. Nothing else is lost: clients reconnect after the restart and replay from the log by `Last-Event-ID` (SDD-006). The log, not the hub, is the source of truth for events.
- Power loss or an OS crash can roll the database back to an earlier commit (section 4); it is never inconsistent.

**Invariants checked after every injected crash (section 18):** `projects.revision` equals the highest `operations.revision` of the project (or 0 with no rows); revisions are contiguous from 1; `sha256(projects.document)` equals `projects.doc_sha256` equals the `doc_sha256` of the row at `projects.revision`; the summary columns equal the summary recomputed from the document; `project_assets` states agree with the document (every asset in the document is `active`, every `active` row is in the document); `PRAGMA integrity_check` returns `ok`.

### 9.3 What the store must make cheap

NFR-001 gives a command round trip 50 ms at p95 on 200 items. The store's share is 25 ms: one `SELECT` of the row (a primary key lookup, document of about 100 KB), up to a few `operations` rows for the conflict check, and four writes inside one transaction. Verified by `BenchmarkCommit200Items` (section 18, check 12), which fails above 25 ms p95 on the CI reference runner and records the number.

The core's share is measured by `BenchmarkApplyBatch200Items` and `BenchmarkApplyBatch200ItemsEdits` in `core/command` (166 text items and 34 captions of 60 words; one `item.move`, and a batch of 20 of them). `ApplyBatch` clones the document once for the whole batch, builds the canonical trees only for the objects the batch changed (an object equal in both documents contributes nothing to the diff or the inverse, so it is not encoded), and compares decoded trees with a typed `jsonEqual` instead of `reflect.DeepEqual`. Measured on a Ryzen 7 5700X3D (`go test -bench ApplyBatch -count=10`, benchstat):

| Benchmark | before | after |
| --- | --- | --- |
| `ApplyBatch200Items` (1 operation) | 53.9 ms, 23.1 MiB, 292,639 allocs | 1.53 ms, 0.85 MiB, 4,245 allocs |
| `ApplyBatch200ItemsEdits` (20 operations) | 657.5 ms, 241 MiB, 3.1 M allocs | 3.32 ms, 1.06 MiB, 5,442 allocs |

The core keeps under 5 ms of the 50 ms budget, so the store and the service own the rest.

## 10. Idempotency keys

SDD-002 §A.7. Table in section 5.4.

- **Scope.** The key is unique per `(project, actor, key)`. Two actors may use the same string.
- **Request hash.** `sha256` of the canonical JSON (sorted keys, no whitespace) of the array of operations **as received**, before identifiers are filled in and before `resolved` is added. Excluded: `base_revision`, `dry_run`, the key. A retry that re-reads and sends a newer `base_revision` with the same operations is the same request.
- **Stored result.** The result JSON as first returned (SDD-003 shape: revision, diff, description, `effective`, lint findings), without the `replayed` field. The service adds `"replayed": true` on every later return. The result is bounded by the 512 KiB request and the diff it produces; no truncation is applied.
- **Expiry.** 24 hours from the commit (`expires_ms = at + 86400000`). `Get` treats `expires_ms <= now` as absent. An expired row with the same key is replaced by `Put` (`INSERT ... ON CONFLICT DO UPDATE ... WHERE expires_ms <= ?`; if the row is not expired the conflict is an error and the commit does not reach this point because step 3 would have answered).
- **Purge.** Step 12 of the commit deletes the project's expired keys in the same transaction; start-up step 8 deletes expired keys of all projects through the `expires_ms` index. A key is never purged while it is valid, and none is evicted to make room: at 10,000 valid keys for one actor and project a new keyed request is refused with `E_RATE_LIMITED` (SDD-007 §9).
- **Dry runs and failures store nothing.** A failed batch has no stored result, so a retry with the same key runs again.
- **Undo and redo.** `IdempotencyRepo` is neutral to the kind of request; if SDD-003 gives `Undo` and `Redo` a key, they use the same table and the same steps.
- **After trash emptying** the keys of the deleted project are gone with it (cascade).

## 11. The operation log

### 11.1 What a row holds

| Column | Content |
| --- | --- |
| `revision`, `base_revision`, `at_ms`, `actor` | ordering, what the caller planned against, when, who (SDD-000 §3). `at_ms` is the service clock, not a sequence; `revision` is the order. |
| `ops` | the batch as applied: `[{"op":"item.split","args":{"item_id":"itm_a","at":"120f","new_id":"itm_b2n4p6r8t0"}}]`, with every created identifier filled in (step 5) and the analysis fields (`resolved.silences`, `resolved.words`) included, so the batch replays identically and with no media (SDD-002 A.3 rule 4, A.9). `[]` for `undo`, `redo`, `migrate` and `import`. |
| `inverse` | the primitives of SDD-002 A.6 for the whole batch, in the order they were applied. Undo applies them in reverse order. |
| `diff` | the changes of SDD-002 A.5, with whole objects for `added` and `removed`. The text form is computed on read by `DescribeDiff`; it is not stored. |
| `skipped` | undo and redo only: the primitives not applied (section 12.3). |
| `kind`, `target_revision`, `closed_by` | undo bookkeeping (section 12). |
| `schema_version`, `doc_sha256` | the document after this batch; used to detect corruption and to check bundles. |
| `idempotency_key` | the key as sent, for audit. |

The `ops` and `diff` of a batch can be large (a `caption.from_transcript` that adds 40 captions with words). There is no truncation in storage. SDD-006 defines what travels in an event.

### 11.2 Queries

Diffs since revision N, for event replay, the conflict check, `E_REVISION_CONFLICT` and history:

```sql
-- all actors: replay and conflict details
SELECT revision, kind, actor, at_ms, base_revision, diff
  FROM operations
 WHERE project_id = ? AND revision > ?
 ORDER BY revision ASC
 LIMIT ?;

-- other actors only: the conflict check of SDD-002 A.7 step 2
SELECT revision, kind, actor, diff
  FROM operations
 WHERE project_id = ? AND revision > ? AND actor <> ?
 ORDER BY revision ASC;
```

Both use the primary key `(project_id, revision)`. `EXPLAIN QUERY PLAN` shows `SEARCH operations USING INDEX sqlite_autoindex_operations_1 (project_id=? AND revision>?)` (SQLite's name for the index behind the composite primary key; it prints `USING PRIMARY KEY` only for rowid and integer keys); checked by a test.

History (descending, paged, optional actor filter):

```sql
SELECT revision, kind, actor, at_ms, base_revision, target_revision, closed_by, diff
  FROM operations
 WHERE project_id = ? AND (? = '' OR actor = ?) AND (? = 0 OR revision < ?)
 ORDER BY revision DESC
 LIMIT ?;
```

A row is shown as undone when it is an `edit` or `redo` with `closed_by` set.

### 11.3 Retention (epic 36)

The log has no retention policy: no row is ever deleted, except by the cascade of emptying the trash (section 13.3), and every row keeps its `inverse`, which is what lets any revision be rebuilt (section 12.4). The rule that goes with checkpoints is therefore a constraint on the future more than a job of today: **a revision that a checkpoint names is kept, and so is every revision after it**, because the document at the checkpoint is rebuilt by walking back from the current one through each later row. Any later policy that compacts old rows may drop only rows older than the oldest checkpoint, and must keep a rebuildable form of that revision (its `doc_sha256` row and the document). The ceiling on automatic checkpoints (section 5.14) keeps the number of pinned revisions bounded.

## 12. Undo and redo

SDD-002 §A.6; ADR-0006 item 6. Undo and redo are service operations (`Undo`, `Redo`) that are themselves committed batches: they go through the same steps as section 9, with the differences below.

### 12.1 Bookkeeping

Every row has `closed_by`: the revision that consumed it, or `NULL` while it is available. The kinds:

| Row kind | Available for | Consumed by |
| --- | --- | --- |
| `edit` | undo by its own actor | an `undo` row (`target_revision` = this revision) |
| `redo` | undo by its own actor | an `undo` row |
| `undo` | redo by its own actor | a `redo` row, or a later `edit` by the same actor |
| `migrate`, `import` | nobody | born closed (`closed_by` = own revision) |

Two statements are the whole algorithm.

```sql
-- the actor's most recent batch not yet undone (served by operations_undo_idx)
SELECT * FROM operations
 WHERE project_id = ? AND actor = ? AND closed_by IS NULL AND kind IN ('edit','redo')
 ORDER BY revision DESC LIMIT 1;

-- the actor's most recent undo not yet redone (served by operations_redo_idx)
SELECT * FROM operations
 WHERE project_id = ? AND actor = ? AND closed_by IS NULL AND kind = 'undo'
 ORDER BY revision DESC LIMIT 1;
```

The query text of the two partial indexes is repeated literally in the queries, because SQLite uses a partial index only when the query's `WHERE` implies the index's; `EXPLAIN QUERY PLAN` shows `USING INDEX operations_undo_idx` and `operations_redo_idx`, checked by a test.

Worked example for one actor and one project:

| Revision | Action | Row | Effect on `closed_by` |
| --- | --- | --- | --- |
| 5 | edit | `edit` 5 | open |
| 6 | edit | `edit` 6 | open |
| 7 | undo | `undo` 7, target 6 | `edit` 6 closed by 7; `undo` 7 open |
| 8 | undo | `undo` 8, target 5 | `edit` 5 closed by 8; `undo` 8 open |
| 9 | redo | `redo` 9, target 8 | `undo` 8 closed by 9; `redo` 9 open |
| 10 | edit | `edit` 10 | `undo` 7 closed by 10 (redo chain ended); `redo` 9 and `edit` 10 open |
| 11 | undo | `undo` 11, target 10 | `edit` 10 closed |
| 12 | undo | `undo` 12, target 9 | `redo` 9 closed (it re-applied edit 5) |

Nothing is ever undone across actors: `local:ui` and `local:claude-code` have separate chains (SDD-000 §3).

### 12.2 Undo, step by step (inside `WriteProject`)

1. Steps 1 and 2 of section 9. A trashed project is `E_INVALID_STATE`.
2. `UndoCandidate(project, actor)`; `ErrNotFound` becomes `E_NOTHING_TO_UNDO`. (Redo: `RedoCandidate`, `E_NOTHING_TO_REDO`.)
3. **What others changed since the candidate:** `Operations.Since(project, candidate.Revision, {ExcludeActor: actor})`. Build a `Guard` from their diffs: for `*.added` and `*.removed` the whole object is marked changed (field `"*"`); for `*.updated` each key of `changes` is a changed field of that object.
4. Decode the candidate's `inverse` (for redo: the `inverse` of the `undo` row). Reverse it. `command.ApplyPrimitives(doc, reversed, guard)` applies each primitive unless:

| Skip reason (`reason`) | Primitive | Condition |
| --- | --- | --- |
| `field_changed_by_other_actor` | `patch` | a field in `set` or `unset` is marked changed for that object |
| `object_changed_by_other_actor` | `patch`, `remove` | the object was marked changed as a whole (added or removed), or for `remove` any field was changed |
| `object_missing` | `patch`, `remove` | the object is not in the document |
| `id_exists` | `put` | an object with that identifier exists |
| `invalid_result` | any | after the primitives that passed the rules above were applied together, `timeline.Validate` reports a violation that names an object this primitive touches (for example an overlap with an item another actor placed, a missing track or asset); `details.violations` lists the codes |

   A `patch` with several fields is split: the fields that pass are applied, the others are skipped with their field names. A skipped primitive does not stop the rest.

   Validation is not run between primitives (SDD-002 §A.6): the remaining primitives are applied together to a working copy and `timeline.Validate` runs once on the result. While it reports violations, every primitive that touches an object named in a violation is dropped as `invalid_result` and the application starts again from the document before the undo. Each round drops at least one primitive, so the loop ends; with nothing left the undo applies nothing (step 5).
5. **The result of the undo is a batch:** `ApplyPrimitives` returns the new document, the `inverse` of what it actually applied, the `diff`, and the skipped list. If nothing at all was applied, the batch is still committed with an empty `diff` and `applied: false` in the result. `applied` is also `false` in a dry run (`UndoRequest.dry_run`, SDD-003): the steps run, the transaction rolls back and nothing is written. **Decision of this SDD.** The alternative (return without committing) leaves the candidate open forever, so one stuck step would block every earlier step of that actor.
6. Write: `Append` with `kind = 'undo'`, `target_revision = candidate.Revision`, `base_revision = R`, `ops = []`, `inverse`, `diff`, `skipped`; then `Close(candidate, N)`. For a redo the same with `kind = 'redo'`. Neither calls `CloseOpenRedo` (only a new `edit` does, section 9 step 9). Then steps 10, 11 and 14 to 16 of section 9.

### 12.3 Result shape

Fields beyond the common edit result of SDD-003 (`revision`, `diff`, `description`, `applied`; `applied` is false in a dry run and when nothing was applied):

```json
{
  "revision": 43,
  "applied": true,
  "kind": "undo",
  "undone": {"revision": 41, "kind": "edit", "actor": "local:ui", "at": "2026-11-14T12:03:10.204Z"},
  "skipped": [
    {
      "primitive_index": 1,
      "primitive": {"type": "patch", "collection": "items", "id": "itm_m1n2p3q4r5", "set": {"start": 0}},
      "fields": ["start"],
      "reason": "field_changed_by_other_actor",
      "by_revision": 42,
      "by_actor": "key:key_h8v2n5q7w3"
    }
  ],
  "diff": [{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"duration": [120, 150]}}],
  "description": "~ item itm_m1n2p3q4r5: duration 4s/120f -> 5s/150f"
}
```

`undone` is `redone` for a redo. `primitive_index` counts in the order the primitives were attempted (reversed). `by_revision` and `by_actor` are present for the two `*_other_actor` reasons. `E_NOTHING_TO_UNDO` and `E_NOTHING_TO_REDO` have no `details`.

### 12.4 Revert and the diff of two revisions (RF-120, RF-121)

`HistoryRevert` and `HistoryDiff` (SDD-003 §2.5.1) need the document at a revision. There is no stored copy of it, so the service rebuilds it from the log:

1. Read the project row and its current document (inside `WriteProject` for a revert; a plain read for a diff). The target `T` must satisfy `1 <= T <= current`; a checkpoint name is resolved to its revision first (`E_NOT_FOUND` when it does not exist).
2. Walk the rows `T+1 .. current` through `Operations.Get`. A row of kind `migrate` or `import` among them refuses the operation with `E_INVALID_STATE` (the schema of the document changed there, so its `inverse` does not describe a step back). A walk of more than 5,000 rows is `E_LIMIT` with the hint to go back in steps through a nearer checkpoint.
3. Give the `inverse` of each row, newest first, to `command.RebuildAt(current, inverses)` (SDD-002 A.6): the lists are applied one after the other, each in reverse order, on one working copy with no guard and no validation in between, and the result is validated once.
4. Compare `SHA-256(timeline.Canonical(rebuilt))` with the `doc_sha256` of the row `T`. A difference is `E_INTERNAL` with the revisions in `details`: the log and the document disagree, which is a defect, never something to continue from.

`HistoryRevert` then calls `command.Revert(current, rebuilt)` and commits the batch like an edit (section 9) as a row of kind `edit`, `ops` `[{"op":"history.revert","args":{"revision":T}}]`, with the `inverse` and `diff` of that call, `base_revision` the revision the caller read, and `closeOpenRedo` as any new edit of the actor. Because it is an `edit` row, the actor's `Undo` (section 12.2) applies its `inverse`, and the guard protects what other actors changed after it, as for any step. It writes nothing when the diff is empty. An idempotency key is not part of the request.

The two properties the tests hold (`TestRevertProperty`, `TestRevertThroughTheService`): revert to `R` leaves a document equal in canonical bytes to the document at `R`, so an export of it equals an export at `R`; and revert followed by undo leaves the document the revert started from.

## 13. Trash

ADR-0020 items 2 and 3; RF-005, RF-131.

### 13.1 Projects

- **Trash.** `ProjectTrash` in `WriteProject`: `Projects.SetState(trashed, by, now)`; request cancellation of the project's non-ingest jobs (section 14.4); after commit publish `project.updated {project_id, state: "trashed"}`. The document, revision and log are untouched; no `operations` row is written. Trashing a trashed project returns it unchanged.
- **Restore.** `SetState(active)`, clears `trashed_ms` and `trashed_by`, publishes `project.updated {state: "active"}`; then reconciles pending assets (section 7 step 7) for that project.
- **While trashed:** `ProjectGet`, `HistoryList`, `MediaInspect` and `Lint` work; `Edit`, `Undo`, `Redo`, `MediaImport`, `JobStart` and template instantiation fail with `E_INVALID_STATE` and the hint "restore the project first". `ProjectList` leaves it out unless the caller asks for trashed projects.
- **No expiry.** Nothing purges the trash automatically (ADR-0020 consequence).
- **Audit.** The row keeps who trashed it and when (`trashed_by`, `trashed_ms`); restore clears them. Trash is not a revision, so the history of these actions is written to the `audit` table (section 5.9) in the same transaction, which is what satisfies RF-134 for operations outside a document.

**With productions (section 5.12).** `ProjectTrash` and `ProjectRestore` act on the production and mirror the state on the rows of its active timeline items inside the same transaction, a `Write` of the store and not a `WriteProject` (a commit loads the row of its timeline inside its own transaction, so it sees the state before the trash or after it, never between). `ProjectItemTrash` of a timeline does the same for that one timeline and cancels its jobs. Trashed items do not count in the production state.

### 13.2 Assets in a project

- **Trash** (`MediaTrash`): for an `active` row, issue the batch `asset.remove{asset_id}` as the caller (it fails with `E_IN_USE` and `details.used_by` when an item uses it). Its diff has `asset.removed`, so the sync rule moves the row to `trashed` with the snapshot. For a `pending` row (ingest not finished or failed) no batch exists: `SetProjectAssetState(trashed)` directly.
- **Restore** (`MediaRestore`): issue the internal batch `asset.add{asset: snapshot}` with the stored snapshot (same `ast_` identifier, same hash). If the hash is already in the document, `asset.add` returns the existing identifier and changes nothing. For a `trashed` row that was never in the document (it was `pending`), restore moves it back to `pending` and reconciliation continues.
- **Undo of an asset removal** reaches the same state through the diff. The file and its artifacts stay in the store while any `project_assets` row references the hash, whatever its state.

### 13.3 Emptying

- **Who.** SDD-007 §5.3 decides: scope `admin`, and the request must come from a browser session in local mode or from an `admin` key in network mode. The local token is refused, and the operation is neither a CLI command nor an MCP tool (ADR-0020 item 2), so no agent surface can reach it. It is the only code path that calls `Projects.Delete`, `Assets.Delete`, `DeleteTrashedProjectAssets` or removes files from `store/`, `artifacts/`, `exports/` and `frames/`. The service operation is `TrashEmpty` (HTTP `POST /api/v1/trash/empty` only; SDD-000 §1).
- **Request.** `{project_id?, older_than_days?}`: with neither, everything in the trash; with `project_id`, that project (and only if it is trashed); `older_than_days` limits by `trashed_ms`. Result: counts of projects, project assets and assets deleted and bytes freed.
- **Transaction** (inside `Write`, with the store lock of 13.4 held):
  1. Select the trashed projects that match; for each, cancel its jobs that are still `queued` or `running` first and wait for them to finish (up to 15 s), then `Projects.Delete` (cascades `operations`, `idempotency_keys`, `project_assets`, `jobs` of that project); delete its `document_backups` rows explicitly (no foreign key).
  2. Delete `trashed` rows of `project_assets` that match.
  3. `Assets.Unreferenced`; for each, `Assets.Delete` (cascades `artifacts`), remembering its store path and artifact paths.
  4. Remember `exports/<project id>/` and `frames/<project id>/` of every deleted project.
  A project that still has a queued or running job after that wait, and an asset whose ingest is queued or running, are left out of the emptying and logged; the next emptying removes them.
- **After commit**, delete the remembered files and directories through `engine/store` (SDD-011).
- **Crash.** Before commit: nothing changed. After commit and before the files are removed: orphan files remain, removed by the sweep of 13.4.

### 13.4 Store lock and orphan sweep

- `storeMu sync.RWMutex` in the service: import (`MediaImport`, bundle import) holds the read lock from "copy the file into `store/` or find it already there" through the insert of its `assets` row; `TrashEmpty` holds the write lock from its transaction through the file removal. This closes the race where an import finds a file that a concurrent empty is about to delete.
- **Orphan sweep** (start-up step 8 and `doctor`): a file under `store/` whose hash has no `assets` row and whose modification time is more than 24 hours old; a directory under `artifacts/` with no `assets` row; a directory under `exports/` or `frames/` whose project does not exist. They are removed. The 24-hour age protects a file that an import copied before its row existed. It also removes, one level deep in `exports/<project id>/`, the files named `bundle-r*.vvbundle.tmp` that a killed bundle export left (section 16.2), but only those whose modification time is older than the start of the running service: an export that runs now writes a newer file. They are counted with `incoming_files`. Nothing else in `exports/` is touched. It also removes the files of `publish/uploads/` (the addresses of upload sessions, SDD-020 §7.4) whose modification time is more than 7 days old, counted with `incoming_files` too. Crash test: `TestCrashBundleExport` kills the child at `bundle_export.after_write`, then the next start's sweep removes the temp file.

## 14. Jobs

ADR-0018. State vocabulary in SDD-000 §6; table in section 5.6.

### 14.1 vukem queue configuration

| Setting | Value | How |
| --- | --- | --- |
| Driver in `serve` | `database`, tables `queue_jobs` and `queue_failed_jobs` | `vukem.WithQueueDriverFunc(func(app) { db, _ := app.DB(); return queue.NewDatabase(db, queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs", Grace: 30 * time.Second}) })`; the tables are created by `queue.Migrations` in `WithMigrations` |
| Driver in an in-process command | `queue.Sync{}` (jobs run inside `Dispatch`) | `vukem.WithQueueDriver(queue.Sync{})` |
| Queues, strict priority | `ingest,export,default` | `QUEUE_NAMES` |
| Workers | `QUEUE_CONCURRENCY=6` | |
| Per-queue cap | `QUEUE_LIMITS=ingest=<STUARTY_MAX_INGESTS>,export=<STUARTY_MAX_EXPORTS>`; the transcribe concurrency is `STUARTY_MAX_TRANSCRIBES` (defaults: ingest 2, transcribe 1) | caps running jobs per process; a second guard behind the check of 14.2 |
| Worker in `serve` | on (`QUEUE_IN_SERVE` default `true`) | |
| Handler options | `queue.Retries(0)`, `queue.OnQueue(...)`, `queue.Timeout(...)` | timeouts: the per-run ceilings of SDD-007 §6.5 plus 10 minutes (ingest 4 h 10 min, transcribe 7 h 10 min, export 2 h 10 min, frame 2 min), so the queue never kills a job that its own deadline still allows |
| Kinds | `video.ingest`, `video.transcribe`, `video.export`, `video.frame` | `Kind()` strings, stable |
| Payload | `{"job_id":"job_..."}` only | |

vukem v0.44.0 reads `QUEUE_NAMES`, `QUEUE_CONCURRENCY` and `QUEUE_LIMITS` from configuration and also takes them in code: `vukem.WithQueueWork(queue.WorkOptions{Queues, Concurrency, Limits, Sleep, StopTimeout})` replaces the configured value for every field that is set, so `serve` needs no `os.Setenv`. The polling interval is `WorkOptions.Sleep` (1 s by default), so a queued job can wait up to that long before it starts; the product keeps the default, which is acceptable for exports and ingest, and a test asserts start latency under 1.5 s.

`service/` does not import vukem, so the queue is behind one interface: `service.Enqueuer.Enqueue(ctx, tx, kind, jobID)` runs inside the write transaction that created the job row. `web/app/jobs` implements it over `queue.DispatchKindTx` (the transaction exposes its `database.Executor`) and registers the handlers `video.ingest`, `video.export` and `video.frame`, which call `service.JobRunner.RunJob`. The recovery of 14.5 is `JobRunner.Recover`, and `JobRepo.QueuedBefore` lists the queued jobs older than an instant for its item 3.

**Enqueue.** `JobStart` runs `Write`: `Jobs.CreateLimited` then `q.DispatchTx(ctx, tx, Payload{JobID})`. On the `database` driver the queue row is inserted in the same transaction as the `jobs` row, so both exist or neither does (`queue.mdx`, "Jobs and transactions"). With `queue.Sync{}` the job runs right after the commit, inside the call, before `Write` returns.

### 14.2 Concurrency limits

Enforced in `CreateLimited` before the row exists, inside the write transaction, so check and insert are atomic:

```sql
SELECT id FROM jobs WHERE kind = ? AND state IN ('queued','running') ORDER BY created_ms;
-- len >= Max (STUARTY_MAX_EXPORTS for export): return *QueueFullError{Running: ids}
SELECT id FROM jobs WHERE kind = ? AND state IN ('queued','running') AND actor = ?;
-- when MaxPerActor > 0: same, for every kind of actor (SDD-007 9.1)
```

The service turns `*QueueFullError` into `E_QUEUE_FULL` (429, retryable) with `details.running` naming the jobs. Queued jobs count, so the queue cannot grow without bound. Per-key limits of other kinds (transcription minutes, import size) are in SDD-007.

### 14.3 Claim, progress, finish

Handlers receive the job id, load the row and claim it. All statements are single conditional updates:

```sql
-- claim: exactly one worker wins; a redelivered message finds state <> 'queued' and returns nil
UPDATE jobs SET state = 'running', started_ms = ?, updated_ms = ?, attempts = attempts + 1, progress = 0, message = ''
 WHERE id = ? AND state = 'queued' AND cancel_requested = 0;

-- progress: only a running job, throttled by the handler (14.6)
UPDATE jobs SET progress = ?, message = ?, updated_ms = ? WHERE id = ? AND state = 'running';

-- finish: only a running job becomes terminal
UPDATE jobs SET state = ?, progress = ?, result = ?, error = ?, finished_ms = ?, updated_ms = ?
 WHERE id = ? AND state = 'running';
```

- If the claim matched no row, the handler re-reads: `state = 'queued' AND cancel_requested = 1` becomes `cancelled` with `E_CANCELLED` by a conditional update; any other state means another delivery owns or finished it, and the handler returns `nil`.
- **`succeeded` only after verification** (ADR-0018 item 6): the handler runs the verification of SDD-004 (streams present, duration within one frame, dimensions, pixel format, `faststart`) and only then calls `Finish(succeeded)`. A failed verification calls `Finish(failed)` with `E_VERIFY_FAILED` and `details.checks`. A handler that returns an error to the queue without calling `Finish` is a bug: the recovery of 14.5 turns the row into `failed`.
- Every successful `Claim`, `Progress`, `Finish` and `Requeue` is followed, after the statement returns, by a `job.updated` event on the project's topic (SDD-006). An ingest job belongs to the asset, so its `project_id` is null in the job object (SDD-003 §2.8) and both `job.updated` and `asset.updated` are published on the topic of every project that references the hash (`ProjectsReferencing`).
- `result` of an export is stored with the shape of SDD-003 §2.8 (`{"export": {revision, path, download_url, bytes, duration {frame, seconds}, width, height, fps, video_codec, audio_codec, loudness_lufs, checks[]}}`). `path` is absolute (`<workspace>/exports/<project id>/<job id>.mp4`) and is returned to local callers only; `download_url` is `/api/v1/jobs/<job id>/file`. `error` is stored on `failed` and on `cancelled` (`E_CANCELLED`).
- A transcription that ran and failed (no output, provider error, no word timings) is finished as `failed` with `E_TRANSCRIBE_FAILED`; `E_INTERNAL` is kept for a crash of the handler (14.5).

### 14.4 Cancellation

`JobCancel` runs `Jobs.RequestCancel`:

1. `UPDATE jobs SET cancel_requested = 1, updated_ms = ? WHERE id = ? AND state IN ('queued','running')`.
2. If the row was `queued`: `UPDATE jobs SET state = 'cancelled', error = <E_CANCELLED>, finished_ms = ? WHERE id = ? AND state = 'queued'`. The queue message later finds the claim refused and does nothing.
3. If `running`: the service looks the job up in an in-process registry `map[jobID]context.CancelFunc` filled by the handler and calls it; the context carries the FFmpeg process, whose tree `contrib/engine` kills (ADR-0018 item 5). The handler also reads `cancel_requested` between steps (every 500 ms in the progress loop), which covers a registry miss. The process tree is gone within 1 s of the cancellation (SDD-007 §6.6 is the guarantee). On observing the cancellation the handler calls `Finish(cancelled, error = E_CANCELLED)` and deletes its partial output.
4. The call returns the row as it is afterwards. If the job finished first, the answer is the final row (`succeeded` or `failed`) with no error: cancellation is a request, and the terminal state is whatever happened first. A job whose output was already complete and verified when the request arrived is reported `succeeded`.
5. Trashing a project requests cancellation of its non-ingest jobs the same way.

### 14.5 Recovery at start-up

Because the workspace has one owner, any row in `running` when a process starts is orphaned (the previous owner died or was stopped while it ran). Section 7 step 6, in one transaction:

1. `Jobs.Running`. For each row: if `attempts >= 3`, `Finish(failed)` with the code of its kind (`E_EXPORT_FAILED` for export, `E_INGEST_FAILED` for ingest, `E_INTERNAL` for transcribe and frame) and `details.reason = "interrupted"`; otherwise `Requeue(message = "restarted after an interruption")`.
2. For each requeued job, dispatch a **new** queue message (`q.DispatchTx`) with no `Unique` key. The old message may still be reserved in `queue_jobs` and will be redelivered after its reservation expires (job timeout plus `Grace`); the claim makes that delivery a no-op whatever the job's state by then. Exactly one delivery runs the job.
3. `queued` rows from before the restart keep their original queue messages (the queue is durable); a `queued` row whose message was lost is repaired the same way by dispatching one more message for every `queued` job older than 1 minute that has no live reservation (the claim makes duplicates harmless).

**Graceful shutdown.** The worker's `StopTimeout` is `SHUTDOWN_TIMEOUT` (15 s); after it, running contexts are cancelled. A handler that sees its context cancelled while `cancel_requested = 0` calls `Requeue(message = "interrupted by shutdown")` and returns the context error; vukem puts the message back without using an attempt (`queue.mdx`, "Graceful shutdown"), and the next start runs it from the beginning. Output files of an interrupted export are written to a temporary name and renamed only after verification, so a restart never sees a partial file under the final name.

### 14.6 Progress throttling

A handler writes progress at most every 250 ms and only when the value moved by at least 0.01 or the message changed. Writes are single statements on the writer connection (they never wait for a project lock). Progress is not part of the log and is not replayable (SDD-006).

### 14.7 In-process mode

`queue.Sync{}` runs the handler inside `Dispatch`; the CLI call waits until the job is terminal (ADR-0008 item 6). There is no worker, so a command asking not to wait cannot be honoured: in this mode the call always waits and the CLI reports so (SDD-003). This mode exists only for one-shot CLI commands; the stdio MCP server is never in it (6.4). A command killed while its job runs leaves a `running` row, recovered by 14.5 at the next start of any process, including the next in-process command.

### 14.8 Measured: crash, kill and soak (P4-SRV-002)

- **Crash suite.** `go test -tags crashtest ./web/app/servicetest/ -run "^(TestCrash|TestKillMid)" -count=100` passes on Windows (355 s): the commit, undo, redo, trash-emptying, migration and bundle import points and, new in this task, the two bundle export points `bundle_export.after_write` (a kill before the rename leaves no bundle at a final name) and `bundle_export.after_rename` (a kill after it leaves a whole archive). After each kill every acknowledged revision is in the log and `PRAGMA integrity_check` is `ok`.
- **Kill of a real `serve`.** `TestKillRealServeDuringConcurrentHTTPCommits` repeated 20 times (164 s), each time the process is killed with `Process.Kill` (`TerminateProcess` on Windows, `SIGKILL` elsewhere) in the middle of four clients committing.
- **Soak.** `TestSoak` (build tag `soak`, `STUARTY_SOAK_DURATION`, one hour by default; the nightly job owned by REL runs `go test -tags soak -run TestSoak -timeout 90m ./web/app/servicetest/`): four workers in process and four over HTTP on loopback run edits (60%), undos (15%), reads (15%), exports (7%) and bundle exports (3%), the HTTP workers paced under the 600 requests a minute of the one local credential. The baseline is taken at rest after a 30 s warm-up; the end is taken after the workers stop and the server closes. Local run of 10 minutes: 93,842 calls (56,125 edits, 14,227 undos, 14,130 reads, 6,639 exports, 2,721 bundle exports), no 5xx; 1,837 calls were refused by a documented limit (`E_QUEUE_FULL` 300, `E_RATE_LIMITED` 1,537); goroutines 7 at the baseline and 6 at the end; resident memory 19.3 MiB at the baseline and 19.8 MiB at the end (the runtime's mapped memory less what it returned; 24 to 28 MiB while running); no `running` job after the shutdown. The full hour is left to the nightly job.

## 15. Templates

ADR-0022; SDD-001 §3.10. A template is a timeline document stored in `templates`.

- **Save** (`TemplateSave`): the service builds the template document from a project (placeholders for marked media items, `template` info), validates it, and calls `Templates.Create` with the denormalized `slots` and summary. No log row: templates have no revisions and are immutable once saved. Saving again from the edited project creates a new template.
- **Instantiate** (`TemplateInstantiate`): in one `Write`, read the template, create the project (revision 0, then a batch of `slot.fill` operations as one revision when values were given), and return the project and the unfilled required slots. The project is a copy; later template changes do not exist.
- **Migration.** Templates migrate with projects (section 8), without log rows.
- **Template file** (RF-052; `TemplateExport`, `TemplateImport`): a zip with the container rules of section 16.1: `manifest.json` (`{"format":"stuarty-template","template_version":1,"app_version":"0.5.0","schema_version":1,"name":...}`), `template.json` (the canonical document) an optional `preview.jpg` and `font.<hex>` for each user font in `fonts` (SDD-001 §3.9). The file is served and accepted as `application/zip`, named `<template id>.stuarty-template.zip`, with the entry allowlist and the size limits of SDD-007 §7.6 (at most 64 MiB compressed, 64 entries, 256 MiB declared). Import validates the document (`timeline.Migrate`, `timeline.Validate`), copies fonts into `<workspace>/fonts/<hash>` after checking their hash, and inserts the row. A template with a newer `schema_version` is refused with `E_VALIDATION` and `details.code = "V_SCHEMA_VERSION"`.

## 16. The project bundle

ADR-0009 item 7; RF-006. Service operations for export and import of a bundle are missing from SDD-000 §1: this SDD names them `ProjectBundleExport` and `ProjectBundleImport` (reported).

### 16.1 Container

A zip file with extension `.vvbundle`, deflate-compressed (the `media.*` and `font.*` entries are stored without compression: they are already compressed), with **flat entry names** (no directories), because vukem's `archive.Zip` derives keys by dropping directories and refuses a name that is not a safe key.

| Entry | Content |
| --- | --- |
| `manifest.json` | see below; first entry |
| `document.json` | the canonical document at `manifest.project.revision` |
| `operations.ndjson` | one JSON object per line, revisions 1 to R in order; absent when exported with `include_log: false` |
| `media.<hex>` | original bytes of every asset in the document, `<hex>` the 64 hex characters of the hash |
| `font.<hex>` | every user font of the document's `fonts` |

```json
{
  "format": "stuarty-bundle",
  "bundle_version": 1,
  "app_version": "0.5.0",
  "created_at": "2026-11-14T12:30:00Z",
  "project": {"id": "prj_7k2m9x4q1b", "name": "Launch teaser", "schema_version": 1, "revision": 42},
  "document": {"sha256": "<hex>", "bytes": 18211},
  "operations": {"count": 42, "sha256": "<hex of operations.ndjson>"},
  "media": [{"hash": "sha256:9f86...", "kind": "video", "name": "IMG_0421.MOV", "bytes": 48211901, "asset_ids": ["ast_3c5v8n2w6d"]}],
  "fonts": [{"hash": "sha256:...", "family": "Brand Sans", "weight": 700, "bytes": 98211}]
}
```

A line of `operations.ndjson` is the `operations` row as JSON with the parsed columns (`ops`, `inverse`, `diff`, `skipped` embedded as JSON values, not strings), without `project_id`. Derived artifacts (proxies, thumbnails, peaks, transcripts) are not included; import regenerates them through ingest. Trashed assets, `idempotency_keys`, `jobs`, exports and caches are not included. `include_log: false` is for sharing without history (actors, edit intent).

### 16.1.1 Bundles with a production (`bundle_version` 2)

A bundle exports the whole project. The format of 16.1 stays and gains: `manifest.json` has `"bundle_version": 2` and a member `production`: `{"name", "items": [{"kind", "title", "position", "text", "format", "timeline"}]}`, the active items of the project in order; `timeline` names the timeline of an item by a key. The Markdown of a brief or a text that is not empty is the entry `item.<n>.md`, `<n>` the index of the item in `items`, and `text` is `{"sha256", "bytes"}` of that entry: the texts stay out of the manifest, whose cap (1 MiB, 16.3) is far below what 256 items of 256 KiB hold. The first active timeline is `document.json` and `operations.ndjson` as before (key `main`); every other active timeline is `timeline.<key>.json` and `timeline.<key>.ndjson` with the key `t1`, `t2`... in item order, listed in the manifest member `timelines` (`key`, `id`, `name`, `schema_version`, `revision`, `document`, `operations`, the last two as in 16.1). Media and fonts are the union over every timeline, each once. Trashed items are not exported. An `output` item is exported with its format and no job (the file is not part of the bundle); an import keeps it as a record with no job. A bundle of one timeline (`ProjectBundleExport` with `timeline_id`) is exactly the format of 16.1 with `bundle_version` 1.

**Import.** A bundle of version 1, as every release up to 0.29.0 wrote it, imports as a project with one timeline and no other item. A bundle of version 2 is checked before anything is written, by the checks of 16.3 applied to every document and log, and by these: at most 63 timelines after the first and 256 items; keys `t<1 to 3 digits>` and timeline ids unique and different from the project's; every timeline is named by exactly one item and every item that is not a timeline names none; at most one brief; the kinds are those of `core/project`, and titles and texts obey its rules; only a brief or a text declares a text, of 1 byte to 256 KiB, and its entry matches the declared hash and size; an entry named `timeline.*` or `item.*` that the manifest does not list is refused. With `as_new` every timeline gets a new id and no log; without it the ids are kept and any that exists is `E_ID_EXISTS`. A binary that knows only version 1 refuses a version 2 bundle with the instruction to update.

### 16.2 Export

1. Take the project lock only long enough to read the row and its `operations` (a consistent snapshot at revision R); then stream the zip from the snapshot.
2. Write to `exports/<project id>/bundle-r<R>-<job id>.vvbundle.tmp`, rename on success; the path is inside the export folder (ADR-0020 item 6). A kill between the write and the rename leaves the temp file; the sweep of 13.4 removes it at the next start.
3. Run as a job of kind `export` with `input.format = "bundle"` (and `input.whole = true` for a whole project; the job is kept on the first active timeline) (so it has progress and cancellation and counts toward the export limit); media is copied with `io.Copy` and its hash is recomputed while streaming; a mismatch with `assets.id` fails the job (`E_EXPORT_FAILED`, `details.reason = "store_corrupt"`).
4. Missing store file: `E_EXPORT_FAILED`, `details.reason = "missing_media"`, `details.hash`.

### 16.3 Import

1. Open the file and call `archive.Zip(r, size, archive.Policy{MaxEntries: 1024, MaxTotalSize: 32 << 30, MaxEntrySize: STUARTY_MAX_IMPORT_BYTES, Key: identity})`. Violations are `E_LIMIT_EXCEEDED`, `E_VALIDATION` or `E_UNSUPPORTED_MEDIA` per `archive.Err*`. The service applies stricter caps by entry: `manifest.json` 1 MiB, `document.json` 64 MiB, `operations.ndjson` 2 GiB, `item.<n>.md` 256 KiB (section 16.1.1). Free disk space must exceed the declared total plus 10%.
2. Read `manifest.json`. `format` must match. `bundle_version` above the supported value: `E_VALIDATION` with the instruction to upgrade. Read `document.json`, check its `sha256`.
3. Run `timeline.Migrate` if `schema_version` is older; refuse a newer one with `E_VALIDATION` and `details.code = "V_SCHEMA_VERSION"`, the message naming both versions (the convention of template and bundle import; every load of a stored document goes through the same decode, so ProjectGet, edit, plan, view and export answer it alike, and the editor page shows that code, message and hint); `timeline.Validate`; any violation is `E_VALIDATION` listing all.
4. Every `assets` entry of the document must have a `media.<hex>` entry or an existing store file; every `fonts` entry likewise. A file that is not shipped but is in the store is accepted only in local mode (the store is the caller's own). In network mode the store is shared, so such a file is accepted only for an asset whose hash is already linked to a project the caller can see; a font is always required in the bundle. The refusal is one generic `E_VALIDATION` without the hash, the same whether the hash is stored or not, so a user can neither link another user's media nor probe for it (SDD-007 T-20). Copy each missing file to a temporary name in `store/` while computing its SHA-256, compare with the manifest hash, rename into place (holding `storeMu` read lock, section 13.4). A `font.<hex>` entry is not copied to `store/`: it is validated and stored by `engine/store.Fonts` in `fonts/<sha256>` (SDD-007 section 7.7), before any media is copied, so a crafted font refuses the whole bundle and stores nothing.
5. If `operations.ndjson` is present: parse every line, require revisions `1..count` contiguous, check `operations.sha256`, and require that the last line's `doc_sha256` equals the document's canonical hash **when the document was not migrated in step 3**.
6. Project id. Option `as_new` (default false; in network mode the server chooses the id, so `as_new: false` is `E_VALIDATION`, like `ProjectCreate` with an `id`). With `as_new: false` the original id is kept; if a project with that id exists, even trashed, the answer is `E_ID_EXISTS`. With `as_new: true` a new `prj_` id is generated, the document's `id` is rewritten, and the log is **not** imported (the history of another project does not belong to a new one); the project starts at revision 1 with one `import` row.
7. One `Write`: insert the project (summary columns computed), the log rows (retaining `at_ms`; every `actor` becomes `imported:<original>` and every open row is closed by itself, so that nobody in this workspace can undo a step they never made, SDD-007 §7.6 item 6), an extra `import` or `migrate` row when step 3 migrated or `as_new` is true (with a `document_backups` row of the original document), `assets` rows for new hashes in state `pending`, `project_assets` rows in state `active` (they are in the document), and an `ingest` job for each hash whose artifacts are missing. `resync_floor` is set to the last revision.
8. After commit, ingest runs; the project opens at once and its assets show `ingesting` until done.

Crash analysis: files copied before the transaction are harmless content-addressed orphans removed by the sweep of 13.4; the single transaction makes the project appear whole or not at all.

## 17. Decisions of this SDD

| # | Decision | Where |
| --- | --- | --- |
| 1 | Stored instants are integer milliseconds in `_ms` columns | 2 |
| 2 | `assets.id` is the content hash (foreign keys can only reference `id`) | 2, 5.5 |
| 3 | `synchronous=NORMAL` is accepted; NFR-013 is read as "crash of the process" | 4 |
| 4 | The workspace lock is a dedicated `lock` file; on Windows a byte-range lock beyond end of file; no heartbeat | 6.2 |
| 5 | `runtime.json` holds no token; the token stays in `token` (SDD-000 over ADR-0019) | 6.1 |
| 6 | The waiting CLI re-tries the lock before reading `runtime.json` | 6.4 |
| 7 | A new `backups/` workspace directory and `document_backups` table | 5.8, 7 |
| 8 | A document migration is a log row of kind `migrate` that closes all undo chains and sets `resync_floor` | 8 |
| 9 | Project locks are held through the after-commit hooks, to keep events in revision order | 9.1 |
| 10 | Undo with nothing applicable still commits an empty batch | 12.2 |
| 11 | A new edit ends the actor's redo chain | 12.1 |
| 12 | Trash and restore of a project are not log rows; they are recorded in `audit` | 13.1, 5.9 |
| 13 | `TrashEmpty`, `ProjectBundleExport` and `ProjectBundleImport` are named here because SDD-000 §1 lacks them | 13.3, 16 |
| 14 | An ingest job belongs to the asset, not to a project | 5.6 |
| 15 | Job recovery at start-up requeues orphan `running` jobs up to 3 attempts | 14.5 |
| 16 | Bundle import keeps the id and the log, or with `as_new` starts a new history | 16.3 |

## 18. Acceptance

The implementation of this SDD is complete when every check passes. File paths are under `service/` and `web/`; `go test` runs on Linux, macOS and Windows CI unless a check says otherwise.

**Migrations and settings**

1. `go test ./web/database/...` creates a fresh workspace, runs `Migrator.Up`, and asserts the columns, types, nullability, defaults, checks and indexes of section 5 against `PRAGMA table_info`, `PRAGMA index_list` and `sqlite_master`. A second `Up` reports nothing to migrate.
2. A test opens the database and asserts `PRAGMA journal_mode` is `wal`, `foreign_keys` is 1, `busy_timeout` is 5000, `synchronous` is 1, and that a workspace path with `?`, `#` or `%` is refused. On the Windows job, a workspace under a path with a space and a non-ASCII letter opens.
3. A test with a database whose `migrations` table has an ID unknown to the binary starts the owner and asserts exit 1 with the "newer version" message and no writes; the same for a project with a higher `schema_version`.
4. A test with pending migrations on a non-fresh database asserts the file `backups/db-*-before-*.sqlite` exists, opens as a valid database, and that only the 5 newest are kept.

**Commit transaction and crash safety**

5. `go test ./service/ -run TestCommit` covers: base equal to current; base older without conflict (applies); base older with conflict (`E_REVISION_CONFLICT` with current revision and text diff of all batches since base); base newer (`E_VALIDATION`); a failing operation (`op_index`, nothing written); `dry_run` (nothing written, `applied: false`); a trashed project (`E_INVALID_STATE`).
6. Crash-injection suite, `go test -tags crashtest ./web/app/servicetest/...` (the suite lives in `web/` because `service/` may not import `web/`, ADR-0001, and the checks need the SQLite store). The binary is built with `-tags crashtest`, which compiles `crashpoint.Hit(name)` into the commit path; a parent test starts the test binary as a child with the point name in the environment variable `STUARTY_CRASH_AT`, the child exits with code 137 at that point, the parent reopens the database and checks the invariants of section 9.2. Points: `commit.after_lock`, `commit.after_load`, `commit.after_apply`, `commit.after_operation_append`, `commit.after_project_commit`, `commit.after_assets_sync`, `commit.after_idempotency_put`, `commit.before_commit`, `commit.after_commit`, `commit.after_publish`. Expected: for every point up to `before_commit` the project is at revision R with the old document; from `after_commit` it is at R+1; in both cases all invariants hold and `PRAGMA integrity_check` is `ok`. The same suite runs for `Undo` and `Redo` (P1-SRV-003); `TrashEmpty`, a bundle import and a document migration get their own points when those paths exist.
7. A test kills (`SIGKILL`; `taskkill /F` on Windows) a real `serve` process in the middle of 200 concurrent commits from 4 goroutines over HTTP, restarts it, and asserts: every revision the clients saw acknowledged exists in the log with an equal `doc_sha256`; revisions are contiguous; the final document equals the replay of the log from the initial document (rewind by inverses reaches the revision-0 document).
8. A race test (`go test -race`) runs 8 goroutines committing to the same project and 8 to others: the revisions of the first project are exactly `1..N` with no gap and no duplicate, and the events captured on the hub topic arrive in revision order.
9. The idempotency tests: same key and same operations returns the stored result with `replayed: true` and writes nothing (row count unchanged); same key with different operations is `E_IDEMPOTENCY_MISMATCH`; an expired key (clock advanced 24 h 1 s) runs again; keys of different actors do not collide; a failed batch stores nothing.
10. Query plan tests: `EXPLAIN QUERY PLAN` of the listing query, the `Since` queries, the undo candidate query and the redo candidate query contains `projects_state_updated_ms_index`, `PRIMARY KEY`, `operations_undo_idx` and `operations_redo_idx` respectively.
11. A property test: for random batches of commands, after every commit the summary columns equal the summary recomputed from the document and `project_assets` agrees with the document.
12. `BenchmarkCommit200Items`: a project of 200 items and 2,000 caption words, one `item.move` per iteration through `Service.Edit` in process: p95 at most 25 ms on the CI reference runner; the result is printed and the check fails above the limit.

**Undo and redo**

13. Table test of the worked example of section 12.1: the exact `closed_by` values and candidates after each step. A second actor's chain is independent.
14. Skip tests, one per reason of section 12.2, each with two actors: B changes a field after A's edit and A's undo skips exactly that field and applies the others; B removes an object; B places an item that makes A's `put` overlap (`invalid_result` with `E_OVERLAP`'s code in `violations`); the result JSON matches section 12.3.
15. Undo then redo of any batch returns a document byte-equal to the original (canonical bytes), for the golden batches of SDD-002 Part C, with no other actor present.
16. An undo whose primitives are all skipped commits a batch with an empty diff, `applied: false`, and consumes the candidate. `E_NOTHING_TO_UNDO` and `E_NOTHING_TO_REDO` are returned when the chain is empty; a new edit by the actor makes redo return `E_NOTHING_TO_REDO`.
17. No undo crosses a migration: after a document migration, `UndoCandidate` returns `ErrNotFound` for every actor.

**Trash**

18. Project trash and restore: state, `trashed_by`, events `project.updated`, edits refused while trashed, listing filters; no `operations` row and no revision change.
19. Asset trash: `MediaTrash` of an unused asset moves the row to `trashed` with a snapshot; `MediaTrash` of a used asset is `E_IN_USE`; `MediaRestore` re-adds the same `ast_` identifier; undo of `asset.remove` yields the same state as restore; importing the same file again restores it.
20. `TrashEmpty` with a key that lacks `admin` is `E_FORBIDDEN` over HTTP, is refused for the local token, and is absent from the CLI command list and from the MCP tool list (a test lists the 15 tools of SDD-000 §1 and finds none). A scripted workload of every other `Service` operation with a key of scopes `read, edit, media, render` never reduces `SELECT count(*) FROM projects`, `assets`, nor the files under `store/`.
21. `TrashEmpty` removes rows, store files, artifacts, exports and frame caches of what it deletes and none of what is referenced elsewhere (a hash used by two projects survives the emptying of one). A crash after commit leaves orphans that the sweep removes only when older than 24 h.

**Workspace ownership**

22. Two OS processes call `TryLock` in a loop: exactly one succeeds at a time; after the owner is killed (`SIGKILL` / `taskkill /F`) the other acquires it within 1 s. Runs on the Windows job.
23. A stale `runtime.json` (dead pid) is removed by the next owner; a CLI started while the owner is still migrating waits and then connects; a CLI started with no owner runs in process, and a second CLI started at the same time waits and then runs in its turn; with a lock held by a process that never writes `runtime.json`, the CLI returns `E_WORKSPACE_LOCKED` after 10 s and exit code 2.
24. Contract tests (ADR-0008) run the same `Service` scenarios against the in-process and the HTTP implementation and compare the resulting documents byte for byte (NFR-025). `BenchmarkCLIRead` with a server owning the workspace: p95 of a read command at most 300 ms (NFR-024).
25. The lock file can be opened and read by another process while held (Windows: open with default sharing succeeds; reading offset 0 succeeds).

**Documents, templates, bundles**

26. Migration of stored documents: for every fixture of every schema version (`core/timeline/testdata/versions/vN/*.json`) a workspace with that document starts and migrates it; the `document_backups` row equals the original bytes; the log has a `migrate` row; `resync_floor` equals its revision. A deliberately failing migration function aborts start-up naming the project, with no backup row and the document unchanged.
27. Template save, list, instantiate and file round trip: the instantiated project equals the template document with slots filled; the exported file imports on a fresh workspace; a newer `schema_version` is refused.
28. Bundle round trip : export a project with two assets and a custom font, import on a fresh workspace, and assert: same document bytes, same log (all columns, except that actors carry the `imported:` prefix and no row is open to undo), same hashes, ingest jobs created, `project_assets` active. With `as_new: true` the id differs and the log is absent. Negative cases: a zip bomb (declared sizes over the policy), an entry name with `..`, two entries that collapse to one key, a media file whose bytes do not match the manifest hash, a log with a gap, a `document_sha256` mismatch, a newer `bundle_version`: each fails with the documented code and writes nothing to the database or `store/`.

**Jobs**

29. Claim: 20 concurrent claims of one job: exactly one returns true. A redelivered queue message for a finished job does nothing.
30. Limits: with `STUARTY_MAX_EXPORTS=2`, the third `JobStart` of kind `export` returns `E_QUEUE_FULL` with the two job ids in `details.running`, and no row is created; 20 concurrent `JobStart` calls create exactly 2.
31. Progress: a handler emitting 1,000 progress calls in 1 s produces at most 5 `UPDATE`s of the row and at most 5 `job.updated` events, and the last state is delivered.
32. Cancel: cancelling a queued job ends `cancelled` before any claim; cancelling a running export ends `cancelled` within 2 s with its FFmpeg process tree gone and no file at the final output path; cancelling after completion returns the final row; cancelling twice is idempotent.
33. Verification gate: a handler whose output fails verification ends `failed` with `E_VERIFY_FAILED`, never `succeeded`.
34. Recovery: kill the process while an export runs; on restart the job is `queued` then `running` again and ends `succeeded`; a job interrupted 3 times ends `failed` with `details.reason = "interrupted"`; a graceful shutdown during an export requeues it with the shutdown message and the next start completes it.
35. In-process: a CLI command that starts an export waits for it and exits with the job's state; the row is terminal when the process exits.
36. A test with `QUEUE_CONNECTION=database` and `queue.Migrations` asserts `queue_jobs` and `queue_failed_jobs` exist after `Migrator.Up`, and that no table named `jobs` is created by vukem's boot.

**Hygiene**

37. `go vet ./...` and `golangci-lint` report nothing; `core/` and `service/` import no vukem package (the import-rule test of ADR-0001); `web/` is the only package importing `github.com/AlexGalhardo/vukem`.
38. A test greps the store implementation for `UPDATE operations` and finds only the three `closed_by` statements, and for `DELETE FROM projects`, `DELETE FROM assets` and `os.Remove` of store paths and finds them reachable only from the `TrashEmpty` path (checked with `go/ast` over `service/` and `web/app/`).
39. `markdownlint` passes on this document.
