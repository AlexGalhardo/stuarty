package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// CommandHelpRequest asks for one command or for the list of them.
type CommandHelpRequest struct {
	Name string `json:"name,omitempty"` // empty: list every public command
}

// CommandSummary is a command in the list.
type CommandSummary struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Scope   string `json:"scope"` // "edit" | "media"
	Phase   string `json:"phase"` // "prototype" | "alpha" | "beta"
}

// CommandDetail is one command with its arguments.
type CommandDetail struct {
	CommandSummary
	Description string         `json:"description"`
	ArgsSchema  map[string]any `json:"args_schema"` // JSON Schema 2020-12 (SDD-002 section A.2)
	Example     map[string]any `json:"example"`     // a valid args value
	Errors      []string       `json:"errors"`
}

// CommandHelpResult is the list, or the detail of one command.
type CommandHelpResult struct {
	Commands []CommandSummary `json:"commands,omitempty"` // when Name is empty
	Command  *CommandDetail   `json:"command,omitempty"`  // when Name is set
	Text     string           `json:"text"`
}

// CommandHelp lists the public commands of the build, or explains one. An
// internal command is as unknown as a name that does not exist (SDD-007
// section 5.4 rule 1).
func (s *svc) CommandHelp(ctx context.Context, req CommandHelpRequest) (*CommandHelpResult, error) {
	if _, err := authorize(ctx, "CommandHelp", ""); err != nil {
		return nil, err
	}

	if req.Name == "" {
		return s.commandList(), nil
	}

	cmd, ok := s.reg.Get(req.Name)
	if !ok || cmd.Internal {
		return nil, s.unknownCommand(req.Name)
	}

	text, err := s.reg.Help(req.Name)
	if err != nil {
		return nil, asError(err)
	}

	schemaRaw, err := s.reg.Schema(req.Name)
	if err != nil {
		return nil, asError(err)
	}

	detail := &CommandDetail{
		CommandSummary: summaryOf(cmd), Description: cmd.Description, Errors: cmd.Errors,
		ArgsSchema: map[string]any{}, Example: map[string]any{},
	}

	if detail.Errors == nil {
		detail.Errors = []string{}
	}

	if json.Unmarshal(schemaRaw, &detail.ArgsSchema) != nil ||
		json.Unmarshal([]byte(cmd.Example), &detail.Example) != nil {
		return nil, internalError()
	}

	return &CommandHelpResult{Command: detail, Text: text}, nil
}

func (s *svc) commandList() *CommandHelpResult {
	res := &CommandHelpResult{Commands: []CommandSummary{}}

	var b strings.Builder

	for _, c := range s.reg.List() {
		if c.Internal {
			continue
		}

		sum := summaryOf(c)
		res.Commands = append(res.Commands, sum)
		fmt.Fprintf(&b, "%s [%s] %s\n", sum.Name, sum.Scope, sum.Summary)
	}

	res.Text = strings.TrimRight(b.String(), "\n")

	return res
}

func summaryOf(c *command.Command) CommandSummary {
	return CommandSummary{Name: c.Name, Summary: c.Summary, Scope: string(c.Scope), Phase: string(c.Phase)}
}

// unknownCommand is E_UNKNOWN_COMMAND listing the public names only.
func (s *svc) unknownCommand(name string) *Error {
	known := make([]string, 0)

	for _, c := range s.reg.List() {
		if !c.Internal {
			known = append(known, c.Name)
		}
	}

	return newError(command.CodeUnknownCommand, "there is no command named %q.", name).
		withHint("use one of the known commands: %s", strings.Join(known, ", ")).withDetail("known", known)
}
