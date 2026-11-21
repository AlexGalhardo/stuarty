package service

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

// AssetLibraryService is the operations that read and write one entry of the asset library (SDD-011 section
// 14.4); AssetList, which lists them, is in Service beside the other lists.
type AssetLibraryService interface {
	AssetGet(ctx context.Context, req AssetRef) (*AssetResult, error)
	AssetUses(ctx context.Context, req AssetUsesRequest) (*AssetUsesResult, error)
	AssetSetInfo(ctx context.Context, req AssetSetInfoRequest) (*AssetResult, error)
	AssetTag(ctx context.Context, req AssetTagRequest) (*AssetResult, error)
	AssetRename(ctx context.Context, req AssetRenameRequest) (*AssetResult, error)
}

// AssetRef names a file of the library by its content hash.
type AssetRef struct {
	Hash string `json:"hash"`
}

// AssetResult is one entry of the library with the projects that use it.
type AssetResult struct {
	Envelope
	LibraryAsset
	Text string `json:"text"`
}

// AssetUsesRequest asks for the projects that use a file.
type AssetUsesRequest struct {
	Hash string `json:"hash"`
	Page
}

// AssetUsesResult lists the projects that use a file.
type AssetUsesResult struct {
	Envelope
	Hash       string     `json:"hash"`
	Uses       []AssetUse `json:"uses"`
	NextCursor *string    `json:"next_cursor"`
	Text       string     `json:"text"`
}

// AssetSetInfoRequest changes what the user records about a file; a member left out stays as it is.
// RightsNote is the user's own record, never a licence check.
type AssetSetInfoRequest struct {
	Hash        string  `json:"hash"`
	Description *string `json:"description,omitempty"`
	RightsNote  *string `json:"rights_note,omitempty"`
	Reusable    *bool   `json:"reusable,omitempty"`
}

// AssetTagRequest adds and removes tags of a file.
type AssetTagRequest struct {
	Hash   string   `json:"hash"`
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// AssetRenameRequest changes the name a file is shown under (SDD-011 section 14.8); the stored file is never
// renamed.
type AssetRenameRequest struct {
	Hash string `json:"hash"`
	Name string `json:"name"`
}

// heldFile is the file with this hash among those the caller may see. A hash nobody holds and a hash held only
// by another user's project answer the same, so the library cannot be probed (SDD-007 T-20).
func (s *svc) heldFile(ctx context.Context, c Caller, in string) (assets.Held, string, error) {
	hash, err := parseHash(in)
	if err != nil {
		return assets.Held{}, "", err
	}

	held, serr := s.libraryHeld(ctx, c, true)
	if serr != nil {
		return assets.Held{}, "", serr
	}

	for i := range held {
		if held[i].Hash == hash {
			return held[i], hash, nil
		}
	}

	return assets.Held{}, hash, newError(command.CodeNotFound, "no file with the hash %s is in the library.", hash).
		withHint("list the files with asset:list")
}

// useNames maps the productions that use a file to their names.
func (s *svc) useNames(ctx context.Context, h *assets.Held) map[string]string {
	names := map[string]string{}

	for _, u := range assets.UsesOf(h, nil) {
		if p, err := s.store.Productions().Get(ctx, u.ProjectID); err == nil {
			names[u.ProjectID] = p.Name
		}
	}

	return names
}

func (s *svc) assetResult(a *LibraryAsset) *AssetResult {
	return &AssetResult{UntrustedNotice: UntrustedNotice, LibraryAsset: *a, Text: assetDetailText(a)}
}

// AssetGet is the entry of one file, with its origin and the projects that use it (RF-321).
func (s *svc) AssetGet(ctx context.Context, req AssetRef) (*AssetResult, error) {
	c, err := authorize(ctx, "AssetGet", "")
	if err != nil {
		return nil, err
	}

	h, _, ferr := s.heldFile(ctx, c, req.Hash)
	if ferr != nil {
		return nil, ferr
	}

	lib := assets.Library{Store: s.store.AssetLibrary(), Owner: c.OwnerUserID}

	entry, gerr := lib.Get(ctx, &h, s.useNames(ctx, &h))
	if gerr != nil {
		return nil, ruleError(gerr)
	}

	return s.assetResult(&entry), nil
}

// AssetUses lists the projects that use a file.
func (s *svc) AssetUses(ctx context.Context, req AssetUsesRequest) (*AssetUsesResult, error) {
	c, err := authorize(ctx, "AssetUses", "")
	if err != nil {
		return nil, err
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	off := 0

	if req.Cursor != "" {
		var derr *Error
		if off, derr = decodeOffset(req.Cursor, "asset-uses", ""); derr != nil {
			return nil, derr
		}
	}

	h, hash, ferr := s.heldFile(ctx, c, req.Hash)
	if ferr != nil {
		return nil, ferr
	}

	all := assets.UsesOf(&h, s.useNames(ctx, &h))
	limit := clampLimit(req.Limit, maxLibraryAssets, maxLibraryAssets)
	start := min(off, len(all))
	end := min(start+limit, len(all))

	res := &AssetUsesResult{UntrustedNotice: UntrustedNotice, Hash: hash, Uses: all[start:end]}
	if res.Uses == nil {
		res.Uses = []AssetUse{}
	}

	res.Text = assetUsesText(hash, res.Uses)

	if end < len(all) {
		cur := offsetCursor("asset-uses", "", end)
		res.NextCursor = &cur
		res.Text += "\nnext_cursor: " + cur
	}

	return res, nil
}

// AssetSetInfo sets the description, the rights note and the reuse mark of a file.
func (s *svc) AssetSetInfo(ctx context.Context, req AssetSetInfoRequest) (*AssetResult, error) {
	c, err := authorize(ctx, "AssetSetInfo", "")
	if err != nil {
		return nil, err
	}

	h, _, ferr := s.heldFile(ctx, c, req.Hash)
	if ferr != nil {
		return nil, ferr
	}

	names := s.useNames(ctx, &h)

	var entry LibraryAsset

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		lib := assets.Library{Store: tx.AssetLibrary(), Owner: c.OwnerUserID}

		var uerr error

		entry, uerr = lib.SetInfo(
			ctx, &h, assets.InfoPatch{Description: req.Description, RightsNote: req.RightsNote, Reusable: req.Reusable},
			c.Actor, s.nowMs(), names,
		)

		return uerr
	})
	if werr != nil {
		return nil, ruleError(werr)
	}

	return s.assetResult(&entry), nil
}

// AssetTag adds and removes tags of a file.
func (s *svc) AssetTag(ctx context.Context, req AssetTagRequest) (*AssetResult, error) {
	c, err := authorize(ctx, "AssetTag", "")
	if err != nil {
		return nil, err
	}

	h, _, ferr := s.heldFile(ctx, c, req.Hash)
	if ferr != nil {
		return nil, ferr
	}

	names := s.useNames(ctx, &h)

	var entry LibraryAsset

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		lib := assets.Library{Store: tx.AssetLibrary(), Owner: c.OwnerUserID}

		var uerr error

		entry, uerr = lib.Tag(ctx, &h, req.Add, req.Remove, c.Actor, s.nowMs(), names)

		return uerr
	})
	if werr != nil {
		return nil, ruleError(werr)
	}

	return s.assetResult(&entry), nil
}

// AssetRename changes the display name of a file in every project of the caller that holds it (SDD-011 section
// 14.8). The name is the row of project_assets, so no revision of any document is written and the file in the
// store keeps its hash.
func (s *svc) AssetRename(ctx context.Context, req AssetRenameRequest) (*AssetResult, error) {
	c, err := authorize(ctx, "AssetRename", "")
	if err != nil {
		return nil, err
	}

	h, _, ferr := s.heldFile(ctx, c, req.Hash)
	if ferr != nil {
		return nil, ferr
	}

	name, bad := asset.RenameTo(h.Name, req.Name)
	if bad != nil {
		return nil, ruleError(bad)
	}

	names := s.useNames(ctx, &h)

	var entry LibraryAsset

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		for _, hd := range h.Holdings {
			row, gerr := tx.Assets().GetProjectAsset(ctx, hd.TimelineID, hd.AssetID)
			if gerr != nil {
				return gerr
			}

			if row.Name == name {
				continue
			}

			row.Name = name
			row.Snapshot = renamedSnapshot(row.Snapshot, name)

			if perr := tx.Assets().PutProjectAsset(ctx, row); perr != nil {
				return perr
			}
		}

		if rerr := tx.AssetLibrary().Reindex(ctx, c.OwnerUserID, h.Hash); rerr != nil {
			return rerr
		}

		h.Name = name

		lib := assets.Library{Store: tx.AssetLibrary(), Owner: c.OwnerUserID}

		var uerr error

		entry, uerr = lib.Get(ctx, &h, names)

		return uerr
	})
	if werr != nil {
		return nil, ruleError(werr)
	}

	return s.assetResult(&entry), nil
}

// renamedSnapshot is the kept Asset object of a use with its name replaced, so that a restore after a trash
// brings the file back under the name it has now. A snapshot that is absent or not an object stays as it is.
func renamedSnapshot(snap json.RawMessage, name string) json.RawMessage {
	if len(snap) == 0 {
		return snap
	}

	var obj map[string]json.RawMessage
	if json.Unmarshal(snap, &obj) != nil || obj == nil {
		return snap
	}

	raw, err := json.Marshal(name)
	if err != nil {
		return snap
	}

	obj["name"] = raw

	out, err := json.Marshal(obj)
	if err != nil {
		return snap
	}

	return out
}
