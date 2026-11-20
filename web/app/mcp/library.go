package mcp

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// libraryMembers are the members each action of the library tool uses (SDD-003 section 4.2): the others are
// refused, and the required ones must be there. write has its own list per kind.
var libraryMembers = map[string]struct{ required, optional []string }{
	"search": {[]string{"query"}, []string{"kinds"}},
	"list":   {[]string{"kind"}, []string{"tags"}},
	"get":    {[]string{"kind", "name"}, nil},
	"render": {[]string{"name"}, []string{"kind", "values"}},
	"lint":   {[]string{"name"}, []string{"kind"}},
}

var libraryWriteMembers = map[string]struct{ required, optional []string }{
	"memory": {[]string{"description"}, []string{"name", "body", "tags"}},
	"skill":  {[]string{"name", "description", "body"}, []string{"files"}},
	"prompt": {[]string{"name", "description", "body"}, []string{"tags"}},
}

// checkLibraryMembers refuses a member the action does not use and names the path of a missing one.
func checkLibraryMembers(action, kind string, raw json.RawMessage) error {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	want, known := libraryMembers[action]

	if action == "write" {
		if kind == "" {
			return &service.Error{
				Code: command.CodeValidation, Path: "kind", Message: "kind is required by the action write.",
			}
		}

		want, known = libraryWriteMembers[kind]
		want.required = append([]string{"kind"}, want.required...)
	}

	if !known {
		return &service.Error{
			Code:    command.CodeValidation,
			Path:    "action",
			Message: "action " + action + " is not a library action.",
			Hint:    "use search, list, get, write, render or lint",
		}
	}

	for name := range given {
		if name != "action" && name != "kind" && !slices.Contains(want.required, name) &&
			!slices.Contains(want.optional, name) {
			return &service.Error{
				Code: command.CodeValidation, Path: name, Message: name + " is not used by the action " + action + ".",
			}
		}
	}

	if _, ok := given["kind"]; ok && !slices.Contains(want.required, "kind") &&
		!slices.Contains(want.optional, "kind") {
		return &service.Error{
			Code: command.CodeValidation, Path: "kind", Message: "kind is not used by the action " + action + ".",
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

// libraryTool is the tool of the library. Every call is made as an agent (agent_view): it never sees a skill
// nobody reviewed or a note nobody kept, and it removes nothing and imports nothing.
func libraryTool(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Action      string                   `json:"action"`
		Kind        string                   `json:"kind"`
		Name        string                   `json:"name"`
		Query       string                   `json:"query"`
		Kinds       []string                 `json:"kinds"`
		Description string                   `json:"description"`
		Body        string                   `json:"body"`
		Tags        []string                 `json:"tags"`
		Values      map[string]string        `json:"values"`
		Files       []service.SkillFileInput `json:"files"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if err := checkLibraryMembers(in.Action, in.Kind, raw); err != nil {
		return nil, err
	}

	switch in.Action {
	case "search":
		res, err := svc.LibrarySearch(ctx, service.LibrarySearchRequest{
			Query: in.Query, Kinds: in.Kinds, AgentView: true,
		})

		return libraryOutput(res, err, func() string { return res.Text })
	case "render":
		if in.Kind != "" && in.Kind != "prompt" {
			return nil, &service.Error{
				Code:    command.CodeValidation,
				Path:    "kind",
				Message: "only a prompt is rendered.",
			}
		}

		res, err := svc.PromptRender(
			ctx,
			service.PromptRenderRequest{Name: in.Name, Values: in.Values, AgentView: true},
		)

		return libraryOutput(res, err, func() string { return res.Text })
	case "lint":
		if in.Kind != "" && in.Kind != "skill" {
			return nil, &service.Error{Code: command.CodeValidation, Path: "kind", Message: "only a skill is linted."}
		}

		res, err := svc.SkillLint(ctx, service.LibraryRef{Name: in.Name})

		return libraryOutput(res, err, func() string { return res.Text })
	case "list":
		return libraryList(ctx, svc, in.Kind, in.Tags)
	case "get":
		return libraryGet(ctx, svc, in.Kind, in.Name)
	}

	return libraryWrite(ctx, svc, in.Kind, in.Name, in.Description, in.Body, in.Tags, in.Files)
}

func libraryOutput[T any](res *T, err error, text func() string) (*toolOutput, error) {
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: text()}, nil
}

func libraryList(ctx context.Context, svc service.Service, kind string, tags []string) (*toolOutput, error) {
	var (
		res *service.LibraryListResult
		err error
	)

	switch kind {
	case "memory":
		res, err = svc.MemoryList(ctx, service.MemoryListRequest{Tags: tags, AgentView: true})
	case "skill":
		res, err = svc.SkillList(ctx, service.SkillListRequest{AgentView: true})
	default:
		res, err = svc.PromptList(ctx, service.PromptListRequest{Tags: tags, AgentView: true})
	}

	return libraryOutput(res, err, func() string { return res.Text })
}

func libraryGet(ctx context.Context, svc service.Service, kind, name string) (*toolOutput, error) {
	ref := service.LibraryRef{Name: name, AgentView: true}

	switch kind {
	case "memory":
		res, err := svc.MemoryGet(ctx, ref)

		return libraryOutput(res, err, func() string { return res.Text })
	case "skill":
		res, err := svc.SkillGet(ctx, ref)

		return libraryOutput(res, err, func() string { return res.Text })
	}

	res, err := svc.PromptGet(ctx, ref)

	return libraryOutput(res, err, func() string { return res.Text })
}

func libraryWrite(
	ctx context.Context, svc service.Service, kind, name, description, body string, tags []string,
	files []service.SkillFileInput,
) (*toolOutput, error) {
	switch kind {
	case "memory":
		res, err := svc.MemoryWrite(ctx, service.MemoryWriteRequest{
			Name: name, Description: description, Tags: tags, Body: body,
		})

		return libraryOutput(res, err, func() string { return res.Text })
	case "skill":
		res, err := svc.SkillWrite(ctx, service.SkillWriteRequest{
			Name: name, Description: description, Body: body, Files: files,
		})

		return libraryOutput(res, err, func() string { return res.Text })
	}

	res, err := svc.PromptWrite(ctx, service.PromptWriteRequest{
		Name: name, Description: description, Tags: tags, Body: body,
	})

	return libraryOutput(res, err, func() string { return res.Text })
}
