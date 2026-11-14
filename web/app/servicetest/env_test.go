// Package servicetest holds the tests of service.Service over the real store.
// They live under web/ because service/ may not import web/ (ADR-0001), and
// SDD-005 section 18 checks 5 to 16 need the SQLite store to mean anything.
package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

const (
	ui    = "local:ui"
	agent = "local:claude-code"
)

// debugOnce makes every command result validate itself (SDD-002 A.3 rule 3).
var debugOnce sync.Once

type env struct {
	t      testing.TB
	store  *store.Store
	svc    service.Service
	dir    workspace.Dir
	clock  *clock
	ids    atomic.Int64
	mu     sync.Mutex
	events []service.Event
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}

func newDir(tb testing.TB) workspace.Dir {
	tb.Helper()

	d, err := workspace.Resolve(func(string) string { return filepath.Join(tb.TempDir(), "ws") })
	if err != nil {
		tb.Fatal(err)
	}

	if err := d.EnsureLayout(); err != nil {
		tb.Fatal(err)
	}

	return d
}

func newEnv(tb testing.TB) *env {
	tb.Helper()

	return newEnvWith(tb, nil)
}

// newEnvWith is newEnv with the chance to complete the options of the service
// (the media and job machinery) before it is built; e.dir and e.store exist.
func newEnvWith(tb testing.TB, with func(e *env, o *service.Options)) *env {
	tb.Helper()

	e := &env{t: tb, dir: newDir(tb), clock: &clock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}}

	st, err := store.Open(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = st.Close() })

	e.store = st

	debugOnce.Do(func() { command.Default().Debug = true })

	opts := service.Options{
		Store: st, Registry: command.Default(), Now: e.clock.now, NewIDs: e.newID,
		Publish: func(ev service.Event) {
			e.mu.Lock()
			e.events = append(e.events, ev)
			e.mu.Unlock()
		},
	}

	if with != nil {
		with(e, &opts)
	}

	e.svc, err = service.New(opts)
	if err != nil {
		tb.Fatal(err)
	}

	return e
}

// newID is a valid identifier from a counter: digits are in the alphabet.
func (e *env) newID(prefix string) string {
	return fmt.Sprintf("%s_%010d", prefix, e.ids.Add(1))
}

func (e *env) as(actor string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: actor, Scopes: service.AllScopes})
}

func (e *env) newProject(name string) string {
	e.t.Helper()

	res, err := e.svc.ProjectCreate(e.as(ui), service.ProjectCreateRequest{Name: name})
	if err != nil {
		e.t.Fatal(err)
	}

	return res.ProjectID
}

func op(name, args string) command.Operation {
	return command.Operation{Op: name, Args: json.RawMessage(args)}
}

func (e *env) edit(actor, project string, base int64, ops ...command.Operation) (*service.EditResult, error) {
	e.t.Helper()

	return e.svc.Edit(e.as(actor), service.EditRequest{ProjectID: project, BaseRevision: base, Operations: ops})
}

func (e *env) mustEdit(actor, project string, base int64, ops ...command.Operation) *service.EditResult {
	e.t.Helper()

	res, err := e.edit(actor, project, base, ops...)
	if err != nil {
		e.t.Fatalf("edit: %v", err)
	}

	return res
}

// withTrack creates a project with one visual track and returns the ids.
func (e *env) withTrack() (project, track string) {
	e.t.Helper()

	project = e.newProject("p")
	res := e.mustEdit(ui, project, 0, op("track.add", `{"kind":"visual","name":"V"}`))

	return project, res.Created[0]
}

func (e *env) addText(actor, project, track string, base int64, start, text string) (*service.EditResult, error) {
	e.t.Helper()

	return e.edit(actor, project, base, op("text.add",
		fmt.Sprintf(`{"track_id":%q,"start":%q,"duration":"1s","text":%q}`, track, start, text)))
}

func code(err error) string {
	if se, ok := errors.AsType[*service.Error](err); ok {
		return se.Code
	}

	return ""
}

func serviceError(t *testing.T, err error) *service.Error {
	t.Helper()

	var se *service.Error
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want a *service.Error", err)
	}

	return se
}

func (e *env) head(project string) service.ProjectHead {
	e.t.Helper()

	h, err := e.store.Projects().Head(context.Background(), project)
	if err != nil {
		e.t.Fatal(err)
	}

	return h
}

func (e *env) document(project string) []byte {
	e.t.Helper()

	row, err := e.store.Projects().Get(context.Background(), project)
	if err != nil {
		e.t.Fatal(err)
	}

	return row.Document
}

func (e *env) operationCount(project string) int {
	e.t.Helper()

	n, err := e.store.Operations().Count(context.Background(), project, "")
	if err != nil {
		e.t.Fatal(err)
	}

	return int(n)
}
