package mcp

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/service"
)

// imageMembers are the members each action of the image tool uses (SDD-021 section 11): the others are refused,
// and the required ones must be there.
var imageMembers = map[string]struct{ required, optional []string }{
	"create": {nil, []string{"preset", "width", "height", "name", "template", "from_template", "values"}},
	"list":   {nil, nil},
	"show":   {[]string{"image_id"}, nil},
	"edit": {
		[]string{"image_id", "base_revision", "operations"}, []string{"idempotency_key", "dry_run"},
	},
	"undo":   {[]string{"image_id"}, []string{"dry_run"}},
	"redo":   {[]string{"image_id"}, []string{"dry_run"}},
	"export": {[]string{"image_id"}, []string{"format", "quality", "preset", "width", "height"}},
}

func checkImageMembers(action string, raw json.RawMessage) error {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	want, known := imageMembers[action]
	if !known {
		return &service.Error{
			Code: command.CodeValidation, Path: "action",
			Message: "action " + action + " is not an image action.",
			Hint:    "use create, list, show, edit, undo, redo or export",
		}
	}

	for name := range given {
		if name != "action" && !slices.Contains(want.required, name) && !slices.Contains(want.optional, name) {
			return &service.Error{
				Code: command.CodeValidation, Path: name, Message: name + " is not used by the action " + action + ".",
			}
		}
	}

	for _, name := range want.required {
		if _, ok := given[name]; !ok {
			return &service.Error{
				Code: command.CodeValidation, Path: name, Message: name + " is required by the action " + action + ".",
			}
		}
	}

	return nil
}

type imageArgs struct {
	Action         string                         `json:"action"`
	ImageID        string                         `json:"image_id"`
	BaseRevision   int64                          `json:"base_revision"`
	Operations     []command.Operation            `json:"operations"`
	IdempotencyKey string                         `json:"idempotency_key"`
	DryRun         bool                           `json:"dry_run"`
	Preset         string                         `json:"preset"`
	Width          int                            `json:"width"`
	Height         int                            `json:"height"`
	Name           string                         `json:"name"`
	Template       bool                           `json:"template"`
	TemplateID     string                         `json:"from_template"`
	Values         map[string]coreimage.SlotValue `json:"values"`
	Format         string                         `json:"format"`
	Quality        int                            `json:"quality"`
	pageArgs
}

func imageTool(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[imageArgs](raw)
	if err != nil {
		return nil, err
	}

	if err := checkImageMembers(in.Action, raw); err != nil {
		return nil, err
	}

	switch in.Action {
	case "create":
		res, err := svc.ImageCreate(ctx, service.ImageCreateRequest{
			Name: in.Name, Preset: in.Preset, Width: in.Width, Height: in.Height,
			Template: in.Template, TemplateID: in.TemplateID, Values: in.Values,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "list":
		res, err := svc.ImageList(ctx, service.ImageListRequest{})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "show":
		res, err := svc.ImageDescribe(ctx, service.ImageDescribeRequest{ImageID: in.ImageID})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "edit":
		res, err := svc.ImageEdit(ctx, service.ImageEditRequest{
			ImageID: in.ImageID, BaseRevision: in.BaseRevision, Operations: in.Operations,
			IdempotencyKey: in.IdempotencyKey, DryRun: in.DryRun,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: service.ImageEditText(res)}, nil
	case "undo", "redo":
		req := service.ImageStepRequest{ImageID: in.ImageID, DryRun: in.DryRun}

		var res *service.ImageStepResult
		if in.Action == "undo" {
			res, err = svc.ImageUndo(ctx, req)
		} else {
			res, err = svc.ImageRedo(ctx, req)
		}

		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: service.ImageStepText(res)}, nil
	}

	res, err := svc.ImageExport(ctx, service.ImageExportRequest{
		ImageID: in.ImageID,
		Format:  in.Format,
		Quality: in.Quality,
		Preset:  in.Preset,
		Width:   in.Width,
		Height:  in.Height,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: service.ImageExportText(res)}, nil
}
