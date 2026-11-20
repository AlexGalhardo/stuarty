package contract

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

// The writing operations (SDD-019) on the four drivers. The switches of drivers_test.go end by asking these
// functions, which keeps the mapping of one domain in one place.

// writingArgs is every member a step of the writing domain can carry.
type writingArgs struct {
	ProjectID      string   `json:"project_id"`
	ItemID         string   `json:"item_id"`
	Title          string   `json:"title"`
	TextKind       string   `json:"text_kind"`
	Text           string   `json:"text"`
	Body           string   `json:"body"`
	Network        string   `json:"network"`
	BaseRevision   *int     `json:"base_revision"`
	Revision       int      `json:"revision"`
	Before         int      `json:"before"`
	Limit          int      `json:"limit"`
	From           int      `json:"from"`
	To             int      `json:"to"`
	WordsPerMinute int      `json:"words_per_minute"`
	State          string   `json:"state"`
	ConnectionID   string   `json:"connection_id"`
	Model          string   `json:"model"`
	Instruction    string   `json:"instruction"`
	MaxTokens      int      `json:"max_tokens"`
	Temperature    *float64 `json:"temperature"`
	DryRun         bool     `json:"dry_run"`
}

// isWritingStep reports whether a step calls a writing operation. The mappings below decode only those: another
// domain may give the same member another type (the from of a motion range is a time, of a comparison a number).
func isWritingStep(call string) bool {
	switch call {
	case "TextList", "TextGet", "TextWrite", "TextRevisions", "TextRevert", "TextDraft", "CaptionValidate",
		"ScriptEstimate":
		return true
	}

	return false
}

func writingStep(s step) (writingArgs, error) {
	var a writingArgs

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return a, err
		}
	}

	return a, nil
}

// writingCall runs a step on a Service. ok is false when the step is not a writing operation.
func writingCall(ctx context.Context, svc service.Service, s step) (res any, ok bool, err error) {
	switch s.Call {
	case "TextList":
		res, err = invoke(ctx, s, svc.TextList)
	case "TextGet":
		res, err = invoke(ctx, s, svc.TextGet)
	case "TextWrite":
		res, err = invoke(ctx, s, svc.TextWrite)
	case "TextRevisions":
		res, err = invoke(ctx, s, svc.TextRevisions)
	case "TextRevert":
		res, err = invoke(ctx, s, svc.TextRevert)
	case "TextDraft":
		res, err = invoke(ctx, s, svc.TextDraft)
	case "CaptionValidate":
		res, err = invoke(ctx, s, svc.CaptionValidate)
	case "ScriptEstimate":
		res, err = invoke(ctx, s, svc.ScriptEstimate)
	default:
		return nil, false, nil
	}

	return res, true, err
}

// writingTool maps a step to a call of an MCP tool: project_item with an action, or job_start with the kind
// text.draft. ok is false for a step that is not a writing one.
func writingTool(s step) (tool string, args map[string]any, ok bool, err error) {
	if !isWritingStep(s.Call) {
		return "", nil, false, nil
	}

	a, err := writingStep(s)
	if err != nil {
		return "", nil, true, err
	}

	args = map[string]any{"project_id": a.ProjectID}
	set := func(k, v string) {
		if v != "" {
			args[k] = v
		}
	}
	number := func(k string, n int) {
		if n != 0 {
			args[k] = n
		}
	}

	switch s.Call {
	case "TextList":
		args["action"] = "text"

		set("text_kind", a.TextKind)
	case "TextGet":
		args["action"] = "text"

		set("item_id", a.ItemID)
		number("revision", a.Revision)
	case "TextWrite":
		args["action"] = "text"

		set("item_id", a.ItemID)
		set("title", a.Title)
		set("text_kind", a.TextKind)

		args["text"] = a.Text

		if a.BaseRevision != nil {
			args["revision"] = *a.BaseRevision
		}
	case "TextRevisions":
		args["action"] = "revisions"

		set("item_id", a.ItemID)
		number("revision", a.From)
		number("until", a.To)
	case "TextRevert":
		args["action"] = "revert"

		set("item_id", a.ItemID)
		number("revision", a.Revision)
	case "CaptionValidate", "ScriptEstimate":
		args["action"] = "check"

		set("item_id", a.ItemID)
		set("network", a.Network)
		number("wpm", a.WordsPerMinute)

		if a.Text != "" {
			args["text"] = a.Text
		}
	case "TextDraft":
		inner := map[string]any{
			"item_id": a.ItemID, "connection_id": a.ConnectionID, "model": a.Model,
			"instruction": a.Instruction,
		}
		if a.Network != "" {
			inner["network"] = a.Network
		}

		if a.MaxTokens != 0 {
			inner["max_tokens"] = a.MaxTokens
		}

		return "job_start", map[string]any{"project_id": a.ProjectID, "kind": "text.draft", "args": inner}, true, nil
	default:
		return "", nil, false, nil
	}

	return "project_item", args, true, nil
}

// writingCommand maps a step to a command of the CLI.
func writingCommand(s step) (name string, argv []string, stdin string, ok bool, err error) {
	if !isWritingStep(s.Call) {
		return "", nil, "", false, nil
	}

	a, err := writingStep(s)
	if err != nil {
		return "", nil, "", true, err
	}

	flag := func(k, v string) []string {
		if v == "" {
			return nil
		}

		return []string{"--" + k, v}
	}
	number := func(k string, n int) []string {
		if n == 0 {
			return nil
		}

		return []string{"--" + k, strconv.Itoa(n)}
	}
	item := func() []string { return nameArg(a.ItemID) }

	var out []string

	switch s.Call {
	case "TextList":
		out = append(flag("project", a.ProjectID), flag("kind", a.TextKind)...)
		out = append(out, flag("state", a.State)...)

		return "text:list", append(out, number("limit", a.Limit)...), "", true, nil
	case "TextGet":
		out = append(item(), flag("project", a.ProjectID)...)

		return "text:show", append(out, number("revision", a.Revision)...), "", true, nil
	case "TextWrite":
		out = append(item(), flag("project", a.ProjectID)...)
		out = append(out, flag("title", a.Title)...)
		out = append(out, flag("kind", a.TextKind)...)

		if a.BaseRevision != nil {
			out = append(out, "--base", strconv.Itoa(*a.BaseRevision))
		}

		return "text:write", append(out, "--text", "-"), a.Text, true, nil
	case "TextRevisions":
		out = append(item(), flag("project", a.ProjectID)...)
		out = append(out, number("from", a.From)...)

		return "text:revisions", append(out, number("until", a.To)...), "", true, nil
	case "TextRevert":
		out = append(item(), flag("project", a.ProjectID)...)

		return "text:revert", append(out, number("revision", a.Revision)...), "", true, nil
	case "TextDraft":
		out = append(item(), flag("project", a.ProjectID)...)
		out = append(out, flag("connection", a.ConnectionID)...)
		out = append(out, flag("model", a.Model)...)
		out = append(out, flag("instruction", a.Instruction)...)
		out = append(out, flag("network", a.Network)...)
		out = append(out, number("max-tokens", a.MaxTokens)...)

		return "text:draft", append(out, boolFlag("dry-run", a.DryRun)...), "", true, nil
	case "CaptionValidate":
		out = append(item(), flag("project", a.ProjectID)...)
		out = append(out, flag("network", a.Network)...)

		if a.Text != "" {
			return "caption:validate", append(out, "--text", "-"), a.Text, true, nil
		}

		if a.Body != "" {
			return "caption:validate", append(out, "--body", "-"), a.Body, true, nil
		}

		return "caption:validate", out, "", true, nil
	case "ScriptEstimate":
		out = append(item(), flag("project", a.ProjectID)...)
		out = append(out, number("wpm", a.WordsPerMinute)...)

		if a.Text != "" {
			return "script:estimate", append(out, "--text", "-"), a.Text, true, nil
		}

		return "script:estimate", out, "", true, nil
	}

	return "", nil, "", false, nil
}
