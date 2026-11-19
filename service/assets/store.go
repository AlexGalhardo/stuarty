package assets

import (
	"context"
	"errors"

	"github.com/AlexGalhardo/stuarty/core/asset"
)

// ErrNotFound is returned by Store.Get for a hash with no library row.
var ErrNotFound = errors.New("assets: no library row")

// Row is the library side of one asset (SDD-005 section 5.5.1).
type Row struct {
	Hash        string
	Origin      asset.Origin
	Description string
	Reusable    bool
	Tags        []string // normalized, sorted
	CreatedBy   string
	CreatedMs   int64
	UpdatedMs   int64
}

// Store is the persistence the library needs. Inside a transaction it is bound to it.
type Store interface {
	// Get returns the row of a hash; ErrNotFound when there is none.
	Get(ctx context.Context, hash string) (Row, error)
	// GetMany returns the rows that exist among the hashes.
	GetMany(ctx context.Context, hashes []string) (map[string]Row, error)
	// Put writes the row and its tags, replacing the ones there, and refreshes the search row of the hash.
	Put(ctx context.Context, row Row) error
	// Reindex refreshes the search row of a hash: the names it is known by, its tags and its origin text.
	Reindex(ctx context.Context, hash string) error
	// Search returns the hashes whose search row matches the expression made by asset.SearchQuery.
	Search(ctx context.Context, match string) (map[string]bool, error)
}
