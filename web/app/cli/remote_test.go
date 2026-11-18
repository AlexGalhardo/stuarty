package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/vukemtest"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

// startOwner runs the HTTP API over a real store, as a server that owns the
// workspace would, and writes the runtime.json and the token a client reads.
func startOwner(tb testing.TB) workspace.Dir {
	tb.Helper()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(tb.TempDir(), "ws") })
	if err != nil {
		tb.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		tb.Fatal(err)
	}

	token, err := dir.EnsureToken()
	if err != nil {
		tb.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = st.Close() })

	svc, err := service.New(service.Options{Store: st})
	if err != nil {
		tb.Fatal(err)
	}

	deps := &httpapi.Deps{
		Service: svc, Registry: command.Default(), Workspace: dir, Token: token,
		Sessions: auth.NewSessions(time.Now), Launch: auth.NewLaunch(time.Now), Now: time.Now,
	}

	app := vukemtest.New(tb, func(opts ...vukem.Option) (*vukem.App, error) { return bootstrap.NewWith(deps, opts...) })

	srv := httptest.NewServer(app.Handler())
	tb.Cleanup(srv.Close)

	err = dir.WriteRuntime(workspace.Runtime{
		Schema: 1, PID: os.Getpid(), Addr: srv.Listener.Addr().String(), Mode: workspace.ModeLocal,
		StartedAt: time.Now().UTC(), Version: "test",
	})
	if err != nil {
		tb.Fatal(err)
	}

	return dir
}

// TestClientModeAgainstARunningOwner runs the CLI the way an agent does while a
// server owns the workspace (SDD-003 section 5.2 step 2): every command goes
// over HTTP through service/remote, the actor travels in the client header, and
// nothing is printed about running in process.
func TestClientModeAgainstARunningOwner(t *testing.T) {
	// workspace.Open probes the owner with a 500 ms timeout (workspace.ProbeTimeout); under the
	// slowdown of coverage instrumentation on a loaded machine the probe can miss and the command
	// falls back to running in process, which is correct behaviour but not what this test checks.
	if testing.CoverMode() != "" {
		t.Skip("the owner probe is a wall-clock deadline; coverage instrumentation can miss it")
	}

	dir := startOwner(t)

	home := func(k string) string {
		if k == workspace.EnvHome {
			return string(dir)
		}

		return ""
	}

	var stdout, stderr bytes.Buffer

	rt := &Runtime{
		Stdout:   &stdout,
		Stderr:   &stderr,
		Getenv:   home,
		Getwd:    os.Getwd,
		Resolver: LocalResolver{Getenv: home},
	}

	do := func(stdin string, args ...string) (int, map[string]any) {
		t.Helper()

		stdout.Reset()
		stderr.Reset()

		rt.Stdin = strings.NewReader(stdin)
		code := rt.Execute(context.Background(), args[0], append(args[1:], "--json", "--actor", "claude-code"))

		if stderr.Len() != 0 {
			t.Fatalf("%v: stderr must be empty with --json: %q", args, stderr.String())
		}

		return code, oneJSONDocument(t, stdout.String())
	}

	code, created := do("", "project:create", "--name", "Launch teaser")
	if code != 0 || created["created"] != true {
		t.Fatalf("project:create: exit %d, %v", code, created)
	}

	id, _ := created["project_id"].(string)

	code, edited := do(
		`[{"op":"track.add","args":{"kind":"visual","name":"Video"}}]`,
		"edit",
		"--project",
		id,
		"--ops",
		"-",
	)
	if code != 0 || edited["revision"] != float64(1) || edited["base_revision"] != float64(0) ||
		edited["actor"] != "local:claude-code" {
		t.Fatalf("edit (base 0 read from the project, actor from the header): exit %d, %v", code, edited)
	}

	if code, shown := do("", "project:show", id); code != 0 || shown["revision"] != float64(1) {
		t.Fatalf("project:show: exit %d, %v", code, shown)
	}

	if code, undone := do("", "undo", "--project", id); code != 0 || undone["kind"] != "undo" {
		t.Fatalf("undo: exit %d, %v", code, undone)
	}

	if code, hist := do(
		"",
		"history",
		"--project",
		id,
	); code != 0 ||
		len(hist["entries"].([]any)) != 2 {
		t.Fatalf("history: exit %d, %v", code, hist)
	}

	if code, linted := do("", "lint", "--project", id); code != 0 || linted["project_id"] != id {
		t.Fatalf("lint: exit %d, %v", code, linted)
	}

	if code, help := do("", "command:help"); code != 0 || help["commands"] == nil {
		t.Fatalf("command:help: exit %d, %v", code, help)
	}

	code, missing := do("", "project:show", "prj_unknown0001")
	if code != ExitFailure || missing["code"] != command.CodeNotFound {
		t.Fatalf("an error envelope must survive the round trip: exit %d, %v", code, missing)
	}

	// A human write over HTTP prints no in-process note.
	stdout.Reset()
	stderr.Reset()

	rt.Stdin = strings.NewReader("")

	if code := rt.Execute(
		context.Background(),
		"project:create",
		[]string{"--name", "Second"},
	); code != 0 ||
		stderr.Len() != 0 {
		t.Fatalf("human write: exit %d, stderr %q", code, stderr.String())
	}
}

func TestAWrongTokenIsUnauthorizedAndExitsOne(t *testing.T) {
	dir := startOwner(t)

	if _, err := dir.RotateToken(); err != nil {
		t.Fatal(err)
	}

	home := func(k string) string {
		if k == workspace.EnvHome {
			return string(dir)
		}

		return ""
	}

	var stdout, stderr bytes.Buffer

	rt := &Runtime{
		Stdout:   &stdout,
		Stderr:   &stderr,
		Getenv:   home,
		Getwd:    os.Getwd,
		Resolver: LocalResolver{Getenv: home},
	}

	if code := rt.Execute(context.Background(), "project:list", []string{"--json"}); code != ExitFailure {
		t.Fatalf("a token the server does not know must exit 1: %d %s", code, stdout.String())
	}

	if doc := oneJSONDocument(t, stdout.String()); doc["code"] != service.CodeUnauthorized {
		t.Fatalf("doc = %v", doc)
	}
}
