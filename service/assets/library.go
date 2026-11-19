package assets

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/AlexGalhardo/stuarty/core/asset"
)

// composed puts the tags a caller typed in NFC before core/asset normalizes them: core imports only the standard
// library, so the same tag typed with a decomposed accent is composed here, for every surface.
func composed(tags []string) []string {
	if tags == nil {
		return nil
	}

	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = norm.NFC.String(t)
	}

	return out
}

// Use is a project that uses a file, with its name.
type Use struct {
	ProjectID string   `json:"project_id"`
	Name      string   `json:"name"       untrusted:"true"`
	Timelines []string `json:"timelines"`
	AssetID   string   `json:"asset_id"`
}

// Poster says where a picture of the file is served from: the timeline that holds it and its identifier there
// (the thumbnail route of SDD-011 section 6.5 is addressed by them).
type Poster struct {
	TimelineID string `json:"timeline_id"`
	AssetID    string `json:"asset_id"`
}

// LibraryAsset is one file of the library (SDD-011 section 14.1).
type LibraryAsset struct {
	Hash        string       `json:"hash"`
	Name        string       `json:"name"                  untrusted:"true"` // the name it was first imported under
	Kind        string       `json:"kind"`
	Bytes       int64        `json:"bytes"`
	Width       int          `json:"width,omitempty"`
	Height      int          `json:"height,omitempty"`
	DurationMS  int64        `json:"duration_ms,omitempty"`
	Origin      asset.Origin `json:"origin"`
	Description string       `json:"description,omitempty" untrusted:"true"`
	Reusable    bool         `json:"reusable"`
	Tags        []string     `json:"tags"`
	CreatedBy   string       `json:"created_by,omitempty"`
	CreatedAt   string       `json:"created_at,omitempty"`
	Projects    int          `json:"projects"` // how many projects use it
	Poster      *Poster      `json:"poster,omitempty"`
	Uses        []Use        `json:"uses,omitempty"`
}

// Held is a file of the workspace the caller may see: the facts of the asset row and the rows of the
// timelines that use it (state active), their production already resolved.
type Held struct {
	Hash       string
	Name       string
	MediaKind  string
	Bytes      int64
	Width      int
	Height     int
	DurationMS int64
	CreatedMs  int64
	Holdings   []asset.Holding
}

// Library is the operations over the held files of one owner: the user in network mode, the empty string in
// local mode.
type Library struct {
	Store Store
	Owner string
}

// InfoPatch is what AssetSetInfo changes; a nil member is left alone.
type InfoPatch struct {
	Description *string
	RightsNote  *string
	Reusable    *bool
}

// Query selects a page of the library.
type Query struct {
	Filter asset.Filter
	Text   string
	Offset int
	Limit  int
}

// Page is a page of the library: Next is the offset of the following page, or -1.
type Page struct {
	Assets []LibraryAsset
	Total  int
	Next   int
}

func stamp(ms int64) string {
	if ms <= 0 {
		return ""
	}

	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// absent is the row of a file with no library row: an imported file, not reusable, with no tags.
func absent(owner string, h *Held) Row {
	return Row{Owner: owner, Hash: h.Hash, Origin: asset.Origin{Kind: asset.Imported}, CreatedMs: h.CreatedMs}
}

func base(h *Held, row Row) LibraryAsset {
	if row.Origin.Kind == "" {
		row.Origin.Kind = asset.Imported
	}

	tags := row.Tags
	if tags == nil {
		tags = []string{}
	}

	var poster *Poster

	for _, hd := range h.Holdings {
		if poster == nil || hd.TimelineID < poster.TimelineID {
			poster = &Poster{TimelineID: hd.TimelineID, AssetID: hd.AssetID}
		}
	}

	return LibraryAsset{
		Poster: poster,
		Hash:   h.Hash, Name: h.Name, Kind: h.MediaKind, Bytes: h.Bytes, Width: h.Width, Height: h.Height,
		DurationMS: h.DurationMS, Origin: row.Origin, Description: row.Description, Reusable: row.Reusable,
		Tags: tags, CreatedBy: row.CreatedBy, CreatedAt: stamp(row.CreatedMs),
		Projects: len(asset.Uses(h.Holdings)),
	}
}

func entryOf(h *Held, row Row) asset.Entry {
	e := asset.Entry{MediaKind: h.MediaKind, Origin: row.Origin, Reusable: row.Reusable, Tags: row.Tags}

	if e.Origin.Kind == "" {
		e.Origin.Kind = asset.Imported
	}

	for _, u := range asset.Uses(h.Holdings) {
		e.Projects = append(e.Projects, u.ProjectID)
		e.Projects = append(e.Projects, u.Timelines...)
	}

	return e
}

// List filters and searches the held files and returns one page, ordered by name and then hash. A bad filter
// is an *asset.RuleError.
func (l *Library) List(ctx context.Context, held []Held, q *Query) (Page, error) {
	f := q.Filter

	f.Tag = norm.NFC.String(f.Tag)
	if bad := f.Validate(); bad != nil {
		return Page{}, bad
	}

	var matched map[string]bool

	if strings.TrimSpace(q.Text) != "" {
		match, ok := asset.SearchQuery(q.Text)
		if !ok {
			return Page{Next: -1}, nil
		}

		var err error

		if matched, err = l.Store.Search(ctx, l.Owner, match); err != nil {
			return Page{}, err
		}
	}

	candidates := make([]*Held, 0, len(held))

	for i := range held {
		if matched != nil && !matched[held[i].Hash] {
			continue
		}

		if f.Kind != "" && held[i].MediaKind != f.Kind {
			continue
		}

		candidates = append(candidates, &held[i])
	}

	hashes := make([]string, len(candidates))
	for i, h := range candidates {
		hashes[i] = h.Hash
	}

	rows, err := l.Store.GetMany(ctx, l.Owner, hashes)
	if err != nil {
		return Page{}, err
	}

	rowOf := func(h *Held) Row {
		if row, ok := rows[h.Hash]; ok {
			return row
		}

		return absent(l.Owner, h)
	}

	kept := candidates[:0]

	for _, h := range candidates {
		if e := entryOf(h, rowOf(h)); f.Match(&e) {
			kept = append(kept, h)
		}
	}

	slices.SortFunc(kept, func(a, b *Held) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.Hash, b.Hash))
	})

	page := Page{Total: len(kept), Next: -1}
	start := min(max(q.Offset, 0), len(kept))
	end := len(kept)

	if q.Limit > 0 && start+q.Limit < end {
		end = start + q.Limit
		page.Next = end
	}

	for _, h := range kept[start:end] {
		page.Assets = append(page.Assets, base(h, rowOf(h)))
	}

	return page, nil
}

// Get is the entry of one held file, with its uses. names maps a project identifier to its name.
func (l *Library) Get(ctx context.Context, h *Held, names map[string]string) (LibraryAsset, error) {
	row, err := l.row(ctx, h)
	if err != nil {
		return LibraryAsset{}, err
	}

	out := base(h, row)
	out.Uses = UsesOf(h, names)

	return out, nil
}

// UsesOf is the uses of a held file, each named from names (project identifier to name).
func UsesOf(h *Held, names map[string]string) []Use {
	var out []Use

	for _, u := range asset.Uses(h.Holdings) {
		out = append(
			out,
			Use{ProjectID: u.ProjectID, Name: names[u.ProjectID], Timelines: u.Timelines, AssetID: u.AssetID},
		)
	}

	return out
}

func (l *Library) row(ctx context.Context, h *Held) (Row, error) {
	row, err := l.Store.Get(ctx, l.Owner, h.Hash)

	switch {
	case err == nil:
		return row, nil
	case errors.Is(err, ErrNotFound):
		return absent(l.Owner, h), nil
	}

	return Row{}, err
}

// SetInfo changes the members of the entry that belong to the user: the description, the rights note and the
// reuse mark. The rest of the origin is the record of the first import and stays. A refusal is an
// *asset.RuleError.
func (l *Library) SetInfo(
	ctx context.Context,
	h *Held,
	p InfoPatch,
	actor string,
	now int64,
	names map[string]string,
) (LibraryAsset, error) {
	row, err := l.row(ctx, h)
	if err != nil {
		return LibraryAsset{}, err
	}

	if row.CreatedBy == "" {
		row.CreatedBy = actor
	}

	if p.Description != nil {
		d := strings.TrimSpace(*p.Description)
		if bad := asset.CheckDescription(d); bad != nil {
			return LibraryAsset{}, bad
		}

		row.Description = d
	}

	if p.RightsNote != nil {
		o := row.Origin
		o.RightsNote = strings.TrimSpace(*p.RightsNote)

		if bad := o.Validate(); bad != nil {
			return LibraryAsset{}, bad
		}

		row.Origin = o
	}

	if p.Reusable != nil {
		row.Reusable = *p.Reusable
	}

	row.UpdatedMs = now

	if err := l.Store.Put(ctx, row); err != nil {
		return LibraryAsset{}, err
	}

	out := base(h, row)
	out.Uses = UsesOf(h, names)

	return out, nil
}

// Tag adds and removes tags. A refusal is an *asset.RuleError.
func (l *Library) Tag(
	ctx context.Context,
	h *Held,
	add, remove []string,
	actor string,
	now int64,
	names map[string]string,
) (LibraryAsset, error) {
	row, err := l.row(ctx, h)
	if err != nil {
		return LibraryAsset{}, err
	}

	if row.CreatedBy == "" {
		row.CreatedBy = actor
	}

	tags, bad := asset.ApplyTags(row.Tags, composed(add), composed(remove))
	if bad != nil {
		return LibraryAsset{}, bad
	}

	row.Tags, row.UpdatedMs = tags, now

	if err := l.Store.Put(ctx, row); err != nil {
		return LibraryAsset{}, err
	}

	out := base(h, row)
	out.Uses = UsesOf(h, names)

	return out, nil
}

// ImportInfo is what an import may say about the files it brings: tags, the reuse mark and a description.
type ImportInfo struct {
	Tags        []string
	Reusable    *bool
	Description string
}

// CheckImport validates the blocks of an import before any file is read, and returns them normalized. A refusal
// is an *asset.RuleError.
func CheckImport(origin *asset.Origin, info *ImportInfo) (asset.Origin, ImportInfo, error) {
	o := asset.Origin{Kind: asset.Imported}

	if origin != nil {
		o = origin.Normalize()
		if bad := o.Validate(); bad != nil {
			return asset.Origin{}, ImportInfo{}, bad
		}
	}

	var in ImportInfo

	if info != nil {
		tags, bad := asset.ApplyTags(nil, composed(info.Tags), nil)
		if bad != nil {
			return asset.Origin{}, ImportInfo{}, bad
		}

		d := strings.TrimSpace(info.Description)
		if bad := asset.CheckDescription(d); bad != nil {
			return asset.Origin{}, ImportInfo{}, bad
		}

		in = ImportInfo{Tags: tags, Reusable: info.Reusable, Description: d}
	}

	return o, in, nil
}

// RecordImport writes the library side, for the owner, of a file that an import linked to a project, inside the import's
// transaction (st is bound to it). A file the library has no row for gets one with the origin of the import,
// the record of its first import; a file it already knows keeps its origin. The tags of the import are added to
// the tags of the file, and the reuse mark and the description, when given, are set. The search row is
// refreshed either way, so the name the new project gives the file is found.
func RecordImport(
	ctx context.Context,
	st Store,
	owner, hash string,
	origin asset.Origin,
	info ImportInfo,
	actor string,
	now int64,
) error {
	row, err := st.Get(ctx, owner, hash)

	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		row = Row{Owner: owner, Hash: hash, Origin: origin, CreatedBy: actor, CreatedMs: now}
	default:
		return err
	}

	if len(info.Tags) > 0 {
		tags, bad := asset.ApplyTags(row.Tags, composed(info.Tags), nil)
		if bad != nil {
			return bad
		}

		row.Tags = tags
	}

	if info.Reusable != nil {
		row.Reusable = *info.Reusable
	}

	if info.Description != "" {
		row.Description = info.Description
	}

	row.UpdatedMs = now

	return st.Put(ctx, row)
}
