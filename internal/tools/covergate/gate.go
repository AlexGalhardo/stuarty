package main

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// module is the import path prefix of every package of the repository.
const module = "github.com/AlexGalhardo/stuarty"

// Floors of SDD-009 section 13.2: the packages under core/ are pure and cheap
// to cover, everything else has the default floor.
const (
	defaultGate = 80.0
	coreGate    = 90.0
)

// excluded reports whether pkg (relative to the module, "" for the root) is
// outside the gate: commands, internal tooling, disposable spikes, the
// desktop shell (SDD-009 section 13.2) and vendored node_modules.
func excluded(pkg string) bool {
	// Go files that npm packages ship (site/node_modules) are not ours.
	if slices.Contains(strings.Split(pkg, "/"), "node_modules") {
		return true
	}

	for _, root := range []string{"web/cmd", "internal/tools", "spikes", "desktop"} {
		if pkg == root || strings.HasPrefix(pkg, root+"/") {
			return true
		}
	}

	return false
}

func gateFor(pkg string) float64 {
	if pkg == "core" || strings.HasPrefix(pkg, "core/") {
		return coreGate
	}

	return defaultGate
}

// Result is the coverage of one package against its gate.
type Result struct {
	Package    string
	Statements int
	Covered    int
	Gate       float64
}

// Percent is the statement coverage, 0 for a package without statements.
func (r Result) Percent() float64 {
	if r.Statements == 0 {
		return 0
	}

	return 100 * float64(r.Covered) / float64(r.Statements)
}

// Pass reports whether the package meets its gate (unrounded, so 89.96% fails
// a 90% gate even though go test prints 90.0%).
func (r Result) Pass() bool { return r.Percent() >= r.Gate }

type block struct {
	stmts   int
	covered bool
}

// Parse reads a coverprofile and returns per-package results, sorted by
// package. Blocks listed more than once (several test binaries covering the
// same package) count once, covered if any run hit them.
func Parse(r io.Reader) ([]Result, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	blocks := map[string]map[string]block{} // package -> block position -> block
	line := 0

	for sc.Scan() {
		line++

		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "mode:") {
			continue
		}

		pos, stmts, count, err := parseLine(text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}

		file, _, _ := strings.Cut(pos, ":")
		if file != module && !strings.HasPrefix(file, module+"/") {
			continue
		}

		pkg := strings.TrimPrefix(strings.TrimPrefix(path.Dir(file), module), "/")
		if excluded(pkg) {
			continue
		}

		if blocks[pkg] == nil {
			blocks[pkg] = map[string]block{}
		}

		b := blocks[pkg][pos]
		b.stmts = stmts
		b.covered = b.covered || count > 0
		blocks[pkg][pos] = b
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	if line == 0 {
		return nil, errors.New("empty coverprofile")
	}

	out := make([]Result, 0, len(blocks))

	for pkg, bs := range blocks {
		res := Result{Package: pkg, Gate: gateFor(pkg)}

		for _, b := range bs {
			res.Statements += b.stmts
			if b.covered {
				res.Covered += b.stmts
			}
		}

		out = append(out, res)
	}

	slices.SortFunc(out, func(a, b Result) int { return cmp.Compare(a.Package, b.Package) })

	return out, nil
}

// parseLine splits "file:1.2,3.4 stmts count".
func parseLine(text string) (pos string, stmts, count int, err error) {
	fields := strings.Fields(text)
	if len(fields) != 3 || !strings.Contains(fields[0], ":") {
		return "", 0, 0, fmt.Errorf("malformed coverprofile line %q", text)
	}

	if stmts, err = strconv.Atoi(fields[1]); err != nil {
		return "", 0, 0, fmt.Errorf("statements in %q: %w", text, err)
	}

	if count, err = strconv.Atoi(fields[2]); err != nil {
		return "", 0, 0, fmt.Errorf("count in %q: %w", text, err)
	}

	return fields[0], stmts, count, nil
}

// Package is a package of the module as `go list` reports it.
type Package struct {
	Path     string   // import path
	Dir      string   // directory
	GoFiles  []string // non-test Go files for the current platform
	hasStmts *bool    // test hook; nil means read the files
}

// HasStatements reports whether the package has code the coverage tool
// counts: a block with at least one statement. A package made only of
// declarations and doc comments (the doc.go of a skeleton) has none and is
// absent from every profile, so it is not "missing".
func (p Package) HasStatements() (bool, error) {
	if p.hasStmts != nil {
		return *p.hasStmts, nil
	}

	fset := token.NewFileSet()

	for _, name := range p.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return false, err
		}

		found := false

		ast.Inspect(f, func(n ast.Node) bool {
			if b, ok := n.(*ast.BlockStmt); ok && len(b.List) > 0 {
				found = true
			}

			return !found
		})

		if found {
			return true, nil
		}
	}

	return false, nil
}

// Missing returns the gated packages that have statements and are absent from
// results: a package whose tests did not run, or that has none, must not pass
// silently (SDD-009 section 13.2).
func Missing(results []Result, pkgs []Package) ([]string, error) {
	seen := map[string]bool{}
	for _, r := range results {
		seen[r.Package] = true
	}

	var out []string

	for _, p := range pkgs {
		if p.Path != module && !strings.HasPrefix(p.Path, module+"/") {
			continue
		}

		rel := strings.TrimPrefix(strings.TrimPrefix(p.Path, module), "/")
		if excluded(rel) || seen[rel] {
			continue
		}

		has, err := p.HasStatements()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Path, err)
		}

		if has {
			out = append(out, rel)
		}
	}

	slices.Sort(out)

	return out, nil
}
