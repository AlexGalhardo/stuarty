package service

import (
	"context"

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
	return &AssetResult{UntrustedNotice: UntrustedNotice, LibraryAsset: *a, Text: libraryDetailText(a)}
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

	lib := assets.Library{Store: s.store.Library()}

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

	res.Text = libraryDetailText(&LibraryAsset{Hash: hash, Uses: res.Uses})

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
		lib := assets.Library{Store: tx.Library()}

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
		lib := assets.Library{Store: tx.Library()}

		var uerr error

		entry, uerr = lib.Tag(ctx, &h, req.Add, req.Remove, c.Actor, s.nowMs(), names)

		return uerr
	})
	if werr != nil {
		return nil, ruleError(werr)
	}

	return s.assetResult(&entry), nil
}
