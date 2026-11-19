package importcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mod = "github.com/AlexGalhardo/stuarty"

// TestRepository runs the rules over the real module. It is the check that
// fails the build when someone breaks ADR-0001.
func TestRepository(t *testing.T) {
	root := moduleRoot(t)

	got, err := Check(root, mod)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range got {
		t.Error(v)
	}
}

func TestRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		imp  string
		want string // substring of the rule, "" for allowed
	}{
		{"core stdlib", "core/timeval/a.go", "math/big", ""},
		{"core own package", "core/command/a.go", mod + "/core/timeline", ""},
		{"core pure library", "core/command/a.go", "github.com/santhosh-tekuri/jsonschema/v6", ""},
		{"core third party", "core/command/a.go", "github.com/google/uuid", "core/ imports only"},
		{"command imports plan", "core/command/a.go", mod + "/core/plan", "never imports core/plan"},
		{"command test imports plan", "core/command/a_test.go", mod + "/core/plan", "never imports core/plan"},
		{"plan imports timeline", "core/plan/a.go", mod + "/core/timeline", ""},
		{"core imports engine", "core/plan/a.go", mod + "/engine/ffmpeg", "core/ imports only"},
		{"core imports vukem", "core/plan/a.go", VukemModule + "/errs", "core/ imports only"},
		{"core imports cgo", "core/plan/a.go", "C", "core/ imports only"},
		{"core test file", "core/plan/a_test.go", VukemModule + "/vukemtest", "core/ imports only"},
		{"engine imports core", "engine/export/a.go", mod + "/core/plan", ""},
		{"engine imports web", "engine/export/a.go", mod + "/web/app/models", "never imports web/"},
		{"engine imports service", "engine/export/a.go", mod + "/service", "never imports service/"},
		{"engine imports vukem http", "engine/store/a.go", VukemModule + "/router", "except contrib/engine"},
		{"engine imports vukem root", "engine/store/a.go", VukemModule, "except contrib/engine"},
		{"engine runs processes", "engine/ffmpeg/a.go", EngineException, ""},
		{"service imports core and engine", "service/a.go", mod + "/engine/store", ""},
		{"service imports vukem", "service/a.go", VukemModule + "/orm", "service/ imports no vukem"},
		{"service imports web", "service/a.go", mod + "/web/routes", "never imports web/"},
		{"tools import vukem", "internal/tools/x/a.go", VukemModule, "only web/"},
		{"root file imports vukem", "main.go", VukemModule + "/config", "only web/"},
		{"web imports vukem", "web/bootstrap/a.go", VukemModule, ""},
		{"web imports engine", "web/app/a.go", mod + "/engine/store", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := forbidden(tc.file, tc.imp, mod)
			if tc.want == "" && got != "" {
				t.Fatalf("forbidden(%s, %s) = %q, want allowed", tc.file, tc.imp, got)
			}

			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("forbidden(%s, %s) = %q, want it to contain %q", tc.file, tc.imp, got, tc.want)
			}
		})
	}
}

func TestCheckWalksFilesAndSkipsNestedModules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, root, "core/plan/a.go", "package plan\n\nimport _ \"github.com/AlexGalhardo/vukem/errs\"\n")
	write(t, root, "core/plan/ok.go", "package plan\n\nimport _ \"fmt\"\n")
	write(
		t,
		root,
		"core/command/b.go",
		"package command\n\nimport _ \"github.com/AlexGalhardo/stuarty/core/plan\"\n",
	)
	write(t, root, "spikes/x/go.mod", "module x\n")
	write(t, root, "spikes/x/a.go", "package x\n\nimport _ \"github.com/AlexGalhardo/vukem\"\n")
	write(t, root, "web/a.go", "package web\n\nimport _ \"github.com/AlexGalhardo/vukem\"\n")

	got, err := Check(root, mod)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[0].File != "core/command/b.go" || got[1].File != "core/plan/a.go" {
		t.Fatalf("violations = %v, want exactly core/command/b.go and core/plan/a.go", got)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			if !strings.Contains(string(data), "module "+mod+"\n") {
				t.Fatalf("%s/go.mod is not module %s", dir, mod)
			}

			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}

		dir = parent
	}
}
