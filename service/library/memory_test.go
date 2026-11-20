package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

var (
	person = Caller{Actor: "local:ui", Person: true}
	agent  = Caller{Actor: "local:claude-code-k3m9x2"}
)

func TestMemoryPersonWritesAKeptNoteAndAnAgentAPendingOne(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	_, e, err := m.SaveMemory(
		ctx,
		person,
		MemoryInput{Description: "Viewers skip long intros", Body: "Cut it.\n", Tags: []string{"retention"}},
	)
	if err != nil || !e.Kept || e.Name != "viewers-skip-long-intros" || e.Actor != "local:ui" ||
		e.Scope != "workspace" {
		t.Fatalf("%v %+v", err, e)
	}

	_, e, err = m.SaveMemory(
		ctx,
		agent,
		MemoryInput{Name: "thumbnail-faces", Description: "Faces get clicks", Body: "x"},
	)
	if err != nil || e.Kept || e.Actor != agent.Actor {
		t.Fatalf("%v %+v", err, e)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, "memory", "thumbnail-faces.md"))
	if !strings.Contains(string(raw), "kept: false") || !strings.Contains(string(raw), "actor: ") {
		t.Fatalf("an agent's note records its actor and is marked until a person keeps it:\n%s", raw)
	}

	pending, _ := m.List(ctx, core.KindMemory, Filter{})
	if len(pending) != 1 || pending[0].Name != "viewers-skip-long-intros" {
		t.Fatalf("the default list holds kept notes only: %+v", pending)
	}

	all, _ := m.List(ctx, core.KindMemory, Filter{IncludeUnreviewed: true})
	if len(all) != 2 {
		t.Fatalf("%+v", all)
	}
}

func TestMemoryOnlyAPersonKeepsAndSetsTheActor(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()
	yes := true

	if _, _, err := m.SaveMemory(
		ctx,
		agent,
		MemoryInput{Name: "a", Description: "d", Keep: &yes},
	); !errors.Is(
		err,
		ErrForbidden,
	) {
		t.Fatalf("an agent cannot keep a note: %v", err)
	}

	if _, _, err := m.SaveMemory(ctx, agent, MemoryInput{Name: "a", Description: "d", Body: "x"}); err != nil {
		t.Fatal(err)
	}

	// a person keeps it
	if _, e, err := m.SaveMemory(
		ctx,
		person,
		MemoryInput{Name: "a", Description: "d", Body: "x"},
	); err != nil ||
		!e.Kept {
		t.Fatalf("%v %+v", err, e)
	}

	// and a person may release it
	no := false
	if _, e, err := m.SaveMemory(
		ctx,
		person,
		MemoryInput{Name: "a", Description: "d", Body: "x", Keep: &no},
	); err != nil ||
		e.Kept {
		t.Fatalf("%v %+v", err, e)
	}
}

func TestMemoryAgentCannotOverwriteOrRemoveAKeptNote(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	if _, _, err := m.SaveMemory(
		ctx,
		person,
		MemoryInput{Name: "rule", Description: "The rule", Body: "Original.\n"},
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.SaveMemory(
		ctx,
		agent,
		MemoryInput{Name: "rule", Description: "Changed", Body: "Hijacked.\n"},
	); !errors.Is(
		err,
		ErrForbidden,
	) {
		t.Fatalf("%v", err)
	}

	if err := m.DeleteMemory(ctx, agent, "rule"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("%v", err)
	}

	mem, _, _ := m.ReadMemory(ctx, "rule")
	if mem.Body != "Original.\n" || !mem.Kept {
		t.Fatalf("%+v", mem)
	}

	// the agent adds a new note instead, and may rewrite and remove its own
	if _, _, err := m.SaveMemory(ctx, agent, MemoryInput{Name: "mine", Description: "Mine", Body: "1"}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.SaveMemory(ctx, agent, MemoryInput{Name: "mine", Description: "Mine", Body: "2"}); err != nil {
		t.Fatal(err)
	}

	if err := m.DeleteMemory(ctx, agent, "mine"); err != nil {
		t.Fatal(err)
	}

	if err := m.DeleteMemory(ctx, person, "rule"); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryCeilingPerActorPerDay(t *testing.T) {
	t.Parallel()

	m, idx, _ := newManager(t)
	ctx := context.Background()

	for i := range core.MaxNotesPerDay {
		if _, _, err := m.SaveMemory(
			ctx,
			agent,
			MemoryInput{Name: fmt.Sprintf("note-%d", i), Description: "d", Body: "x"},
		); err != nil {
			t.Fatalf("note %d: %v", i, err)
		}
	}

	var le *LimitError
	if _, _, err := m.SaveMemory(
		ctx,
		agent,
		MemoryInput{Name: "one-more", Description: "d", Body: "x"},
	); !errors.As(
		err,
		&le,
	) {
		t.Fatalf("the ceiling must stop the next note: %v", err)
	}

	// rewriting one of its own notes is not a new note
	if _, _, err := m.SaveMemory(ctx, agent, MemoryInput{Name: "note-1", Description: "again", Body: "y"}); err != nil {
		t.Fatal(err)
	}

	// another agent and a person are not limited by it
	other := Caller{Actor: "local:other-agent"}
	if _, _, err := m.SaveMemory(ctx, other, MemoryInput{Name: "other-1", Description: "d", Body: "x"}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.SaveMemory(ctx, person, MemoryInput{Name: "person-1", Description: "d", Body: "x"}); err != nil {
		t.Fatal(err)
	}

	// the count survives a rebuilt index
	if err := idx.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Reindex(ctx); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.SaveMemory(
		ctx,
		agent,
		MemoryInput{Name: "after-rebuild", Description: "d", Body: "x"},
	); !errors.As(
		err,
		&le,
	) {
		t.Fatalf("%v", err)
	}

	// the next day it may write again
	next := New(m.files.dir, idx, func() time.Time { return testNow.Add(24 * time.Hour) })
	if _, _, err := next.SaveMemory(
		ctx,
		agent,
		MemoryInput{Name: "tomorrow", Description: "d", Body: "x"},
	); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryCeilingOfNotesInTheWorkspace(t *testing.T) {
	t.Parallel()

	m, idx, _ := newManager(t)
	ctx := context.Background()

	for i := range core.MaxNotes {
		e := Entry{
			Key:   fmt.Sprintf("memory/n%d", i),
			Kind:  core.KindMemory,
			Name:  fmt.Sprintf("n%d", i),
			State: StateOK,
		}
		if err := idx.Put(ctx, e, ""); err != nil {
			t.Fatal(err)
		}
	}

	// The scan drops rows that have no file, so give the manager files for them.
	for i := range core.MaxNotes {
		write(t, filepath.Join(m.files.dir, "memory", fmt.Sprintf("n%d.md", i)),
			fmt.Sprintf("---\nname: n%d\ndescription: d\n---\nx\n", i))
	}

	var le *LimitError
	if _, _, err := m.SaveMemory(
		ctx,
		person,
		MemoryInput{Name: "over", Description: "d", Body: "x"},
	); !errors.As(
		err,
		&le,
	) {
		t.Fatalf("%v", err)
	}
}

func TestMemoryScopeAndProjectKeys(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	const prj = "prj_0123456789"

	limited := Caller{Actor: "key:key_0123456789", ProjectID: prj}

	if _, _, err := m.SaveMemory(
		ctx,
		limited,
		MemoryInput{Name: "w", Description: "d", Scope: "workspace", Body: "x"},
	); !errors.Is(
		err,
		ErrForbidden,
	) {
		t.Fatalf("a key limited to a project does not write to the workspace: %v", err)
	}

	if _, _, err := m.SaveMemory(
		ctx,
		limited,
		MemoryInput{Name: "p", Description: "d", Scope: prj, Body: "x"},
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.SaveMemory(ctx, person, MemoryInput{Name: "ws", Description: "d", Body: "x"}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.SaveMemory(
		ctx,
		person,
		MemoryInput{Name: "other", Description: "d", Scope: "prj_abcdefghjk", Body: "x"},
	); err != nil {
		t.Fatal(err)
	}

	got, _ := m.List(ctx, core.KindMemory, Filter{IncludeUnreviewed: true, ProjectID: prj})
	if len(got) != 2 {
		t.Fatalf("a limited key sees its project and the workspace: %+v", got)
	}

	scoped, _ := m.List(ctx, core.KindMemory, Filter{IncludeUnreviewed: true, Scope: prj})
	if len(scoped) != 1 || scoped[0].Name != "p" {
		t.Fatalf("%+v", scoped)
	}

	if err := m.DeleteMemory(ctx, limited, "other"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("%v", err)
	}

	if _, _, err := m.SaveMemory(
		ctx,
		person,
		MemoryInput{Name: "bad", Description: "d", Scope: "elsewhere", Body: "x"},
	); err == nil {
		t.Fatal("a scope that is not workspace or a project is refused")
	}

	// with no scope, the note of such a key goes to its project (the MCP tool has no scope argument)
	if mem, _, err := m.SaveMemory(ctx, limited, MemoryInput{Name: "n", Description: "d", Body: "x"}); err != nil ||
		mem.Scope != prj {
		t.Fatalf("%v %+v", err, mem)
	}
}

func TestMemoryDerivedNamesAreUnique(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	var names []string

	for range 3 {
		_, e, err := m.SaveMemory(ctx, person, MemoryInput{Description: "Same words", Body: "x"})
		if err != nil {
			t.Fatal(err)
		}

		names = append(names, e.Name)
	}

	if strings.Join(names, ",") != "same-words,same-words-2,same-words-3" {
		t.Fatalf("%v", names)
	}

	if _, _, err := m.SaveMemory(ctx, person, MemoryInput{Description: "???", Body: "x"}); err == nil {
		t.Fatal("a description with no letters makes no name")
	}
}

func TestMemoryRefusesWhatBreaksTheRules(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	for name, in := range map[string]MemoryInput{
		"no description": {Name: "a", Body: "x"},
		"long body":      {Name: "a", Description: "d", Body: strings.Repeat("x", core.MaxMemoryBody+1)},
		"bad tag":        {Name: "a", Description: "d", Tags: []string{"Bad Tag"}},
		"bad name":       {Name: "A b", Description: "d"},
	} {
		var ve *ValidationError
		if _, _, err := m.SaveMemory(ctx, person, in); !errors.As(err, &ve) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMemoryRewriteKeepsTheCreationTime(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	first, _, err := m.SaveMemory(ctx, person, MemoryInput{Name: "a", Description: "d", Body: "1"})
	if err != nil {
		t.Fatal(err)
	}

	later := New(m.files.dir, m.idx, func() time.Time { return testNow.Add(time.Hour) })

	second, _, err := later.SaveMemory(ctx, person, MemoryInput{Name: "a", Description: "d", Body: "2"})
	if err != nil || second.Created != first.Created || second.Updated == first.Updated {
		t.Fatalf("%v %+v %+v", err, first, second)
	}
}
