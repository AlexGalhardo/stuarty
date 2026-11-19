package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

func TestEveryCommandHasADomain(t *testing.T) {
	t.Parallel()

	all, _ := CommandNames()

	for _, name := range all {
		d := domainOf(name)
		if d == "" || !command.KnownDomain(d) {
			t.Errorf("%s has no domain of SDD-000 section 1.1 in commandDomains", name)
		}
	}

	for name := range commandDomains {
		if !slices.Contains(all, name) {
			t.Errorf("commandDomains names %s, which is not a command", name)
		}
	}
}

// Every command can be written as `stuarty <domain> <words>` and reaches itself: the grouped form needs no
// knowledge beyond the name, and no two commands of a domain collide.
func TestEveryCommandIsReachableByItsDomain(t *testing.T) {
	t.Parallel()

	all, _ := CommandNames()

	for _, name := range all {
		words := strings.Split(name, ":")

		got, rest, err := resolveGrouped(domainOf(name), words)
		if err != nil || got != name || len(rest) != 0 {
			t.Errorf("%s %v resolves to %q %v, %v", domainOf(name), words, got, rest, err)
		}
	}
}

func TestGroupRewritesTheArguments(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"image", "edit", "--base", "2"}, []string{"image:edit", "--base", "2"}},
		{[]string{"image", "image", "edit"}, []string{"image:edit"}},
		{[]string{"video", "edit", "--project", "prj_a"}, []string{"edit", "--project", "prj_a"}},
		{[]string{"video", "lint"}, []string{"lint"}},
		{[]string{"video", "export", "--wait"}, []string{"export", "--wait"}},
		{[]string{"video", "job", "show", "job_a"}, []string{"job:show", "job_a"}},
		{[]string{"project", "list"}, []string{"project:list"}},
		{[]string{"project", "item", "add", "x"}, []string{"item:add", "x"}},
		{[]string{"project", "bundle", "export"}, []string{"bundle:export"}},
		{[]string{"memory", "skill", "list", "--all"}, []string{"skill:list", "--all"}},
		{[]string{"asset", "media", "import", "a.mp4"}, []string{"media:import", "a.mp4"}},
		{[]string{"workspace", "doctor"}, []string{"doctor"}},
	} {
		var out, errOut bytes.Buffer

		got, code, matched := Group(tc.args, &out, &errOut)
		if !matched || code != ExitOK || !slices.Equal(got, tc.want) || out.Len() != 0 || errOut.Len() != 0 {
			t.Errorf(
				"%v: %v, exit %d, matched %v, out %q, err %q; want %v",
				tc.args,
				got,
				code,
				matched,
				out.String(),
				errOut.String(),
				tc.want,
			)
		}
	}
}

func TestGroupRefusesWhatItCannotResolve(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		args []string
		code string
		hint string
	}{
		"ambiguous in asset":  {[]string{"asset", "import"}, "E_VALIDATION", "media:import, font:import"},
		"ambiguous in memory": {[]string{"memory", "list", "--json"}, "E_VALIDATION", "memory:list"},
		"unknown verb":        {[]string{"image", "nope"}, "E_UNKNOWN_COMMAND", "image:edit"},
		"domain with none":    {[]string{"writing", "edit"}, "E_UNKNOWN_COMMAND", "the commands of writing are"},
	} {
		var out, errOut bytes.Buffer

		got, code, matched := Group(tc.args, &out, &errOut)
		text := out.String() + errOut.String()

		if !matched || code != ExitFailure || got != nil || !strings.Contains(text, tc.code) ||
			!strings.Contains(text, tc.hint) {
			t.Errorf(
				"%s: %v, exit %d, matched %v, %q; want %s with %q",
				name,
				got,
				code,
				matched,
				text,
				tc.code,
				tc.hint,
			)
		}
	}
}

func TestGroupLeavesOtherArgumentsAlone(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{}, {"project:list"}, {"edit"}, {"serve"}, {"mcp"}, {"migrate:status"}, {"nope"}, {"--help"}} {
		if got, _, matched := Group(args, &bytes.Buffer{}, &bytes.Buffer{}); matched || got != nil {
			t.Errorf("%v must not be grouped: %v", args, got)
		}
	}
}

func TestGroupWithOnlyADomainListsItsCommands(t *testing.T) {
	t.Parallel()

	for domain, want := range map[string]string{
		"image": "image:edit", "video": "lint", "memory": "skill:list", "project": "project:create", "writing": "no commands yet",
	} {
		var out, errOut bytes.Buffer

		got, code, matched := Group([]string{domain}, &out, &errOut)
		if !matched || code != ExitOK || got != nil || errOut.Len() != 0 || !strings.Contains(out.String(), want) {
			t.Errorf(
				"%s: %v, exit %d, matched %v, %q, %q; want %q",
				domain,
				got,
				code,
				matched,
				out.String(),
				errOut.String(),
				want,
			)
		}
	}
}
