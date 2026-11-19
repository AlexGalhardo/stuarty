package mcp

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// TestNoOutboundTools is SDD-003 section 9 check 25 and T10: no tool takes a
// destination, and the package has no way to open a connection of its own.
func TestNoOutboundTools(t *testing.T) {
	t.Parallel()

	defs, err := loadDefinitions(command.Default())
	if err != nil {
		t.Fatal(err)
	}

	banned := []string{
		"url", "uri", "endpoint", "host", "webhook", "destination", "dest", "to", "target", "output", "out",
	}

	for name, d := range defs {
		schema, _ := d.tool.InputSchema.(map[string]any)
		props, _ := schema["properties"].(map[string]any)

		for prop, v := range props {
			if slices.Contains(banned, prop) {
				t.Errorf("%s takes a %q argument", name, prop)
			}

			if m, _ := v.(map[string]any); m["format"] == "uri" || m["format"] == "iri" {
				t.Errorf("%s.%s is a URL", name, prop)
			}
		}
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}

		for _, bad := range []string{"http.Client", "http.Get(", "http.Post(", "net.Dial", "DefaultClient"} {
			if bytes.Contains(src, []byte(bad)) {
				t.Errorf("%s uses %s: the MCP adapter opens no connection", f, bad)
			}
		}
	}
}

// TestToolsListIsByteEqualAcrossCalls: tools/list is static (ADR-0020), so two
// calls answer the same bytes, and the 18 tools of the build are all there.
func TestToolsListIsByteEqualAcrossCalls(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")

	first, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	second, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(marshal(t, first), marshal(t, second)) {
		t.Error("two tools/list calls differ")
	}

	if len(first.Tools) != 17 {
		t.Errorf("tools/list has %d tools, want the 17 of SDD-000 section 1", len(first.Tools))
	}
}
