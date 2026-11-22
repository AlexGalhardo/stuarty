package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

var edit = &spec{
	name:     "edit",
	timeline: true,
	summary:  "apply a batch of operations to a project (its lint block is lint_scope fast: lint runs every rule)",
	usage: []string{
		"stuarty edit --project <id> --ops <file|-> [--base n] [--key k]",
		"stuarty edit --project <id> <op> [k=v | k:=json]...",
	},
	example:    "stuarty edit --project prj_7k2m9x4q1b item.move item_id=itm_m1n2p3q4r5 start=2s",
	project:    true,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "ops", Kind: kindString, Usage: "a JSON file with the batch, or - for standard input: " +
			`{"operations":[{"op":"item.move","args":{"item_id":"itm_m1n2p3q4r5","start":"2s"}}]}, ` +
			"or only the array of {op, args}; base_revision, idempotency_key and dry_run may sit beside operations"},
		{Name: "base", Kind: kindInt, Usage: "the revision the batch was planned against (default: the current one)"},
		{Name: "key", Kind: kindString, Usage: "idempotency key"},
	},
	run: runEdit,
}

func runEdit(ctx context.Context, c *call) (*output, error) {
	req, err := editRequest(c)
	if err != nil {
		return nil, err
	}

	if !c.args.has("base") && !req.baseGiven {
		cur, gerr := c.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: c.projectID, TimelineID: c.timelineID})
		if gerr != nil {
			return nil, gerr
		}

		req.BaseRevision = cur.Revision
	}

	res, err := c.svc.Edit(ctx, req.EditRequest)
	if err != nil {
		return nil, err
	}

	return &output{value: res, text: service.EditText(res)}, nil
}

// batchRequest is an EditRequest plus whether the batch named its own base.
type batchRequest struct {
	service.EditRequest
	baseGiven bool
}

// batchObject is the object form of --ops (SDD-003 section 5.5).
type batchObject struct {
	Operations     []command.Operation `json:"operations"`
	BaseRevision   *int64              `json:"base_revision"`
	IdempotencyKey string              `json:"idempotency_key"`
	DryRun         bool                `json:"dry_run"`
}

// editRequest builds the EditRequest from either input form of SDD-003
// section 5.5. Flags override the members of the object form.
func editRequest(c *call) (*batchRequest, error) {
	req := &batchRequest{ProjectID: c.projectID, TimelineID: c.timelineID}
	positional := c.args.Positional

	switch {
	case c.args.has("ops") && len(positional) > 0:
		return nil, validation("ops", "give either --ops or an operation, not both.")
	case c.args.has("ops"):
		raw, err := c.readOps(c.args.str("ops"))
		if err != nil {
			return nil, err
		}

		if err := decodeBatch(raw, req); err != nil {
			return nil, err
		}
	case len(positional) > 0:
		op, err := shorthandOperation(positional)
		if err != nil {
			return nil, err
		}

		req.Operations = []command.Operation{op}
	default:
		return nil, validation("ops", "pass --ops <file|-> or an operation such as item.move item_id=itm_...")
	}

	if c.args.has("base") {
		req.BaseRevision, req.baseGiven = int64(c.args.integer("base")), true
	}

	if c.args.has("key") {
		req.IdempotencyKey = c.args.str("key")
	}

	if c.args.has("dry-run") {
		req.DryRun = c.args.boolean("dry-run")
	}

	return req, nil
}

// readOps reads the batch from a file of the current directory or from standard input.
func (c *call) readOps(src string) ([]byte, error) {
	var (
		raw []byte
		err error
	)

	if src == "-" {
		raw, err = io.ReadAll(c.rt.Stdin)
	} else {
		raw, err = os.ReadFile(src)
	}

	if err != nil {
		return nil, validation("ops", "cannot read the batch %s: %s.", src, readReason(err))
	}

	return raw, nil
}

func decodeBatch(raw []byte, req *batchRequest) error {
	trimmed := bytes.TrimSpace(raw)

	if len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &req.Operations); err != nil {
			return validation("ops", "the batch is not a JSON array of operations: %s.", command.JSONProblem(err))
		}

		return nil
	}

	var obj batchObject

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&obj); err != nil {
		return validation("ops", "the batch is neither a JSON array nor an object with operations: %s.", command.JSONProblem(err))
	}

	req.Operations, req.IdempotencyKey, req.DryRun = obj.Operations, obj.IdempotencyKey, obj.DryRun

	if obj.BaseRevision != nil {
		req.BaseRevision, req.baseGiven = *obj.BaseRevision, true
	}

	return nil
}

// shorthandOperation reads "<op> key=value key:=json ..." (SDD-003 section 5.5
// form 2). "=" is always a string; ":=" is a JSON literal.
func shorthandOperation(words []string) (command.Operation, error) {
	args := map[string]json.RawMessage{}

	for _, w := range words[1:] {
		key, raw, err := splitAssignment(w)
		if err != nil {
			return command.Operation{}, err
		}

		if _, dup := args[key]; dup {
			return command.Operation{}, validation("args."+key, "the argument %q is given twice.", key)
		}

		args[key] = raw
	}

	body, err := json.Marshal(args)
	if err != nil {
		return command.Operation{}, err
	}

	return command.Operation{Op: words[0], Args: body}, nil
}

func splitAssignment(word string) (string, json.RawMessage, error) {
	if key, lit, ok := strings.Cut(word, ":="); ok && key != "" && !strings.Contains(key, "=") {
		if !json.Valid([]byte(lit)) {
			return "", nil, validation("args."+key, "the value of %s is not valid JSON.", key)
		}

		return key, json.RawMessage(lit), nil
	}

	key, val, ok := strings.Cut(word, "=")
	if !ok || key == "" {
		return "", nil, validation("", "an operation argument is key=value or key:=json, got %q.", word)
	}

	raw, err := json.Marshal(val)
	if err != nil {
		return "", nil, err
	}

	return key, raw, nil
}
