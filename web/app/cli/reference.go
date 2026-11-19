package cli

// FlagDoc is a flag in the generated CLI reference.
type FlagDoc struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
	Usage   string `json:"usage"`
	Repeat  bool   `json:"repeat,omitempty"`
}

// CommandDoc is a command in the generated CLI reference.
type CommandDoc struct {
	Name    string    `json:"name"`
	Summary string    `json:"summary"`
	Usage   []string  `json:"usage"`
	Example string    `json:"example,omitempty"`
	Flags   []FlagDoc `json:"flags,omitempty"`
	// Globals names the global flags the command accepts.
	Globals []string `json:"globals"`
	// Pending names the task that delivers a command that is still a stub.
	Pending string `json:"pending,omitempty"`
	// Person marks a command only a person in the app may succeed at; the skill for agents leaves it out.
	Person bool `json:"-"`
}

// ReferenceDoc is what the documentation site renders as the CLI reference
// (SDD-010 section 8): the commands, the global flags and the exit codes.
type ReferenceDoc struct {
	Globals   []FlagDoc    `json:"globals"`
	ExitCodes []ExitDoc    `json:"exit_codes"`
	Commands  []CommandDoc `json:"commands"`
	// ProcessCommands are the commands the binary runs itself: serve, open and the users.
	ProcessCommands []CommandDoc `json:"process_commands"`
}

// ExitDoc is one exit code.
type ExitDoc struct {
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

func flagDoc(d flagDef) FlagDoc {
	return FlagDoc{Name: d.Name, Type: kindName(d.Kind), Default: d.Default, Usage: d.Usage, Repeat: d.Repeat}
}

// Reference describes every command of the binary, from the same declarations
// that --help and the argument parser use, so the reference cannot drift.
func Reference() ReferenceDoc {
	doc := ReferenceDoc{
		ExitCodes: []ExitDoc{
			{ExitOK, "success"},
			{
				ExitFailure,
				"a failure that retrying will not fix, a failed file of media:import, a failed job, or lint --strict with an error finding",
			},
			{
				ExitTransient,
				"a retryable error (E_REVISION_CONFLICT, E_WORKSPACE_LOCKED, E_QUEUE_FULL, E_RATE_LIMITED, E_ASSET_NOT_READY) or a --wait that timed out",
			},
		},
	}

	for _, n := range []string{"json", "project", "workspace", "actor", "key-file", "dry-run", "wait", "timeout"} {
		doc.Globals = append(doc.Globals, flagDoc(globalFlag(n)))
	}

	for _, s := range registry() {
		c := CommandDoc{
			Name:    s.name,
			Summary: s.summary,
			Usage:   s.usage,
			Example: s.example,
			Pending: s.pending,
			Person:  s.person,
		}

		c.Globals = append(c.Globals, baseGlobals...)
		if s.project {
			c.Globals = append(c.Globals, "project")
		}

		c.Globals = append(c.Globals, s.globals...)

		for _, f := range s.flags {
			c.Flags = append(c.Flags, flagDoc(f))
		}

		doc.Commands = append(doc.Commands, c)
	}

	for i := range processCommands {
		p := &processCommands[i]
		doc.ProcessCommands = append(doc.ProcessCommands, CommandDoc{
			Name: p.name, Summary: p.summary, Usage: p.usage, Example: p.example, Globals: []string{},
		})
	}

	return doc
}
