package testpublish

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// The two callers of the tests: the person at the app and an agent with a local token.
const (
	UI    = "local:ui"
	Token = "local:claude-code"
)

// ShiftedClock is the real clock moved by Offset. A scheduler that reads it sees a lease or a time that is
// further on than the wall clock, so a test waits for nothing: one process takes a lease, is killed, and the
// next one runs on a clock past the end of that lease.
type ShiftedClock struct{ Offset time.Duration }

// Now implements the clock of the scheduler and of the service.
func (c ShiftedClock) Now() time.Time { return time.Now().Add(c.Offset) }

// After waits for real: a heartbeat or a poll is a real interval.
func (ShiftedClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Options is what a process needs to open the service of a workspace.
type Options struct {
	// Dir is the workspace folder; it is created with its layout when it does not exist.
	Dir string
	// Connector is the network the service publishes to.
	Connector social.Connector
	// VaultKey is the master key of the vault (32 bytes): every process of one test shares it, so the secret
	// one process sealed is the secret the next one opens.
	VaultKey []byte
	// Offset moves the clock of the service and of its scheduler from the wall clock.
	Offset time.Duration
}

// Env is the service of a workspace over the real store, as a process of the product has it.
type Env struct {
	Dir   workspace.Dir
	Store *store.Store
	Svc   service.Service
	Sched *pub.Scheduler
}

// Open opens the store of the workspace, the vault and the service with the connector registered. Closing the
// Env closes the store; the files stay.
func Open(ctx context.Context, o Options) (*Env, error) {
	dir, err := workspace.Resolve(func(string) string { return o.Dir })
	if err != nil {
		return nil, err
	}

	if err := dir.EnsureLayout(); err != nil {
		return nil, err
	}

	st, err := store.Open(ctx, dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		return nil, err
	}

	reg := social.NewRegistry()
	if err := reg.Register(o.Connector); err != nil {
		_ = st.Close()

		return nil, err
	}

	clock := ShiftedClock{Offset: o.Offset}

	svc, err := service.New(service.Options{
		Store: st, Registry: command.Default(), Now: clock.Now, SchedulerClock: clock,
		Limits:    service.Limits{MinFreeBytes: -1},
		Workspace: string(dir),
		Connections: connections.NewManager(connections.Options{
			Store: st.Connections(), Now: clock.Now,
			Vault: vault.Open(filepath.Join(string(dir), "vault.json"), vault.NewMemoryProvider(o.VaultKey)),
		}),
		Social: reg,
	})
	if err != nil {
		_ = st.Close()

		return nil, err
	}

	sched, ok := service.SchedulerOf(svc)
	if !ok {
		_ = st.Close()

		return nil, fmt.Errorf("the service has no scheduler")
	}

	return &Env{Dir: dir, Store: st, Svc: svc, Sched: sched}, nil
}

// Close closes the store.
func (e *Env) Close() error { return e.Store.Close() }

// Person is the context of a person at the app: the session cookie, every scope.
func Person() context.Context {
	return service.WithCaller(
		context.Background(),
		service.Caller{Actor: UI, Scopes: service.AllScopes, Browser: true},
	)
}

// Agent is the context of an agent with the local token.
func Agent() context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: Token, Scopes: service.AllScopes})
}

// NewProject makes a project with a timeline and returns its id.
func (e *Env) NewProject(name string) (string, error) {
	res, err := testproject.CreateWithTimeline(service.WithCaller(
		context.Background(), service.Caller{Actor: UI, Scopes: service.AllScopes}), e.Svc, name)
	if err != nil {
		return "", err
	}

	return res.ProjectID, nil
}

type signInOps interface {
	SocialConnectBegin(
		ctx context.Context,
		req service.SocialConnectBeginRequest,
	) (*service.SocialConnectBeginResult, error)
	SocialConnectComplete(ctx context.Context, req service.SocialConnectCompleteRequest) (*service.SocialAccount, error)
}

// SignIn runs the sign-in of a network against its fake server, as the person at the app does, and returns
// the id of the connection.
func (e *Env) SignIn(network string) (string, error) {
	ops, ok := e.Svc.(signInOps)
	if !ok {
		return "", fmt.Errorf("the service has no sign-in operations")
	}

	person := Person()

	begun, err := ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: network, ClientID: "client-id-1", ClientSecret: ClientSecretCanary, RedirectURI: Redirect,
	})
	if err != nil {
		return "", err
	}

	u, err := url.Parse(begun.AuthorizeURL)
	if err != nil || u.Query().Get("state") == "" {
		return "", fmt.Errorf("authorize address %q: %v", begun.AuthorizeURL, err)
	}

	if _, err := ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: u.Query().Get("state"), Code: "c"},
	); err != nil {
		return "", err
	}

	return begun.ConnectionID, nil
}

// Scheduled is a publication with one target that was scheduled and approved by a person.
type Scheduled struct {
	PublicationID string
	TargetID      string
	// At is the instant the target is due, as the service recorded it.
	At time.Time
}

// ScheduleAndApprove makes a publication of the export with one target due after the given time, schedules
// it and approves its request as a person would.
func (e *Env) ScheduleAndApprove(project, exportID, connectionID string, after time.Duration) (Scheduled, error) {
	agent := Agent()
	at := time.Now().Add(after).UTC().Format("2006-01-02T15:04")

	created, err := e.Svc.PublicationCreate(agent, service.PublicationCreateRequest{
		ProjectID: project, Title: "Week one", Text: "A caption", Export: exportID,
		Targets: []pub.TargetInput{{
			ConnectionID: connectionID, At: at, Zone: "UTC", Fields: pub.Fields{MadeForKids: new(false)},
		}},
	})
	if err != nil {
		return Scheduled{}, err
	}

	id := created.Targets[0].ID

	if _, err := e.Svc.PublicationSchedule(
		agent, service.PublicationScheduleRequest{ID: created.ID, TargetID: id},
	); err != nil {
		return Scheduled{}, err
	}

	got, err := e.Svc.PublicationGet(agent, service.PublicationGetRequest{ID: created.ID})
	if err != nil {
		return Scheduled{}, err
	}

	t := got.Target(id)
	if t == nil || t.ApprovalID == "" {
		return Scheduled{}, fmt.Errorf("target %s has no approval request: %+v", id, t)
	}

	if _, err := e.Svc.ApprovalDecide(
		Person(), service.ApprovalDecideRequest{ID: t.ApprovalID, Decision: "approve"},
	); err != nil {
		return Scheduled{}, err
	}

	got, err = e.Svc.PublicationGet(agent, service.PublicationGetRequest{ID: created.ID})
	if err != nil {
		return Scheduled{}, err
	}

	due, err := time.ParseInLocation("2006-01-02T15:04", at, time.UTC)
	if err != nil {
		return Scheduled{}, err
	}

	return Scheduled{PublicationID: created.ID, TargetID: id, At: due}, nil
}

// Target reads the target of a publication.
func (e *Env) Target(s Scheduled) (*pub.Target, error) {
	got, err := e.Svc.PublicationGet(Agent(), service.PublicationGetRequest{ID: s.PublicationID})
	if err != nil {
		return nil, err
	}

	t := got.Target(s.TargetID)
	if t == nil {
		return nil, fmt.Errorf("no target %s", s.TargetID)
	}

	return t, nil
}
