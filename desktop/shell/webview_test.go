package shell

import (
	"path/filepath"
	"slices"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestWebviewArgsPassTheDocumentedVariable(t *testing.T) {
	got := WebviewArgs(envOf(map[string]string{WebviewArgsEnv: "--remote-debugging-port=9223  --foo"}))
	if !slices.Equal(got, []string{"--remote-debugging-port=9223", "--foo"}) {
		t.Errorf("args %q", got)
	}

	if got := WebviewArgs(envOf(nil)); len(got) != 0 {
		t.Errorf("args %q", got)
	}
}

func TestWebviewDataPathIsPerWorkspaceUnlessNamed(t *testing.T) {
	ws := filepath.Join("home", "ws")

	if got := WebviewDataPath(envOf(nil), ws); got != filepath.Join(ws, "webview") {
		t.Errorf("path %q", got)
	}

	if got := WebviewDataPath(envOf(map[string]string{WebviewDataEnv: "x"}), ws); got != "x" {
		t.Errorf("path %q", got)
	}
}
