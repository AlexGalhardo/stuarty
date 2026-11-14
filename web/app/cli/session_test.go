package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
)

// realRuntime runs the CLI over the real in-process Service: workspace.Open,
// the SQLite store and service.New, in a temporary workspace.
type realRuntime struct {
	t      *testing.T
	ws     string
	rt     *Runtime
	stdin  string
	actor  string
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newRealRuntime(t *testing.T) *realRuntime {
	t.Helper()

	r := &realRuntime{t: t, ws: filepath.Join(t.TempDir(), "ws")}
	r.rt = &Runtime{
		Stdout: &r.stdout, Stderr: &r.stderr,
		Getenv:   func(string) string { return "" },
		Getwd:    os.Getwd,
		Resolver: LocalResolver{Getenv: func(string) string { return "" }},
	}

	return r
}

// do runs a command with --workspace and --json and returns the exit code and the document.
func (r *realRuntime) do(args ...string) (int, map[string]any) {
	r.t.Helper()

	actor := r.actor
	if actor == "" {
		actor = "claude-code"
	}

	r.stdout.Reset()
	r.stderr.Reset()
	r.rt.Stdin = strings.NewReader(r.stdin)

	code := r.rt.Execute(
		context.Background(),
		args[0],
		append(args[1:], "--workspace", r.ws, "--json", "--actor", actor),
	)

	if r.stderr.Len() != 0 {
		r.t.Fatalf("stderr must be empty with --json: %q", r.stderr.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(r.stdout.Bytes(), &doc); err != nil {
		r.t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout.String())
	}

	return code, doc
}

// TestSessionAWithoutMedia is the part of session A of SDD-003 section 5.6 that
// needs no media: create, two tracks, a lint, the project view, undo, and the
// conflict of session B. The import, the clip, the view and the export join
// when media:import, view and export exist.
func TestSessionAWithoutMedia(t *testing.T) {
	r := newRealRuntime(t)

	code, created := r.do("project:create", "--name", "Launch teaser")
	if code != 0 || created["revision"] != float64(0) || created["created"] != true {
		t.Fatalf("project:create: exit %d, %v", code, created)
	}

	id, _ := created["project_id"].(string)
	if !strings.HasPrefix(id, "prj_") {
		t.Fatalf("project id = %v", created["project_id"])
	}

	r.stdin = `[{"op":"track.add","args":{"kind":"visual","name":"Video"}},{"op":"track.add","args":{"kind":"visual","name":"Captions"}}]`

	code, edited := r.do("edit", "--project", id, "--ops", "-")
	if code != 0 || edited["revision"] != float64(1) || edited["base_revision"] != float64(0) ||
		edited["applied"] != true {
		t.Fatalf("edit: exit %d, %v", code, edited)
	}

	if code, listed := r.do("project:list"); code != 0 || len(listed["projects"].([]any)) != 1 {
		t.Fatalf("project:list: exit %d, %v", code, listed)
	}

	if code, linted := r.do("lint", "--project", id); code != 0 || linted["project_id"] != id {
		t.Fatalf("lint: exit %d, %v", code, linted)
	}

	if code, shown := r.do("project:show", id); code != 0 || shown["revision"] != float64(1) {
		t.Fatalf("project:show: exit %d, %v", code, shown)
	}

	// Session B, the conflict: another actor renames a track at revision 2, then
	// this actor plans a rename of the same track against revision 1. The
	// conflict is retryable and exits 2.
	created1, _ := edited["created"].([]any)
	track, _ := created1[0].(string)

	r.actor = "other"

	if code, renamed := r.do(
		"edit",
		"--project",
		id,
		"track.update",
		"track_id="+track,
		"name=Theirs",
	); code != 0 ||
		renamed["revision"] != float64(2) {
		t.Fatalf("shorthand edit: exit %d, %v", code, renamed)
	}

	r.actor = ""

	code, conflict := r.do("edit", "--project", id, "--base", "1", "track.update", "track_id="+track, "name=Mine")
	if code != ExitTransient || conflict["code"] != command.CodeRevisionConflict || conflict["retryable"] != true {
		t.Fatalf("a stale base must be a conflict with exit 2: exit %d, %v", code, conflict)
	}

	// Dry run changes nothing.
	r.stdin = `[{"op":"track.add","args":{"kind":"visual","name":"Dry"}}]`

	code, dry := r.do("edit", "--project", id, "--ops", "-", "--dry-run")
	if code != 0 || dry["dry_run"] != true || dry["applied"] == true {
		t.Fatalf("dry run: exit %d, %v", code, dry)
	}

	if _, shown := r.do("project:show", id); shown["revision"] != float64(2) {
		t.Fatalf("a dry run must not change the revision: %v", shown)
	}

	code, undone := r.do("undo", "--project", id)
	if code != 0 || undone["kind"] != "undo" || undone["revision"] != float64(3) {
		t.Fatalf("undo: exit %d, %v", code, undone)
	}

	if code, hist := r.do("history", "--project", id); code != 0 || len(hist["entries"].([]any)) != 3 {
		t.Fatalf("history: exit %d, %v", code, hist)
	}

	code, nothing := r.do("redo", "--project", id)
	if code != 0 || nothing["kind"] != "redo" {
		t.Fatalf("redo: exit %d, %v", code, nothing)
	}

	code, missing := r.do("project:show", "prj_unknown0001")
	if code != ExitFailure || missing["code"] != command.CodeNotFound {
		t.Fatalf("an unknown project: exit %d, %v", code, missing)
	}
}

func TestHumanSessionPrintsTheInProcessNote(t *testing.T) {
	r := newRealRuntime(t)

	r.rt.Stdin = strings.NewReader("")

	code := r.rt.Execute(context.Background(), "project:create", []string{"--name", "x", "--workspace", r.ws})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr.String())
	}

	if !strings.HasPrefix(r.stdout.String(), "note:") && !strings.Contains(r.stdout.String(), "created prj_") {
		t.Fatalf("stdout = %q", r.stdout.String())
	}

	if !strings.Contains(r.stderr.String(), "no server owns "+r.ws) {
		t.Fatalf("stderr = %q", r.stderr.String())
	}
}

// The tests below are the end-to-end sessions of SDD-003 section 5.6 and the
// latency budget of NFR-024. They need Service methods that do not exist yet.

func TestSessionAImportCutCaptionExport(t *testing.T) {
	t.Skip("needs media:import, view and export: tasks P1-SRV-004, P1-RND-005 and P1-RND-004")
}

func TestSessionBBlockedExport(t *testing.T) {
	t.Skip("needs export and its lint gate: task P1-RND-004")
}

func BenchmarkCLIRead(b *testing.B) {
	b.Skip("needs the server and the CLI client of task P1-SRV-005 (NFR-024 is measured against a running owner)")
}
