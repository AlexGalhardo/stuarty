package servicetest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

func (e *env) checkpoints(project string) []service.Checkpoint {
	e.t.Helper()

	res, err := e.svc.CheckpointList(e.as(ui), service.CheckpointListRequest{ProjectID: project})
	if err != nil {
		e.t.Fatal(err)
	}

	return res.Checkpoints
}

func TestCheckpointCreateListRenameRemove(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack() // r1

	if _, err := e.addText(ui, project, track, 1, "0s", "a"); err != nil { // r2
		t.Fatal(err)
	}

	cp, err := e.svc.CheckpointCreate(e.as(ui), service.CheckpointCreateRequest{
		ProjectID: project, Name: "  before the intro ", Note: "the first cut",
	})
	if err != nil {
		t.Fatal(err)
	}

	if cp.Name != "before the intro" || cp.Revision != 2 || cp.Note != "the first cut" || cp.Actor != ui || cp.Auto {
		t.Fatalf("checkpoint = %+v", cp)
	}

	// It survives further edits and still names r2.
	if _, err := e.addText(ui, project, track, 2, "2s", "b"); err != nil {
		t.Fatal(err)
	}

	list := e.checkpoints(project)
	if len(list) != 1 || list[0].Revision != 2 || list[0].Name != "before the intro" {
		t.Fatalf("list = %+v", list)
	}

	// The same name again, and the name rules.
	_, err = e.svc.CheckpointCreate(
		e.as(ui),
		service.CheckpointCreateRequest{ProjectID: project, Name: "before the intro"},
	)
	if code(err) != command.CodeIDExists {
		t.Fatalf("duplicate name: %v", err)
	}

	for name, want := range map[string]string{
		"": command.CodeValidation, "a/b": command.CodeValidation, strings.Repeat("x", 61): command.CodeValidation,
	} {
		_, err = e.svc.CheckpointCreate(e.as(ui), service.CheckpointCreateRequest{ProjectID: project, Name: name})
		if code(err) != want {
			t.Errorf("name %q: %v, want %s", name, err, want)
		}
	}

	// A later revision, a revision that does not exist.
	later, err := e.svc.CheckpointCreate(e.as(agent), service.CheckpointCreateRequest{ProjectID: project, Name: "now"})
	if err != nil || later.Revision != 3 || later.Actor != agent {
		t.Fatalf("checkpoint at the head = %+v, %v", later, err)
	}

	_, err = e.svc.CheckpointCreate(
		e.as(ui),
		service.CheckpointCreateRequest{ProjectID: project, Name: "far", Revision: 99},
	)
	if code(err) != command.CodeValidation {
		t.Fatalf("a revision ahead: %v", err)
	}

	res, err := e.svc.CheckpointList(e.as(ui), service.CheckpointListRequest{ProjectID: project})
	if err != nil || res.Revision != 3 || len(res.Checkpoints) != 2 || res.Checkpoints[0].Name != "now" {
		t.Fatalf("list = %+v, %v (newest revision first)", res, err)
	}

	if !strings.Contains(res.Text, `r2 "before the intro" local:ui`) || !strings.Contains(res.Text, " - ") {
		t.Errorf("text:\n%s", res.Text)
	}

	renamed, err := e.svc.CheckpointRename(e.as(ui), service.CheckpointRenameRequest{
		ProjectID: project, Name: "before the intro", NewName: "intro done",
	})
	if err != nil || renamed.Name != "intro done" || renamed.Revision != 2 || renamed.Note != "the first cut" {
		t.Fatalf("rename = %+v, %v (the note stays when none is given)", renamed, err)
	}

	note := "new note"

	renamed, err = e.svc.CheckpointRename(e.as(ui), service.CheckpointRenameRequest{
		ProjectID: project, Name: "intro done", NewName: "intro done", Note: &note,
	})
	if err != nil || renamed.Note != "new note" {
		t.Fatalf("note change = %+v, %v", renamed, err)
	}

	_, err = e.svc.CheckpointRename(
		e.as(ui),
		service.CheckpointRenameRequest{ProjectID: project, Name: "now", NewName: "intro done"},
	)
	if code(err) != command.CodeIDExists {
		t.Fatalf("rename onto a taken name: %v", err)
	}

	_, err = e.svc.CheckpointRename(
		e.as(ui),
		service.CheckpointRenameRequest{ProjectID: project, Name: "nope", NewName: "x"},
	)
	if code(err) != command.CodeNotFound {
		t.Fatalf("rename of an unknown name: %v", err)
	}

	// Removing a checkpoint removes no revision.
	revisions := e.operationCount(project)

	gone, err := e.svc.CheckpointRemove(
		e.as(ui),
		service.CheckpointRemoveRequest{ProjectID: project, Name: "intro done"},
	)
	if err != nil || gone.Revision != 2 {
		t.Fatalf("remove = %+v, %v", gone, err)
	}

	if e.operationCount(project) != revisions || len(e.checkpoints(project)) != 1 {
		t.Fatalf(
			"revisions %d -> %d, checkpoints %d",
			revisions,
			e.operationCount(project),
			len(e.checkpoints(project)),
		)
	}

	_, err = e.svc.CheckpointRemove(e.as(ui), service.CheckpointRemoveRequest{ProjectID: project, Name: "intro done"})
	if code(err) != command.CodeNotFound {
		t.Fatalf("second remove: %v", err)
	}
}

func TestCheckpointScopesAndTrash(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, _ := e.withTrack()

	readOnly := service.WithCaller(
		e.as(ui),
		service.Caller{Actor: "key:reader", Scopes: []string{service.ScopeRead}},
	)

	if _, err := e.svc.CheckpointCreate(
		readOnly,
		service.CheckpointCreateRequest{ProjectID: project, Name: "x"},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("create with the read scope: %v", err)
	}

	if _, err := e.svc.CheckpointList(readOnly, service.CheckpointListRequest{ProjectID: project}); err != nil {
		t.Errorf("list with the read scope: %v", err)
	}

	if _, err := e.svc.CheckpointCreate(
		e.as(ui),
		service.CheckpointCreateRequest{ProjectID: project, Name: "keep"},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectTrash(e.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.CheckpointCreate(
		e.as(ui),
		service.CheckpointCreateRequest{ProjectID: project, Name: "y"},
	); code(
		err,
	) != command.CodeInvalidState {
		t.Errorf("create in the trash: %v", err)
	}

	if len(e.checkpoints(project)) != 1 {
		t.Errorf("the checkpoint of a trashed project is still listed")
	}
}

func TestCheckpointAutomaticBeforeAnAgentsFirstEditOfTheDay(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack() // r1 by the UI: no automatic checkpoint

	if n := len(e.checkpoints(project)); n != 0 {
		t.Fatalf("a person's edit made %d checkpoints", n)
	}

	// A dry run and a no-op by an agent make none.
	if res, err := e.svc.Edit(e.as(agent), service.EditRequest{
		ProjectID:    project,
		BaseRevision: 1,
		DryRun:       true,
		Operations: []command.Operation{
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"x"}`, track)),
		},
	}); err != nil || res.Applied {
		t.Fatalf("dry run = %+v, %v", res, err)
	}

	if n := len(e.checkpoints(project)); n != 0 {
		t.Fatalf("a dry run made %d checkpoints", n)
	}

	// The agent's first edit of the day: a checkpoint at the revision the edit started from.
	if _, err := e.addText(agent, project, track, 1, "0s", "a"); err != nil { // r2
		t.Fatal(err)
	}

	list := e.checkpoints(project)
	if len(list) != 1 || !list[0].Auto || list[0].Revision != 1 || list[0].Actor != agent ||
		list[0].Name != "auto local:claude-code 2026-11-14" {
		t.Fatalf("checkpoints = %+v", list)
	}

	// The second edit of the same day makes none; another agent makes its own; a new day makes a new one.
	if _, err := e.addText(agent, project, track, 2, "2s", "b"); err != nil { // r3
		t.Fatal(err)
	}

	if _, err := e.addText("key:key_h8v2n5q7w3", project, track, 3, "4s", "c"); err != nil { // r4
		t.Fatal(err)
	}

	if n := len(e.checkpoints(project)); n != 2 {
		t.Fatalf("after two agents, %d checkpoints, want 2", n)
	}

	e.clock.advance(24 * time.Hour)

	if _, err := e.addText(agent, project, track, 4, "6s", "d"); err != nil { // r5
		t.Fatal(err)
	}

	list = e.checkpoints(project)
	if len(list) != 3 || list[0].Revision != 4 || !strings.HasSuffix(list[0].Name, "2026-11-15") {
		t.Fatalf("after a new day, checkpoints = %+v", list)
	}

	// The checkpoint of r1 is what a revert to it will use: it survives later edits.
	if list[len(list)-1].Revision != 1 {
		t.Errorf("the oldest checkpoint = %+v", list[len(list)-1])
	}
}

func TestCheckpointAutomaticCeilingKeepsNamedOnes(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()

	if _, err := e.svc.CheckpointCreate(
		e.as(ui),
		service.CheckpointCreateRequest{ProjectID: project, Name: "mine"},
	); err != nil {
		t.Fatal(err)
	}

	base := int64(1)

	for day := range 35 {
		res, err := e.addText(agent, project, track, base, fmt.Sprintf("%ds", day), "x")
		if err != nil {
			t.Fatalf("day %d: %v", day, err)
		}

		base = res.Revision

		e.clock.advance(24 * time.Hour)
	}

	auto, named := 0, 0

	for _, c := range e.checkpoints(project) {
		if c.Auto {
			auto++
		} else {
			named++
		}
	}

	if auto != 30 || named != 1 {
		t.Fatalf("automatic %d, named %d; want 30 and 1", auto, named)
	}

	// The oldest automatic ones went; the newest stayed.
	all := e.checkpoints(project)
	if newest := all[0]; !newest.Auto || newest.Revision != base-1 {
		t.Errorf("newest = %+v, want an automatic checkpoint at r%d", newest, base-1)
	}
}
