package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/service"
)

func TestWriteRollsBackAndRunsHooksAfterCommit(t *testing.T) {
	s := testStore(t)
	boom := errors.New("boom")

	var hooks []string
	err := s.Write(ctx, func(_ context.Context, tx service.Tx) error {
		mustCreate(t, tx, newProject("prj_rolled", 1))
		tx.AfterCommit(func() { hooks = append(hooks, "rolled back") })
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Projects().Get(ctx, "prj_rolled"); !errors.Is(err, service.ErrNotFound) {
		t.Error("a failed Write must leave nothing")
	}
	if len(hooks) != 0 {
		t.Errorf("hooks of a rolled back transaction ran: %v", hooks)
	}

	err = s.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		mustCreate(t, tx, newProject("prj_ok", 1))
		tx.AfterCommit(func() { hooks = append(hooks, "first") })
		tx.AfterCommit(func() { hooks = append(hooks, "second") })
		// The transaction sees its own write; the reader pool does not yet.
		if _, err := tx.Projects().Get(ctx, "prj_ok"); err != nil {
			t.Errorf("read your own write: %v", err)
		}
		if len(hooks) != 0 {
			t.Error("hooks must not run before the commit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 2 || hooks[0] != "first" || hooks[1] != "second" {
		t.Errorf("hooks = %v, want both in order before Write returned", hooks)
	}
	if _, err := s.Projects().Get(ctx, "prj_ok"); err != nil {
		t.Errorf("committed project: %v", err)
	}
}

func TestWriteProjectLoadsRowAndMapsNotFound(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))
	if err := s.Projects().SetState(ctx, "prj_a", service.ProjectTrashed, "local:ui", 2); err != nil {
		t.Fatal(err)
	}

	err := s.WriteProject(ctx, "prj_a", func(_ context.Context, tx service.ProjectTx) error {
		if p := tx.Project(); p.ID != "prj_a" || p.State != service.ProjectTrashed || len(p.Document) == 0 {
			t.Errorf("Project() = %+v (a trashed project is passed to fn, with its document)", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	called := false
	err = s.WriteProject(
		ctx,
		"prj_missing",
		func(context.Context, service.ProjectTx) error { called = true; return nil },
	)
	if !errors.Is(err, service.ErrNotFound) || called {
		t.Errorf("err = %v, called = %v; want ErrNotFound without calling fn", err, called)
	}
	if n := len(s.locks.m); n != 0 {
		t.Errorf("%d lock entries left behind", n)
	}
}

// SDD-005 section 18 check 8 at the store level: commits to one project are
// strictly ordered, and so are the hooks that publish them.
func TestWriteProjectOrdersCommitsAndHooks(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1), newProject("prj_b", 1))

	const perProject = 16
	var (
		mu     sync.Mutex
		events = map[string][]int64{}
		wg     sync.WaitGroup
	)
	commit := func(id string) {
		err := s.WriteProject(ctx, id, func(ctx context.Context, tx service.ProjectTx) error {
			rev := tx.Project().Revision
			upd := service.ProjectUpdate{
				Name:          "n",
				SchemaVersion: 1,
				Document:      []byte(`{}`),
				DocSHA256:     "x",
				UpdatedMs:     rev + 10,
			}
			if err := tx.Projects().Commit(ctx, id, rev, upd); err != nil {
				return err
			}
			if err := tx.Operations().Append(ctx, op(id, rev+1, service.OpEdit, "local:ui")); err != nil {
				return err
			}
			tx.AfterCommit(func() {
				time.Sleep(time.Millisecond) // widen the window a missing lock would need
				mu.Lock()
				events[id] = append(events[id], rev+1)
				mu.Unlock()
			})
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	}
	for range perProject {
		wg.Go(func() { commit("prj_a") })
		wg.Go(func() { commit("prj_b") })
	}
	wg.Wait()

	for _, id := range []string{"prj_a", "prj_b"} {
		h, _ := s.Projects().Head(ctx, id)
		if h.Revision != perProject {
			t.Errorf("%s revision = %d, want %d", id, h.Revision, perProject)
		}
		for i, rev := range events[id] {
			if rev != int64(i+1) {
				t.Errorf("%s events out of order: %v", id, events[id])
				break
			}
		}
		page, _, _ := s.Operations().Page(ctx, id, service.HistoryQuery{Limit: 200})
		if len(page) != perProject {
			t.Errorf("%s has %d operations rows, want %d contiguous", id, len(page), perProject)
		}
	}
	if n := len(s.locks.m); n != 0 {
		t.Errorf("%d lock entries left behind", n)
	}
}

func TestWriteProjectWaitsWhileContextIsLive(t *testing.T) {
	s := testStore(t)
	mustCreate(t, s, newProject("prj_a", 1))

	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.WriteProject(ctx, "prj_a", func(context.Context, service.ProjectTx) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	err := s.WriteProject(short, "prj_a", func(context.Context, service.ProjectTx) error {
		t.Error("fn must not run while the lock is held")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context error", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := len(s.locks.m); n != 0 {
		t.Errorf("%d lock entries left behind", n)
	}
}
