package mcp

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// handlers are the tools whose Service methods exist. media_import,
// media_inspect, view, job_start and job_get are in Pending until theirs do:
// add the handler here and drop the name from Pending.
func (a *adapter) handlers() map[string]callFunc {
	return map[string]callFunc{
		"project_list":   projectList,
		"project_create": projectCreate,
		"project_get":    projectGet,
		"edit":           edit,
		"command_help":   commandHelp,
		"history":        history,
		"lint":           lintTool,
	}
}

// decode reads the arguments, which parseArguments already checked against the
// schema, into a struct.
func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	return v, nil
}

type pageArgs struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

func (p pageArgs) page() service.Page { return service.Page{Cursor: p.Cursor, Limit: p.Limit} }

func projectList(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		State string `json:"state"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.ProjectList(ctx, service.ProjectListRequest{State: in.State, Page: in.page()})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func projectCreate(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Name   string `json:"name"`
		Preset string `json:"preset"`
		FPS    string `json:"fps"`
		ID     string `json:"id"`
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.ProjectCreate(
		ctx,
		service.ProjectCreateRequest{Name: in.Name, Preset: in.Preset, FPS: in.FPS, ID: in.ID},
	)
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func projectGet(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID string `json:"project_id"`
		Detail    string `json:"detail"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.ProjectGet(
		ctx,
		service.ProjectGetRequest{ProjectID: in.ProjectID, Detail: in.Detail, Page: in.page()},
	)
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func edit(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID      string              `json:"project_id"`
		BaseRevision   int64               `json:"base_revision"`
		Operations     []command.Operation `json:"operations"`
		IdempotencyKey string              `json:"idempotency_key"`
		DryRun         bool                `json:"dry_run"`
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.Edit(ctx, service.EditRequest{
		ProjectID: in.ProjectID, BaseRevision: in.BaseRevision, Operations: in.Operations,
		IdempotencyKey: in.IdempotencyKey, DryRun: in.DryRun,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: service.EditText(res)}, nil
}

func commandHelp(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.CommandHelp(ctx, service.CommandHelpRequest{Name: in.Name})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// history is three operations behind one tool (SDD-003 section 4.1): list, undo
// and redo.
func history(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID string `json:"project_id"`
		Action    string `json:"action"`
		Actor     string `json:"actor"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	switch in.Action {
	case "undo", "redo":
		req := service.UndoRequest{ProjectID: in.ProjectID}

		var res *service.UndoResult

		if in.Action == "undo" {
			res, err = svc.Undo(ctx, req)
		} else {
			res, err = svc.Redo(ctx, req)
		}

		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: service.UndoText(res)}, nil
	default:
		res, err := svc.HistoryList(
			ctx,
			service.HistoryListRequest{ProjectID: in.ProjectID, Actor: in.Actor, Page: in.page()},
		)
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}
}

func lintTool(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID string `json:"project_id"`
		Severity  string `json:"severity"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.Lint(ctx, service.LintRequest{ProjectID: in.ProjectID, Severity: in.Severity, Page: in.page()})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}
