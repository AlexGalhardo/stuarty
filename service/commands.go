package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// CommandHelpRequest asks for one command or for the list of them.
type CommandHelpRequest struct {
	Name string `json:"name,omitempty"` // empty: list every public command
	// Domain is the kind of document the commands edit: "video" (default) or "image" (SDD-021).
	Domain string `json:"domain,omitempty"`
}

// CommandSummary is a command in the list.
type CommandSummary struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Scope   string `json:"scope"` // "edit" | "media"
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

	reg, derr := s.registryOf(req.Domain)
	if derr != nil {
		return nil, derr
	}

	if req.Name == "" {
		return commandList(reg), nil
	}

	cmd, ok := reg.Get(req.Name)
	if !ok || cmd.Internal {
		return nil, unknownCommand(reg, req.Name)
	}

	text, err := reg.Help(req.Name)
	if err != nil {
		return nil, asError(err)
	}

	schemaRaw, err := reg.Schema(req.Name)
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

func commandList(reg *command.Registry) *CommandHelpResult {
	res := &CommandHelpResult{Commands: []CommandSummary{}}

	var b strings.Builder

	for _, c := range reg.List() {
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
	return CommandSummary{Name: c.Name, Summary: c.Summary, Scope: string(c.Scope)}
}

// unknownCommand is E_UNKNOWN_COMMAND listing the public names only.
func unknownCommand(reg *command.Registry, name string) *Error {
	known := make([]string, 0)

	for _, c := range reg.List() {
		if !c.Internal {
			known = append(known, c.Name)
		}
	}

	return newError(command.CodeUnknownCommand, "there is no command named %q.", name).
		withHint("use one of the known commands: %s", strings.Join(known, ", ")).withDetail("known", known)
}

// registryOf is the registry of a domain: the video's by default, or the image's.
func (s *svc) registryOf(domain string) (*command.Registry, *Error) {
	switch domain {
	case "", "video":
		return s.reg, nil
	case "image":
		return coreimage.Registry(), nil
	}

	return nil, newError(command.CodeValidation, "the domain %q is not video or image.", domain).
		withPath("domain").withHint("use video or image")
}
