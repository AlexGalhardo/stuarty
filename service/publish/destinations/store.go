package destinations

import (
	"context"
	"errors"
	"time"

	"github.com/AlexGalhardo/stuarty/core/writing"
)

// The errors of the store of the blog destinations.
var (
	// ErrNotFound is a post, or a remote id, that is not there.
	ErrNotFound = errors.New("destinations: not found")
	// ErrSlugTaken is a slug that another post already uses in the read API.
	ErrSlugTaken = errors.New("destinations: the slug is used by another post")
)

// PublishedPost is a post a blog_api destination published: what the read API serves (SDD-020 section 47).
type PublishedPost struct {
	Slug         string
	PostItemID   string
	ProjectID    string
	ConnectionID string
	OwnerUserID  string
	Meta         writing.PostMeta
	// Status is draft or published; the routes list published ones only.
	Status      string
	Markdown    string
	HTML        string
	Images      []Image
	PublishedAt time.Time
	UpdatedAt   time.Time
}

// ListFilter selects published posts for a page of the read API.
type ListFilter struct {
	// OwnerUserID limits the posts to those of one user (network mode); empty is every owner.
	OwnerUserID string
	// ProjectID limits the posts to one project (a key limited to a project); empty is every project.
	ProjectID string
	// AfterMs and AfterSlug are the position of the last post of the previous page; zero starts at the newest.
	AfterMs   int64
	AfterSlug string
	Limit     int
}

// Store is where the blog tables live (SDD-005 section 5.15.1). On a transaction it is bound to it.
type Store interface {
	// Remote returns what an earlier send left, or ErrNotFound.
	Remote(ctx context.Context, connectionID, postItemID string) (Remote, error)
	// PutRemote records the remote id of a post at a connection, replacing an earlier one.
	PutRemote(ctx context.Context, r Remote) error
	// DeleteRemote forgets the remote id (the post is gone at the destination).
	DeleteRemote(ctx context.Context, connectionID, postItemID string) error
	// PutPublished publishes a post: it adds the row, or replaces the one of the same post and connection, and
	// keeps the time of the first publication. A slug another post uses is ErrSlugTaken.
	PutPublished(ctx context.Context, p PublishedPost) error
	// Published returns a published post by its slug, or ErrNotFound. A status of draft is returned too: the
	// caller decides.
	Published(ctx context.Context, slug string) (PublishedPost, error)
	// ListPublished returns the posts with the status published, newest first.
	ListPublished(ctx context.Context, f ListFilter) ([]PublishedPost, error)
}
