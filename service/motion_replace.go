package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/motion"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// maxReplaceClips is the length of a batch of Edit; a key that reaches more clips is refused, not split, because a
// replacement is one batch.
const maxReplaceClips = 200

// RenderOffer says that an earlier render of the same animation (the same render key) is used by clips of the
// timeline: the clips are replaced by MotionReplace, or by clip.replace_media in one edit.
type RenderOffer struct {
	AssetID string   `json:"asset_id"`
	Name    string   `json:"name"     untrusted:"true"`
	ItemIDs []string `json:"item_ids"`
}

// MotionReplaceRequest replaces, in one batch, the media of every clip that reads an earlier render of an animation
// with a new render of it (SDD-011 section 15.4).
type MotionReplaceRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	AssetID    string `json:"asset_id"`              // the new render, an asset of the timeline
	// RenderKey is the key of the animation; default the render key of the origin of the new asset.
	RenderKey string `json:"render_key,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// MotionReplaceResult is the edit of the batch and the clips it replaced.
type MotionReplaceResult struct {
	EditResult
	RenderKey string   `json:"render_key"`
	AssetID   string   `json:"asset_id"`
	Replaced  []string `json:"replaced"` // the item ids
}

// docOrigins reads the library origin of every asset of the document, by asset id, from the rows of the owner (the
// library is kept per owner, SDD-011 DS-22); an asset with no row is imported.
func (s *svc) docOrigins(ctx context.Context, owner string, doc *timeline.Document) (map[string]asset.Origin, error) {
	hashes := make([]string, 0, len(doc.Assets))
	for id := range doc.Assets {
		hashes = append(hashes, doc.Assets[id].Hash)
	}

	rows, err := s.store.AssetLibrary().GetMany(ctx, owner, hashes)
	if err != nil {
		return nil, asError(err)
	}

	out := make(map[string]asset.Origin, len(doc.Assets))
	for id := range doc.Assets {
		out[id] = rows[doc.Assets[id].Hash].Origin
	}

	return out, nil
}

// earlierRenders is the set of assets of the document, other than the new one, that were rendered externally under
// the key.
func earlierRenders(origins map[string]asset.Origin, key, newAsset string) map[string]bool {
	out := map[string]bool{}

	for id := range origins {
		o := origins[id]
		if id != newAsset && o.Kind == asset.RenderedExternally && o.RenderKey == key {
			out[id] = true
		}
	}

	return out
}

// renderOffers lists, for a file just imported under a render key, the earlier renders of the key that clips of
// the timeline use (SDD-011 section 15.4). It offers and changes nothing; an error reading the project is no offer.
func (s *svc) renderOffers(ctx context.Context, owner, projectID, newAsset, key string) []RenderOffer {
	row, err := s.store.Projects().Get(ctx, projectID)
	if err != nil {
		return nil
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil
	}

	origins, oerr := s.docOrigins(ctx, owner, doc)
	if oerr != nil {
		return nil
	}

	earlier := earlierRenders(origins, key, newAsset)

	var offers []RenderOffer

	for id := range earlier {
		items := motion.Targets(doc, map[string]bool{id: true}, newAsset)
		if len(items) > 0 {
			offers = append(offers, RenderOffer{AssetID: id, Name: doc.Assets[id].Name, ItemIDs: items})
		}
	}

	sort.Slice(offers, func(i, j int) bool { return offers[i].AssetID < offers[j].AssetID })

	return offers
}

// offerReplacements adds to an import under a render key the offer of each file: the earlier renders of the key that
// clips use, in the result and in its text (SDD-011 section 15.4).
func (s *svc) offerReplacements(
	ctx context.Context,
	c Caller,
	projectID, key string,
	dryRun bool,
	res *MediaImportResult,
) {
	if key == "" || dryRun {
		return
	}

	for i := range res.Assets {
		a := &res.Assets[i]
		if a.Error != nil || a.AssetID == "" {
			continue
		}

		a.Replaces = s.renderOffers(ctx, c.OwnerUserID, projectID, a.AssetID, key)
		for j := range a.Replaces {
			res.Text += "\n" + strings.TrimRight(offerText(a.AssetID, &a.Replaces[j]), "\n")
		}
	}
}

// offerText is the line of the text of an import that names the offer.
func offerText(newAsset string, o *RenderOffer) string {
	name := untrusted.Inline(untrusted.SourceAssetName, untrusted.Attrs{"asset": o.AssetID}, o.Name, 120)

	return fmt.Sprintf("replaces: %s (%s) is an earlier render of this animation, used by %s; "+
		"replace them with motion:replace --asset %s, or clip.replace_media in one edit\n",
		o.AssetID, name, strings.Join(o.ItemIDs, ", "), newAsset)
}

// MotionReplace replaces the media of every clip that reads an earlier render of the animation by the new render,
// in one batch: one revision, one history entry, all or nothing, undone by one undo (SDD-011 section 15.4).
func (s *svc) MotionReplace(ctx context.Context, req MotionReplaceRequest) (*MotionReplaceResult, error) {
	c, err := s.authorizeProject(ctx, "MotionReplace", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	if !timeline.ValidID(timeline.PrefixAsset, req.AssetID) {
		return nil, newError(command.CodeValidation, "the asset id is not valid.").withPath("asset_id").
			withHint("an asset id is ast_ and 10 characters from 0-9 and a-z, without i, l, o and u")
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	if _, ok := doc.Assets[req.AssetID]; !ok {
		return nil, s.motionAssetMissing(ctx, row.ID, req.AssetID)
	}

	origins, oerr := s.docOrigins(ctx, c.OwnerUserID, doc)
	if oerr != nil {
		return nil, oerr
	}

	key := strings.TrimSpace(req.RenderKey)
	if key == "" {
		key = origins[req.AssetID].RenderKey
	}

	if key == "" {
		return nil, newError(command.CodeValidation, "the asset %s has no render key and none was given.", req.AssetID).
			withPath("render_key").withHint("import the file with origin render_key, or pass render_key")
	}

	targets := motion.Targets(doc, earlierRenders(origins, key, req.AssetID), req.AssetID)
	if len(targets) == 0 {
		return nil, newError(
			command.CodeNotFound,
			"no clip of the timeline reads an earlier render with the key %q.",
			key,
		).
			withPath("render_key").
			withHint("place the new asset with clip.add; there is nothing to replace")
	}

	if len(targets) > maxReplaceClips {
		return nil, newError(
			"E_LIMIT_EXCEEDED",
			"the key reaches %d clips; a batch has at most %d.",
			len(targets),
			maxReplaceClips,
		).
			withLimit("clips", len(targets), maxReplaceClips).
			withHint("replace some of them with clip.replace_media in edit calls")
	}

	ops := make([]command.Operation, len(targets))
	for i, id := range targets {
		ops[i] = command.Operation{
			Op:   "clip.replace_media",
			Args: json.RawMessage(fmt.Sprintf(`{"item_id":%q,"asset_id":%q}`, id, req.AssetID)),
		}
	}

	res, eerr := s.Edit(ctx, EditRequest{
		ProjectID: row.ID, BaseRevision: row.Revision, Operations: ops, DryRun: req.DryRun,
	})
	if eerr != nil {
		return nil, eerr
	}

	slices.Sort(targets)

	return &MotionReplaceResult{EditResult: *res, RenderKey: key, AssetID: req.AssetID, Replaced: targets}, nil
}
