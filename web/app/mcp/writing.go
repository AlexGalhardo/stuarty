package mcp

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service"
)

// The texts of a production on MCP (SDD-019 section 8): no tool of their own (NFR-020), so they are four actions of
// project_item (text, revisions, revert, check) and a job kind of job_start (text.draft). Each maps to one Service
// operation; the text of a result is the service's and holds every body in a block that says it is data.

func isWritingAction(action string) bool {
	switch action {
	case "text", "revisions", "revert", "check":
		return true
	}

	return false
}

func writingItem(ctx context.Context, svc service.Service, in *itemArgs) (*toolOutput, error) {
	switch in.Action {
	case "text":
		return textAction(ctx, svc, in)
	case "revisions":
		if in.ItemID == "" {
			return nil, missingArg("item_id", in.Action)
		}

		req := service.TextRevisionsRequest{ProjectID: in.ProjectID, ItemID: in.ItemID, To: in.Until}
		if in.Revision != nil {
			req.From = *in.Revision
		}

		res, err := svc.TextRevisions(ctx, req)
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "revert":
		if in.ItemID == "" {
			return nil, missingArg("item_id", in.Action)
		}

		if in.Revision == nil {
			return nil, missingArg("revision", in.Action)
		}

		res, err := svc.TextRevert(ctx, service.TextRevertRequest{
			ProjectID: in.ProjectID, ItemID: in.ItemID, Revision: *in.Revision,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	return checkAction(ctx, svc, in)
}

// textAction is project_item with action text: with no item_id it lists the texts of the project or, with text,
// makes one; with an item_id it reads the text or, with text, saves a new revision of it.
func textAction(ctx context.Context, svc service.Service, in *itemArgs) (*toolOutput, error) {
	switch {
	case in.ItemID == "" && in.Text == nil:
		res, err := svc.TextList(ctx, service.TextListRequest{ProjectID: in.ProjectID, TextKind: in.TextKind})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case in.Text == nil:
		req := service.TextGetRequest{ProjectID: in.ProjectID, ItemID: in.ItemID}
		if in.Revision != nil {
			req.Revision = *in.Revision
		}

		res, err := svc.TextGet(ctx, req)
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	res, err := svc.TextWrite(ctx, service.TextWriteRequest{
		ProjectID: in.ProjectID, ItemID: in.ItemID, Title: in.Title, TextKind: in.TextKind, Text: *in.Text,
		BaseRevision: in.Revision,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// checkAction is project_item with action check: a caption against the limits of a network (with network), or the
// duration of a script (otherwise). Given only an item_id, the kind of the text decides which.
func checkAction(ctx context.Context, svc service.Service, in *itemArgs) (*toolOutput, error) {
	if in.ItemID == "" && in.Text == nil {
		return nil, missingArg("item_id", in.Action)
	}

	text := ""
	if in.Text != nil {
		text = *in.Text
	}

	caption := in.Network != ""

	if in.ItemID != "" && in.Network == "" && in.WordsPerMinute == 0 {
		got, err := svc.TextGet(ctx, service.TextGetRequest{ProjectID: in.ProjectID, ItemID: in.ItemID})
		if err != nil {
			return nil, err
		}

		caption = got.Item.TextKind == string(writing.KindCaption)
	}

	if caption {
		res, err := svc.CaptionValidate(ctx, service.CaptionValidateRequest{
			ProjectID: in.ProjectID, ItemID: in.ItemID, Text: text, Network: in.Network,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	res, err := svc.ScriptEstimate(ctx, service.ScriptEstimateRequest{
		ProjectID: in.ProjectID, ItemID: in.ItemID, Text: text, WordsPerMinute: in.WordsPerMinute,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// textDraft is job_start with kind text.draft: args {item_id, connection_id, model, instruction, network?,
// max_tokens?, temperature?}. A person approves; the draft becomes a new revision of the text.
func textDraft(
	ctx context.Context, svc service.Service, projectID string, args json.RawMessage, waitMS int,
) (*toolOutput, error) {
	var a service.DraftArgs

	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()

	if len(bytes.TrimSpace(args)) == 0 || dec.Decode(&a) != nil || dec.More() {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "args",
			Message: "args must be one object with item_id, connection_id, model, instruction and optionally " +
				"network, max_tokens and temperature.",
		}
	}

	res, err := svc.TextDraft(ctx, service.TextDraftRequest{
		ProjectID: projectID, ItemID: a.ItemID, ConnectionID: a.ConnectionID, Model: a.Model,
		Instruction: a.Instruction, Network: a.Network, MaxTokens: a.MaxTokens, Temperature: a.Temperature,
		WaitMS: waitMS,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}
