package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpRootListsTheProductCommandsAndServe(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		var out, errOut bytes.Buffer

		code, handled := Help(args, &out, &errOut)
		if !handled || code != ExitOK || errOut.Len() != 0 {
			t.Fatalf("%v: handled %v, exit %d, stderr %q", args, handled, code, errOut.String())
		}

		for _, want := range []string{"project:list", "serve", "open", "user:create"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%v: the root help does not list %s", args, want)
			}
		}

		if strings.Contains(out.String(), "migrate") {
			t.Errorf("%v: the root help lists a framework command", args)
		}
	}
}

func TestHelpCommandEqualsCommandDashHelp(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"project:list", "serve", "user:create"} {
		var viaHelp, errOut bytes.Buffer

		if code, handled := Help([]string{"help", name}, &viaHelp, &errOut); !handled || code != ExitOK {
			t.Fatalf("help %s: handled %v, exit %d, %q", name, handled, code, errOut.String())
		}

		if !strings.Contains(viaHelp.String(), name) || !strings.Contains(viaHelp.String(), "usage:") {
			t.Errorf("help %s: %q", name, viaHelp.String())
		}

		if IsCommand(name) {
			continue
		}

		var viaFlag bytes.Buffer
		if code, handled := Help([]string{name, "--help"}, &viaFlag, &errOut); !handled || code != ExitOK ||
			viaFlag.String() != viaHelp.String() {
			t.Errorf("%s --help differs from help %s: %q", name, name, viaFlag.String())
		}
	}
}

func TestHelpUnknownCommandIsAnErrorNotTheFrameworkList(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer

	code, handled := Help([]string{"help", "project:lst"}, &out, &errOut)
	if !handled || code != ExitFailure || out.Len() != 0 || !strings.Contains(errOut.String(), "project:lst") {
		t.Fatalf("handled %v, exit %d, %q, %q", handled, code, out.String(), errOut.String())
	}
}

func TestHelpLeavesOtherArgumentsAlone(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"project:list", "--help"}, {"migrate"}, {"serve"}} {
		if _, handled := Help(args, &bytes.Buffer{}, &bytes.Buffer{}); handled {
			t.Errorf("%v must not be handled by Help", args)
		}
	}
}

func TestReferenceDocumentsTheProcessCommands(t *testing.T) {
	t.Parallel()

	names := map[string]bool{}
	for _, c := range Reference().ProcessCommands {
		names[c.Name] = true

		if c.Summary == "" || len(c.Usage) == 0 {
			t.Errorf("%s: needs a summary and a usage line", c.Name)
		}
	}

	for _, want := range []string{"serve", "open", "user:create", "user:passwd", "workspace:backup", "workspace:restore"} {
		if !names[want] {
			t.Errorf("the reference lacks %s", want)
		}
	}
}

func TestHelpUserCommandsTakeAnEmail(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"user:create": "stuarty user:create <email> [--admin]",
		"user:passwd": "stuarty user:passwd <email>",
	} {
		var out, errOut bytes.Buffer

		if code, handled := Help([]string{"help", name}, &out, &errOut); !handled || code != ExitOK {
			t.Fatalf("help %s: handled %v, exit %d, %q", name, handled, code, errOut.String())
		}

		if !strings.Contains(out.String(), want) || strings.Contains(out.String(), "<name>") {
			t.Errorf("help %s: want usage %q, got %q", name, want, out.String())
		}
	}
}

// VV-113: the help of edit shows the file format of --ops.
func TestEditHelpShowsTheBatchFormat(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if code := h.run("edit", "--help"); code != ExitOK {
		t.Fatalf("edit --help exit %d", code)
	}

	for _, want := range []string{`"operations"`, `"op"`, `"args"`} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("edit --help does not show %s: %s", want, h.stdout.String())
		}
	}
}

// VV-114: the mcp command names the workspace and the server in serverInfo.
func TestMCPDescription(t *testing.T) {
	t.Parallel()

	if got, want := mcpDescription("/w", "http://127.0.0.1:3401"),
		"Stuarty MCP server for the workspace /w, server http://127.0.0.1:3401"; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}

	if got := mcpDescription("", ""); got != "Stuarty MCP server" {
		t.Errorf("description = %q", got)
	}
}

func TestUnknownCommandNamesTheNearestOne(t *testing.T) {
	t.Parallel()

	if h := unknownHint("project:lst"); !strings.Contains(h, `"project:list"`) {
		t.Fatalf("hint = %q", h)
	}

	if h := unknownHint("projetos"); strings.Contains(h, "did you mean") {
		t.Fatalf("a far name got a suggestion: %q", h)
	}
}

func TestRejectArgsOnlyForTheCommandsWithoutArguments(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer

	if code, rejected := RejectArgs("serve", []string{"--nope"}, &out, &errOut); !rejected || code != ExitFailure ||
		!strings.Contains(errOut.String(), "E_VALIDATION") {
		t.Fatalf("serve --nope: %d %v %q", code, rejected, errOut.String())
	}

	if _, rejected := RejectArgs("serve", nil, &out, &errOut); rejected {
		t.Fatal("serve with no argument was rejected")
	}

	if _, rejected := RejectArgs("user:create", []string{"a@b.c"}, &out, &errOut); rejected {
		t.Fatal("user:create takes arguments")
	}
}

// VV-147: the commands that take an id state its form in --help.
func TestHelpStatesTheIDRule(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"edit", "history", "lint"} {
		var out, errOut bytes.Buffer

		if code, handled := Help([]string{"help", name}, &out, &errOut); !handled || code != 0 {
			t.Fatalf("%s --help: handled %v, exit %d", name, handled, code)
		}

		if !strings.Contains(out.String(), "without i, l, o and u, such as trk_k3m9x4q1bz") {
			t.Errorf("%s --help does not state the id rule:\n%s", name, out.String())
		}
	}
}
