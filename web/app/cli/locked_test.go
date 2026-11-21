package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	rules "github.com/AlexGalhardo/stuarty/core/account"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	"github.com/AlexGalhardo/stuarty/web/app/owner"
)

// TestSecurity_T107_ACommandOfALockedWorkspaceAnswersELocked: with no server running, a CLI command owns the
// workspace in process, and a workspace that has an account is locked there: every command that reads or writes
// the user's data answers E_LOCKED, with the hint to unlock, and never opens the workspace to a person who did
// not type the password (SDD-007 section 3.9 item 4).
func TestSecurity_T107_ACommandOfALockedWorkspaceAnswersELocked(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ws")

	getenv := func(k string) string {
		switch k {
		case workspace.EnvHome:
			return home
		case "STUARTY_MASTER_KEY_FILE":
			return filepath.Join(t.TempDir(), "master.key")
		case "STUARTY_MIN_FREE_BYTES":
			return "1"
		}

		return ""
	}

	ctx := context.Background()

	o, err := owner.Open(ctx, owner.Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := o.Account.Create(ctx, rules.Input{
		Username: "stuart", Email: "stuart@example.com", Password: "Correct-Horse-9", Confirmation: "Correct-Horse-9",
	}, nil); err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer

		rt := &cli.Runtime{
			Stdout: &stdout, Stderr: &stderr, Getenv: getenv, Getwd: func() (string, error) { return home, nil },
			Resolver: cli.LocalResolver{Getenv: getenv},
		}

		code := rt.Execute(ctx, args[0], args[1:])

		return code, stdout.String(), stderr.String()
	}

	for _, args := range [][]string{
		{"project:list", "--json"},
		{"project:create", "--name", "x", "--json"},
		{"project:show", "prj_0000000001", "--json"},
		{"memory:list", "--json"},
		{"template:list", "--json"},
	} {
		code, stdout, stderr := run(args...)

		var doc map[string]any
		if err := json.Unmarshal(
			[]byte(strings.TrimSpace(stdout+stderr)),
			&doc,
		); err != nil ||
			doc["code"] != "E_LOCKED" {
			t.Errorf("%v = exit %d, stdout %q, stderr %q: want E_LOCKED", args, code, stdout, stderr)

			continue
		}

		if code == 0 || !strings.Contains(stdout+stderr, "unlock Stuarty with your password") {
			t.Errorf("%v: exit %d, no hint: %s%s", args, code, stdout, stderr)
		}
	}
}
