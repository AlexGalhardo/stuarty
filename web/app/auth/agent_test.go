package auth

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
)

// The Caller of a session token is bound to its project, is never a person and holds no admin
// scope (SDD-017 section 6.1, SDD-007 T-72).
func TestAgentCallerIsBoundAndNotAPerson(t *testing.T) {
	t.Parallel()

	g := agenthost.Grant{
		SessionID: "cht_abcdefghjk", ProjectID: "prj_0000000001", Owner: "usr_0000000001",
		Scopes: agenthost.GrantScopes(),
	}

	local := AgentCaller(g, false)
	if local.Actor != "local:chat-abcdefghjk" || local.ProjectID != g.ProjectID || local.Browser || local.Operator {
		t.Errorf("local caller = %+v", local)
	}

	if _, err := Actor(local.Actor[len("local:"):]); err != nil {
		t.Errorf("the actor label is not a valid label: %v", err)
	}

	network := AgentCaller(g, true)
	if network.Actor != "key:cht_abcdefghjk" || network.OwnerUserID != "usr_0000000001" || network.Browser {
		t.Errorf("network caller = %+v", network)
	}

	for _, s := range local.Scopes {
		if s == service.ScopeAdmin {
			t.Error("the caller holds admin")
		}
	}

	// The scopes are a copy: a caller cannot widen the grant.
	local.Scopes[0] = "admin"

	if again := AgentCaller(g, false); again.Scopes[0] == "admin" {
		t.Error("the caller shares the scopes of the grant")
	}
}
