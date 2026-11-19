package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/web/app/owner"
)

// SDD-005 section 7 step 2: the message of a workspace written by a newer version reaches every
// surface that opens the workspace, not only serve, and nothing is written.
func TestNewerWorkspaceIsRefusedWithItsReasonOnEverySurface(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")

	getenv := func(k string) string {
		if k == "STUARTY_HOME" {
			return home
		}

		return ""
	}

	o, err := owner.Open(context.Background(), owner.Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	_, err = o.Store.DB().ExecContext(
		context.Background(),
		"INSERT INTO migrations (id, batch, checksum, ran_at) VALUES ('29990101000000_from_the_future', 9, 'x', ?)",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runBinary(t, bin, home, "project:list", "--json")

	var doc struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &doc); jerr != nil || stderr != "" || code != 1 {
		t.Fatalf("project:list --json: exit %d, %q, %q", code, stdout, stderr)
	}

	if doc.Code == "E_INTERNAL" || !strings.Contains(doc.Message, "newer version") ||
		!strings.Contains(doc.Message, "29990101000000_from_the_future") {
		t.Errorf("project:list --json = %+v, want the newer version reason", doc)
	}

	stdout, stderr, code = runBinary(t, bin, home, "project:list")
	if code != 1 || stdout != "" || strings.Contains(stderr, "E_INTERNAL") ||
		!strings.Contains(stderr, "newer version") {
		t.Errorf("project:list: exit %d, %q, %q", code, stdout, stderr)
	}

	_, stderr, code = runBinary(t, bin, home, "serve")
	if code != 1 || !strings.Contains(stderr, "newer version") {
		t.Errorf("serve: exit %d, %q", code, stderr)
	}
}
