package mcp

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// served lists, per tool, the domains whose operations it routes (SDD-003 section 4.8). A tool that is not here
// takes no `domain` argument: it serves the video domain, as it did before domains existed, or is a tool of one
// domain by its own nature. A tool gets an entry in the epic that registers the first non-video operation behind
// it, never earlier, because every argument costs bytes of every tools/list (NFR-020).
var served = map[string][]string{
	"edit":         {command.DomainVideo, command.DomainImage},
	"history":      {command.DomainVideo, command.DomainImage},
	"command_help": command.DomainNames(),
}

// routeDomain is the one router of the domain argument: it names the domain that answers a call of a tool. A
// missing domain is video, so a call written before domains existed behaves as it did. An unknown domain, and a
// domain the tool does not serve, are E_VALIDATION with the path domain and a hint that names what the tool serves.
func routeDomain(tool, domain string) (string, error) {
	if domain == "" {
		domain = command.DomainVideo
	}

	if !command.KnownDomain(domain) {
		return "", &service.Error{
			Code: command.CodeValidation, Path: "domain",
			Message: "the domain is not one of the domains of the product.",
			Hint:    "use one of " + strings.Join(command.DomainNames(), ", ") + "; command_help lists them",
		}
	}

	if !slices.Contains(served[tool], domain) {
		return "", &service.Error{
			Code:    command.CodeValidation,
			Path:    "domain",
			Message: "the tool " + tool + " has no operations in the domain " + domain + " yet.",
			Hint: "the domains of " + tool + " are " + strings.Join(
				served[tool],
				", ",
			) + "; command_help lists every domain",
		}
	}

	return domain, nil
}

// checkMembers refuses the members a domain does not use and the ones it needs, the way the image tool does for
// its actions: a call is one domain's, never a mix.
func checkMembers(tool, domain string, raw json.RawMessage, required, refused []string) error {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	for _, name := range refused {
		if _, has := given[name]; has {
			return &service.Error{
				Code: command.CodeValidation, Path: name,
				Message: name + " is not used by " + tool + " in the domain " + domain + ".",
			}
		}
	}

	for _, name := range required {
		if _, has := given[name]; !has {
			return &service.Error{
				Code: command.CodeValidation, Path: name,
				Message: name + " is required by " + tool + " in the domain " + domain + ".",
			}
		}
	}

	return nil
}

// historyMemberNames are the members of the history tool that an action takes or refuses besides the domain and
// the action: the image domain refuses the ones its action does not use.
var historyMemberNames = []string{
	"project_id", "timeline_id", "actor", "cursor", "limit", "revision", "until", "name", "new_name", "note", "dry_run",
}

// imageHistory is the history tool in the image domain: undo and redo, and since epic 50 revert, the diff and the
// checkpoints, which are those of a timeline for an image document (SDD-003 section 2.5.1). An image has no change
// log to list on this tool (the list is HTTP and the editor, SDD-000 section 1). A member that the action does not
// use is refused; an image undo has no dry run, so a dry_run that were ignored there would undo for real.
func imageHistory(ctx context.Context, svc service.Service, raw json.RawMessage, in *historyArgs) (*toolOutput, error) {
	uses, known := historyMembers[in.Action]
	if !known || in.Action == "list" {
		return nil, &service.Error{
			Code:    command.CodeValidation,
			Path:    "action",
			Message: "an image has no change log to list on this tool.",
			Hint:    "use undo, redo, revert, diff, checkpoints, checkpoint, rename_checkpoint or remove_checkpoint with domain image",
		}
	}

	var refused []string

	for _, name := range historyMemberNames {
		if !slices.Contains(uses, name) || in.Action == "undo" && name == "dry_run" ||
			in.Action == "redo" && name == "dry_run" {
			refused = append(refused, name)
		}
	}

	if err := checkMembers("history", command.DomainImage, raw, []string{"image_id"}, refused); err != nil {
		return nil, err
	}

	switch in.Action {
	case "undo", "redo":
		req := service.ImageStepRequest{ImageID: in.ImageID}

		var (
			res *service.ImageStepResult
			err error
		)

		if in.Action == "undo" {
			res, err = svc.ImageUndo(ctx, req)
		} else {
			res, err = svc.ImageRedo(ctx, req)
		}

		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: service.ImageStepText(res)}, nil
	case "revert":
		res, err := svc.ImageRevert(ctx, service.ImageRevertRequest{
			ImageID: in.ImageID, Revision: in.Revision, Checkpoint: in.Name, DryRun: in.DryRun,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "diff":
		res, err := svc.ImageDiff(ctx, service.ImageDiffRequest{
			ImageID: in.ImageID, From: in.Revision, FromCheckpoint: in.Name, Until: in.Until,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	return imageCheckpointAction(ctx, svc, in)
}

func imageCheckpointAction(ctx context.Context, svc service.Service, in *historyArgs) (*toolOutput, error) {
	var (
		cp    *service.Checkpoint
		err   error
		state string
	)

	switch in.Action {
	case "checkpoints":
		res, lerr := svc.ImageCheckpointList(ctx, service.ImageCheckpointListRequest{ImageID: in.ImageID})
		if lerr != nil {
			return nil, lerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "checkpoint":
		state = "created"
		cp, err = svc.ImageCheckpointCreate(ctx, service.ImageCheckpointCreateRequest{
			ImageID: in.ImageID, Name: in.Name, Note: in.Note, Revision: in.Revision,
		})
	case "rename_checkpoint":
		state = "renamed"
		req := service.ImageCheckpointRenameRequest{ImageID: in.ImageID, Name: in.Name, NewName: in.NewName}

		if in.Note != "" {
			req.Note = &in.Note
		}

		cp, err = svc.ImageCheckpointRename(ctx, req)
	default:
		state = "removed"
		cp, err = svc.ImageCheckpointRemove(
			ctx,
			service.ImageCheckpointRemoveRequest{ImageID: in.ImageID, Name: in.Name},
		)
	}

	if err != nil {
		return nil, err
	}

	return &toolOutput{value: cp, text: service.CheckpointText(state, cp)}, nil
}

// toolDomains is the domains each tool belongs to: what the parity test of web/contract compares with the domain
// of the Service operations the tool serves. A tool that routes by the domain argument (served) lists those
// domains too; command_help is the discovery of every domain.
var toolDomains = map[string][]string{
	"project_list":   {command.DomainProject},
	"project_create": {command.DomainProject},
	"project_get":    {command.DomainProject},
	"project_trash":  {command.DomainProject},
	// project_item also serves the texts of a production (text, revisions, revert, check; SDD-019).
	"project_item": {command.DomainProject, command.DomainWriting},
	"edit":         {command.DomainVideo, command.DomainImage},
	"history":      {command.DomainVideo, command.DomainImage},
	"view":         {command.DomainVideo},
	"lint":         {command.DomainVideo},
	// job_start also starts text.generate, image.generate, voice.generate and video.generate, the generations of
	// the provider domain (SDD-016 sections 10, 6.1 and 6.4), and text.draft, the draft of a text of the writing domain (SDD-019
	// section 6), and publish.upload, a publication of the publish domain (SDD-020 section 13). job_get also
	// reads the approval queue and the state of a publication (SDD-015 section 10).
	"job_start": {command.DomainVideo, command.DomainProvider, command.DomainWriting, command.DomainPublish},
	"job_get": {
		command.DomainVideo,
		command.DomainPublish,
	},
	"job_cancel":    {command.DomainVideo},
	"template":      {command.DomainVideo},
	"media_import":  {command.DomainAsset},
	"media_inspect": {command.DomainAsset},
	"font_import":   {command.DomainAsset},
	"image":         {command.DomainImage},
	"library":       {command.DomainMemory},
	// command_help answers for every domain; it is itself an operation of the workspace domain.
	"command_help": command.DomainNames(),
	"connection":   {command.DomainWorkspace},
}

// ToolDomains lists the domains a tool belongs to.
func ToolDomains(tool string) []string { return slices.Clone(toolDomains[tool]) }

// ServesDomain reports whether a tool routes the operations of a domain through its domain argument.
func ServesDomain(tool, domain string) bool { return slices.Contains(served[tool], domain) }

// ToolNames lists the tools in the order of tools/list.
func ToolNames() []string { return slices.Clone(order) }
