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
}

type assetTagIn struct {
	assetRefIn
	Add    []string `json:"add,omitempty"    doc:"Tags to add; at most 20 tags on a file."`
	Remove []string `json:"remove,omitempty" doc:"Tags to take off."`
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
	})

	return deref(res), err
}

func (h *apiHandler) assetTag(c *web.Ctx, in assetTagIn) (service.AssetResult, error) {
	res, err := h.Service.AssetTag(c.Context(), service.AssetTagRequest{Hash: in.Hash, Add: in.Add, Remove: in.Remove})

	return deref(res), err
}
