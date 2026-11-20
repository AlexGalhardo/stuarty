package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
)

// listOverEverySurface reads the operations of one domain (all of them with an empty one) the four ways, each as the
// service.CommandHelpResult it carries.
func listOverEverySurface(t *testing.T, domain string) map[string]service.CommandHelpResult {
	t.Helper()

	direct, ok := newDirect(t), true
	m, ok2 := newMCP(t).(*mcpDriver)
	h, ok3 := newHTTP(t).(*httpDriver)
	c, ok4 := newCLI(t).(*cliDriver)

	if !ok || !ok2 || !ok3 || !ok4 {
		t.Fatal("a driver has an unexpected type")
	}

	out := map[string]service.CommandHelpResult{}

	res, err := direct.service().CommandHelp(callerCtx(), service.CommandHelpRequest{Domain: domain})
	if err != nil {
		t.Fatal(err)
	}

	out["direct"] = roundTrip(t, res)

	args := map[string]any{}
	if domain != "" {
		args["domain"] = domain
	}

	call, err := m.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "command_help", Arguments: args})
	if err != nil || call.IsError {
		t.Fatalf("mcp command_help %q: %v %+v", domain, err, call)
	}

	out["mcp"] = roundTrip(t, call.StructuredContent)

	path := "/api/v1/commands"
	if domain != "" {
		path += "?domain=" + domain
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.url+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+h.token)

	resp, err := h.send(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %v %s", path, resp.StatusCode, err, raw)
	}

	var over service.CommandHelpResult
	if err := json.Unmarshal(raw, &over); err != nil {
		t.Fatal(err)
	}

	out["http"] = over

	cliArgs := []string{"--json"}
	if domain != "" {
		cliArgs = append(cliArgs, "--domain", domain)
	}

	out["cli"] = cliHelp(t, c, cliArgs)

	return out
}

func cliHelp(t *testing.T, d *cliDriver, args []string) service.CommandHelpResult {
	t.Helper()

	var stdout, stderr bytes.Buffer

	rt := &cli.Runtime{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Getenv: func(string) string { return "" }, Getwd: func() (string, error) { return t.TempDir(), nil },
		Resolver: fixedResolver{svc: d.e.Service},
	}

	if code := rt.Execute(context.Background(), "command:help", args); code != cli.ExitOK {
		t.Fatalf("command:help %v: exit %d: %s", args, code, stdout.String())
	}

	var out service.CommandHelpResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	return out
}

// shape is what the surfaces share: the operations and the domains. The text is for a reader, and the MCP adapter
// leaves it out of the structured result (SDD-003 section 7.1, S-9).
func shape(r service.CommandHelpResult) map[string]any {
	return map[string]any{"commands": r.Commands, "domains": r.Domains}
}

func roundTrip(t *testing.T, v any) service.CommandHelpResult {
	t.Helper()

	var out service.CommandHelpResult
	if err := json.Unmarshal([]byte(canonical(t, v)), &out); err != nil {
		t.Fatal(err)
	}

	return out
}

// TestDiscoveryByDomainIsTheSameOnEverySurface: command_help, GET /commands, command:help and the Service give one
// answer for every domain, the domain list included (SDD-003 section 4.8).
func TestDiscoveryByDomainIsTheSameOnEverySurface(t *testing.T) {
	for _, domain := range append([]string{""}, command.DomainNames()...) {
		got := listOverEverySurface(t, domain)
		want := canonical(t, shape(got["direct"]))

		for surface, res := range got {
			if canonical(t, shape(res)) != want {
				t.Errorf(
					"domain %q: %s differs from the Service\n got: %s\nwant: %s",
					domain, surface, canonical(t, shape(res)), want,
				)
			}
		}
	}
}

// batchPaths is how a batch of operations of a registry reaches each surface: the MCP tool that takes the domain,
// the CLI command and the HTTP route. A domain whose registry has commands and no entry here fails the build, and
// so does an entry that a surface does not serve (epic 13): a later epic that registers operations adds its row.
var batchPaths = map[string]struct{ tool, command, route string }{
	command.DomainVideo: {"edit", "edit", "POST /projects/{}/edit"},
	command.DomainImage: {"edit", "image:edit", "POST /images/{}/edit"},
}

// TestEveryRegistryHasABatchPathOnEverySurface iterates the domains of the registry, not a table of this test: a
// domain with edit operations must be reachable by MCP, the CLI and HTTP, whatever the domain is.
func TestEveryRegistryHasABatchPathOnEverySurface(t *testing.T) {
	tools := listedTools(t)
	commands, stubs := cli.CommandNames()
	routes := httpRoutes(t, newHTTP(t).(*httpDriver))

	list, err := newDirect(t).service().CommandHelp(callerCtx(), service.CommandHelpRequest{})
	if err != nil {
		t.Fatal(err)
	}

	withOperations := 0

	for _, d := range list.Domains {
		if d.Operations == 0 {
			continue
		}

		withOperations++

		p, ok := batchPaths[d.Name]
		if !ok {
			t.Errorf(
				"the domain %s has %d edit operations and no batch path: give it an MCP route, a CLI command and an HTTP route",
				d.Name,
				d.Operations,
			)

			continue
		}

		if !slices.Contains(tools, p.tool) || !mcp.ServesDomain(p.tool, d.Name) {
			t.Errorf("%s: the MCP tool %s is not listed or does not serve the domain", d.Name, p.tool)
		}

		if !slices.Contains(commands, p.command) || slices.Contains(stubs, p.command) {
			t.Errorf("%s: the CLI command %s is missing or a stub", d.Name, p.command)
		}

		if !slices.Contains(routes, p.route) {
			t.Errorf("%s: the HTTP route %s is not served", d.Name, p.route)
		}
	}

	if withOperations < 2 {
		t.Errorf("%d domains have edit operations, want at least video and image", withOperations)
	}

	for name := range batchPaths {
		if !command.KnownDomain(name) {
			t.Errorf("batchPaths names %s, which is no domain", name)
		}
	}
}

// TestEveryOperationHasADomainOnEverySurface: the domain of a Service operation is the domain of the CLI command
// that calls it and one of the domains of the MCP tool that serves it.
func TestEveryOperationHasADomainOnEverySurface(t *testing.T) {
	t.Parallel()

	for _, r := range parity {
		if r.noMethod {
			// The event stream is no Service method (SDD-003 section 7.1, exception E-1); it belongs to the project.
			// A stream with no command (the chat, SDD-017 section 8) has no CLI domain to check.
			if got := cli.CommandDomain(r.command); r.command != "" && got != command.DomainProject {
				t.Errorf("%s: the CLI command %s is in %q, want project", r.method, r.command, got)
			}

			continue
		}

		domain, ok := service.OperationDomain(r.method)
		if !ok {
			t.Errorf("%s has no domain in service.OperationDomains", r.method)

			continue
		}

		if r.command != "" {
			if got := cli.CommandDomain(r.command); got != domain {
				t.Errorf("%s is in the domain %s, the CLI command %s in %q", r.method, domain, r.command, got)
			}
		}

		if r.tool != "" && !slices.Contains(mcp.ToolDomains(r.tool), domain) {
			t.Errorf(
				"%s is in the domain %s, the MCP tool %s serves %v",
				r.method,
				domain,
				r.tool,
				mcp.ToolDomains(r.tool),
			)
		}

		if strings.TrimSpace(domain) == "" {
			t.Errorf("%s has an empty domain", r.method)
		}
	}
}
