package contract

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// The revert, diff and checkpoint operations (SDD-003 section 2.5.1, epic 36) on the four drivers. The switches of
// drivers_test.go end by asking these functions, which keeps the mapping of one operation family in one place.

func init() {
	// The text of a checkpoint after an operation on it is rendered by the adapter from the result, like an edit's.
	for _, call := range []string{"CheckpointCreate", "CheckpointRename", "CheckpointRemove"} {
		renderedByAdapter[call] = true
	}
}

// TestHistoryScenarioIsTheSameThroughEverySurface (epic 36): three texts, a checkpoint, the diff by revision and by
// name, a dry run, a revert by name and by revision, its undo, the refusals, rename and remove. The results are the
// same on the four surfaces, and so are the final documents.
func TestHistoryScenarioIsTheSameThroughEverySurface(t *testing.T) {
	sameThroughEverySurface(t, "history", "prj_7k2m9x4q1b")
}

// historyCall runs a step on a Service. ok is false when the step is not one of these operations.
func historyCall(ctx context.Context, svc service.Service, s step) (res any, ok bool, err error) {
	switch s.Call {
	case "HistoryRevert":
		res, err = invoke(ctx, s, svc.HistoryRevert)
	case "HistoryDiff":
		res, err = invoke(ctx, s, svc.HistoryDiff)
	case "CheckpointCreate":
		res, err = invoke(ctx, s, svc.CheckpointCreate)
	case "CheckpointList":
		res, err = invoke(ctx, s, svc.CheckpointList)
	case "CheckpointRename":
		res, err = invoke(ctx, s, svc.CheckpointRename)
	case "CheckpointRemove":
		res, err = invoke(ctx, s, svc.CheckpointRemove)
	default:
		return nil, false, nil
	}

	return res, true, err
}

// historyStep is every member a step of these operations can carry.
type historyStep struct {
	ProjectID      string  `json:"project_id"`
	TimelineID     string  `json:"timeline_id"`
	Revision       int64   `json:"revision"`
	Checkpoint     string  `json:"checkpoint"`
	BaseRevision   int64   `json:"base_revision"`
	DryRun         bool    `json:"dry_run"`
	From           int64   `json:"from"`
	FromCheckpoint string  `json:"from_checkpoint"`
	Until          int64   `json:"until"`
	Name           string  `json:"name"`
	NewName        string  `json:"new_name"`
	Note           *string `json:"note"`
}

// historyCalls are the operations these drivers encode; a step of any other operation is never decoded here, since
// its members (a motion step's string "from", for one) need not fit historyStep.
var historyCalls = map[string]bool{
	"HistoryRevert": true, "HistoryDiff": true, "CheckpointCreate": true, "CheckpointList": true,
	"CheckpointRename": true, "CheckpointRemove": true,
}

func readHistoryStep(s step) (historyStep, error) {
	var a historyStep

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return a, err
		}
	}

	return a, nil
}

// historyTool maps a step to a call of the MCP tool history. ok is false for a step that is not one of these
// operations. The tool has no base_revision: a step that carries one is not encodable and fails the scenario.
func historyTool(s step) (tool string, args map[string]any, ok bool, err error) {
	if !historyCalls[s.Call] {
		return "", nil, false, nil
	}

	a, err := readHistoryStep(s)
	if err != nil {
		return "", nil, true, err
	}

	args = map[string]any{}
	set := func(k string, v any) {
		switch x := v.(type) {
		case string:
			if x != "" {
				args[k] = x
			}
		case int64:
			if x != 0 {
				args[k] = x
			}
		case bool:
			if x {
				args[k] = x
			}
		}
	}

	set("project_id", a.ProjectID)
	set("timeline_id", a.TimelineID)

	switch s.Call {
	case "HistoryRevert":
		args["action"] = "revert"

		set("revision", a.Revision)
		set("name", a.Checkpoint)
		set("dry_run", a.DryRun)
	case "HistoryDiff":
		args["action"] = "diff"

		set("revision", a.From)
		set("name", a.FromCheckpoint)
		set("until", a.Until)
	case "CheckpointList":
		args["action"] = "checkpoints"
	case "CheckpointCreate":
		args["action"] = "checkpoint"

		set("name", a.Name)
		set("revision", a.Revision)

		if a.Note != nil {
			set("note", *a.Note)
		}
	case "CheckpointRename":
		args["action"] = "rename_checkpoint"

		set("name", a.Name)
		set("new_name", a.NewName)

		if a.Note != nil {
			set("note", *a.Note)
		}
	case "CheckpointRemove":
		args["action"] = "remove_checkpoint"

		set("name", a.Name)
	default:
		return "", nil, false, nil
	}

	if a.BaseRevision != 0 {
		return "", nil, true, errNoBaseOnTools
	}

	return "history", args, true, nil
}

var errNoBaseOnTools = &toolMemberError{"base_revision is not a member of the history tool"}

type toolMemberError struct{ msg string }

func (e *toolMemberError) Error() string { return e.msg }

// historyCommand maps a step to a command of the CLI.
func historyCommand(s step) (name string, argv []string, ok bool, err error) {
	if !historyCalls[s.Call] {
		return "", nil, false, nil
	}

	a, err := readHistoryStep(s)
	if err != nil {
		return "", nil, true, err
	}

	argv = []string{"--project", a.ProjectID}
	flag := func(k, v string) {
		if v != "" {
			argv = append(argv, "--"+k, v)
		}
	}
	number := func(k string, n int64) {
		if n != 0 {
			flag(k, strconv.FormatInt(n, 10))
		}
	}

	flag("timeline", a.TimelineID)

	switch s.Call {
	case "HistoryRevert":
		number("revision", a.Revision)
		flag("checkpoint", a.Checkpoint)
		number("base", a.BaseRevision)

		if a.DryRun {
			argv = append(argv, "--dry-run")
		}

		return "revert", argv, true, nil
	case "HistoryDiff":
		number("from", a.From)
		flag("from-checkpoint", a.FromCheckpoint)
		number("until", a.Until)

		return "diff", argv, true, nil
	case "CheckpointList":
		return "checkpoint:list", argv, true, nil
	case "CheckpointCreate":
		flag("name", a.Name)
		number("revision", a.Revision)

		if a.Note != nil {
			argv = append(argv, "--note", *a.Note)
		}

		return "checkpoint:create", argv, true, nil
	case "CheckpointRename":
		flag("name", a.Name)
		flag("new-name", a.NewName)

		if a.Note != nil {
			argv = append(argv, "--note", *a.Note)
		}

		return "checkpoint:rename", argv, true, nil
	case "CheckpointRemove":
		flag("name", a.Name)

		return "checkpoint:remove", argv, true, nil
	}

	return "", nil, false, nil
}
