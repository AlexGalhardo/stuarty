package contract

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// The revert, diff and checkpoint operations of an image document (SDD-003 section 2.5.1, SDD-021 section 4, epic
// 50) on the four drivers. They are those of a timeline with the id of the image: the MCP tool is history with
// domain image, and the CLI commands are image:revert, image:diff and image:checkpoint:*.

func init() {
	for _, call := range []string{"ImageCheckpointCreate", "ImageCheckpointRename", "ImageCheckpointRemove"} {
		renderedByAdapter[call] = true
	}
}

// TestImageHistoryScenarioIsTheSameThroughEverySurface: edits of an agent, the automatic and a named checkpoint,
// the diff by revision and by name, a dry run, a revert by name and by revision, its undo, the refusals, rename
// and remove. The results are the same on the four surfaces, and so are the final documents.
func TestImageHistoryScenarioIsTheSameThroughEverySurface(t *testing.T) {
	sameThroughEverySurface(t, "image-history")
}

// imageHistoryCall runs a step on a Service. ok is false when the step is not one of these operations.
func imageHistoryCall(ctx context.Context, svc service.Service, s step) (res any, ok bool, err error) {
	switch s.Call {
	case "ImageRevert":
		res, err = invoke(ctx, s, svc.ImageRevert)
	case "ImageDiff":
		res, err = invoke(ctx, s, svc.ImageDiff)
	case "ImageCheckpointCreate":
		res, err = invoke(ctx, s, svc.ImageCheckpointCreate)
	case "ImageCheckpointList":
		res, err = invoke(ctx, s, svc.ImageCheckpointList)
	case "ImageCheckpointRename":
		res, err = invoke(ctx, s, svc.ImageCheckpointRename)
	case "ImageCheckpointRemove":
		res, err = invoke(ctx, s, svc.ImageCheckpointRemove)
	default:
		return nil, false, nil
	}

	return res, true, err
}

// imageHistoryStep is every member a step of these operations can carry.
type imageHistoryStep struct {
	ImageID        string  `json:"image_id"`
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

var imageHistoryCalls = map[string]bool{
	"ImageRevert": true, "ImageDiff": true, "ImageCheckpointCreate": true, "ImageCheckpointList": true,
	"ImageCheckpointRename": true, "ImageCheckpointRemove": true,
}

func readImageHistoryStep(s step) (imageHistoryStep, error) {
	var a imageHistoryStep

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return a, err
		}
	}

	return a, nil
}

// imageHistoryTool maps a step to a call of the MCP tool history with domain image. The tool has no
// base_revision: a step that carries one is not encodable and fails the scenario.
func imageHistoryTool(s step) (tool string, args map[string]any, ok bool, err error) {
	if !imageHistoryCalls[s.Call] {
		return "", nil, false, nil
	}

	a, err := readImageHistoryStep(s)
	if err != nil {
		return "", nil, true, err
	}

	args = map[string]any{"domain": "image"}
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

	set("image_id", a.ImageID)

	switch s.Call {
	case "ImageRevert":
		args["action"] = "revert"

		set("revision", a.Revision)
		set("name", a.Checkpoint)
		set("dry_run", a.DryRun)
	case "ImageDiff":
		args["action"] = "diff"

		set("revision", a.From)
		set("name", a.FromCheckpoint)
		set("until", a.Until)
	case "ImageCheckpointList":
		args["action"] = "checkpoints"
	case "ImageCheckpointCreate":
		args["action"] = "checkpoint"

		set("name", a.Name)
		set("revision", a.Revision)

		if a.Note != nil {
			set("note", *a.Note)
		}
	case "ImageCheckpointRename":
		args["action"] = "rename_checkpoint"

		set("name", a.Name)
		set("new_name", a.NewName)

		if a.Note != nil {
			set("note", *a.Note)
		}
	case "ImageCheckpointRemove":
		args["action"] = "remove_checkpoint"

		set("name", a.Name)
	}

	if a.BaseRevision != 0 {
		return "", nil, true, errNoBaseOnTools
	}

	return "history", args, true, nil
}

// imageHistoryCommand maps a step to a command of the CLI.
func imageHistoryCommand(s step) (name string, argv []string, ok bool, err error) {
	if !imageHistoryCalls[s.Call] {
		return "", nil, false, nil
	}

	a, err := readImageHistoryStep(s)
	if err != nil {
		return "", nil, true, err
	}

	argv = []string{a.ImageID}
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

	switch s.Call {
	case "ImageRevert":
		number("revision", a.Revision)
		flag("checkpoint", a.Checkpoint)
		number("base", a.BaseRevision)

		if a.DryRun {
			argv = append(argv, "--dry-run")
		}

		return "image:revert", argv, true, nil
	case "ImageDiff":
		number("from", a.From)
		flag("from-checkpoint", a.FromCheckpoint)
		number("until", a.Until)

		return "image:diff", argv, true, nil
	case "ImageCheckpointList":
		return "image:checkpoint:list", argv, true, nil
	case "ImageCheckpointCreate":
		flag("name", a.Name)
		number("revision", a.Revision)

		if a.Note != nil {
			argv = append(argv, "--note", *a.Note)
		}

		return "image:checkpoint:create", argv, true, nil
	case "ImageCheckpointRename":
		flag("name", a.Name)
		flag("new-name", a.NewName)

		if a.Note != nil {
			argv = append(argv, "--note", *a.Note)
		}

		return "image:checkpoint:rename", argv, true, nil
	case "ImageCheckpointRemove":
		flag("name", a.Name)

		return "image:checkpoint:remove", argv, true, nil
	}

	return "", nil, false, nil
}

// The scenario above would pass if every surface answered nothing: this reads what each step must hold.
func TestImageHistoryScenarioSaysWhatItShould(t *testing.T) {
	steps, fixtures := loadScenario(t, "image-history")
	got := run(t, newDirect(t), steps, fixtures)

	for i, want := range map[int][]string{
		7:  {`"name":"auto local:contract 2026`, `"name":"two layers"`, `"auto":true`},
		8:  {`"count":1`, `"verb":"changed"`, `"noun":"layer"`},
		9:  {`"count":1`, `"verb":"changed"`},
		11: {`"dry_run":true`, `"applied":false`, `"target":3`},
		12: {`"applied":true`, `"kind":"revert"`, `"revision":6`},
		13: {`"same":true`, `"applied":false`},
		14: {`"kind":"undo"`, `"step":6`},
		15: {`"applied":true`, `"target":3`},
		18: {`"name":"keeper"`, `"note":"kept"`},
	} {
		for _, frag := range want {
			if !strings.Contains(got[i].result, frag) {
				t.Errorf("step %d (%s) lacks %s: %s", i, steps[i].Call, frag, got[i].result)
			}
		}
	}

	for _, i := range []int{6, 10, 16, 17, 20} {
		if got[i].fail == "" {
			t.Errorf("step %d (%s) was accepted", i, steps[i].Call)
		}
	}
}
