package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// addrEnv is the address of a process under test: a free loopback port, in STUARTY_ADDR (which
// beats HOST and PORT) with those two emptied.
func addrEnv(t *testing.T) []string {
	t.Helper()

	return []string{"STUARTY_ADDR=127.0.0.1:" + freePort(t), "HOST=", "PORT="}
}

// lastEnv returns the value a process would see for key: the last entry wins.
func lastEnv(env []string, key string) (string, bool) {
	value, found := "", false

	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			value, found = v, true
		}
	}

	return value, found
}

// unsafeAddr says why an environment would let the process take the default port, or "".
func unsafeAddr(env []string) string {
	addr, ok := lastEnv(env, "STUARTY_ADDR")
	if !ok || addr == "" {
		return "STUARTY_ADDR is not set: the process would probe and serve on the default port " + defaultPort
	}

	if _, port, err := net.SplitHostPort(addr); err != nil || port == defaultPort {
		return "STUARTY_ADDR=" + addr + " is not a port of its own"
	}

	return ""
}

// stuartyCmd is the only way a test starts the stuarty binary. The environment is the process
// environment, then base, then extra; it must name an address of its own, or the test fails
// before anything starts.
func stuartyCmd(ctx context.Context, t *testing.T, bin string, base, extra []string, args ...string) *exec.Cmd {
	t.Helper()

	return stuartyCmdEnv(ctx, t, bin, append(append(os.Environ(), base...), extra...), args...)
}

// stuartyCmdEnv is stuartyCmd with the whole environment given, for a test that filters the
// process one.
func stuartyCmdEnv(ctx context.Context, t *testing.T, bin string, env []string, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env

	if why := unsafeAddr(cmd.Env); why != "" {
		t.Fatalf("stuarty %v: %s", args, why)
	}

	return cmd
}

func TestUnsafeAddrRefusesWhatWouldTakeTheDefaultPort(t *testing.T) {
	for _, c := range []struct {
		name string
		env  []string
		bad  bool
	}{
		{"nothing set", nil, true},
		{"only PORT", []string{"HOST=127.0.0.1", "PORT=9000"}, true},
		{"empty address", []string{"STUARTY_ADDR="}, true},
		{"the default port", []string{"STUARTY_ADDR=127.0.0.1:" + defaultPort}, true},
		{"a later empty value wins", []string{"STUARTY_ADDR=127.0.0.1:9000", "STUARTY_ADDR="}, true},
		{"a port of its own", []string{"STUARTY_ADDR=127.0.0.1:9000"}, false},
		{"an ephemeral port", []string{"STUARTY_ADDR=0.0.0.0:0"}, false},
	} {
		if got := unsafeAddr(c.env) != ""; got != c.bad {
			t.Errorf("%s: unsafe = %v, want %v", c.name, got, c.bad)
		}
	}

	if why := unsafeAddr(append(addrEnv(t), "HOST=", "PORT=")); why != "" {
		t.Errorf("addrEnv is not safe: %s", why)
	}
}

// Every process these tests start goes through stuartyCmd, in this file: elsewhere the only
// process allowed is go build (VV-295).
func TestNoTestStartsTheBinaryWithoutAnAddress(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("test files: %v, %v", files, err)
	}

	for _, name := range files {
		if name == "isolation_test.go" {
			continue
		}

		bad, err := processStarts(name, nil)
		if err != nil {
			t.Fatal(err)
		}

		for _, b := range bad {
			t.Errorf("%s: use stuartyCmd, which requires an address of its own", b)
		}
	}
}

// processStarts lists what in a test file could start a process past stuartyCmd: exec.Command
// or exec.CommandContext used for anything but a call of go build (a call of the binary, a
// function value, go run), an exec.Cmd made by hand, os.StartProcess, a dot import of os/exec or
// os, and a command whose Env, Path or Args is assigned after it was made, which could drop the
// address stuartyCmd checked. The import names are resolved, so an alias is no way around it.
func processStarts(name string, src any) ([]string, error) {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}

	var bad []string

	report := func(n ast.Node, what string) {
		bad = append(bad, fset.Position(n.Pos()).String()+": "+what)
	}

	execName, osName := "", ""

	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		local := path[strings.LastIndex(path, "/")+1:]

		if imp.Name != nil {
			local = imp.Name.Name
		}

		if (path == "os/exec" || path == "os") && local == "." {
			report(imp, "dot import of "+path)
		}

		switch path {
		case "os/exec":
			execName = local
		case "os":
			osName = local
		}
	}

	is := func(e ast.Expr, pkg string, names ...string) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		id, ok := sel.X.(*ast.Ident)

		return ok && pkg != "" && id.Name == pkg && slices.Contains(names, sel.Sel.Name)
	}

	goBuild := map[ast.Node]bool{}

	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if is(n.Fun, execName, "Command", "CommandContext") && isGoBuild(n) {
				goBuild[n.Fun] = true
			}

			if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "new" && len(n.Args) == 1 && is(n.Args[0], execName, "Cmd") {
				report(n, "an exec.Cmd made by hand")
			}
		case *ast.SelectorExpr:
			if is(n, execName, "Command", "CommandContext") && !goBuild[n] {
				report(n, "exec."+n.Sel.Name+" of something other than go build")
			}

			if is(n, osName, "StartProcess") {
				report(n, "os.StartProcess")
			}
		case *ast.CompositeLit:
			if is(n.Type, execName, "Cmd") {
				report(n, "an exec.Cmd made by hand")
			}
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && slices.Contains([]string{"Env", "Path", "Args"}, sel.Sel.Name) {
					report(lhs, sel.Sel.Name+" assigned after the command was made")
				}
			}
		}

		return true
	})

	return bad, nil
}

// isGoBuild says whether a call of exec.Command or exec.CommandContext runs go build.
func isGoBuild(call *ast.CallExpr) bool {
	first := 0
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "CommandContext" {
		first = 1
	}

	if len(call.Args) < first+2 {
		return false
	}

	for i, want := range []string{"go", "build"} {
		lit, ok := call.Args[first+i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return false
		}

		if v, err := strconv.Unquote(lit.Value); err != nil || v != want {
			return false
		}
	}

	return true
}

func TestProcessStartsFindsEveryWayAround(t *testing.T) {
	const head = "package main\n\nimport (\n\t\"context\"\n\tx \"os/exec\"\n\t\"os\"\n)\n\n" +
		"var _ = context.Background\nvar _ = os.Getenv\n\n"

	for _, c := range []struct {
		name string
		body string
		bad  bool
	}{
		{"go build", `func f() { _ = x.Command("go", "build", "-o", "b", ".") }`, false},
		{"go build with a context", `func f(ctx context.Context) { _ = x.CommandContext(ctx, "go", "build", ".") }`, false},
		{"the type of a field", `type p struct{ cmd *x.Cmd }; var _ *x.ExitError`, false},
		{"the binary under an alias", `func f(bin string) { _ = x.Command(bin, "serve") }`, true},
		{"with a context", `func f(ctx context.Context, bin string) { _ = x.CommandContext(ctx, bin) }`, true},
		{"go run", `func f() { _ = x.Command("go", "run", ".") }`, true},
		{"a go held in a variable", `func f() { g := "go"; _ = x.Command(g, "build") }`, true},
		{"a function value", `var run = x.Command`, true},
		{"a Cmd by hand", `func f(bin string) { _ = &x.Cmd{Path: bin} }`, true},
		{"new Cmd", `func f() { c := new(x.Cmd); _ = c }`, true},
		{"os.StartProcess", `func f(bin string) { _, _ = os.StartProcess(bin, nil, nil) }`, true},
		{"Env reassigned", `func f(c *x.Cmd) { c.Env = os.Environ() }`, true},
		{"Env appended", `func f(c *x.Cmd) { c.Env = append(c.Env, "STUARTY_ADDR=") }`, true},
		{"Path reassigned", `func f(c *x.Cmd, bin string) { c.Path = bin }`, true},
	} {
		bad, err := processStarts(c.name+".go", head+c.body+"\n")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if got := len(bad) > 0; got != c.bad {
			t.Errorf("%s: found %v, want a finding = %v", c.name, bad, c.bad)
		}
	}

	bad, err := processStarts("dot.go", "package main\n\nimport . \"os/exec\"\n\nvar _ = Command\n")
	if err != nil || len(bad) == 0 {
		t.Errorf("a dot import of os/exec: %v, %v", bad, err)
	}
}
