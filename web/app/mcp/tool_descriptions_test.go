package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// definedActions is the values of the action member of every tool of definitions/, by tool; a tool with no
// action member has a nil list.
func definedActions(t *testing.T) map[string][]string {
	t.Helper()

	files, err := definitionFiles.ReadDir("definitions")
	if err != nil {
		t.Fatal(err)
	}

	out := map[string][]string{}

	for _, f := range files {
		raw, err := definitionFiles.ReadFile("definitions/" + f.Name())
		if err != nil {
			t.Fatal(err)
		}

		var def struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"inputSchema"`
		}

		if err := json.Unmarshal(raw, &def); err != nil {
			t.Fatalf("%s: %v", f.Name(), err)
		}

		out[def.Name] = def.InputSchema.Properties["action"].Enum
	}

	return out
}

// Task 4 of epic 76: every tool of Stuarty's MCP server, and every one of its actions, has a plain description
// in the catalogue of core/chat, and the catalogue holds nothing that is not a tool of the server.
func TestEveryToolAndActionHasAPlainDescription(t *testing.T) {
	t.Parallel()

	defined, catalogue := definedActions(t), chat.StuartyTools()

	for name, actions := range defined {
		have, ok := catalogue[name]
		if !ok {
			t.Errorf(
				"the tool %s has no entry in chat.StuartyTools: add it and its messages in ui.agent_activity",
				name,
			)

			continue
		}

		for _, a := range actions {
			if !slices.Contains(have, a) {
				t.Errorf("the action %s of the tool %s has no entry in chat.StuartyTools", a, name)
			}
		}

		for _, a := range have {
			if !slices.Contains(actions, a) {
				t.Errorf("chat.StuartyTools lists the action %s of %s, which the tool does not have", a, name)
			}
		}
	}

	for name := range catalogue {
		if _, ok := defined[name]; !ok {
			t.Errorf("chat.StuartyTools lists %s, which is not a tool of the server", name)
		}
	}
}

// lookup follows a dotted key through the nested objects of a language file.
func lookup(tree map[string]any, key string) (string, bool) {
	var cur any = tree

	for part := range strings.SplitSeq(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}

		if cur, ok = m[part]; !ok {
			return "", false
		}
	}

	s, ok := cur.(string)

	return s, ok && s != ""
}

func TestEveryDescriptionKeyIsInBothLanguages(t *testing.T) {
	t.Parallel()

	for _, lang := range []string{"en", "pt-BR"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "resources", "lang", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}

		var tree map[string]any
		if err := json.Unmarshal(raw, &tree); err != nil {
			t.Fatal(err)
		}

		for tool, actions := range chat.StuartyTools() {
			keys := []string{chat.DescriptionKey(tool, "")}
			for _, a := range actions {
				keys = append(keys, chat.DescriptionKey(tool, a))
			}

			for _, key := range keys {
				if _, ok := lookup(tree, key); !ok {
					t.Errorf("%s.json has no message %s", lang, key)
				}
			}
		}
	}
}
