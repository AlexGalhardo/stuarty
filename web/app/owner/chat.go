package owner

import (
	"net"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
)

// MCPURL is the address of this server's own MCP endpoint as the hosted agent reaches it (SDD-017
// section 6.1): the public origin in network mode, the loopback address and port the listener bound
// in local mode. It is empty until the listener is bound, and an agent started before then gets no
// MCP server.
func (o *Owner) MCPURL() string {
	if o.mode == config.ModeNetwork && o.appURL != "" {
		return o.appURL + "/mcp"
	}

	v, _ := o.mcpURL.Load().(string)

	return v
}

// mcpURLFor is the MCP address for a bound address. The Host header of a request must be the loopback
// name and the real port (SDD-007 section 3.6), so an unspecified address is reached as 127.0.0.1.
func mcpURLFor(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return ""
	}

	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}

	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}

	return "http://" + host + ":" + port + "/mcp"
}

// chatStreams is the host for the event stream of a chat session; nil when there is none, so that
// the adapter does not hold a typed nil.
func (o *Owner) chatStreams() httpapi.ChatStreams {
	if o.Agents == nil {
		return nil
	}

	return o.Agents
}

// agentLookup is the registry of session tokens for the authentication; nil when there is none.
func (o *Owner) agentLookup() auth.AgentTokens {
	if o.agentTokens == nil {
		return nil
	}

	return o.agentTokens
}
