package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const maxOutputBytes = 8000

const toolName = "stuarty"

var toolSchema = json.RawMessage(`{"type":"object","properties":{` +
	`"args":{"type":"string","description":"the arguments after the program name, for example: project:list --json"},` +
	`"stdin":{"type":"string","description":"text sent to the standard input of the command, when it reads one"}},` +
	`"required":["args"]}`)

// Runner runs one stuarty command line and returns what it printed and its exit code.
type Runner func(ctx context.Context, args []string, stdin string) (output string, exitCode int, err error)

// commandRecord is one command the agent ran.
type commandRecord struct {
	Args     string `json:"args"`
	Stdin    string `json:"stdin,omitempty"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

// runResult is one run of one scenario, saved as its transcript.
type runResult struct {
	Scenario     string          `json:"scenario"`
	Run          int             `json:"run"`
	Success      bool            `json:"success"`
	Reason       string          `json:"reason,omitempty"`
	Turns        int             `json:"turns"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	Commands     []commandRecord `json:"commands"`
	Messages     []message       `json:"messages"`
}

type agent struct {
	api       *client
	model     string
	maxTokens int
	maxTurns  int
	system    string
	// flow is the world of the first-flow scenario; nil for the others.
	flow *flowEnv
}

// splitArgs splits a command line the way a POSIX shell does for quotes and backslashes, with no
// expansion of any kind: the harness never starts a shell.
func splitArgs(line string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		quote   rune
		inToken bool
		escaped bool
	)

	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)

			escaped = false
		case r == '\\' && quote != '\'':
			escaped, inToken = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inToken = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inToken {
				args = append(args, cur.String())
				cur.Reset()

				inToken = false
			}
		default:
			cur.WriteRune(r)

			inToken = true
		}
	}

	if quote != 0 || escaped {
		return nil, errors.New("unterminated quote or escape")
	}

	if inToken {
		args = append(args, cur.String())
	}

	return args, nil
}

// execute runs the tool call of the agent and returns the text to hand back.
func execute(ctx context.Context, run Runner, input json.RawMessage, rec *runResult) (string, bool) {
	var in struct {
		Args  string `json:"args"`
		Stdin string `json:"stdin"`
	}

	if err := json.Unmarshal(input, &in); err != nil {
		return "the tool input is not valid JSON: " + err.Error(), true
	}

	args, err := splitArgs(in.Args)
	if err != nil || len(args) == 0 {
		return "the args are not a valid command line", true
	}

	out, code, err := run(ctx, args, in.Stdin)
	if err != nil {
		return "the command could not be started: " + err.Error(), true
	}

	if len(out) > maxOutputBytes {
		out = out[:maxOutputBytes] + "\n[output truncated]"
	}

	rec.Commands = append(rec.Commands, commandRecord{Args: in.Args, Stdin: in.Stdin, ExitCode: code, Output: out})

	return fmt.Sprintf("exit code %d\n%s", code, out), code != 0
}

// judge says whether every required command ran with exit code 0.
func judge(sc scenario, cmds []commandRecord) (bool, string) {
	var missing []string

	for _, want := range sc.Required {
		ok := slices.ContainsFunc(cmds, func(c commandRecord) bool {
			args, err := splitArgs(c.Args)

			return err == nil && len(args) > 0 && c.ExitCode == 0 && commandName(args) == want
		})
		if !ok {
			missing = append(missing, want)
		}
	}

	if len(missing) > 0 {
		return false, "never succeeded: " + strings.Join(missing, ", ")
	}

	return true, ""
}

// commandName is the first argument that is not a global flag.
func commandName(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}

	return ""
}

// play runs one scenario once. An API failure ends the run as a failure with the reason; only a
// cancelled context is returned as an error.
func (a *agent) play(ctx context.Context, sc scenario, n int, run Runner) (runResult, error) {
	res := runResult{Scenario: sc.Name, Run: n}
	tools := []toolDef{{
		Name: toolName, InputSchema: toolSchema,
		Description: "Runs one stuarty command and returns its exit code and output.",
	}}
	res.Messages = []message{{Role: "user", Content: []block{{Type: "text", Text: sc.Prompt}}}}

	var before flowBefore

	if sc.Flow {
		var why string
		if before, why = beforeFlow(ctx, a.flow, run); why != "" {
			res.Reason = why

			return res, nil
		}
	}

	var apiFailure string

	for res.Turns < a.maxTurns && apiFailure == "" {
		if err := ctx.Err(); err != nil {
			return res, fmt.Errorf("run cancelled: %w", err)
		}

		res.Turns++

		resp, err := a.api.send(ctx, request{
			Model: a.model, MaxTokens: a.maxTokens, System: a.system, Tools: tools, Messages: res.Messages,
		})
		if err != nil {
			apiFailure = err.Error()

			continue
		}

		res.InputTokens += resp.Usage.InputTokens
		res.OutputTokens += resp.Usage.OutputTokens
		res.Messages = append(res.Messages, message{Role: "assistant", Content: resp.Content})

		var results []block

		for i := range resp.Content {
			b := &resp.Content[i]

			if b.Type != "tool_use" {
				continue
			}

			text, isErr := "unknown tool "+b.Name, true
			if b.Name == toolName {
				text, isErr = execute(ctx, run, b.Input, &res)
			}

			results = append(results, block{Type: "tool_result", ToolUseID: b.ID, Content: text, IsError: isErr})
		}

		if len(results) == 0 {
			break
		}

		res.Messages = append(res.Messages, message{Role: "user", Content: results})
	}

	if apiFailure != "" {
		res.Reason = apiFailure

		return res, nil
	}

	res.Success, res.Reason = judge(sc, res.Commands)
	if res.Success && sc.Flow {
		res.Success, res.Reason = settleFlow(ctx, a.flow, run, before)
	}

	if !res.Success && res.Turns >= a.maxTurns {
		res.Reason += " (turn limit reached)"
	}

	return res, nil
}
