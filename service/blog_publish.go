package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AlexGalhardo/stuarty/core/command"
	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service/connections"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// snapshotPost marks the snapshot of a target that carries a blog post (SDD-020 section 43).
const snapshotPost = "post"

// blogSnapshot is the post as the person was shown it: the package and the connection it goes to. The digest of the
// snapshot is what the approval holds, and the send checks the image files against the digests in it.
type blogSnapshot struct {
	Kind         string               `json:"kind"`
	Network      string               `json:"network"`
	ConnectionID string               `json:"connection_id"`
	ProjectID    string               `json:"project_id"`
	PostItem     string               `json:"post_item"`
	Fields       pub.Fields           `json:"fields"`
	Package      destinations.Package `json:"package"`
}

// blogRegistry is the registry of the destinations, or an error when the build has none.
func (s *svc) blogDestination(kind string) (destinations.Destination, bool) { return s.blog.Get(kind) }

// resolveBlogTarget routes a target to the blog path when its connection is a blog destination, or when the
// publication carries a post: handled is false for a video going to a social network, which keeps its own path.
func (s *svc) resolveBlogTarget(ctx context.Context, c Caller, p *pub.Post) (*pub.Resolved, bool, error) {
	if p.ConnectionID != "" {
		if row, err := s.conns.Get(ctx, p.ConnectionID); err == nil && destinations.IsKind(row.Kind) {
			out, rerr := s.resolveBlog(ctx, c, &row, p)

			return out, true, rerr
		}
	}

	if p.PostItem != "" {
		return nil, true, validationAt(
			"connection_id",
			"a post goes only to a blog destination: files, WordPress, a webhook or the read API.",
		).
			withHint("create one in the Connections page")
	}

	return nil, false, nil
}

// resolveBlog is Resolve for a target whose connection is a blog destination (SDD-020 section 43): no request is made.
func (s *svc) resolveBlog(
	ctx context.Context,
	c Caller,
	row *connections.Connection,
	p *pub.Post,
) (*pub.Resolved, error) {
	dest, ok := s.blogDestination(row.Kind)
	if !ok {
		return nil, validationAt("connection_id", "the connection is not one of a blog destination.")
	}

	if p.PostItem == "" {
		return nil, validationAt("post", "a blog destination takes a post, not a video.").
			withHint("give the publication the id of a text of the kind post")
	}

	if v := p.Fields.Visibility; v != "" && v != string(corepub.Private) && v != string(corepub.Public) {
		return nil, validationAt("fields.visibility", "a post is private (a draft) or public.")
	}

	desc := dest.Describe()
	out := &pub.Resolved{Network: desc.Kind, NetworkName: desc.Name, Account: row.Label}

	if !p.Strict {
		if _, err := s.textSource(ctx, c, p.ProjectID, p.PostItem, writing.KindPost); err != nil {
			return nil, err
		}

		return out, nil
	}

	pkg, err := s.blogPackage(ctx, c, p.ProjectID, p.PostItem, p.Fields.Visibility)
	if err != nil {
		return nil, err
	}

	snap := blogSnapshot{
		Kind: snapshotPost, Network: desc.Kind, ConnectionID: row.ID, ProjectID: p.ProjectID, PostItem: p.PostItem,
		Fields: p.Fields, Package: *pkg,
	}

	if out.Snapshot, err = json.Marshal(snap); err != nil {
		return nil, internalError()
	}

	out.FileName = pkg.Slug + "/index.md"
	out.FileBytes = int64(len(pkg.Markdown)) + pkg.TotalBytes()

	return out, nil
}

// blogSnapshotOf reads the snapshot of a target and says whether it is a post.
func blogSnapshotOf(raw json.RawMessage) (*blogSnapshot, bool) {
	var probe struct {
		Kind string `json:"kind"`
	}

	if json.Unmarshal(raw, &probe) != nil || probe.Kind != snapshotPost {
		return nil, false
	}

	var snap blogSnapshot
	if json.Unmarshal(raw, &snap) != nil {
		return nil, false
	}

	return &snap, true
}

// sendBlog sends the approved package of a target to its destination (SDD-020 sections 43 to 47).
func (s *svc) sendBlog(
	ctx context.Context, t *pub.Target, snap *blogSnapshot, report func(float64, string),
) (pub.Sent, error) {
	if snap.ConnectionID != t.ConnectionID {
		return pub.Sent{}, newError(command.CodeInvalidState, "the approved post names another connection.").
			withHint("schedule the post again and approve it")
	}

	row, err := s.conns.Get(ctx, t.ConnectionID)
	if err != nil {
		return pub.Sent{}, connectionError(err)
	}

	dest, ok := s.blogDestination(row.Kind)
	if !ok || row.Kind != snap.Network {
		return pub.Sent{}, newError(
			command.CodeInvalidState,
			"the connection is no longer the destination that was approved.",
		).
			withHint("schedule the post again and approve it")
	}

	pkg := snap.Package
	if cerr := pkg.Check(); cerr != nil {
		return pub.Sent{}, newError(command.CodeInvalidState, "the approved post is not valid: %s.", cerr.Error()).
			withHint("schedule the post again and approve it")
	}

	// Every image is checked against what the person approved before anything is sent, so a change of a file is one
	// clear refusal and not a failure halfway through a write.
	for _, im := range pkg.Images {
		f, oerr := s.openApproved(im)
		if oerr != nil {
			return pub.Sent{}, oerr
		}

		_ = f.Close()
	}

	in := &destinations.SendInput{
		Package: pkg, Open: s.openApproved, Delivery: t.ID, Report: report,
		Publish: func(ctx context.Context, p destinations.Package) error { return s.publishToAPI(ctx, &row, snap, &p) },
	}

	if r, rerr := s.store.Blog().Remote(ctx, row.ID, snap.PostItem); rerr == nil {
		in.Prev = &r
	} else if !errors.Is(rerr, destinations.ErrNotFound) {
		return pub.Sent{}, asError(rerr)
	}

	var res destinations.Result

	run := func(cred connections.Credential) error {
		in.Cred = cred

		var serr error

		res, serr = dest.Send(ctx, in)

		return serr
	}

	if row.Kind == destinations.KindWebhook {
		err = s.conns.WithPublic(ctx, row.ID, run)
	} else {
		err = s.conns.With(ctx, row.ID, run)
	}

	if err != nil {
		return pub.Sent{}, s.blogError(err, dest)
	}

	remote := destinations.Remote{
		ConnectionID: row.ID, PostItemID: snap.PostItem, RemoteID: res.RemoteID, URL: res.URL, UpdatedAt: s.now(),
	}

	if werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return tx.Blog().PutRemote(ctx, remote)
	}); werr != nil {
		return pub.Sent{}, asError(werr)
	}

	return pub.Sent{RemoteID: res.RemoteID, URL: res.URL, RemoteState: res.State}, nil
}

// blogError maps what a destination returned: a failure of the destination becomes E_PUBLISH with the hint the
// destination wrote, and the refusal of the gate and the failures of the vault keep their own codes.
func (s *svc) blogError(err error, dest destinations.Destination) error {
	if se, ok := social.AsError(err); ok {
		return publishError(se, dest.Describe().Hints)
	}

	return connectionError(err)
}

// publishToAPI stores an approved package for the read API. It is an upsert, so a send that runs again after an
// interruption replaces the same row.
func (s *svc) publishToAPI(
	ctx context.Context,
	row *connections.Connection,
	snap *blogSnapshot,
	p *destinations.Package,
) error {
	owner := ""

	if head, err := s.store.Productions().Get(ctx, snap.ProjectID); err == nil && head.OwnerUserID != nil {
		owner = *head.OwnerUserID
	}

	now := s.now()

	return s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		err := tx.Blog().PutPublished(ctx, destinations.PublishedPost{
			Slug:         p.Slug,
			PostItemID:   snap.PostItem,
			ProjectID:    snap.ProjectID,
			ConnectionID: row.ID,
			OwnerUserID:  owner,
			Meta:         p.Meta,
			Status:       p.Status,
			Markdown:     p.Markdown,
			HTML:         p.HTML,
			Images:       p.Images,
			PublishedAt:  now,
			UpdatedAt:    now,
		})

		return err
	})
}
