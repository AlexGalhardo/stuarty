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

// Every process these tests start goes through stuartyCmd: exec.Command is called only there and
// to build the binary with go (VV-295).
func TestNoTestStartsTheBinaryWithoutAnAddress(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("test files: %v, %v", files, err)
	}

	for _, name := range files {
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
				return true
			}

			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "exec" {
				return true
			}

			first := 0
			if sel.Sel.Name == "CommandContext" {
				first = 1
			}

			if lit, ok := call.Args[first].(*ast.BasicLit); ok && lit.Value == `"go"` {
				return true
			}

			if name != "isolation_test.go" {
				t.Errorf("%s: exec.%s of the binary: use stuartyCmd, which requires an address of its own",
					name, sel.Sel.Name)
			}

			return true
		})
	}
}
