package auth

import (
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
)

// AgentTokens is what the authentication needs from the registry of session tokens: the grant
// of a live secret (SDD-017 section 6.1).
type AgentTokens interface {
	Lookup(secret string) (agenthost.Grant, bool)
}

// AgentCaller is the caller a session token stands for: bound to the project of the session,
// with the scopes of the grant (never admin), and never a person: Browser stays false, so every
// operation of a person refuses it (SDD-017 section 10). In local mode the actor is
// local:chat-<10 characters of the session id>; in network mode it is key:<session id>, so the
// operation log tells the agent from the person who started the session and undo is scoped to it.
func AgentCaller(g agenthost.Grant, network bool) service.Caller {
	actor := "local:chat-" + strings.TrimPrefix(g.SessionID, "cht_")
	if network {
		actor = "key:" + g.SessionID
	}

	return service.Caller{
		Actor: actor, Scopes: append([]string(nil), g.Scopes...), OwnerUserID: g.Owner, ProjectID: g.ProjectID,
	}
}
