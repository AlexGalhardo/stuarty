// Command app is the stuarty binary: the HTTP server by default, or a
// command such as migrate, route:list or about (vukem <command> delegates here).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	vukem "github.com/AlexGalhardo/vukem"
)

func main() {
	// A variable set under its former name (VUKEM_VIDEO_*) is read under the new one, with a warning
	// (SDD-000 section 13.1), before anything reads the environment.
	if _, err := config.ApplyFormerVariables(config.ProcessEnviron(), os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "stuarty:", err)
		os.Exit(1)
	}

	args := os.Args

	if len(args) > 1 {
		// Help comes from the product table, never from the framework (VV-029).
		if code, handled := cli.Help(args[1:], os.Stdout, os.Stderr); handled {
			os.Exit(code)
		}
	}

	if len(args) > 1 {
		if code, rejected := cli.RejectArgs(args[1], args[2:], os.Stdout, os.Stderr); rejected {
			os.Exit(code)
		}
	}

	switch {
	case len(args) > 1 && args[1] == "open":
		// open runs before the application is built: see opener.
		os.Exit(runOpen(args[2:], os.Stdout, os.Stderr))
	case len(args) == 1:
		// SDD-003 section 5.1: no arguments lists the commands and starts nothing.
		cli.List(os.Stdout)
		os.Exit(cli.ExitOK)
	case args[1] == "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := runServe(ctx, args[2:], os.Stderr)
		stop()
		os.Exit(code)
	case isUserCommand(args[1]):
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := runUser(ctx, args[1], args[2:], processUserIO())
		stop()
		os.Exit(code)
	case args[1] == "mcp":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := runMCP(ctx, args[2:], os.Stderr)
		stop()
		os.Exit(code)
	case cli.IsCommand(args[1]) || !isFrameworkCommand(args[1]):
		// A name no product table and no framework command knows is answered by the product, in its own
		// error format, without building the application (VV-078).
		os.Exit(runProduct(args[1], args[2:]))
	}

	build := bootstrap.New
	if args[1] == "route:list" {
		// The listing must show the routes of the owner, which only an app built
		// over the API dependencies has.
		build = bootstrap.Inspect
	}

	os.Exit(vukem.Main(build))
}

// frameworkCommands are the first words of the commands vukem adds to the binary (about, migrate:status,
// route:list ...). They need the application built; any other name is a mistake of the person.
var frameworkCommands = []string{
	"about", "api", "bench", "boost", "cache", "codegen", "db", "hash", "key", "migrate", "openapi", "queue",
	"route", "schedule", "session", "ssg",
}

func isFrameworkCommand(name string) bool {
	first, _, _ := strings.Cut(name, ":")

	return slices.Contains(frameworkCommands, first)
}

// runProduct runs a product command (SDD-003 section 5). It does not go through
// vukem.RunMain: that builds the whole application first, which needs APP_KEY
// and opens providers a command that only talks to the Service does not use,
// and it prints the text of an exit error, which --json forbids (VF-037). The
// command parses its own arguments and returns the exit code of SDD-000 section 8.
func runProduct(name string, args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return cli.NewRuntime(cli.LocalResolver{}).Execute(ctx, name, args)
}
