package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/core/writing"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// The image types a post may carry, by what the bytes say they are (SDD-020 section 43).
var postImageTypes = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/gif":  "gif",
	"image/webp": "webp",
}

// postImage is an image of a project the post uses, as the store knows it.
type postImage struct {
	assetID     string
	hash        string // the hex of the sha256, which is also the key in the store
	bytes       int64
	name        string // the original name of the file, untrusted
	contentType string
	ext         string
}

// postImages resolves the asset ids a post uses against the project: an active image asset, at most the size of an
// image of a post. A missing or foreign id is absent from the result, and a caller turns that into an issue.
func (s *svc) postImages(ctx context.Context, projectID string, ids []string) (map[string]*postImage, error) {
	out := map[string]*postImage{}
	store := enginestore.New(s.workspace)

	for _, id := range ids {
		pa, err := s.store.Assets().GetProjectAsset(ctx, projectID, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}

			return nil, asError(err)
		}

		if pa.State != ProjectAssetActive {
			continue
		}

		row, err := s.store.Assets().Get(ctx, pa.AssetHash)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}

			return nil, asError(err)
		}

		hexHash := strings.TrimPrefix(row.Hash, "sha256:")
		if row.Kind != "image" || row.IngestState != IngestReady || !enginestore.ValidHash(hexHash) {
			continue
		}

		ct, serr := sniffImage(store, hexHash)
		if serr != nil {
			continue
		}

		out[id] = &postImage{
			assetID: id, hash: hexHash, bytes: row.SizeBytes, name: row.OriginalName, contentType: ct,
			ext: postImageTypes[ct],
		}
	}

	return out, nil
}

// sniffImage reads the first bytes of a stored file and answers its content type when it is one a post may carry.
func sniffImage(store *enginestore.Store, hexHash string) (string, error) {
	f, err := store.Open(hexHash)
	if err != nil {
		return "", err
	}

	defer func() { _ = f.Close() }()

	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)

	ct := http.DetectContentType(head[:n])
	if _, ok := postImageTypes[ct]; !ok {
		return "", errors.New("not an image a post may carry")
	}

	return ct, nil
}

// imageNames gives each image of a post its file name in images/: cover.<ext>, or <n>-<slug of the name>.<ext>.
func imageNames(post *writing.Post, ids []string, found map[string]*postImage) map[string]string {
	names := map[string]string{}
	n := 0

	for _, id := range ids {
		im := found[id]
		if im == nil {
			continue
		}

		if id == post.Meta.Cover {
			names[id] = "cover." + im.ext

			continue
		}

		n++

		base := writing.Slugify(strings.TrimSuffix(im.name, filepath.Ext(im.name)))
		if base == "" {
			base = "image"
		}

		if len(base) > 40 {
			base = strings.TrimRight(base[:40], "-")
		}

		names[id] = strconv.Itoa(n) + "-" + base + "." + im.ext
	}

	return names
}

// PostCheck validates a stored post and renders it.
func (s *svc) PostCheck(ctx context.Context, req PostCheckRequest) (*PostCheckResult, error) {
	c, err := s.authorizeProject(ctx, "PostCheck", req.ProjectID)
	if err != nil {
		return nil, err
	}

	body, serr := s.textSource(ctx, c, req.ProjectID, req.ItemID, writing.KindPost)
	if serr != nil {
		return nil, serr
	}

	// The ids are read from the body first, so that the project is asked about those only.
	first, _ := writing.ParsePost(body)
	ids := writing.Images(first.Markdown, first.Meta.Cover)

	found, ferr := s.postImages(ctx, req.ProjectID, ids)
	if ferr != nil {
		return nil, ferr
	}

	have := make([]string, 0, len(found))
	for id := range found {
		have = append(have, id)
	}

	post, issues := writing.CheckPost(body, have)
	names := imageNames(&post, ids, found)

	res := &PostCheckResult{
		UntrustedNotice: UntrustedNotice, ProjectID: req.ProjectID, ItemID: req.ItemID,
		Meta: post.Meta, Slug: post.Meta.EffectiveSlug(), OK: len(issues) == 0, Issues: orEmpty(issues),
		Images: []PostImageReport{}, Words: writing.CountWords(post.Markdown), Markdown: post.Markdown,
		HTML: writing.RenderHTML(post.Markdown, imageSrc(names)),
	}

	for _, id := range ids {
		r := PostImageReport{AssetID: id, Role: "body"}
		if id == post.Meta.Cover {
			r.Role = "cover"
		}

		if im := found[id]; im != nil {
			r.Found, r.Name, r.Bytes, r.ContentType = true, names[id], im.bytes, im.contentType
		}

		res.Images = append(res.Images, r)
	}

	res.Text = postCheckText(res)

	return res, nil
}

func orEmpty(i []writing.PostIssue) []writing.PostIssue {
	if i == nil {
		return []writing.PostIssue{}
	}

	return i
}

func imageSrc(names map[string]string) writing.ImageSrc {
	return func(id string) (string, bool) {
		n, ok := names[id]

		return "images/" + n, ok
	}
}

func postCheckText(r *PostCheckResult) string {
	var b strings.Builder

	b.WriteString(textview.Note)
	fmt.Fprintf(
		&b,
		"\npost %s: %s, %d words, %d images",
		textview.Quote(r.Slug, 80),
		okWord(r.OK),
		r.Words,
		len(r.Images),
	)

	for _, i := range r.Issues {
		b.WriteString("\n" + i.Path + ": " + i.Code + ": " + i.Message)
	}

	return b.String()
}

func okWord(ok bool) string {
	if ok {
		return "ok"
	}

	return "has issues"
}

// blogPackage builds the package of a post for a destination: the post checked, its images resolved from the store
// with their digests, the Markdown and the HTML with the images written as images/<name>. Every issue is returned at
// once as E_VALIDATION.
func (s *svc) blogPackage(
	ctx context.Context, c Caller, projectID, itemID, visibility string,
) (*destinations.Package, error) {
	body, serr := s.textSource(ctx, c, projectID, itemID, writing.KindPost)
	if serr != nil {
		return nil, serr
	}

	first, _ := writing.ParsePost(body)
	ids := writing.Images(first.Markdown, first.Meta.Cover)

	found, ferr := s.postImages(ctx, projectID, ids)
	if ferr != nil {
		return nil, ferr
	}

	have := make([]string, 0, len(found))
	for id := range found {
		have = append(have, id)
	}

	post, issues := writing.CheckPost(body, have)
	if len(issues) > 0 {
		return nil, postIssuesError(issues)
	}

	names := imageNames(&post, ids, found)

	pkg := &destinations.Package{
		Slug: post.Meta.EffectiveSlug(), Status: destinations.StatusDraft, Meta: post.Meta,
		Images: []destinations.Image{},
	}

	if visibility == string(publish.Public) {
		pkg.Status = destinations.StatusPublished
	}

	var total int64

	for _, id := range ids {
		im := found[id]
		if im.bytes > destinations.MaxImageBytes {
			return nil, validationAt("body.images", "the image %s is over 10 MiB.", id)
		}

		total += im.bytes

		role := "body"
		if id == post.Meta.Cover {
			role = "cover"
		}

		pkg.Images = append(pkg.Images, destinations.Image{
			Name: names[id], Role: role, AssetID: id, Hash: im.hash, ContentType: im.contentType, Bytes: im.bytes,
			SHA256: im.hash,
		})
	}

	if total > destinations.MaxTotalBytes {
		return nil, validationAt("body.images", "the images of a post total at most 50 MiB.")
	}

	src := imageSrc(names)
	to := func(id string) (string, bool) { return src(id) }
	pkg.Markdown = writing.RewriteImages(post.Markdown, to)
	pkg.HTML = writing.RenderHTML(post.Markdown, src)

	if err := pkg.Check(); err != nil {
		return nil, validationAt("post", "%s.", err.Error())
	}

	return pkg, nil
}

// postIssuesError is E_VALIDATION on the path of the first issue, with every issue in the details.
func postIssuesError(issues []writing.PostIssue) *Error {
	e := validationAt(issues[0].Path, "%s", issues[0].Message).withDetail("issues", issues)

	return e.withHint("check the post with post:check and fix every issue")
}

// openApproved opens an image of an approved package from the store and checks it is the file that was approved:
// its size and its SHA-256. The file is read into memory (an image of a post is at most 10 MiB).
func (s *svc) openApproved(im destinations.Image) (io.ReadCloser, error) {
	f, err := enginestore.New(s.workspace).Open(im.Hash)
	if err != nil {
		return nil, newError(command.CodeInvalidState, "an image of the approved post is gone from the workspace.").
			withHint("schedule the post again and approve it")
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, im.Bytes+1))
	sum := sha256.Sum256(data)

	if err != nil || int64(len(data)) != im.Bytes || hex.EncodeToString(sum[:]) != im.SHA256 {
		return nil, newError(
			command.CodeInvalidState,
			"an image of the approved post is not the file that was approved.",
		).
			withHint("schedule the post again and approve it")
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// The read API (SDD-020 section 47). Every operation reads the table of published posts and nothing else.

const (
	publishedPageDefault = 20
	publishedPageMax     = 50
)

func publishedSummary(p *destinations.PublishedPost) PublishedPostSummary {
	cover := ""
	if c := slices.IndexFunc(p.Images, func(i destinations.Image) bool { return i.Role == "cover" }); c >= 0 {
		cover = "images/" + p.Images[c].Name
	}

	return PublishedPostSummary{
		Slug: p.Slug, Title: p.Meta.Title, Summary: p.Meta.Summary, Tags: append([]string{}, p.Meta.Tags...),
		Language: p.Meta.Language, CanonicalURL: p.Meta.Canonical, Cover: cover, PublishedAt: p.PublishedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// etagOf is the strong tag of a value: the SHA-256 of its JSON.
func etagOf(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}

	sum := sha256.Sum256(raw)

	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func (s *svc) postsCaller(ctx context.Context, method string) (Caller, error) {
	return authorize(ctx, method, "")
}

func (s *svc) PublishedPostList(ctx context.Context, req PublishedPostListRequest) (*PublishedPostListResult, error) {
	c, err := s.postsCaller(ctx, "PublishedPostList")
	if err != nil {
		return nil, err
	}

	limit := req.Limit

	switch {
	case limit == 0:
		limit = publishedPageDefault
	case limit < 0 || limit > publishedPageMax:
		return nil, validationAt("limit", "the page size is 1 to %d.", publishedPageMax)
	}

	f := destinations.ListFilter{OwnerUserID: c.OwnerUserID, ProjectID: c.ProjectID, Limit: limit + 1}

	if req.Cursor != "" {
		ms, slug, ok := decodePostCursor(req.Cursor)
		if !ok {
			return nil, validationAt("cursor", "the cursor is not one this server made.")
		}

		f.AfterMs, f.AfterSlug = ms, slug
	}

	posts, err := s.store.Blog().ListPublished(ctx, f)
	if err != nil {
		return nil, asError(err)
	}

	res := &PublishedPostListResult{
		UntrustedNotice: UntrustedNotice,
		Posts:           []PublishedPostSummary{},
	}

	if len(posts) > limit {
		last := posts[limit-1]
		res.NextCursor = encodePostCursor(last.PublishedAt.UnixMilli(), last.Slug)
		posts = posts[:limit]
	}

	for i := range posts {
		res.Posts = append(res.Posts, publishedSummary(&posts[i]))
	}

	res.ETag = etagOf(res)

	return res, nil
}

func encodePostCursor(ms int64, slug string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(ms, 10) + ":" + slug))
}

func decodePostCursor(cursor string) (int64, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", false
	}

	msText, slug, ok := strings.Cut(string(raw), ":")
	ms, perr := strconv.ParseInt(msText, 10, 64)

	return ms, slug, ok && perr == nil && ms > 0 && slug != ""
}

// publishedOf reads a published post the caller may see: the status must be published, the owner the caller's, and
// the project the one of a key limited to a project. Everything else is the same not found.
func (s *svc) publishedOf(ctx context.Context, c Caller, slug string) (*destinations.PublishedPost, error) {
	notFound := newError(command.CodeNotFound, "there is no published post with that slug.")

	p, err := s.store.Blog().Published(ctx, slug)
	if errors.Is(err, destinations.ErrNotFound) {
		return nil, notFound
	}

	if err != nil {
		return nil, asError(err)
	}

	if p.Status != destinations.StatusPublished || (c.OwnerUserID != "" && p.OwnerUserID != c.OwnerUserID) ||
		(c.ProjectID != "" && p.ProjectID != c.ProjectID) {
		return nil, notFound
	}

	return &p, nil
}

func (s *svc) PublishedPostGet(ctx context.Context, req PublishedPostGetRequest) (*PublishedPostResult, error) {
	c, err := s.postsCaller(ctx, "PublishedPostGet")
	if err != nil {
		return nil, err
	}

	p, err := s.publishedOf(ctx, c, req.Slug)
	if err != nil {
		return nil, err
	}

	res := &PublishedPostResult{
		UntrustedNotice: UntrustedNotice, PublishedPostSummary: publishedSummary(p),
		Markdown: p.Markdown, HTML: p.HTML, Images: []PublishedPostImage{},
	}

	for _, im := range p.Images {
		res.Images = append(res.Images, PublishedPostImage{
			Name: im.Name, Role: im.Role, ContentType: im.ContentType, Bytes: im.Bytes, SHA256: im.SHA256,
			URL: destinations.PostPath(p.Slug) + "/media/" + im.Name,
		})
	}

	res.ETag = etagOf(res)

	return res, nil
}

func (s *svc) PublishedPostMedia(
	ctx context.Context,
	req PublishedPostMediaRequest,
) (*PublishedPostMediaResult, error) {
	c, err := s.postsCaller(ctx, "PublishedPostMedia")
	if err != nil {
		return nil, err
	}

	p, err := s.publishedOf(ctx, c, req.Slug)
	if err != nil {
		return nil, err
	}

	i := slices.IndexFunc(p.Images, func(im destinations.Image) bool { return im.Name == req.Name })
	if i < 0 {
		return nil, newError(command.CodeNotFound, "the post has no image with that name.")
	}

	im := p.Images[i]

	abs, perr := enginestore.New(s.workspace).Path(im.Hash)
	if perr != nil {
		return nil, internalError()
	}

	rel, rerr := filepath.Rel(s.workspace, abs)
	if rerr != nil || strings.HasPrefix(rel, "..") {
		return nil, internalError()
	}

	return &PublishedPostMediaResult{
		Path: filepath.ToSlash(rel), ContentType: im.ContentType, Bytes: im.Bytes, SHA256: im.SHA256,
	}, nil
}
