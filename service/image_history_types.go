package service

// The revert, the checkpoints and the diff of an image document (SDD-021 section 4, SDD-003 section 2.5.1): the
// requests and results of the timeline's, with the id of the image in place of the id of the project.

// ImageRevertRequest takes an image back to an earlier revision. Exactly one of Revision and Checkpoint names
// the target.
type ImageRevertRequest struct {
	ImageID      string `json:"image_id"`
	Revision     int64  `json:"revision,omitempty"`
	Checkpoint   string `json:"checkpoint,omitempty"`
	BaseRevision int64  `json:"base_revision,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
}

// ImageRevertResult is an ImageEditResult (its diff is the change the revert makes) with the target.
type ImageRevertResult struct {
	ImageEditResult
	Kind   string `json:"kind"`   // "revert"
	Target int64  `json:"target"` // the revision the document was taken back to
	Same   bool   `json:"same"`   // the document already equals that revision: nothing was written
	Text   string `json:"text"  `
}

// ImageCheckpointCreateRequest names a revision of an image, the current one when Revision is 0.
type ImageCheckpointCreateRequest struct {
	ImageID  string `json:"image_id"`
	Name     string `json:"name"`
	Note     string `json:"note,omitempty"`
	Revision int64  `json:"revision,omitempty"`
}

// ImageCheckpointListRequest lists the checkpoints of an image.
type ImageCheckpointListRequest struct {
	ImageID string `json:"image_id"`
}

// ImageCheckpointRenameRequest changes the name of a checkpoint, and its note when Note is present.
type ImageCheckpointRenameRequest struct {
	ImageID string  `json:"image_id"`
	Name    string  `json:"name"`
	NewName string  `json:"new_name"`
	Note    *string `json:"note,omitempty"`
}

// ImageCheckpointRemoveRequest removes a checkpoint, never a revision.
type ImageCheckpointRemoveRequest struct {
	ImageID string `json:"image_id"`
	Name    string `json:"name"`
}

// ImageCheckpointListResult is the checkpoints of an image, newest revision first.
type ImageCheckpointListResult struct {
	Envelope
	ImageID     string       `json:"image_id"`
	Revision    int64        `json:"revision"`
	Checkpoints []Checkpoint `json:"checkpoints"`
	Text        string       `json:"text"       `
}

// ImageDiffRequest asks for the difference between two revisions of an image. From is a revision or, as
// FromCheckpoint, the name of a checkpoint; Until is a revision, the current one when 0.
type ImageDiffRequest struct {
	ImageID        string `json:"image_id"`
	From           int64  `json:"from,omitempty"`
	FromCheckpoint string `json:"from_checkpoint,omitempty"`
	Until          int64  `json:"until,omitempty"`
}

// ImageDiffResult is the difference between two revisions of an image: its layers, its canvas and its files.
type ImageDiffResult struct {
	Envelope
	ImageID  string      `json:"image_id"`
	Revision int64       `json:"revision"` // the current revision of the image
	From     int64       `json:"from"`
	To       int64       `json:"to"`
	Count    int         `json:"count"`
	Entries  []DiffEntry `json:"entries"`
	Text     string      `json:"text"   `
}
