package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	vukem "github.com/AlexGalhardo/vukem"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// call is what a handler receives: the connected Service, the parsed flags
// and the project the command acts on.
type call struct {
	rt        *Runtime
	svc       service.Service
	args      *parsed
	projectID string
}

type handler func(ctx context.Context, c *call) (*output, error)

// spec describes one command: its flags, its help and its handler.
type spec struct {
	name    string
	summary string
	usage   []string
	example string
	// flags are the flags beyond the global ones.
	flags []flagDef
	// globals names the global flags the command accepts (see globalFlag).
	globals []string
	// project is true when the command acts on a project: the id comes from
	// the first positional argument, --project or VUKEM_VIDEO_PROJECT.
	project bool
	// projectPositional lets the first positional argument be the project id.
	projectPositional bool
	// positional is true when positional arguments are allowed beyond the project id.
	positional bool
	// write is true for a command that writes or starts a job.
	write bool
	// pending names the task that delivers the command; the handler is a stub until then.
	pending string
	run     handler
}

// globalFlag returns a global flag of SDD-003 section 5.1.
func globalFlag(name string) flagDef {
	switch name {
	case "json":
		return flagDef{Name: "json", Kind: kindBool, Default: "false", Usage: "print one JSON document"}
	case "project":
		return flagDef{Name: "project", Kind: kindString, Usage: "project id (default $" + EnvProject + ")"}
	case "workspace":
		return flagDef{
			Name:  "workspace",
			Kind:  kindString,
			Usage: "workspace directory (default $VUKEM_VIDEO_HOME, else ~/.vukem-video)",
		}
	case "actor":
		return flagDef{
			Name:    "actor",
			Kind:    kindString,
			Default: DefaultActor,
			Usage:   "label of the actor local:<label>",
		}
	case "key-file":
		return flagDef{Name: "key-file", Kind: kindString, Usage: "file holding an API key for a remote server"}
	case "dry-run":
		return flagDef{Name: "dry-run", Kind: kindBool, Default: "false", Usage: "validate and report, change nothing"}
	case "wait":
		return flagDef{
			Name:    "wait",
			Kind:    kindBool,
			Default: "true",
			Usage:   "block until the job is final (--no-wait to return at once)",
		}
	case "timeout":
		return flagDef{Name: "timeout", Kind: kindDuration, Default: "10m", Usage: "longest --wait"}
	}

	panic("cli: unknown global flag " + name)
}

// baseGlobals are accepted by every command.
var baseGlobals = []string{"json", "workspace", "actor", "key-file"}

func (s *spec) flagDefs() []flagDef {
	names := slices.Clone(baseGlobals)
	if s.project {
		names = append(names, "project")
	}

	names = append(names, s.globals...)

	defs := make([]flagDef, 0, len(names)+len(s.flags))

	for _, n := range names {
		defs = append(defs, globalFlag(n))
	}

	return append(defs, s.flags...)
}

// Execute runs one command by name with its arguments (flags and positional
// arguments in any order) and returns the exit code. Nothing but the result
// reaches standard output in --json mode, and nothing reaches standard error.
func (rt *Runtime) Execute(ctx context.Context, name string, args []string) int {
	s := lookup(name)
	if s == nil {
		return writeError(rt, jsonRequested(args), &service.Error{
			Code: command.CodeUnknownCommand, Message: fmt.Sprintf("there is no command %q.", name),
			Hint: "run vukem-video with no arguments to list the commands",
		})
	}

	if s.pending != "" {
		return rt.pendingCommand(s, args)
	}

	p, err := parseArgs(s.flagDefs(), args)
	if err != nil {
		return writeError(rt, jsonRequested(args), err)
	}

	asJSON := p.boolean("json")

	if p.Help {
		fmt.Fprint(rt.Stdout, s.help())

		return ExitOK
	}

	c, err := rt.prepare(s, p)
	if err != nil {
		return writeError(rt, asJSON, err)
	}

	conn, err := rt.connect(ctx, s, p)
	if err != nil {
		return writeError(rt, asJSON, err)
	}

	defer func() {
		if conn.Close != nil {
			_ = conn.Close()
		}
	}()

	c.svc = conn.Service

	label, _ := actorFrom(p.str("actor"))
	ctx = service.WithCaller(ctx, service.Caller{Actor: "local:" + label, Scopes: service.AllScopes})

	out, err := s.run(ctx, c)
	if err != nil {
		return writeError(rt, asJSON, err)
	}

	if s.write && conn.InProcess && !asJSON {
		fmt.Fprintf(rt.Stderr, "note: no server owns %s; this ran in process. "+
			"Run \"vukem-video serve\" to share the workspace with the UI and other clients.\n", conn.Workspace)
	}

	return writeOutput(rt, asJSON, out)
}

// pendingCommand answers a command whose task has not landed: help works, and
// anything else is a clear failure that names the task. It never calls a Service.
func (rt *Runtime) pendingCommand(s *spec, args []string) int {
	if slices.Contains(args, "--help") || slices.Contains(args, "-h") {
		fmt.Fprint(rt.Stdout, s.help())

		return ExitOK
	}

	return writeError(rt, jsonRequested(args), &service.Error{
		Code:    command.CodeInternal,
		Message: fmt.Sprintf("%s is not available yet: it arrives with task %s.", s.name, s.pending),
	})
}

// jsonRequested reports --json before the arguments are parsed, so a bad
// invocation still gets a JSON error document when the caller asked for JSON.
func jsonRequested(args []string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}

		if a == "--json" || a == "--json=true" {
			return true
		}
	}

	return false
}

// prepare resolves the project id and checks the positional arguments.
func (rt *Runtime) prepare(s *spec, p *parsed) (*call, error) {
	c := &call{rt: rt, args: p}

	if _, err := actorFrom(p.str("actor")); err != nil {
		return nil, err
	}

	if !s.project {
		if len(p.Positional) > 0 && !s.positional {
			return nil, validation("", "%s takes no positional argument, got %q.", s.name, p.Positional[0])
		}

		return c, nil
	}

	id, rest, err := projectID(rt, s, p)
	if err != nil {
		return nil, err
	}

	c.projectID = id
	p.Positional = rest

	if len(rest) > 0 && !s.positional {
		return nil, validation("", "%s takes no positional argument, got %q.", s.name, rest[0])
	}

	return c, nil
}

// projectID takes the project from --project or the environment. When the
// command takes the id as its first positional argument (project:show), the
// positional form is accepted too and a conflict is E_VALIDATION (SDD-003 5.4).
func projectID(rt *Runtime, s *spec, p *parsed) (id string, rest []string, err error) {
	rest = p.Positional
	flagID := p.str("project")

	if s.projectPositional && len(rest) > 0 {
		if flagID != "" && flagID != rest[0] {
			return "", nil, validation(
				"project",
				"the project is given twice and the two differ: %q and %q.",
				rest[0],
				flagID,
			)
		}

		flagID, rest = rest[0], rest[1:]
	}

	if flagID == "" {
		flagID = rt.Getenv(EnvProject)
	}

	if flagID == "" {
		return "", nil, &service.Error{
			Code: command.CodeValidation, Path: "project", Message: "the command needs a project.",
			Hint: "pass --project <id> or set " + EnvProject,
		}
	}

	return flagID, rest, nil
}

// connect resolves the actor and finds the Service (SDD-003 section 5.2).
func (rt *Runtime) connect(ctx context.Context, s *spec, p *parsed) (*Connection, error) {
	label, err := actorFrom(p.str("actor"))
	if err != nil {
		return nil, err
	}

	conn, err := rt.Resolver.Resolve(ctx, Target{
		Workspace: p.str("workspace"), Actor: label, KeyFile: p.str("key-file"), Write: s.write,
	})
	if err != nil {
		return nil, err
	}

	if conn == nil || conn.Service == nil {
		return nil, errors.New("cli: the resolver returned no service")
	}

	return conn, nil
}

// help renders --help in at most 40 lines: description, usage, flags, one example.
func (s *spec) help() string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s: %s\n\nusage:\n", s.name, s.summary)

	for _, u := range s.usage {
		fmt.Fprintf(&b, "  %s\n", u)
	}

	b.WriteString("\nflags:\n")

	for _, d := range s.flagDefs() {
		def := ""
		if d.Default != "" && d.Kind != kindBool {
			def = fmt.Sprintf(" (default %s)", d.Default)
		}

		fmt.Fprintf(&b, "  --%s  %s%s\n", d.Name, d.Usage, def)
	}

	if s.pending != "" {
		fmt.Fprintf(&b, "\nnot available yet: arrives with task %s\n", s.pending)
	}

	if s.example != "" {
		fmt.Fprintf(&b, "\nexample:\n  %s\n", s.example)
	}

	return b.String()
}

// List prints the short command list of "vukem-video" with no arguments.
func List(w io.Writer) {
	fmt.Fprintln(w, "vukem-video commands:")

	for _, s := range registry() {
		fmt.Fprintf(w, "  %-16s %s\n", s.name, s.summary)
	}

	fmt.Fprintln(w, "\nrun vukem-video <command> --help for the arguments of a command")
}

// adapter makes a spec a vukem.Command. vukem hands every argument to Run
// untouched when the binary inserts "--" after the command name (SDD-003 5.1).
type adapter struct {
	rt *Runtime
	s  *spec
}

func (a adapter) Name() string        { return a.s.name }
func (a adapter) Description() string { return a.s.summary }
func (a adapter) Flags(*flag.FlagSet) {}
func (a adapter) NoBoot() bool        { return true }
func (a adapter) Run(ctx context.Context, _ *vukem.App, args []string) error {
	if code := a.rt.Execute(ctx, a.s.name, args); code != ExitOK {
		return &vukem.ExitError{Code: code}
	}

	return nil
}

// Commands returns the product commands for app.AddCommand. They do not boot
// the application: the Service comes from the Resolver.
func Commands(rt *Runtime) []vukem.Command {
	out := make([]vukem.Command, 0, len(registry()))

	for _, s := range registry() {
		out = append(out, adapter{rt: rt, s: s})
	}

	return out
}
