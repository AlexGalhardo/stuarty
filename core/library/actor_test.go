package library

import "testing"

func TestAgentName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"local:claude-code-k3m9x2": "Claude Code",
		"local:mcp-k3m9x2":         "MCP",
		"local:cursor-ab12cd":      "Cursor",
		"local:claude-code":        "Claude Code",
		"local:cli":                "CLI",
		"local:api":                "API",
		"local:chat-k3m9x2p4q1":    "Chat agent",
		"key:cht_k3m9x2p4q1":       "Chat agent",
		"key:key_ab12cd34ef":       "Key ab12",
		"local:ui":                 "",
		"local:":                   "",
		"user:12":                  "",
		"":                         "",
		"something-else":           "",
	}

	for actor, want := range cases {
		if got := AgentName(actor); got != want {
			t.Errorf("AgentName(%q) = %q, want %q", actor, got, want)
		}
	}
}
