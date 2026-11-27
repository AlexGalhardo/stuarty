// Package recordings is the storage contract of the screen recordings (SDD-025, SDD-005 section 5.18): the row,
// and the Store the database implements. No frame and no audio is in it, only what a list shows.
package recordings

import (
	"context"
	"errors"
)

// ErrNotFound means there is no such recording.
var ErrNotFound = errors.New("recordings: not found")

// Row is one recording. State and Review hold the values of core/recording. SourceLabel is the title of a
// captured window, the user's own text, and is sealed in the database (SDD-005 section 5.17.2).
type Row struct {
	ID           string
	ProjectID    string
	State        string
	Review       string
	SourceKind   string
	SourceLabel  string
	Width        int
	Height       int
	FPS          int
	Encoder      string
	StartedMs    int64
	EndedMs      int64
	DurationMs   int64
	Frames       int64
	Dropped      int64
	Bytes        int64
	StopReason   string
	ErrorCode    string
	ErrorMessage string
	AssetHash    string
	AssetID      string
	Actor        string
	CreatedMs    int64
	// Tracks and Pauses are JSON lists (core/recording Track and Pause), EventsCount the pointer events saved
	// (SDD-025 section 16.8). They hold no device name and no pointer position.
	Tracks      string
	Pauses      string
	EventsCount int64
	// Plan is the JSON of a prepared recording a person has still to start, empty for every other (SDD-025 section
	// 19.1). Marks and Proposals are JSON lists (core/recording Mark and Proposal; SDD-025 section 19.6).
	Plan      string
	Marks     string
	Proposals string
}

// Store is the recordings table.
type Store interface {
	Insert(ctx context.Context, r Row) error
	Get(ctx context.Context, id string) (Row, error)
	// Find is Get with the source label left out (empty): it needs no unlocked studio, so the service settles a
	// recording with it while the studio is locked (the recovery of start-up). Update never writes the label.
	Find(ctx context.Context, id string) (Row, error)
	// Update writes every member but the id, the project, the actor and the creation time.
	Update(ctx context.Context, r Row) error
	// ListByProject is the recordings of a project, newest first.
	ListByProject(ctx context.Context, projectID string) ([]Row, error)
	// ListRecent returns the most recent recordings of the owner, or the local workspace when owner is empty.
	ListRecent(ctx context.Context, owner string) ([]Row, error)
	// ListByAssetHash is the recordings whose screen asset has that content hash, with no source label (the
	// emptying of the trash needs none).
	ListByAssetHash(ctx context.Context, hash string) ([]Row, error)
	// Open is the recordings that are not final: prepared, recording, paused or finishing. A recording prepared
	// with a plan, which no helper has started, is not open.
	Open(ctx context.Context) ([]Row, error)
}
