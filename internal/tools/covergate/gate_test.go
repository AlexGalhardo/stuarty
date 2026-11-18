package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const m = "github.com/AlexGalhardo/vukem-video"

func profile(lines ...string) string {
	return "mode: atomic\n" + strings.Join(lines, "\n") + "\n"
}

func TestParse(t *testing.T) {
	t.Parallel()

	in := profile(
		m+"/service/s.go:1.1,2.2 3 1",
		m+"/service/s.go:3.1,4.2 1 0",
		m+"/engine/store/s.go:1.1,2.2 4 2",
		m+"/engine/store/s.go:3.1,4.2 6 0",
		// The same block from a second test binary: counted once, covered.
		m+"/engine/store/s.go:3.1,4.2 6 5",
		m+"/core/timeline/t.go:1.1,2.2 9 1",
		m+"/core/timeline/t.go:3.1,4.2 1 0",
		m+"/web/cmd/app/main.go:1.1,2.2 50 0",
		m+"/internal/tools/covergate/gate.go:1.1,2.2 50 0",
		m+"/spikes/x/main.go:1.1,2.2 50 0",
		m+"/desktop/main.go:1.1,2.2 50 0",
		"example.com/other/x.go:1.1,2.2 50 0",
	)

	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}

	want := []Result{
		{Package: "core/timeline", Statements: 10, Covered: 9, Gate: 90},
		{Package: "engine/store", Statements: 10, Covered: 10, Gate: 80},
		{Package: "service", Statements: 4, Covered: 3, Gate: 80},
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Parse() = %+v, want %+v", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "empty coverprofile"},
		{"too few fields", profile(m + "/a.go:1.1,2.2 3"), "malformed"},
		{"no position", profile("nofile 3 1"), "malformed"},
		{"bad statements", profile(m + "/a.go:1.1,2.2 x 1"), "statements"},
		{"bad count", profile(m + "/a.go:1.1,2.2 3 y"), "count"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Parse(strings.NewReader(tt.in)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse(%q) = %v, want error containing %q", tt.in, err, tt.want)
			}
		})
	}
}

func TestResultPass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		r    Result
		want bool
	}{
		{Result{Statements: 10, Covered: 8, Gate: 80}, true},
		{Result{Statements: 10000, Covered: 7999, Gate: 80}, false},
		{Result{Statements: 10000, Covered: 8996, Gate: 90}, false}, // prints as 90.0%
		{Result{Statements: 0, Covered: 0, Gate: 80}, false},
	}

	for _, tt := range tests {
		if got := tt.r.Pass(); got != tt.want {
			t.Errorf("%+v.Pass() = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestGateFor(t *testing.T) {
	t.Parallel()

	tests := map[string]float64{
		"core": 90, "core/timeval": 90, "core/command": 90,
		"corelib": 80, "engine": 80, "service/workspace": 80, "web/app/mcp": 80, "": 80,
	}

	for pkg, want := range tests {
		if got := gateFor(pkg); got != want {
			t.Errorf("gateFor(%q) = %v, want %v", pkg, got, want)
		}
	}
}

func TestExcluded(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"": false, "core": false, "service": false, "web": false, "web/app": false, "webcmd": false,
		"web/cmd": true, "web/cmd/app": true, "internal/tools": true, "internal/tools/covergate": true,
		"internal/outbound": false, "internal": false, "spikes/export-pipeline": true, "desktop": true,
		"desktop/shell": true, "desktopx": false,
		"site/node_modules/flatted/golang/pkg/flatted": true, "node_modules": true, "site/node_modules_x": false,
	}

	for pkg, want := range tests {
		if got := excluded(pkg); got != want {
			t.Errorf("excluded(%q) = %v, want %v", pkg, got, want)
		}
	}
}

func TestHasStatements(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("doc.go", "// Package x is a skeleton.\npackage x\n\ntype T struct{}\n\nfunc Empty() {}\n")
	write("code.go", "package x\n\nfunc F() int {\n\treturn 1\n}\n")

	tests := []struct {
		name  string
		files []string
		want  bool
	}{
		{"declarations and an empty body", []string{"doc.go"}, false},
		{"no files", nil, false},
		{"code", []string{"doc.go", "code.go"}, true},
	}

	for _, tt := range tests {
		got, err := Package{Dir: dir, GoFiles: tt.files}.HasStatements()
		if err != nil || got != tt.want {
			t.Errorf("%s: HasStatements() = %v, %v, want %v", tt.name, got, err, tt.want)
		}
	}

	write("bad.go", "package x\nfunc {")

	if _, err := (Package{Dir: dir, GoFiles: []string{"bad.go"}}).HasStatements(); err == nil {
		t.Error("HasStatements() on a file that does not parse: want an error")
	}
}

func TestMissing(t *testing.T) {
	t.Parallel()

	yes, no := true, false
	pkgs := []Package{
		{Path: m + "/core/timeval", hasStmts: &yes},
		{Path: m + "/core/plan", hasStmts: &yes},
		{Path: m + "/core/doc", hasStmts: &no},
		{Path: m + "/service", hasStmts: &yes},
		{Path: m + "/web/cmd/app", hasStmts: &yes},
		{Path: m, hasStmts: &yes},
		{Path: "example.com/other", hasStmts: &yes},
	}
	results := []Result{{Package: "core/timeval"}, {Package: "service"}}

	got, err := Missing(results, pkgs)
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"", "core/plan"}; !reflect.DeepEqual(want, got) {
		t.Errorf("Missing() = %q, want %q", got, want)
	}
}

func writeProfile(t *testing.T, content string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

func TestRun(t *testing.T) {
	t.Parallel()

	yes := true
	base := func() []Package {
		return []Package{{Path: m + "/core/timeval", hasStmts: &yes}, {Path: m + "/service", hasStmts: &yes}}
	}
	allCovered := func() ([]Package, error) { return base(), nil }
	extra := func() ([]Package, error) {
		return append(base(), Package{Path: m + "/core/plan", hasStmts: &yes}), nil
	}
	broken := func() ([]Package, error) { return nil, errors.New("go list: boom") }

	good := profile(m+"/core/timeval/t.go:1.1,2.2 1 1", m+"/service/s.go:1.1,2.2 1 1")

	tests := []struct {
		name        string
		flags       []string
		content     string
		list        func() ([]Package, error)
		missingFile bool
		wantCode    int
		wantStdout  []string
		wantStderr  string
	}{
		{
			name: "all pass", content: good, list: allCovered,
			wantStdout: []string{"covergate: 2 packages meet their gate"},
		},
		{
			name: "verbose lists every package", flags: []string{"-v"}, content: good, list: allCovered,
			wantStdout: []string{"ok   core/timeval", "100.0% (gate 90%)", "ok   service", "(gate 80%)"},
		},
		{
			name: "core below its floor",
			content: profile(
				m+"/core/timeval/t.go:1.1,2.2 85 1",
				m+"/core/timeval/t.go:3.1,4.2 15 0",
				m+"/service/s.go:1.1,2.2 1 1",
			),
			list:       allCovered,
			wantCode:   1,
			wantStdout: []string{"FAIL core/timeval", "85.0% (gate 90%)"},
			wantStderr: "1 package(s) below the coverage gate",
		},
		{
			name: "same coverage passes outside core",
			content: profile(
				m+"/core/timeval/t.go:1.1,2.2 1 1",
				m+"/service/s.go:1.1,2.2 85 1",
				m+"/service/s.go:3.1,4.2 15 0",
			),
			list:       allCovered,
			wantStdout: []string{"covergate: 2 packages meet their gate"},
		},
		{
			name: "package with statements missing", content: good, list: extra, wantCode: 1,
			wantStdout: []string{"FAIL core/plan", "missing from the profile (gate 90%)"},
			wantStderr: "1 package(s) below",
		},
		{name: "go list fails", content: good, list: broken, wantCode: 1, wantStderr: "go list: boom"},
		{name: "no args", list: allCovered, wantCode: 2, wantStderr: "usage"},
		{name: "bad flag", flags: []string{"-x", "f"}, list: allCovered, wantCode: 2},
		{name: "missing file", missingFile: true, list: allCovered, wantCode: 1, wantStderr: "covergate:"},
		{name: "malformed file", content: "garbage\n", list: allCovered, wantCode: 1, wantStderr: "malformed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := append([]string(nil), tt.flags...)

			switch {
			case tt.content != "":
				args = append(args, writeProfile(t, tt.content))
			case tt.missingFile:
				args = append(args, filepath.Join(t.TempDir(), "none.out"))
			}

			var stdout, stderr bytes.Buffer

			if code := run(args, &stdout, &stderr, tt.list); code != tt.wantCode {
				t.Fatalf(
					"run() = %d, want %d; stdout:\n%s\nstderr:\n%s",
					code,
					tt.wantCode,
					stdout.String(),
					stderr.String(),
				)
			}

			for _, w := range tt.wantStdout {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("stdout = %q, want %q", stdout.String(), w)
				}
			}

			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
