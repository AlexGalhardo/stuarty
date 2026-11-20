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
	// Domain is the domain whose commands are listed or explained: one of SDD-000 section 1.1, "video" when empty.
	// With no Name and no Domain the result also lists the domains (SDD-003 section 4.8).
	Domain string `json:"domain,omitempty"`
}

// DomainSummary is a domain in the discovery list: its description and how many edit operations it has.
type DomainSummary struct {
	Name       string `json:"name"`
	Summary    string `json:"summary"`
	Operations int    `json:"operations"`
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
	// Domains is every domain with its operation count, when neither Name nor Domain was asked.
	Domains []DomainSummary `json:"domains,omitempty"`
	Text    string          `json:"text"`
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
		if req.Domain == "" {
			return Discovery(s.reg), nil
		}

		res := commandList(reg)
		if len(res.Commands) == 0 {
			res.Text = "No edit operations in the domain " + reg.Domain() + " yet."
		}

		return res, nil
	}

	cmd, ok := reg.Get(req.Name)
	if (!ok || cmd.Internal) && req.Domain == "" {
		// No domain asked: a name that exists in exactly one other domain is that domain's command.
		if other, c, found := uniqueInOtherDomain(s.reg, req.Name); found {
			reg, cmd, ok = other, c, true
		}
	}

	if !ok || cmd.Internal {
		return nil, unknownCommand(reg, req.Domain, req.Name)
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

// uniqueInOtherDomain finds a public command named name in the domains other than the timeline one, only when exactly
// one of them has it; an ambiguous name stays unknown, and the hint points at the domain parameter.
func uniqueInOtherDomain(video *command.Registry, name string) (*command.Registry, *command.Command, bool) {
	var (
		reg   *command.Registry
		found *command.Command
		n     int
	)

	for _, d := range command.Domains() {
		if d.Name == command.DomainVideo {
			continue
		}

		r, err := registryFor(video, d.Name)
		if err != nil {
			continue
		}

		if c, ok := r.Get(name); ok && !c.Internal {
			reg, found = r, c
			n++
		}
	}

	return reg, found, n == 1
}

// unknownCommand is E_UNKNOWN_COMMAND listing the public names only.
func unknownCommand(reg *command.Registry, domain, name string) *Error {
	known := make([]string, 0)

	for _, c := range reg.List() {
		if !c.Internal {
			known = append(known, c.Name)
		}
	}

	if len(known) == 0 {
		return newError(command.CodeUnknownCommand, "there is no command named %q.", name).
			withHint("this domain has no edit operations yet; command_help without a domain lists the domains").
			withDetail("known", known)
	}

	hint := "use one of the known commands: " + strings.Join(known, ", ")
	if domain == "" {
		hint += "; the operations of another domain are listed with the domain parameter (for example domain=image)"
	}

	return newError(command.CodeUnknownCommand, "there is no command named %q.", name).
		withHint("%s", hint).withDetail("known", known)
}

// Discovery is the first discovery call: command_help with no name and no domain. It lists the operations of the
// timeline registry, then one line per domain and the structured member (SDD-003 section 4.8). It needs no store, so
// the budget check of NFR-020 measures it without a workspace.
func Discovery(video *command.Registry) *CommandHelpResult {
	res := commandList(video)

	var b strings.Builder

	b.WriteString(res.Text)
	b.WriteString("\n\nDomains (command_help domain=<name> lists the operations of one):")

	for _, d := range command.Domains() {
		reg, _ := registryFor(video, d.Name)
		n := 0

		for _, c := range reg.List() {
			if !c.Internal {
				n++
			}
		}

		res.Domains = append(res.Domains, DomainSummary{Name: d.Name, Summary: d.Summary, Operations: n})
		fmt.Fprintf(&b, "\n%s: %s", d.Name, d.Summary)

		if n > 0 {
			fmt.Fprintf(&b, " (%d operations)", n)
		}
	}

	res.Text = b.String()

	return res
}

// registryOf is the registry of a domain: the timeline's for video (the default), the image documents' for image,
// and an empty one for a domain that has no edit commands yet.
func (s *svc) registryOf(domain string) (*command.Registry, *Error) {
	return registryFor(s.reg, domain)
}

func registryFor(video *command.Registry, domain string) (*command.Registry, *Error) {
	switch domain {
	case "", command.DomainVideo:
		return video, nil
	case command.DomainImage:
		return coreimage.Registry(), nil
	}

	if !command.KnownDomain(domain) {
		return nil, newError(command.CodeValidation, "the domain %q is not one of the domains of the product.", domain).
			withPath("domain").withHint("use one of %s", strings.Join(command.DomainNames(), ", "))
	}

	return command.NewRegistry(domain), nil
}
