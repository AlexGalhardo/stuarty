package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

// A skip inside a TestSecurity_ test would pass silently, so SDD-007 section 12
// allows it in two cases only: the test name carries its platform, or the skip is
// tied to one of the documented environment conditions by a marker comment on the
// line of the call or the line above it: "// skip-env: <condition>".
var (
	platformName = regexp.MustCompile(`_(Linux|Windows|Darwin)(_|$)`)
	skipMarker   = regexp.MustCompile(`skip-env:\s*(\S+)`)
)

// skipEnvironments are the conditions of SDD-007 section 12 that a security test
// may skip on without a platform in its name.
var skipEnvironments = map[string]bool{
	"pinned-ffmpeg":     true,
	"symlink-privilege": true,
	"short-mode":        true,
	"rlimits":           true,
	"bwrap":             true,
	"gpu-adapter":       true,
	"browser":           true,
	"relative-path":     true,
}

func isSkipCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	switch sel.Sel.Name {
	case "Skip", "Skipf", "SkipNow":
		return true
	}

	return false
}

// skipProblems lists the skips of the security tests in one test source.
func skipProblems(path string, src []byte) []string {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot parse: %v", path, err)}
	}

	markers := map[int]string{}

	for _, group := range file.Comments {
		for _, c := range group.List {
			if m := skipMarker.FindStringSubmatch(c.Text); m != nil {
				markers[fset.Position(c.Pos()).Line] = m[1]
			}
		}
	}

	var out []string

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "TestSecurity_") {
			continue
		}

		if platformName.MatchString(fn.Name.Name) {
			continue
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isSkipCall(call) {
				return true
			}

			line := fset.Position(call.Pos()).Line
			cond, marked := markers[line]

			if !marked {
				cond, marked = markers[line-1]
			}

			switch {
			case !marked:
				out = append(out, fmt.Sprintf(
					"%s:%d: %s skips with no platform in its name and no \"skip-env:\" marker",
					path, line, fn.Name.Name))
			case !skipEnvironments[cond]:
				out = append(out, fmt.Sprintf(
					"%s:%d: %s skips on the undocumented condition %q", path, line, fn.Name.Name, cond))
			}

			return true
		})
	}

	return out
}

// skipsOf checks every test source of the repository.
func skipsOf(root string) ([]string, error) {
	repo, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}

	defer func() { _ = repo.Close() }()

	var problems []string

	err = fs.WalkDir(repo.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := repo.ReadFile(path)
		if err != nil {
			return err
		}

		problems = append(problems, skipProblems(path, raw)...)

		return nil
	})

	return problems, err
}
