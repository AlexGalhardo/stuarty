package mcp

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// handlers are the tools whose Service methods exist. view is in Pending until
// the Service has a frame method: add the handler here and drop the name from
// Pending.
func (a *adapter) handlers() map[string]callFunc {
	return map[string]callFunc{
		"project_list":   projectList,
		"project_create": projectCreate,
		"project_get":    projectGet,
		"edit":           edit,
		"command_help":   commandHelp,
		"history":        history,
		"lint":           lintTool,
		"media_import":   mediaImport,
		"media_inspect":  mediaInspect,
		"job_start":      jobStart,
		"job_get":        jobGet,
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

func mediaImport(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID string   `json:"project_id"`
		Paths     []string `json:"paths"`
		AssetIDs  []string `json:"asset_ids"`
		WaitMS    *int     `json:"wait_ms"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if len(in.AssetIDs) > 0 && len(in.AssetIDs) != len(in.Paths) {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "asset_ids",
			Message: "asset_ids must have as many entries as paths.",
		}
	}

	files := make([]service.ImportFile, len(in.Paths))
	for i, p := range in.Paths {
		files[i].Path = p

		if len(in.AssetIDs) > 0 {
			files[i].AssetID = in.AssetIDs[i]
		}
	}

	wait := defaultImportWaitMS
	if in.WaitMS != nil {
		wait = *in.WaitMS
	}

	res, err := svc.MediaImport(ctx, service.MediaImportRequest{ProjectID: in.ProjectID, Files: files, WaitMS: wait})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// defaultImportWaitMS is the default of wait_ms in the media_import definition.
const defaultImportWaitMS = 10000

// mediaInspect is two operations behind one tool (SDD-003 section 4.1): with an
// asset_id it describes the asset, without one it lists the assets.
func mediaInspect(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID  string   `json:"project_id"`
		AssetID    string   `json:"asset_id"`
		Sections   []string `json:"sections"`
		MinSilence string   `json:"min_silence"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.AssetID == "" {
		res, lerr := svc.MediaList(ctx, service.MediaListRequest{ProjectID: in.ProjectID, Page: in.page()})
		if lerr != nil {
			return nil, lerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	res, err := svc.MediaInspect(ctx, service.MediaInspectRequest{
		ProjectID: in.ProjectID, AssetID: in.AssetID, Sections: in.Sections, MinSilence: in.MinSilence, Page: in.page(),
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func jobStart(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID string          `json:"project_id"`
		Kind      string          `json:"kind"`
		Args      json.RawMessage `json:"args"`
		WaitMS    int             `json:"wait_ms"`
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.JobStart(ctx, service.JobStartRequest{
		ProjectID: in.ProjectID, Kind: in.Kind, Args: in.Args, WaitMS: in.WaitMS,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// jobGet reads one job (job_id) or lists the jobs of a project (project_id).
func jobGet(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		JobID     string `json:"job_id"`
		ProjectID string `json:"project_id"`
		State     string `json:"state"`
		WaitMS    int    `json:"wait_ms"`
	}](raw)
	if err != nil {
		return nil, err
	}

	switch {
	case in.JobID != "":
		res, gerr := svc.JobGet(ctx, service.JobGetRequest{JobID: in.JobID, WaitMS: in.WaitMS})
		if gerr != nil {
			return nil, gerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case in.ProjectID != "":
		res, lerr := svc.JobList(ctx, service.JobListRequest{ProjectID: in.ProjectID, State: in.State})
		if lerr != nil {
			return nil, lerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	default:
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "job_id",
			Message: "give job_id, or project_id to list its jobs.",
		}
	}
}
