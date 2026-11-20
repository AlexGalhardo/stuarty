package service

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/core/writing"
)

// ScopePostsRead is the scope of the read API of the blog (SDD-020 section 47): a key that holds only it reaches the
// routes of the published posts and nothing else.
const ScopePostsRead = "posts:read"

// BlogService is the part of the service that checks a blog post (SDD-019 section 13) and serves the posts a blog_api
// destination published (SDD-020 section 47). Service embeds it.
type BlogService interface {
	PostCheck(ctx context.Context, req PostCheckRequest) (*PostCheckResult, error)
	PublishedPostList(ctx context.Context, req PublishedPostListRequest) (*PublishedPostListResult, error)
	PublishedPostGet(ctx context.Context, req PublishedPostGetRequest) (*PublishedPostResult, error)
	PublishedPostMedia(ctx context.Context, req PublishedPostMediaRequest) (*PublishedPostMediaResult, error)
}

// PostCheckRequest names a stored post.
type PostCheckRequest struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
}

// PostImageReport is one image the post uses: its reference, where it goes in a package, and whether the project has
// it as an image.
type PostImageReport struct {
	// AssetID is the id in the text; Role is cover or body.
	AssetID string `json:"asset_id"`
	Role    string `json:"role"`
	// Name is the file name the image gets in images/.
	Name string `json:"name"`
	// Found is true when the id is an active image asset of the project.
	Found       bool   `json:"found"`
	Bytes       int64  `json:"bytes,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// PostCheckResult is the check of a post: its metadata, every problem and the sanitised HTML.
type PostCheckResult struct {
	Envelope
	ProjectID string           `json:"project_id"`
	ItemID    string           `json:"item_id"`
	Meta      writing.PostMeta `json:"meta"`
	// Slug is the slug the post is sent with: the one of the front matter, or the one made from the title.
	Slug string `json:"slug"`
	// OK is true when the post has no issue.
	OK     bool                `json:"ok"`
	Issues []writing.PostIssue `json:"issues"`
	Images []PostImageReport   `json:"images"`
	Words  int                 `json:"words"`
	// Markdown is the body after the front matter, which the page shows as the preview; the page computes none of it.
	Markdown string `json:"markdown" untrusted:"true"`
	// HTML is the sanitised rendering of the body, with the images written as images/<name>. It is for a destination:
	// a surface that draws a preview draws the structure of the text, never this field as markup (SDD-019 section 11).
	HTML string `json:"html" untrusted:"true"`
	Text string `json:"text"`
}

// PublishedPostListRequest is a page of the published posts.
type PublishedPostListRequest struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// PublishedPostSummary is a post of the list: its metadata and no body.
type PublishedPostSummary struct {
	Slug         string    `json:"slug"`
	Title        string    `json:"title"           untrusted:"true"`
	Summary      string    `json:"summary"         untrusted:"true"`
	Tags         []string  `json:"tags"            untrusted:"true"`
	Language     string    `json:"language"`
	CanonicalURL string    `json:"canonical_url"   untrusted:"true"`
	Cover        string    `json:"cover,omitempty"`
	PublishedAt  time.Time `json:"published_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PublishedPostListResult is a page of the list. NextCursor is empty on the last page; ETag is the strong tag of the
// content of the page, for If-None-Match.
type PublishedPostListResult struct {
	Envelope
	Posts      []PublishedPostSummary `json:"posts"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	ETag       string                 `json:"-"`
}

// PublishedPostGetRequest names a post by its slug.
type PublishedPostGetRequest struct {
	Slug string `json:"slug"`
}

// PublishedPostImage is an image of a published post, as the read API lists it.
type PublishedPostImage struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	SHA256      string `json:"sha256"`
	// URL is the route that serves the image.
	URL string `json:"url"`
}

// PublishedPostResult is one published post with its body.
type PublishedPostResult struct {
	Envelope
	PublishedPostSummary
	Markdown string               `json:"markdown" untrusted:"true"`
	HTML     string               `json:"html"     untrusted:"true"`
	Images   []PublishedPostImage `json:"images"`
	ETag     string               `json:"-"`
}

// PublishedPostMediaRequest names an image of a published post.
type PublishedPostMediaRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// PublishedPostMediaResult is where the bytes of an image are: a path relative to the workspace, which an adapter
// serves.
type PublishedPostMediaResult struct {
	Path        string
	ContentType string
	Bytes       int64
	SHA256      string
}
