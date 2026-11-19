package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// fakeEnv is an environment in a map.
type fakeEnv map[string]string

func (f fakeEnv) environ() Environ {
	return Environ{
		Get:   func(n string) string { return f[n] },
		Set:   func(n, v string) error { f[n] = v; return nil },
		Unset: func(n string) error { delete(f, n); return nil },
	}
}

// TestFormerVariables walks the table: every pair is read under its former name with one warning, loses
// to its current name, and the table is the list of SDD-000 section 13.1.
func TestFormerVariables(t *testing.T) {
	t.Parallel()

	if len(FormerVariables) == 0 {
		t.Fatal("the table is empty")
	}

	seen := map[string]bool{}

	for _, p := range FormerVariables {
		if !strings.HasPrefix(p.Former, "VUKEM_VIDEO_") || !strings.HasPrefix(p.Current, "STUARTY_") ||
			strings.TrimPrefix(p.Former, "VUKEM_VIDEO_") != strings.TrimPrefix(p.Current, "STUARTY_") {
			t.Errorf("%s -> %s is not a pair of the same variable", p.Former, p.Current)
		}

		if seen[p.Former] || seen[p.Current] {
			t.Errorf("%s -> %s is in the table twice", p.Former, p.Current)
		}

		seen[p.Former], seen[p.Current] = true, true

		t.Run(p.Current, func(t *testing.T) {
			t.Parallel()

			// Only the former name is set: it is used, moved, and warned about once.
			env := fakeEnv{p.Former: "value"}

			var warn bytes.Buffer

			applied, err := ApplyFormerVariables(env.environ(), &warn)
			if err != nil {
				t.Fatal(err)
			}

			if len(applied) != 1 || applied[0] != p {
				t.Errorf("applied = %v, want only %v", applied, p)
			}

			if env[p.Current] != "value" {
				t.Errorf("%s = %q, want the value of %s", p.Current, env[p.Current], p.Former)
			}

			if _, ok := env[p.Former]; ok {
				t.Errorf("%s is still set after the copy", p.Former)
			}

			if n := strings.Count(warn.String(), "\n"); n != 1 || !strings.Contains(warn.String(), p.Former) ||
				!strings.Contains(warn.String(), p.Current) {
				t.Errorf("want one warning naming both variables, got %q", warn.String())
			}

			// Both are set: the current one wins and the former one is reported as ignored.
			both := fakeEnv{p.Former: "old", p.Current: "new"}
			warn.Reset()

			applied, err = ApplyFormerVariables(both.environ(), &warn)
			if err != nil || len(applied) != 0 {
				t.Fatalf("applied = %v, err = %v, want nothing applied", applied, err)
			}

			if both[p.Current] != "new" {
				t.Errorf("%s = %q, want the current value to win", p.Current, both[p.Current])
			}

			if !strings.Contains(warn.String(), "ignored") || strings.Count(warn.String(), "\n") != 1 {
				t.Errorf("want one warning that the former name is ignored, got %q", warn.String())
			}
		})
	}
}

// TestFormerVariablesAreQuietWhenUnused: with no former variable set, nothing is written and nothing changes.
func TestFormerVariablesAreQuietWhenUnused(t *testing.T) {
	t.Parallel()

	env := fakeEnv{"STUARTY_HOME": "/w", "APP_KEY": "k"}

	var warn bytes.Buffer

	applied, err := ApplyFormerVariables(env.environ(), &warn)
	if err != nil || len(applied) != 0 || warn.Len() != 0 {
		t.Errorf("applied = %v, err = %v, warning = %q, want none", applied, err, warn.String())
	}

	if len(env) != 2 {
		t.Errorf("the environment changed: %v", env)
	}
}

// TestFormerVariablesReportAFailedSet: a failure to write the environment is an error, not a silent skip.
func TestFormerVariablesReportAFailedSet(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	env := Environ{
		Get:   func(n string) string { return map[string]string{"VUKEM_VIDEO_HOME": "/w"}[n] },
		Set:   func(string, string) error { return boom },
		Unset: func(string) error { return nil },
	}

	if _, err := ApplyFormerVariables(env, &bytes.Buffer{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the failure of Set", err)
	}
}

// TestFormerVariablesMatchTheSpecification: the table is the list of SDD-000 section 13.1, and every
// variable this package and the workspace package read is in it.
func TestFormerVariablesMatchTheSpecification(t *testing.T) {
	t.Parallel()

	text, err := os.ReadFile(filepath.Join("..", "..", "docs", "sdds", "000-shared-definitions.md"))
	if err != nil {
		t.Fatal(err)
	}

	_, after, ok := strings.Cut(string(text), "### 13.1 ")
	if !ok {
		t.Fatal("SDD-000 has no section 13.1")
	}

	section, _, _ := strings.Cut(after, "### 13.2 ")
	row := regexp.MustCompile("(?m)^\\| `(VUKEM_VIDEO_[A-Z0-9_]+)` \\| `(STUARTY_[A-Z0-9_]+)` \\|$")

	var spec []FormerVariable
	for _, m := range row.FindAllStringSubmatch(section, -1) {
		spec = append(spec, FormerVariable{Former: m[1], Current: m[2]})
	}

	if !slices.Equal(spec, FormerVariables) {
		t.Errorf("SDD-000 section 13.1 and FormerVariables differ:\nSDD-000: %v\ncode:    %v", spec, FormerVariables)
	}

	current := map[string]bool{}
	for _, p := range FormerVariables {
		current[p.Current] = true
	}

	for _, name := range []string{
		EnvMode, EnvAddr, EnvImportRoots, EnvTranscribeURL, EnvTranscribeKey,
		EnvSandbox, EnvAllowRoot, EnvFFmpeg, EnvFFprobe, workspace.EnvHome, workspace.EnvURL, workspace.EnvKey,
	} {
		if !current[name] {
			t.Errorf("%s is read by this package but has no former name in the table", name)
		}
	}
}
