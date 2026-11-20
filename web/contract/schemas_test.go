package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
)

// helpers read command_help the five ways SDD-003 section 9 lists, each as the
// service.CommandHelpResult it carries.

func canonical(t *testing.T, v any) string {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}

	out, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}

func helpOverMCP(t *testing.T, d *mcpDriver, name string) service.CommandHelpResult {
	t.Helper()

	args := map[string]any{}
	if name != "" {
		args["name"] = name
	}

	res, err := d.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "command_help", Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("command_help %q: %v %+v", name, err, res)
	}

	var out service.CommandHelpResult
	if err := json.Unmarshal([]byte(canonical(t, res.StructuredContent)), &out); err != nil {
		t.Fatal(err)
	}

	return out
}

func helpOverHTTP(t *testing.T, d *httpDriver, name string) service.CommandHelpResult {
	t.Helper()

	path := "/api/v1/commands"
	if name != "" {
		path += "/" + name
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, d.url+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+d.token)

	res, err := d.send(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %v", path, res.StatusCode, err)
	}

	var out service.CommandHelpResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}

	return out
}

func helpOverCLI(t *testing.T, d *cliDriver, name string) service.CommandHelpResult {
	t.Helper()

	var stdout, stderr bytes.Buffer

	rt := &cli.Runtime{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Getenv: func(string) string { return "" }, Getwd: func() (string, error) { return t.TempDir(), nil },
		Resolver: fixedResolver{svc: d.e.Service},
	}

	args := []string{"--json"}
	if name != "" {
		args = append([]string{name}, args...)
	}

	if code := rt.Execute(context.Background(), "command:help", args); code != cli.ExitOK {
		t.Fatalf("command:help %q: exit %d: %s", name, code, stdout.String())
	}

	var out service.CommandHelpResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	return out
}

func openAPIArgs(t *testing.T) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(filepath.FromSlash(openAPIFile))
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}

	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	return doc.Components.Schemas
}

// surfaces are the ways of reading command_help, as functions of a name.
func helpSurfaces(t *testing.T) map[string]func(name string) service.CommandHelpResult {
	t.Helper()

	direct, httpd := newDirect(t), newHTTP(t)

	m, ok := newMCP(t).(*mcpDriver)
	if !ok {
		t.Fatal("newMCP is not an mcpDriver")
	}

	h, ok := httpd.(*httpDriver)
	if !ok {
		t.Fatal("newHTTP is not an httpDriver")
	}

	c, ok := newCLI(t).(*cliDriver)
	if !ok {
		t.Fatal("newCLI is not a cliDriver")
	}

	return map[string]func(string) service.CommandHelpResult{
		"direct": func(name string) service.CommandHelpResult {
			res, err := direct.service().CommandHelp(callerCtx(), service.CommandHelpRequest{Name: name})
			if err != nil {
				t.Fatal(err)
			}

			var out service.CommandHelpResult
			if err := json.Unmarshal([]byte(canonical(t, res)), &out); err != nil {
				t.Fatal(err)
			}

			return out
		},
		"mcp":  func(name string) service.CommandHelpResult { return helpOverMCP(t, m, name) },
		"http": func(name string) service.CommandHelpResult { return helpOverHTTP(t, h, name) },
		"cli":  func(name string) service.CommandHelpResult { return helpOverCLI(t, c, name) },
	}
}

// TestSchemasMatch: the args schema of every public command is the same through
// command_help, GET /commands/{name}, command:help <name> --json, the Service and
// the args.<name> component of the OpenAPI document.
func TestSchemasMatch(t *testing.T) {
	surfaces := helpSurfaces(t)
	components := openAPIArgs(t)

	for _, name := range command.Default().Names() {
		cmd, _ := command.Default().Get(name)
		if cmd.Internal {
			continue
		}

		want := canonical(t, surfaces["direct"](name).Command.ArgsSchema)

		for surface, read := range surfaces {
			res := read(name)
			if res.Command == nil {
				t.Errorf("%s %s: no command in the answer", surface, name)

				continue
			}

			if got := canonical(t, res.Command.ArgsSchema); got != want {
				t.Errorf("%s %s: args schema differs\n got: %s\nwant: %s", surface, name, got, want)
			}
		}

		component, ok := components["args."+name]
		if !ok {
			t.Errorf("the OpenAPI document has no args.%s component", name)

			continue
		}

		// The component of the document is the schema without the $schema keyword
		// of the draft, which only the standalone schema carries.
		inline := surfaces["direct"](name).Command.ArgsSchema
		delete(inline, "$schema")

		if got, wantInline := canonical(t, component), canonical(t, inline); got != wantInline {
			t.Errorf("openapi args.%s differs\n got: %s\nwant: %s", name, got, wantInline)
		}
	}
}

// TestSummariesMatch: the one-line summary and the scope of every
// public command are the same in the list of every surface, which all read the
// registry.
func TestSummariesMatch(t *testing.T) {
	surfaces := helpSurfaces(t)
	want := canonical(t, surfaces["direct"]("").Commands)

	for surface, read := range surfaces {
		if got := canonical(t, read("").Commands); got != want {
			t.Errorf("%s: the command list differs from the Service's\n got: %s\nwant: %s", surface, got, want)
		}
	}

	for _, c := range surfaces["direct"]("").Commands {
		cmd, ok := command.Default().Get(c.Name)
		if !ok || cmd.Summary != c.Summary || strings.TrimSpace(c.Summary) == "" {
			t.Errorf("%s: the summary %q is not the registry's", c.Name, c.Summary)
		}
	}
}
