package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/cli"
	"github.com/AlexGalhardo/vukem-video/web/contract/testenv"
)

type fixedResolver struct{ svc service.Service }

func (r fixedResolver) Resolve(context.Context, cli.Target) (*cli.Connection, error) {
	return &cli.Connection{Service: r.svc, Workspace: "/ws"}, nil
}

// TestUntrustedText is §9 check 11 through the CLI adapter, with the hostile
// names of the MCP fixture: the human text carries the note, escapes and cuts
// the names, and --json keeps the original under `untrusted`.
func TestUntrustedText(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	names := testenv.HostileNames(t, "../mcp/testdata/untrusted/names.json")

	var stdout, stderr bytes.Buffer

	rt := &cli.Runtime{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Getenv: func(string) string { return "" }, Getwd: func() (string, error) { return t.TempDir(), nil },
		Resolver: fixedResolver{svc: env.Service},
	}

	run := func(args ...string) (int, string) {
		t.Helper()

		stdout.Reset()
		stderr.Reset()

		code := rt.Execute(t.Context(), args[0], args[1:])
		if stderr.Len() != 0 {
			t.Fatalf("%v: standard error: %q", args, stderr.String())
		}

		return code, stdout.String()
	}

	_, created := run("project:create", "--name", "Hostile", "--json")

	var made struct {
		ProjectID string `json:"project_id"`
	}

	if err := json.Unmarshal([]byte(created), &made); err != nil {
		t.Fatal(err)
	}

	args := []string{"media:import"}
	for i, n := range names {
		args = append(args, env.WriteFile(t, n.Name, strings.Repeat("x", 64+i)))
	}

	args = append(args, "--project", made.ProjectID)

	code, text := run(args...)
	if code != cli.ExitOK {
		t.Fatalf("media:import: exit %d: %s", code, text)
	}

	for _, p := range testenv.UntrustedTextProblems(text) {
		t.Errorf("media:import: %s", p)
	}

	code, list := run("media:list", "--project", made.ProjectID)
	if code != cli.ExitOK {
		t.Fatalf("media:list: exit %d", code)
	}

	for _, p := range testenv.UntrustedTextProblems(list) {
		t.Errorf("media:list: %s", p)
	}

	if strings.Contains(list, strings.Repeat("x", 150)) || !strings.Contains(list, "…") {
		t.Errorf("a 150 character name must be cut with the marker: %s", list)
	}

	_, doc := run("media:list", "--project", made.ProjectID, "--json")

	var listed struct {
		Assets []struct {
			AssetID string `json:"asset_id"`
		} `json:"assets"`
	}

	if err := json.Unmarshal([]byte(doc), &listed); err != nil || len(listed.Assets) != len(names) {
		t.Fatalf("media:list --json: %v %s", err, doc)
	}

	stored := map[string]bool{}

	for _, a := range listed.Assets {
		code, one := run("media:inspect", a.AssetID, "--project", made.ProjectID)
		if code != cli.ExitOK {
			t.Fatalf("media:inspect: exit %d", code)
		}

		for _, p := range testenv.UntrustedTextProblems(one) {
			t.Errorf("media:inspect %s: %s", a.AssetID, p)
		}

		_, raw := run("media:inspect", a.AssetID, "--project", made.ProjectID, "--json")

		var res struct {
			Untrusted struct {
				Name string `json:"name"`
			} `json:"untrusted"`
		}

		if err := json.Unmarshal([]byte(raw), &res); err != nil {
			t.Fatal(err)
		}

		stored[res.Untrusted.Name] = true
	}

	for _, n := range names {
		if !stored[n.Name] && !stored[testenv.StoredName(n.Name)] {
			t.Errorf("--json lost the name %q (have %v)", n.Name, stored)
		}
	}
}
