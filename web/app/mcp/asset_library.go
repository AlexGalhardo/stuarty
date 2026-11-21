package mcp

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The asset library through the media tools (SDD-011 section 14.5): the tool list has no room for another tool
// (NFR-020), so media_inspect without a project lists the library and with a hash reads one entry, and
// media_import with a hash and no paths edits one.

// strict decodes an object member and refuses a key it does not know.
func strict(raw json.RawMessage, field string, into any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(into); err != nil {
		return &service.Error{
			Code:    command.CodeValidation,
			Path:    field,
			Message: field + " is not a valid object: " + command.JSONProblem(err) + ".",
		}
	}

	return nil
}

// defaultLibraryLimit keeps the answer of a list inside the budget of a tool result.
const defaultLibraryLimit = 50

func libraryInspect(
	ctx context.Context,
	svc service.Service,
	hash, projectID string,
	rawLib json.RawMessage,
	page service.Page,
) (*toolOutput, error) {
	if hash != "" {
		res, err := svc.AssetGet(ctx, service.AssetRef{Hash: hash})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	var lib struct {
		Tag      string `json:"tag"`
		Reusable *bool  `json:"reusable"`
		Origin   string `json:"origin"`
		Text     string `json:"text"`
		Kind     string `json:"kind"`
		Section  string `json:"section"`
		Project  string `json:"project"`
	}

	if err := strict(rawLib, "library", &lib); err != nil {
		return nil, err
	}

	if lib.Project == "" {
		lib.Project = projectID
	}

	if page.Limit == 0 {
		page.Limit = defaultLibraryLimit
	}

	res, err := svc.AssetList(ctx, service.AssetListRequest{
		Kind: lib.Kind, Section: lib.Section, Tag: lib.Tag, Project: lib.Project, Origin: lib.Origin,
		Reusable: lib.Reusable, Text: lib.Text, Limit: page.Limit, Cursor: page.Cursor,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// importBlocks reads the origin and the info of an import.
func importBlocks(rawOrigin, rawInfo json.RawMessage) (*asset.Origin, *service.AssetInfo, error) {
	var (
		origin *asset.Origin
		info   *service.AssetInfo
	)

	if len(rawOrigin) > 0 && string(rawOrigin) != "null" {
		origin = new(asset.Origin)
		if err := strict(rawOrigin, "origin", origin); err != nil {
			return nil, nil, err
		}
	}

	if len(rawInfo) > 0 && string(rawInfo) != "null" {
		info = new(service.AssetInfo)
		if err := strict(rawInfo, "info", info); err != nil {
			return nil, nil, err
		}
	}

	return origin, info, nil
}

// libraryEdit is media_import with a hash and no paths: the description, the rights note and the reuse mark are
// set, then the tags are added and removed, then the file is renamed when info.name is given.
func libraryEdit(ctx context.Context, svc service.Service, hash string, rawInfo json.RawMessage) (*toolOutput, error) {
	var in struct {
		Tags       []string `json:"tags"`
		RemoveTags []string `json:"remove_tags"`
		Reusable   *bool    `json:"reusable"`
		Desc       *string  `json:"description"`
		RightsNote *string  `json:"rights_note"`
		Name       *string  `json:"name"`
	}

	if err := strict(rawInfo, "info", &in); err != nil {
		return nil, err
	}

	var res *service.AssetResult

	if in.Reusable != nil || in.Desc != nil || in.RightsNote != nil {
		var err error

		res, err = svc.AssetSetInfo(ctx, service.AssetSetInfoRequest{
			Hash: hash, Description: in.Desc, RightsNote: in.RightsNote, Reusable: in.Reusable,
		})
		if err != nil {
			return nil, err
		}
	}

	if len(in.Tags) > 0 || len(in.RemoveTags) > 0 {
		var err error

		res, err = svc.AssetTag(ctx, service.AssetTagRequest{Hash: hash, Add: in.Tags, Remove: in.RemoveTags})
		if err != nil {
			return nil, err
		}
	}

	if in.Name != nil {
		var err error

		res, err = svc.AssetRename(ctx, service.AssetRenameRequest{Hash: hash, Name: *in.Name})
		if err != nil {
			return nil, err
		}
	}

	if res == nil {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "info",
			Message: "info has nothing to change: give tags, remove_tags, reusable, description, rights_note or name.",
		}
	}

	return &toolOutput{value: res, text: res.Text}, nil
}
