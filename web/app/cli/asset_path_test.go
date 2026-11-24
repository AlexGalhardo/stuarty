package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	"github.com/AlexGalhardo/stuarty/web/app/owner"
)

// SDD-003 E-6 and T-123: `asset:path` exists so that an agent that tries gets E_PRIVILEGED_SESSION and its hint
// instead of "unknown command". The CLI runs with the local token that every agent holds, so against a real
// workspace it never answers a path, whatever the hash, and the refusal names no place on the disk.
func TestAssetPathAlwaysRefusesTheCLIAndNamesNoPlace(t *testing.T) {
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

	// Make the workspace once so that the command owns an existing one.
	o, err := owner.Open(ctx, owner.Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	hash := "sha256:" + strings.Repeat("a", 64)

	var stdout, stderr bytes.Buffer

	rt := &cli.Runtime{
		Stdout: &stdout, Stderr: &stderr, Getenv: getenv, Getwd: func() (string, error) { return home, nil },
		Resolver: cli.LocalResolver{Getenv: getenv},
	}

	if code := rt.Execute(ctx, "asset:path", []string{hash, "--json"}); code == cli.ExitOK {
		t.Fatalf("asset:path succeeded: %s%s", stdout.String(), stderr.String())
	}

	var doc struct {
		Code  string `json:"code"`
		Hint  string `json:"hint"`
		Error struct {
			Code string `json:"code"`
			Hint string `json:"hint"`
		} `json:"error"`
	}

	body := strings.TrimSpace(stdout.String() + stderr.String())
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("the refusal is not JSON: %q", body)
	}

	if code := doc.Code + doc.Error.Code; code != service.CodePrivilegedSession {
		t.Errorf("code = %q, want %s: %s", code, service.CodePrivilegedSession, body)
	}

	if doc.Hint+doc.Error.Hint == "" {
		t.Errorf("the refusal has no hint: %s", body)
	}

	if strings.Contains(body, home) || strings.Contains(body, strings.ReplaceAll(home, `\`, `\\`)) {
		t.Errorf("the refusal names the workspace: %s", body)
	}
}
