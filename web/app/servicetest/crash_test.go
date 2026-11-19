//go:build crashtest

package servicetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// SDD-005 section 18 check 6: the child is this test binary, started with the
// crash point in its environment; the point ends it with exit code 137 and
// the parent reopens the database and checks the invariants of section 9.2.

const (
	roleEnv    = "STUARTY_CRASH_ROLE"
	projectEnv = "STUARTY_CRASH_PROJECT"
	trackEnv   = "STUARTY_CRASH_TRACK"
	actionEnv  = "STUARTY_CRASH_ACTION"
)

// TestCrashChild is the body of the child process; as a normal test it does nothing.
func TestCrashChild(t *testing.T) {
	if os.Getenv(roleEnv) != "child" {
		t.Skip("only runs as the child of a crash test")
	}

	dir := workspace.Dir(os.Getenv(workspace.EnvHome))

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	svc, err := service.New(service.Options{Store: st})
	if err != nil {
		t.Fatal(err)
	}

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: ui, Scopes: service.AllScopes})
	project, track := os.Getenv(projectEnv), os.Getenv(trackEnv)

	switch os.Getenv(actionEnv) {
	case "edit":
		_, err = svc.Edit(ctx, service.EditRequest{
			ProjectID:      project,
			BaseRevision:   1,
			IdempotencyKey: "crash-test-key",
			Operations: []command.Operation{
				op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"x"}`, track)),
			},
		})
	case "undo":
		_, err = svc.Undo(ctx, service.UndoRequest{ProjectID: project})
	case "redo":
		_, err = svc.Redo(ctx, service.UndoRequest{ProjectID: project})
	case "trash_empty":
		human := service.WithCaller(context.Background(), service.Caller{
			Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
		})
		_, err = svc.TrashEmpty(human, service.TrashEmptyRequest{})
	}

	// Reaching this line means the crash point was never hit.
	t.Fatalf("the child finished (%v) without reaching %s", err, os.Getenv(crashpoint.CrashEnv))
}

var commitPoints = []string{
	"commit.after_lock", "commit.after_load", "commit.after_apply", "commit.after_operation_append",
	"commit.after_project_commit", "commit.after_assets_sync", "commit.after_idempotency_put",
	"commit.before_commit", "commit.after_commit", "commit.after_publish",
}

// durable is where the commit is durable: from these points the project is at R+1.
func durable(point string) bool {
	return point == "commit.after_commit" || point == "commit.after_publish"
}

func TestCrashSuite(t *testing.T) {
	t.Parallel()

	for _, action := range []string{"edit", "undo", "redo"} {
		for _, point := range commitPoints {
			if action != "edit" && point == "commit.after_idempotency_put" {
				continue // undo and redo carry no key
			}

			t.Run(action+"/"+point, func(t *testing.T) {
				t.Parallel()
				crashAt(t, action, point)
			})
		}
	}
}

func crashAt(t *testing.T, action, point string) {
	t.Helper()

	e := newEnv(t)
	project, track := e.withTrack() // r1

	switch action {
	case "undo":
		// something to undo besides the track: the child undoes r2
		e.mustEdit(
			ui,
			project,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"4s","duration":"1s","text":"seed"}`, track)),
		)
	case "redo":
		e.mustEdit(
			ui,
			project,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"4s","duration":"1s","text":"seed"}`, track)),
		)

		if _, err := e.undo(ui, project); err != nil {
			t.Fatal(err)
		}
	}

	before := e.head(project)
	docBefore := string(e.document(project))

	if err := e.store.Close(); err != nil { // the child owns the database now
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashChild$", "-test.v")
	cmd.Env = append(os.Environ(),
		roleEnv+"=child", workspace.EnvHome+"="+string(e.dir), projectEnv+"="+project, trackEnv+"="+track,
		actionEnv+"="+action, crashpoint.CrashEnv+"="+point)

	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out

	err := cmd.Run()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 137 {
		t.Fatalf("child = %v, want exit 137 at %s\n%s", err, point, out.String())
	}

	st, err := store.Open(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatalf("reopen after the crash: %v", err)
	}

	defer st.Close()

	after, err := st.Projects().Get(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}

	if durable(point) {
		if after.Revision != before.Revision+1 {
			t.Errorf(
				"crash at %s: revision %d, want %d (the commit was durable)",
				point,
				after.Revision,
				before.Revision+1,
			)
		}
	} else if after.Revision != before.Revision || string(after.Document) != docBefore {
		t.Errorf(
			"crash at %s: revision %d and a changed document, want revision %d untouched",
			point,
			after.Revision,
			before.Revision,
		)
	}

	invariants(t, st, project)
}

// SDD-005 section 18 check 21: a crash before the commit of an emptying changes
// nothing; after it the project is gone and the database is consistent.
func TestCrashTrashEmpty(t *testing.T) {
	t.Parallel()

	for _, point := range []string{"trash_empty.after_lock", "trash_empty.before_commit", "trash_empty.after_commit"} {
		t.Run(point, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			project, track := e.withTrack()

			if _, err := e.svc.ProjectTrash(e.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
				t.Fatal(err)
			}

			if err := e.store.Close(); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashChild$", "-test.v")
			cmd.Env = append(os.Environ(),
				roleEnv+"=child", workspace.EnvHome+"="+string(e.dir), projectEnv+"="+project, trackEnv+"="+track,
				actionEnv+"=trash_empty", crashpoint.CrashEnv+"="+point)

			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out

			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 137 {
				t.Fatalf("child = %v, want exit 137 at %s: %s", err, point, out.String())
			}

			st, err := store.Open(
				context.Background(),
				e.dir,
				store.Options{SchemaVersion: timeline.CurrentSchemaVersion},
			)
			if err != nil {
				t.Fatalf("reopen after the crash: %v", err)
			}

			defer st.Close()

			_, err = st.Projects().Get(context.Background(), project)
			if gone := point == "trash_empty.after_commit"; gone != errors.Is(err, service.ErrNotFound) {
				t.Errorf("crash at %s: project lookup = %v", point, err)
			}
		})
	}
}
