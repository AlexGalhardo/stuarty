package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// commandDomains is the domain of every command of the table (SDD-000 section 1.1, SDD-003 section 5.1). It is
// what groups the commands: `stuarty <domain> <verb>` runs the command of that domain whose name ends in the
// verb, and the commands keep the names they always had (the surface is frozen). TestEveryCommandHasADomain
// fails when a command has none or the table names a command that does not exist, and the contract test checks
// each entry against the domain of the Service operation the command calls.
var commandDomains = map[string]string{
	"project:list": command.DomainProject, "project:create": command.DomainProject,
	"project:show": command.DomainProject, "project:trash": command.DomainProject,
	"project:restore": command.DomainProject, "project:describe": command.DomainProject,
	"project:duplicate": command.DomainProject, "item:add": command.DomainProject,
	"item:move": command.DomainProject, "item:rename": command.DomainProject,
	"item:trash": command.DomainProject, "item:restore": command.DomainProject,
	"brief:show": command.DomainProject, "brief:set": command.DomainProject,
	"briefing:questions": command.DomainProject, "briefing:answer": command.DomainProject,
	"bundle:export": command.DomainProject, "bundle:import": command.DomainProject,
	"events": command.DomainProject,

	"edit": command.DomainVideo, "history": command.DomainVideo, "undo": command.DomainVideo,
	"redo": command.DomainVideo, "lint": command.DomainVideo, "view": command.DomainVideo,
	"revert": command.DomainVideo, "diff": command.DomainVideo, "checkpoint:create": command.DomainVideo,
	"checkpoint:list": command.DomainVideo, "checkpoint:rename": command.DomainVideo,
	"checkpoint:remove": command.DomainVideo,
	"export":            command.DomainVideo, "transcribe": command.DomainVideo, "job:show": command.DomainVideo,
	"job:cancel": command.DomainVideo, "job:list": command.DomainVideo, "template:list": command.DomainVideo,
	"template:save": command.DomainVideo, "template:new": command.DomainVideo,
	"template:export": command.DomainVideo, "template:import": command.DomainVideo,
	"otio:export": command.DomainVideo, "otio:import": command.DomainVideo, "export:presets": command.DomainVideo,
	"export:check": command.DomainVideo, "export:batch": command.DomainVideo,

	"image:create": command.DomainImage, "image:list": command.DomainImage, "image:show": command.DomainImage,
	"image:edit": command.DomainImage, "image:undo": command.DomainImage, "image:redo": command.DomainImage,
	"image:export": command.DomainImage,

	"media:import": command.DomainAsset, "media:inspect": command.DomainAsset, "media:list": command.DomainAsset,
	"media:trash": command.DomainAsset, "media:restore": command.DomainAsset, "font:import": command.DomainAsset,
	"fonts": command.DomainAsset, "asset:list": command.DomainAsset, "asset:show": command.DomainAsset,
	"asset:uses": command.DomainAsset, "asset:set": command.DomainAsset, "asset:tag": command.DomainAsset,

	"motion:brief": command.DomainVideo, "motion:check": command.DomainVideo, "motion:replace": command.DomainVideo,

	"memory:list": command.DomainMemory, "memory:show": command.DomainMemory, "memory:write": command.DomainMemory,
	"memory:remove": command.DomainMemory, "skill:list": command.DomainMemory, "skill:show": command.DomainMemory,
	"skill:write": command.DomainMemory, "skill:import": command.DomainMemory, "skill:export": command.DomainMemory,
	"skill:lint": command.DomainMemory, "prompt:list": command.DomainMemory, "prompt:show": command.DomainMemory,
	"prompt:write": command.DomainMemory, "prompt:render": command.DomainMemory,
	"library:search": command.DomainMemory, "library:reindex": command.DomainMemory,

	"approval:list": command.DomainPublish, "approval:show": command.DomainPublish,
	"approval:decide": command.DomainPublish, "approval:rule:list": command.DomainPublish,
	"approval:rule:set": command.DomainPublish, "approval:rule:remove": command.DomainPublish,
	"social:network:list": command.DomainPublish, "social:connect": command.DomainPublish,
	"social:account:list": command.DomainPublish, "publish:validate": command.DomainPublish,
	"publish:now": command.DomainPublish, "publish:status": command.DomainPublish,
	"publication:create": command.DomainPublish, "publication:list": command.DomainPublish,
	"publication:show": command.DomainPublish, "publication:update": command.DomainPublish,
	"publication:schedule": command.DomainPublish, "publication:send-now": command.DomainPublish,
	"publication:retry": command.DomainPublish, "publication:cancel": command.DomainPublish,
	"cost:show": command.DomainProvider, "cost:ceiling": command.DomainProvider,
	"provider:list": command.DomainProvider, "provider:models": command.DomainProvider,
	"provider:price": command.DomainProvider, "text:generate": command.DomainProvider,
	"image:generate": command.DomainProvider, "voice:generate": command.DomainProvider,
	"voice:list": command.DomainProvider,

	"text:list": command.DomainWriting, "text:show": command.DomainWriting, "text:write": command.DomainWriting,
	"text:revisions": command.DomainWriting, "text:revert": command.DomainWriting, "text:draft": command.DomainWriting,
	"caption:validate": command.DomainWriting, "script:estimate": command.DomainWriting,

	"command:help": command.DomainWorkspace, "connection:list": command.DomainWorkspace,
	"connection:show": command.DomainWorkspace, "connection:add": command.DomainWorkspace,
	"connection:remove": command.DomainWorkspace, "connection:test": command.DomainWorkspace,
	"brand:show": command.DomainWorkspace, "brand:set": command.DomainWorkspace, "doctor": command.DomainWorkspace,
	"audit": command.DomainWorkspace, "tools:install": command.DomainWorkspace, "version": command.DomainWorkspace,
	"mcp": command.DomainWorkspace, "agent:docs": command.DomainWorkspace, "token:rotate": command.DomainWorkspace,

	"agent:profiles": command.DomainWorkspace, "chat:sessions": command.DomainWorkspace,
	"chat:session": command.DomainWorkspace, "chat:transcript": command.DomainWorkspace,
}

// domainOf is the domain of a command of the table, empty for a name that is not in it.
func domainOf(name string) string { return commandDomains[name] }

// verbOf is the part of a command name after the colon: `edit` for `image:edit`, the whole name when there is none.
func verbOf(name string) string {
	_, verb, found := strings.Cut(name, ":")
	if !found {
		return name
	}

	return verb
}

// domainCommands lists the commands of one domain in the order of the table.
func domainCommands(domain string) []*spec {
	var out []*spec

	for _, s := range registry() {
		if domainOf(s.name) == domain {
			out = append(out, s)
		}
	}

	return out
}

// resolveGrouped turns the words after a domain into the command they name, and what is left of the arguments.
// It tries, in this order: the words joined by colons (`memory skill list` is `skill:list`), the first word
// as a whole name (`video edit` is `edit`), and the first word as the verb of exactly one command of the domain
// (`image edit` is `image:edit`). A verb that several commands of the domain share is ambiguous and names them.
func resolveGrouped(domain string, words []string) (name string, rest []string, err *service.Error) {
	if len(words) == 0 {
		return "", nil, &service.Error{
			Code: command.CodeValidation, Message: "name a command of the domain " + domain + ".",
			Hint: "run stuarty " + domain + " to list them",
		}
	}

	first, tail := words[0], words[1:]
	in := domainCommands(domain)
	has := func(candidate string) bool {
		return slices.ContainsFunc(in, func(s *spec) bool { return s.name == candidate })
	}

	// The longest run of words that joins by colons into a command of the domain: `approval rule list`.
	for n := min(len(words), 3); n >= 2; n-- {
		if slices.ContainsFunc(words[:n], func(w string) bool { return strings.HasPrefix(w, "-") }) {
			continue
		}

		if joined := strings.Join(words[:n], ":"); has(joined) {
			return joined, words[n:], nil
		}
	}

	if has(first) {
		return first, tail, nil
	}

	var matches []string

	for _, s := range in {
		if verbOf(s.name) == first {
			matches = append(matches, s.name)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], tail, nil
	case 0:
		return "", nil, &service.Error{
			Code:    command.CodeUnknownCommand,
			Message: fmt.Sprintf("there is no %q in the domain %s.", first, domain),
			Hint: "the commands of " + domain + " are " + strings.Join(
				namesOf(in),
				", ",
			) + "; run stuarty " + domain,
		}
	}

	return "", nil, &service.Error{
		Code: command.CodeValidation, Path: first,
		Message: fmt.Sprintf("%q names several commands of the domain %s.", first, domain),
		Hint:    "use the full name: " + strings.Join(matches, ", "),
	}
}

func namesOf(in []*spec) []string {
	names := make([]string, len(in))
	for i, s := range in {
		names[i] = s.name
	}

	return names
}

// Group answers `stuarty <domain> [<words>...]` (SDD-003 section 5.1). With only a domain it lists that domain's
// commands. With words it names the command, and the rewritten arguments are that command's name and what
// follows, ready for Execute. args is what follows the program name. A first word that is not a domain, or that is
// also the name of a command, is not grouped: matched is false and the arguments are the caller's to run.
func Group(args []string, stdout, stderr io.Writer) (rewritten []string, code int, matched bool) {
	if len(args) == 0 || !command.KnownDomain(args[0]) || lookup(args[0]) != nil || lookupProcess(args[0]) != nil {
		return nil, ExitOK, false
	}

	domain := args[0]

	words := args[1:]

	if len(words) == 0 || strings.HasPrefix(words[0], "-") {
		listDomain(stdout, domain)

		return nil, ExitOK, true
	}

	name, rest, err := resolveGrouped(domain, words)
	if err != nil {
		rt := &Runtime{Stdout: stdout, Stderr: stderr}

		return nil, writeError(rt, jsonRequested(args), err), true
	}

	return append([]string{name}, rest...), ExitOK, true
}

// listDomain prints the commands of one domain.
func listDomain(w io.Writer, domain string) {
	fmt.Fprintf(w, "stuarty %s: ", domain)

	for _, d := range command.Domains() {
		if d.Name == domain {
			fmt.Fprintln(w, d.Summary)
		}
	}

	in := domainCommands(domain)
	if len(in) == 0 {
		fmt.Fprintln(w, "  no commands yet")

		return
	}

	for _, s := range in {
		fmt.Fprintf(w, "  %-16s %s\n", s.name, s.summary)
	}

	fmt.Fprintf(w, "\nrun stuarty %s <verb> (for example stuarty %s %s) or the full command name\n",
		domain, domain, verbOf(in[0].name))
}

// CommandDomain is the domain of a command of the table, empty for a name that is not in it. The contract test
// checks it against the domain of the Service operation the command calls.
func CommandDomain(name string) string { return domainOf(name) }
