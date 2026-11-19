package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
)

// call is what a handler receives: the connected Service, the parsed flags
// and the project the command acts on.
type call struct {
	rt        *Runtime
	svc       service.Service
	args      *parsed
	projectID string
	// timelineID is --timeline, for a command that takes it.
	timelineID string
	events     EventOpener
	download   Downloader
	// inProcess is true when this process owns the workspace: the person at the
	// terminal of the owner machine, who may install helpers and sweep the workspace.
	inProcess bool
	// workspace and server say which workspace and which server answer (the mcp command reports them).
	workspace, server string
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
	// the first positional argument, --project or STUARTY_PROJECT.
	project bool
	// projectPositional lets the first positional argument be the project id.
	projectPositional bool
	// positional is true when positional arguments are allowed beyond the project id.
	positional bool
	// timeline is true when the command acts on one timeline of the project and takes --timeline (SDD-003 section 1.7).
	timeline bool
	// write is true for a command that writes or starts a job.
	write bool
	// stream is true for a command that owns standard output while it runs (mcp):
	// it prints nothing of its own, and the actor is not fixed by --actor but by
	// the protocol.
	stream bool
	// offline is true for a command that needs no Service: it never opens the
	// workspace and c.svc is nil.
	offline bool
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
			Usage: "workspace directory (default $STUARTY_HOME, else ~/.stuarty)",
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
	case "timeline":
		return flagDef{Name: "timeline", Kind: kindString, Usage: "timeline id; needed when the project has several"}
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

	if s.timeline {
		names = append(names, "timeline")
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
			Hint: unknownHint(name),
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

	if s.offline {
		out, err := s.run(ctx, c)
		if err != nil {
			return writeError(rt, asJSON, err)
		}

		return writeOutput(rt, asJSON, out)
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
	c.events = conn.Events
	c.download = conn.Download
	c.inProcess = conn.InProcess
	c.workspace, c.server = conn.Workspace, conn.Server

	if !s.stream {
		label, _ := actorFrom(p.str("actor"))
		ctx = service.WithCaller(ctx, service.Caller{Actor: "local:" + label, Scopes: service.AllScopes})
	}

	out, err := s.run(ctx, c)
	if err != nil {
		return writeError(rt, asJSON, namingWorkspace(err, conn))
	}

	if s.stream {
		return ExitOK
	}

	if s.write && conn.InProcess && !asJSON {
		fmt.Fprintf(rt.Stderr, "note: no server owns %s; this ran in process. "+
			"Run \"stuarty serve\" to share the workspace with the UI and other clients.\n", conn.Workspace)
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
	c.timelineID = p.str("timeline")
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

// idRuleHelp states the form of an id for the commands that take one (VV-147).
const idRuleHelp = "ids: a prefix (prj_, ast_, trk_, itm_) and 10 characters from 0-9 and a-z without i, l, o and u, such as trk_k3m9x4q1bz"

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

	if s.project {
		b.WriteString("\n" + idRuleHelp + "\n")
	}

	if s.example != "" {
		fmt.Fprintf(&b, "\nexample:\n  %s\n", s.example)
	}

	return b.String()
}

// processCommand is a command the binary dispatches itself, before the
// application is built, because it owns the process or the workspace lock
// (SDD-000 section 1, SDD-003 section 5.4). It is listed and documented here,
// next to the commands of the registry, so --help and the reference cover it.
type processCommand struct {
	name, summary, example string
	usage                  []string
	// noArgs is true for a command that takes no flag and no argument: its configuration is the environment.
	noArgs bool
}

var processCommands = []processCommand{
	{
		name:    "serve",
		summary: "run the server that owns the workspace, configured by environment variables (see the configuration reference)",
		usage:   []string{"stuarty serve"},
		example: "STUARTY_ADDR=127.0.0.1:7878 stuarty serve",
		noArgs:  true,
	},
	{
		name:    "open",
		summary: "open the editor in the browser of the running server",
		usage:   []string{"stuarty open"},
		example: "stuarty open",
		noArgs:  true,
	},
	{
		name:    "user:create",
		summary: "create a user of network mode (the password is asked, never an argument)",
		usage:   []string{"stuarty user:create <email> [--admin]"},
		example: "stuarty user:create ana@example.com --admin",
	},
	{
		name:    "user:passwd",
		summary: "change the password of a user of network mode",
		usage:   []string{"stuarty user:passwd <email>"},
		example: "stuarty user:passwd ana@example.com",
	},
}

func lookupProcess(name string) *processCommand {
	for i := range processCommands {
		if processCommands[i].name == name {
			return &processCommands[i]
		}
	}

	return nil
}

func (p *processCommand) help() string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s: %s\n\nusage:\n", p.name, p.summary)

	for _, u := range p.usage {
		fmt.Fprintf(&b, "  %s\n", u)
	}

	fmt.Fprintf(&b, "\nexample:\n  %s\n", p.example)

	return b.String()
}

// List prints the short command list of "stuarty" with no arguments.
func List(w io.Writer) {
	fmt.Fprintln(w, "stuarty commands:")

	for _, s := range registry() {
		fmt.Fprintf(w, "  %-16s %s\n", s.name, s.summary)
	}

	for i := range processCommands {
		fmt.Fprintf(w, "  %-16s %s\n", processCommands[i].name, processCommands[i].summary)
	}

	fmt.Fprintln(w, "\nrun stuarty help <command> or stuarty <command> --help for the arguments of a command")
}

// Help answers the help requests of the binary before any framework code runs:
// --help, -h and help for the list, "help <command>" and "<process command>
// --help" for one command, --version. A product command with --help is the
// business of Runtime.Execute. handled is false when args are not a help request.
func Help(args []string, stdout, stderr io.Writer) (code int, handled bool) {
	if len(args) == 0 {
		return ExitOK, false
	}

	switch {
	case len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help"):
		List(stdout)

		return ExitOK, true
	case args[0] == "--version" || args[0] == "-v":
		fmt.Fprintln(stdout, "stuarty "+agentdocs.Version)

		return ExitOK, true
	case args[0] == "help" && len(args) >= 2:
		return helpFor(args[1], stdout, stderr), true
	case lookupProcess(args[0]) != nil && (slices.Contains(args[1:], "--help") || slices.Contains(args[1:], "-h")):
		return helpFor(args[0], stdout, stderr), true
	}

	return ExitOK, false
}

func helpFor(name string, stdout, stderr io.Writer) int {
	if s := lookup(name); s != nil {
		fmt.Fprint(stdout, s.help())

		return ExitOK
	}

	if p := lookupProcess(name); p != nil {
		fmt.Fprint(stdout, p.help())

		return ExitOK
	}

	fmt.Fprintf(stderr, "error E_VALIDATION: unknown command %q; run stuarty help to list the commands\n", name)

	return ExitFailure
}

// unknownHint names the closest command, when one is near, and where the list is.
func unknownHint(name string) string {
	best, bestDist := "", 3

	names := make([]string, 0, len(registry())+len(processCommands))
	for _, s := range registry() {
		names = append(names, s.name)
	}

	for i := range processCommands {
		names = append(names, processCommands[i].name)
	}

	for _, n := range names {
		if d := editDistance(name, n); d < bestDist {
			best, bestDist = n, d
		}
	}

	if best == "" {
		return "run stuarty help to list the commands"
	}

	return fmt.Sprintf("did you mean %q? run stuarty help to list the commands", best)
}

// editDistance is the Levenshtein distance of two short command names.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}

		prev = cur
	}

	return prev[len(b)]
}

// RejectArgs answers a command of the process that takes no flag or argument and was given one, with the
// same E_VALIDATION document as every other command, before the command opens or locks anything
// (VV-078). --help and -h are the business of Help.
func RejectArgs(name string, args []string, stdout, stderr io.Writer) (code int, rejected bool) {
	p := lookupProcess(name)
	if p == nil || !p.noArgs || len(args) == 0 {
		return ExitOK, false
	}

	msg := fmt.Sprintf("%s takes no argument, got %q.", name, args[0])
	if strings.HasPrefix(args[0], "-") {
		msg = fmt.Sprintf("unknown flag %q for %s.", args[0], name)
	}

	rt := &Runtime{Stdout: stdout, Stderr: stderr}
	err := &service.Error{
		Code: command.CodeValidation, Message: msg,
		Hint: fmt.Sprintf("%s is configured by environment variables: run stuarty help %s", name, name),
	}

	return writeError(rt, jsonRequested(args), err), true
}

// IsCommand reports whether name is a product command.
func IsCommand(name string) bool { return lookup(name) != nil }

// namingWorkspace adds the workspace this process opened to a not-found answer when no server owns it.
// Without STUARTY_HOME or --workspace that is the default ~/.stuarty, which may not be the
// workspace of a running server, so a project that exists there looks missing (VV-151).
func namingWorkspace(err error, conn *Connection) error {
	se := asServiceError(err)
	if !conn.InProcess || conn.Workspace == "" || se.Code != command.CodeNotFound {
		return err
	}

	out := *se
	note := fmt.Sprintf("this command read the workspace %s, which no server owns; "+
		"to read another one use --workspace <dir> or set STUARTY_HOME", conn.Workspace)

	if out.Hint == "" {
		out.Hint = note
	} else {
		out.Hint += "; " + note
	}

	return &out
}
