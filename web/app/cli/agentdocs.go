package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
)

// AgentDocsInputs is what the generator of the agent material reads from the
// product: the command registry, the lint catalogue and the commands of this
// CLI (SDD-003 section 8.2). The generator and its tests call it.
func AgentDocsInputs() agentdocs.Inputs {
	in := agentdocs.Inputs{
		Commands: command.Default().List(), Lint: lint.Catalog(),
		ToolDomains: map[string][]string{}, ToolRoutes: map[string][]string{},
	}

	for _, tool := range mcp.ToolNames() {
		in.ToolDomains[tool] = mcp.ToolDomains(tool)

		for _, d := range command.DomainNames() {
			if mcp.ServesDomain(tool, d) {
				in.ToolRoutes[tool] = append(in.ToolRoutes[tool], d)
			}
		}
	}

	commands := Reference().Commands

	// The commands of the library are for the skills of the library (SDD-018), not for the editor skill, whose
	// size is limited (SDD-003 section 8.1); the MCP tool library and command:help name them.
	inLibrary := map[string]bool{}
	for _, s := range libraryCommands() {
		inLibrary[s.name] = true
	}

	// The commands of the AI provider layer (SDD-016 section 10) wait for the budget epic 13 gives the skill: the
	// editor skill has no room for them. An agent starts a generation with job_start (kind text.generate) and
	// reads these commands from --help.
	for _, s := range providerCommands() {
		inLibrary[s.name] = true
	}

	for i := range commands {
		c := &commands[i]
		if c.Person {
			continue
		}

		usage := ""
		if len(c.Usage) > 0 {
			usage = strings.TrimPrefix(c.Usage[0], "stuarty ")
		}

		cmd := agentdocs.CLICommand{
			Name: c.Name, Domain: c.Domain, Summary: c.Summary, Usage: usage, Pending: c.Pending != "",
		}
		in.GuideCLI = append(in.GuideCLI, cmd)

		if !inLibrary[c.Name] {
			in.CLI = append(in.CLI, cmd)
		}
	}

	return in
}

var agentDocs = &spec{
	name:    "agent:docs",
	summary: "print or write the material for agents: the skill, the AGENTS.md snippet, the MCP instructions",
	usage: []string{
		"stuarty agent:docs --print skill|agents|instructions",
		"stuarty agent:docs --write <dir>",
		"stuarty agent:docs --print guide:<domain>",
	},
	example: "stuarty agent:docs --write ~/.claude/skills/stuarty",
	offline: true,
	flags: []flagDef{
		{
			Name:  "print",
			Kind:  kindString,
			Usage: "print one of skill, agents or instructions, or guide:<domain> for the page of a domain",
		},
		{Name: "write", Kind: kindString, Usage: "write the three files into this directory"},
	},
	run: runAgentDocs,
}

// agentDocsDoc is the --json document of agent:docs --write.
type agentDocsDoc struct {
	Directory string   `json:"directory"`
	Files     []string `json:"files"`
	Text      string   `json:"text"`
}

func runAgentDocs(_ context.Context, c *call) (*output, error) {
	switch {
	case c.args.has("print") == c.args.has("write"):
		return nil, validation("print", "give exactly one of --print and --write.")
	case c.args.has("print"):
		kind := c.args.str("print")

		if domain, isGuide := strings.CutPrefix(kind, "guide:"); isGuide {
			b, err := agentdocs.Guide(domain)
			if err != nil {
				return nil, validation("print", "--print guide:<domain> takes one of %s, not %q.",
					strings.Join(command.DomainNames(), ", "), domain)
			}

			return &output{
				value: map[string]string{"file": agentdocs.GuideFile(domain), "text": string(b)},
				text:  string(b),
			}, nil
		}

		name, ok := agentdocs.Kinds[kind]
		if !ok {
			return nil, validation("print", "--print is skill, agents, instructions or guide:<domain>, not %q.", kind)
		}

		b, err := agentdocs.File(name)
		if err != nil {
			return nil, err
		}

		return &output{value: map[string]string{"file": name, "text": string(b)}, text: string(b)}, nil
	}

	dir, err := c.rt.abs(c.args.str("write"))
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}

	for _, name := range agentdocs.Files {
		b, err := agentdocs.File(name)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}

	files := slices.Clone(agentdocs.Files)
	text := "wrote " + strings.Join(files, ", ") + " to " + dir

	return &output{value: agentDocsDoc{Directory: dir, Files: files, Text: text}, text: text}, nil
}
