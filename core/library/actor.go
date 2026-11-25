package library

import (
	"regexp"
	"strings"
)

// sessionSuffix is the id the stdio MCP server appends to the client name of its actor label: a dash and 6
// characters of [0-9a-z] (SDD-003 section 4.5).
var sessionSuffix = regexp.MustCompile(`^(.+)-[0-9a-z]{6}$`)

// acronyms are the words of a client label that are written in capitals.
var acronyms = map[string]string{"mcp": "MCP", "cli": "CLI", "api": "API", "ide": "IDE", "ui": "UI"}

// AgentName is the name a screen shows for the actor that wrote an entry (SDD-018 section 18): "Claude Code" for
// `local:claude-code-k3m9x2`, "Chat agent" for the agent hosted in the chat, "Key ab12" for a key. A person (the
// browser, `local:ui`, or a user of network mode) and an actor the product does not know have no agent name:
// the empty string, and the screen says nothing of an agent.
func AgentName(actor string) string {
	switch {
	case strings.HasPrefix(actor, "key:cht_"):
		return "Chat agent"
	case strings.HasPrefix(actor, "key:key_"):
		id := strings.TrimPrefix(actor, "key:key_")

		return "Key " + id[:min(4, len(id))]
	case strings.HasPrefix(actor, "local:"):
	default:
		return ""
	}

	label := strings.TrimPrefix(actor, "local:")

	switch {
	case label == "" || label == "ui":
		return ""
	case strings.HasPrefix(label, "chat-"):
		return "Chat agent"
	}

	if m := sessionSuffix.FindStringSubmatch(label); m != nil {
		label = m[1]
	}

	words := strings.Split(label, "-")
	for i, w := range words {
		switch {
		case w == "":
		case acronyms[w] != "":
			words[i] = acronyms[w]
		default:
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}

	return strings.Join(words, " ")
}
