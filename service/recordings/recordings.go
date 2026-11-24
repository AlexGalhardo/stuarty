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
}

// Store is the recordings table.
type Store interface {
	Insert(ctx context.Context, r Row) error
	Get(ctx context.Context, id string) (Row, error)
	// Update writes every member but the id, the project, the actor and the creation time.
	Update(ctx context.Context, r Row) error
	// ListByProject is the recordings of a project, newest first.
	ListByProject(ctx context.Context, projectID string) ([]Row, error)
	// Open is the recordings that are not final: prepared, recording or finishing.
	Open(ctx context.Context) ([]Row, error)
}
