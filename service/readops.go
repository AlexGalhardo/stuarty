package service

import (
	"context"
	"os"
	"sort"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// SnapPointsRequest asks for the snap points of a project (SDD-012 section 7.8).
type SnapPointsRequest struct {
	ProjectID string `json:"project_id"`
	// Revision, when set, is the revision the caller holds; a project that has moved on answers
	// E_REVISION_CONFLICT with the current revision, so a cached answer is never mixed with a new one.
	Revision int64 `json:"revision,omitempty"`
	// Words adds the start and end of every caption word.
	Words bool `json:"words,omitempty"`
}

// SnapPointsResult is the snap structure of SDD-012 section 7.8.
type SnapPointsResult struct {
	Revision int64 `json:"revision"`
	plan.Snap
}

// FontFamily is one bundled family and the weights shipped for it.
type FontFamily struct {
	Family  string `json:"family"`
	Weights []int  `json:"weights"`
}

// FontListRequest has no members.
type FontListRequest struct{}

// ImportedFont is a font file of the font store, named by the hash that font.add and the brand kit take.
type ImportedFont struct {
	Hash  string `json:"hash"`
	Bytes int64  `json:"bytes"`
}

// FontListResult is the bundled set (SDD-001 section 3.9), by family name, and the fonts imported into the
// font store with their hash (VV-094). In network mode only the fonts the caller owns are listed.
type FontListResult struct {
	Fonts    []FontFamily   `json:"fonts"`
	Imported []ImportedFont `json:"imported"`
}

// maxSnapEntries bounds the cache of snap structures; it is cleared when full.
const maxSnapEntries = 64

// snapCache keeps the snap structure of a revision, so a gesture on an
// unchanged project does not compute it twice. A revision never changes
// meaning, so an entry is never stale.
type snapCache struct {
	mu       sync.Mutex
	m        map[snapKey]plan.Snap
	computes int64
}

type snapKey struct {
	project  string
	revision int64
	words    bool
}

func (c *snapCache) get(k snapKey, compute func() (plan.Snap, error)) (plan.Snap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := c.m[k]; ok {
		return v, nil
	}

	if c.m == nil || len(c.m) >= maxSnapEntries {
		c.m = make(map[snapKey]plan.Snap, maxSnapEntries)
	}

	v, err := compute()
	if err != nil {
		return plan.Snap{}, err
	}

	c.computes++
	c.m[k] = v

	return v, nil
}

func (s *svc) SnapPoints(ctx context.Context, req SnapPointsRequest) (*SnapPointsResult, error) {
	c, err := authorize(ctx, "SnapPoints", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.Revision < 0 {
		return nil, newError(command.CodeValidation, "the revision %d is not valid.", req.Revision).
			withPath("revision").
			withHint("send a revision of 1 or more, or none")
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if req.Revision != 0 && req.Revision != row.Revision {
		return nil, newError(
			command.CodeRevisionConflict,
			"the project is at revision %d, not %d.", row.Revision, req.Revision,
		).
			withPath("revision").
			withHint("ask again without revision, or with the current one").
			withDetail("current_revision", row.Revision)
	}

	snap, derr := s.snaps.get(snapKey{row.ID, row.Revision, req.Words}, func() (plan.Snap, error) {
		doc, err := decodeRow(&row)
		if err != nil {
			return plan.Snap{}, err
		}

		return plan.SnapPoints(doc, req.Words), nil
	})
	if derr != nil {
		return nil, derr
	}

	return &SnapPointsResult{Revision: row.Revision, Snap: snap}, nil
}

func (s *svc) FontList(ctx context.Context, _ FontListRequest) (*FontListResult, error) {
	c, err := authorize(ctx, "FontList", "")
	if err != nil {
		return nil, err
	}

	res := &FontListResult{
		Fonts:    make([]FontFamily, 0, len(timeline.BundledWeights)),
		Imported: s.importedFonts(ctx, c),
	}

	for family, weights := range timeline.BundledWeights {
		res.Fonts = append(res.Fonts, FontFamily{Family: family, Weights: append([]int(nil), weights...)})
	}

	sort.Slice(res.Fonts, func(i, j int) bool { return res.Fonts[i].Family < res.Fonts[j].Family })

	return res, nil
}

// importedFonts lists the stored font files the caller may name: every file in local mode, and in network
// mode the files the caller imported or brought, never another user's.
func (s *svc) importedFonts(ctx context.Context, c Caller) []ImportedFont {
	out := []ImportedFont{}

	if s.fonts == nil {
		return out
	}

	var owned map[string]bool

	if c.OwnerUserID != "" {
		owned = map[string]bool{}

		hashes, _ := s.store.Fonts().Owned(ctx, c.OwnerUserID)
		for _, h := range hashes {
			owned[h] = true
		}
	}

	entries, err := os.ReadDir(s.fonts.Dir())
	if err != nil {
		return out
	}

	for _, e := range entries {
		hash := "sha256:" + e.Name()
		if !e.Type().IsRegular() || !isFontHash(hash) || owned != nil && !owned[hash] {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		out = append(out, ImportedFont{Hash: hash, Bytes: info.Size()})
	}

	return out
}
