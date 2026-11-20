package agenthost

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// capturingIssuer is the registry, and remembers the secret it issued.
type capturingIssuer struct {
	*Tokens
	secret string
}

func (c *capturingIssuer) Issue(g Grant, expires time.Time) (string, func(), error) {
	secret, revoke, err := c.Tokens.Issue(g, expires)
	c.secret = secret

	return secret, revoke, err
}

// SDD-017 section 6.1: the agent is given the address of Stuarty's MCP server and a token bound to
// the project, in the protocol message and nowhere else; the token dies with the session.
func TestScopedTokenIsGivenToTheAgentInTheProtocolMessage(t *testing.T) {
	t.Parallel()

	tokens := NewTokens(nil)
	spy := &capturingIssuer{Tokens: tokens}
	e := newEnv(t, Limits{}, func(o *Options) {
		o.Tokens = spy
		o.MCPURL = func() string { return "http://127.0.0.1:7878/mcp" }
	})

	s := e.start(t, "prj_0000000042", e.profile("mcp-info"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	msg := f.next(kind(chat.KindAgentMessage))

	var params struct {
		CWD        string `json:"cwd"`
		MCPServers []struct {
			Type    string   `json:"type"`
			Name    string   `json:"name"`
			URL     string   `json:"url"`
			Headers []Header `json:"headers"`
		} `json:"mcpServers"`
	}

	if err := json.Unmarshal([]byte(msg.Text), &params); err != nil {
		t.Fatalf("the agent reported %q: %v", msg.Text, err)
	}

	if len(params.MCPServers) != 1 {
		t.Fatalf("mcpServers = %+v", params.MCPServers)
	}

	srv := params.MCPServers[0]
	if srv.Type != "http" || srv.Name != MCPServerName || srv.URL != "http://127.0.0.1:7878/mcp" ||
		len(srv.Headers) != 1 || srv.Headers[0].Name != "Authorization" {
		t.Errorf("server = %+v", srv)
	}

	// What the agent gives back to the person is scrubbed: the agent had the secret in its own
	// message and the transcript shows a mark in its place.
	if srv.Headers[0].Value != "Bearer "+SecretMark {
		t.Errorf("the header in the transcript is %q", srv.Headers[0].Value)
	}

	secret := spy.secret
	g, found := tokens.Lookup(secret)
	if !found || g.ProjectID != "prj_0000000042" || g.SessionID != s.ID || len(g.Scopes) != 4 {
		t.Fatalf("grant = %+v, %v", g, found)
	}

	for _, sc := range g.Scopes {
		if sc == "admin" {
			t.Error("the grant holds admin")
		}
	}

	if !strings.HasPrefix(params.CWD, e.dir) {
		t.Errorf("the working directory %q is outside the workspace %q", params.CWD, e.dir)
	}

	// The secret is not in the transcript the store keeps.
	stored, _ := e.store.EventList(t.Context(), s.ID, 0, 1000)
	for _, ev := range stored {
		if strings.Contains(ev.Text, secret) {
			t.Errorf("the token is in a %s event", ev.Kind)
		}
	}

	// It dies with the session.
	e.host.End(s.ID, chat.EndUser)

	if _, found := tokens.Lookup(secret); found {
		t.Error("the token outlived the session")
	}
}
