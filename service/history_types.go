package service

import (
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// HistoryRevertRequest takes a timeline back to an earlier revision (SDD-003 section 2.5.1). Exactly one
// of Revision and Checkpoint names the target.
type HistoryRevertRequest struct {
	ProjectID    string `json:"project_id"`
	TimelineID   string `json:"timeline_id,omitempty"`
	Revision     int64  `json:"revision,omitempty"`
	Checkpoint   string `json:"checkpoint,omitempty"`
	BaseRevision int64  `json:"base_revision,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
}

// HistoryRevertResult is an EditResult (its diff is the change the revert makes) with the target.
type HistoryRevertResult struct {
	EditResult
	Kind   string `json:"kind"`   // "revert"
	Target int64  `json:"target"` // the revision the document was taken back to
	Same   bool   `json:"same"`   // the document already equals that revision: nothing was written
	Text   string `json:"text"`
}

// HistoryDiffRequest asks for the difference between two revisions. From is a revision or, as
// FromCheckpoint, the name of a checkpoint; To is a revision, the current one when 0.
type HistoryDiffRequest struct {
	ProjectID      string `json:"project_id"`
	TimelineID     string `json:"timeline_id,omitempty"`
	From           int64  `json:"from,omitempty"`
	FromCheckpoint string `json:"from_checkpoint,omitempty"`
	Until          int64  `json:"until,omitempty"`
}

// DiffEntry is one change between two revisions (SDD-002 A.5), classified.
type DiffEntry = command.DiffEntry

// HistoryDiffResult is the difference between two revisions.
type HistoryDiffResult struct {
	Envelope
	ProjectID string      `json:"project_id"`
	Revision  int64       `json:"revision"` // the current revision of the timeline
	From      int64       `json:"from"`
	To        int64       `json:"to"`
	Count     int         `json:"count"`
	Entries   []DiffEntry `json:"entries"`
	Text      string      `json:"text"`
}

// Checkpoint is a name and a note on a revision (SDD-005 section 5.14).
type Checkpoint struct {
	Envelope
	Name     string    `json:"name"`
	Note     string    `json:"note"`
	Revision int64     `json:"revision"`
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
	Auto     bool      `json:"auto"`
}

// CheckpointCreateRequest names a revision, the current one when Revision is 0.
type CheckpointCreateRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"`
	Name       string `json:"name"`
	Note       string `json:"note,omitempty"`
	Revision   int64  `json:"revision,omitempty"`
}

// CheckpointListRequest lists the checkpoints of a timeline.
type CheckpointListRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"`
}

// CheckpointRenameRequest changes the name of a checkpoint, and its note when Note is present.
type CheckpointRenameRequest struct {
	ProjectID  string  `json:"project_id"`
	TimelineID string  `json:"timeline_id,omitempty"`
	Name       string  `json:"name"`
	NewName    string  `json:"new_name"`
	Note       *string `json:"note,omitempty"`
}

// CheckpointRemoveRequest removes a checkpoint, never a revision.
type CheckpointRemoveRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"`
	Name       string `json:"name"`
}

// CheckpointListResult is the checkpoints of a timeline, newest revision first.
type CheckpointListResult struct {
	Envelope
	ProjectID   string       `json:"project_id"`
	Revision    int64        `json:"revision"`
	Checkpoints []Checkpoint `json:"checkpoints"`
	Text        string       `json:"text"`
}
