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

// imageHistory is the history tool in the image domain: an image has undo and redo and no change log yet, so
// the list is refused with the actions that exist.
func imageHistory(
	ctx context.Context, svc service.Service, raw json.RawMessage, action, imageID string,
) (*toolOutput, error) {
	// The members of revert, diff and checkpoints are of timelines only; an image undo has no dry run, so a
	// dry_run that were ignored here would undo for real.
	refused := []string{
		"project_id", "timeline_id", "actor", "cursor", "limit", "revision", "until", "name", "new_name", "note",
		"dry_run",
	}
	if err := checkMembers("history", command.DomainImage, raw, []string{"image_id"}, refused); err != nil {
		return nil, err
	}

	if action != "undo" && action != "redo" {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "action",
			Message: "an image has no change log to list yet.",
			Hint:    "use action undo or redo with domain image",
		}
	}

	req := service.ImageStepRequest{ImageID: imageID}

	var (
		res *service.ImageStepResult
		err error
	)

	if action == "undo" {
		res, err = svc.ImageUndo(ctx, req)
	} else {
		res, err = svc.ImageRedo(ctx, req)
	}

	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: service.ImageStepText(res)}, nil
}

// toolDomains is the domains each tool belongs to: what the parity test of web/contract compares with the domain
// of the Service operations the tool serves. A tool that routes by the domain argument (served) lists those
// domains too; command_help is the discovery of every domain.
var toolDomains = map[string][]string{
	"project_list": {command.DomainProject}, "project_create": {command.DomainProject},
	"project_get": {command.DomainProject}, "project_trash": {command.DomainProject},
	// project_item also serves the texts of a production (text, revisions, revert, check; SDD-019).
	"project_item": {command.DomainProject, command.DomainWriting},
	"edit":         {command.DomainVideo, command.DomainImage}, "history": {command.DomainVideo, command.DomainImage},
	"view": {command.DomainVideo}, "lint": {command.DomainVideo},
	// job_start also starts text.generate, image.generate, voice.generate and video.generate, the generations of
	// the provider domain (SDD-016 sections 10, 6.1 and 6.4), and text.draft, the draft of a text of the writing domain (SDD-019
	// section 6).
	"job_start": {command.DomainVideo, command.DomainProvider, command.DomainWriting},
	"job_get":   {command.DomainVideo}, "job_cancel": {command.DomainVideo}, "template": {command.DomainVideo},
	"media_import": {command.DomainAsset}, "media_inspect": {command.DomainAsset}, "font_import": {command.DomainAsset},
	"image":   {command.DomainImage},
	"library": {command.DomainMemory},
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
