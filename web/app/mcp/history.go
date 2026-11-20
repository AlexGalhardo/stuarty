package mcp

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// historyArgs is the input of the history tool.
type historyArgs struct {
	Domain     string `json:"domain"`
	ImageID    string `json:"image_id"`
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id"`
	Action     string `json:"action"`
	Actor      string `json:"actor"`
	Revision   int64  `json:"revision"`
	Until      int64  `json:"until"`
	Name       string `json:"name"`
	NewName    string `json:"new_name"`
	Note       string `json:"note"`
	DryRun     bool   `json:"dry_run"`
	pageArgs
}

// historyMembers is, per action, the members beyond the ones every action takes that the action uses
// (SDD-003 section 7.1, S-11): the adapter refuses a member the action does not use.
var historyMembers = map[string][]string{
	"list":              {"actor", "cursor", "limit"},
	"undo":              {"dry_run"},
	"redo":              {"dry_run"},
	"revert":            {"revision", "name", "dry_run"},
	"diff":              {"revision", "name", "until"},
	"checkpoints":       {},
	"checkpoint":        {"name", "note", "revision"},
	"rename_checkpoint": {"name", "new_name", "note"},
	"remove_checkpoint": {"name"},
}

// checkHistoryMembers refuses the members that the new actions of epic 36 added and that the action does not
// use; the members of list, undo and redo keep the leniency they always had.
func checkHistoryMembers(raw json.RawMessage, action string) error {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	for _, name := range []string{"revision", "until", "name", "new_name", "note", "dry_run"} {
		if _, has := given[name]; !has || slices.Contains(historyMembers[action], name) {
			continue
		}

		return &service.Error{
			Code:    command.CodeValidation,
			Path:    name,
			Message: name + " is not used by history with action " + action + ".",
		}
	}

	if action == "list" || action == "undo" || action == "redo" {
		return nil
	}

	for _, name := range []string{"actor", "cursor", "limit"} {
		if _, has := given[name]; has && !slices.Contains(historyMembers[action], name) {
			return &service.Error{
				Code:    command.CodeValidation,
				Path:    name,
				Message: name + " is not used by history with action " + action + ".",
			}
		}
	}

	return nil
}

// videoHistory serves every action of history on a timeline.
func videoHistory(ctx context.Context, svc service.Service, raw json.RawMessage, in *historyArgs) (*toolOutput, error) {
	if err := checkHistoryMembers(raw, in.Action); err != nil {
		return nil, err
	}

	switch in.Action {
	case "undo", "redo":
		req := service.UndoRequest{ProjectID: in.ProjectID, TimelineID: in.TimelineID, DryRun: in.DryRun}

		var (
			res *service.UndoResult
			err error
		)

		if in.Action == "undo" {
			res, err = svc.Undo(ctx, req)
		} else {
			res, err = svc.Redo(ctx, req)
		}

		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: service.UndoText(res)}, nil
	case "revert":
		return revertAction(ctx, svc, in)
	case "diff":
		res, err := svc.HistoryDiff(ctx, service.HistoryDiffRequest{
			ProjectID:      in.ProjectID,
			TimelineID:     in.TimelineID,
			From:           in.Revision,
			FromCheckpoint: in.Name,
			Until:          in.Until,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "checkpoints", "checkpoint", "rename_checkpoint", "remove_checkpoint":
		return checkpointAction(ctx, svc, in)
	default:
		res, err := svc.HistoryList(ctx, service.HistoryListRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, Actor: in.Actor, Page: in.page(),
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}
}

func revertAction(ctx context.Context, svc service.Service, in *historyArgs) (*toolOutput, error) {
	res, err := svc.HistoryRevert(ctx, service.HistoryRevertRequest{
		ProjectID: in.ProjectID, TimelineID: in.TimelineID, Revision: in.Revision, Checkpoint: in.Name,
		DryRun: in.DryRun,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func checkpointAction(ctx context.Context, svc service.Service, in *historyArgs) (*toolOutput, error) {
	var (
		cp    *service.Checkpoint
		err   error
		state string
	)

	switch in.Action {
	case "checkpoints":
		res, lerr := svc.CheckpointList(
			ctx,
			service.CheckpointListRequest{ProjectID: in.ProjectID, TimelineID: in.TimelineID},
		)
		if lerr != nil {
			return nil, lerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "checkpoint":
		state = "created"
		cp, err = svc.CheckpointCreate(ctx, service.CheckpointCreateRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, Name: in.Name, Note: in.Note, Revision: in.Revision,
		})
	case "rename_checkpoint":
		state = "renamed"
		req := service.CheckpointRenameRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, Name: in.Name, NewName: in.NewName,
		}

		if in.Note != "" {
			req.Note = &in.Note
		}

		cp, err = svc.CheckpointRename(ctx, req)
	default:
		state = "removed"
		cp, err = svc.CheckpointRemove(ctx, service.CheckpointRemoveRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, Name: in.Name,
		})
	}

	if err != nil {
		return nil, err
	}

	return &toolOutput{value: cp, text: service.CheckpointText(state, cp)}, nil
}
