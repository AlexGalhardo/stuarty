package httpapi

import (
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The library routes of SDD-011 section 14.5. Each handler binds, calls one Service method and returns.

type assetRefIn struct {
	Hash string `json:"hash" validate:"required" doc:"sha256: and 64 hex digits, as the library lists it."`
}

type assetUsesIn struct {
	assetRefIn
	pageIn
}

type assetSetIn struct {
	assetRefIn
	Description *string `json:"description,omitempty" doc:"What the file is, in your words; at most 2,000 characters."`
	RightsNote  *string `json:"rights_note,omitempty" doc:"Your own record of the licence or the permission; the product does not check it."`
	Reusable    *bool   `json:"reusable,omitempty"    doc:"Mark the file to be offered again, or take the mark off."`
	SoundKind   *string `json:"sound_kind,omitempty"  doc:"music, voice or effect, for an audio file; an empty value clears it."`
}

type assetTagIn struct {
	assetRefIn
	Add    []string `json:"add,omitempty"    doc:"Tags to add; at most 20 tags on a file."`
	Remove []string `json:"remove,omitempty" doc:"Tags to take off."`
}

type assetRenameIn struct {
	assetRefIn
	Name string `json:"name" validate:"required" doc:"The new name, 1 to 120 characters, with no path separator; the extension of the file is kept."`
}

// parseReusable reads the query reusable: absent keeps every file, true and false filter.
func parseReusable(raw string) (*bool, error) {
	switch raw {
	case "":
		return nil, nil
	case "true":
		return new(true), nil
	case "false":
		return new(false), nil
	}

	return nil, &service.Error{
		Code: command.CodeValidation, Path: "reusable", Message: "reusable must be true or false.",
		Hint: "use reusable=true or reusable=false, or leave it out",
	}
}

func (h *apiHandler) assetGet(c *web.Ctx, in assetRefIn) (service.AssetResult, error) {
	res, err := h.Service.AssetGet(c.Context(), service.AssetRef{Hash: in.Hash})

	return deref(res), err
}

func (h *apiHandler) assetUses(c *web.Ctx, in assetUsesIn) (service.AssetUsesResult, error) {
	res, err := h.Service.AssetUses(c.Context(), service.AssetUsesRequest{
		Hash: in.Hash, Cursor: in.Cursor, Limit: in.Limit,
	})

	return deref(res), err
}

func (h *apiHandler) assetSet(c *web.Ctx, in assetSetIn) (service.AssetResult, error) {
	res, err := h.Service.AssetSetInfo(c.Context(), service.AssetSetInfoRequest{
		Hash: in.Hash, Description: in.Description, RightsNote: in.RightsNote, Reusable: in.Reusable,
		SoundKind: in.SoundKind,
	})

	return deref(res), err
}

func (h *apiHandler) assetRename(c *web.Ctx, in assetRenameIn) (service.AssetResult, error) {
	res, err := h.Service.AssetRename(c.Context(), service.AssetRenameRequest{Hash: in.Hash, Name: in.Name})

	return deref(res), err
}

// assetReveal is POST /api/v1/assets/{hash}/reveal (SDD-011 section 14.10): the path of a stored file for a person
// in a privileged browser session of a local server. In network mode the route does not exist: it answers the 404
// of a missing route to every caller before the service is asked, so no server ever names a path on its disk. The
// answer holds a path, so no cache keeps it.
func (h *apiHandler) assetReveal(c *web.Ctx, in assetRefIn) (service.AssetRevealResult, error) {
	if h.Network != nil {
		return service.AssetRevealResult{}, &service.Error{
			Code:    command.CodeNotFound,
			Message: "this route exists in local mode only.",
			Details: map[string]any{"reason": "network_mode"},
		}
	}

	c.SetHeader("Cache-Control", "no-store")

	res, err := h.Service.AssetReveal(c.Context(), service.AssetRevealRequest{Hash: in.Hash})

	return deref(res), err
}

func (h *apiHandler) assetTag(c *web.Ctx, in assetTagIn) (service.AssetResult, error) {
	res, err := h.Service.AssetTag(c.Context(), service.AssetTagRequest{Hash: in.Hash, Add: in.Add, Remove: in.Remove})

	return deref(res), err
}
