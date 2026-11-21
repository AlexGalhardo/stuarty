package servicetest

import (
	"context"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
)

// agentCtx is the context of a call made with the session token of a hosted agent bound to a
// project.
func agentCtx(t *testing.T, tokens *agenthost.Tokens, session, project string) context.Context {
	t.Helper()

	secret, _, err := tokens.Issue(agenthost.Grant{
		SessionID: session, ProjectID: project, Scopes: agenthost.GrantScopes(),
	}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	g, ok := tokens.Lookup(secret)
	if !ok {
		t.Fatal("a token that was just issued is not found")
	}

	return service.WithCaller(context.Background(), auth.AgentCaller(g, false))
}

// SDD-017 section 6.1, SDD-007 T-06 and T-72, acceptance criteria 2 and 3 of epic 14: the token
// of a session reaches its own project and no other; what it edits is in the operation log with
// the agent as the actor, and the agent can undo it.
func TestScopedTokenAnotherProjectIsRefused(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	tokens := agenthost.NewTokens(nil)
	mine, other := e.newProject("mine"), e.newProject("other")
	agentCall := agentCtx(t, tokens, "cht_0000000007", mine)

	if _, err := e.svc.ProjectGet(agentCall, service.ProjectGetRequest{ProjectID: mine}); err != nil {
		t.Fatalf("ProjectGet of its own project = %v", err)
	}

	res, err := e.svc.Edit(agentCall, service.EditRequest{
		ProjectID: mine, BaseRevision: 0, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	})
	if err != nil {
		t.Fatalf("Edit of its own project = %v", err)
	}

	hist, err := e.svc.HistoryList(e.as(ui), service.HistoryListRequest{ProjectID: mine})
	if err != nil {
		t.Fatal(err)
	}

	var found bool

	for _, h := range hist.Entries {
		if h.Revision == res.Revision && h.Actor == "local:chat-0000000007" {
			found = true
		}
	}

	if !found {
		t.Errorf("the edit is not in the log with the agent as its actor: %+v", hist.Entries)
	}

	if _, err := e.svc.Undo(agentCall, service.UndoRequest{ProjectID: mine}); err != nil {
		t.Errorf("the agent cannot undo its own edit: %v", err)
	}

	refused := map[string]func() error{
		"ProjectGet": func() error {
			_, err := e.svc.ProjectGet(agentCall, service.ProjectGetRequest{ProjectID: other})
			return err
		},
		"Edit": func() error {
			_, err := e.svc.Edit(agentCall, service.EditRequest{
				ProjectID:    other,
				BaseRevision: 0,
				Operations:   []command.Operation{op("track.add", `{"kind":"visual"}`)},
			})
			return err
		},
		"HistoryList": func() error {
			_, err := e.svc.HistoryList(agentCall, service.HistoryListRequest{ProjectID: other})
			return err
		},
		"Undo": func() error {
			_, err := e.svc.Undo(agentCall, service.UndoRequest{ProjectID: other})
			return err
		},
		"Lint": func() error {
			_, err := e.svc.Lint(agentCall, service.LintRequest{ProjectID: other})
			return err
		},
		"MediaList": func() error {
			_, err := e.svc.MediaList(agentCall, service.MediaListRequest{ProjectID: other})
			return err
		},
		"ProjectTrash": func() error {
			_, err := e.svc.ProjectTrash(agentCall, service.ProjectTrashRequest{ProjectID: other})
			return err
		},
		"ProjectCreate": func() error {
			_, err := testproject.CreateWithTimeline(agentCall, e.svc, "x")
			return err
		},
	}

	for name, call := range refused {
		if c := code(call()); c != service.CodeForbidden {
			t.Errorf("%s on another project = %q, want %s", name, c, service.CodeForbidden)
		}
	}

	list, err := e.svc.ProjectList(agentCall, service.ProjectListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Projects) != 1 || list.Projects[0].ProjectID != mine {
		t.Errorf("the agent sees %+v", list.Projects)
	}

	got, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: other})
	if err != nil || got.Revision != 0 {
		t.Errorf("the other project changed: %+v, %v", got, err)
	}
}
