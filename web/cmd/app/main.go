// Command app is the vukem-video binary: the HTTP server by default, or a
// command such as migrate, route:list or about (vukem <command> delegates here).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	vukem "github.com/AlexGalhardo/vukem"

	"github.com/AlexGalhardo/vukem-video/web/app/cli"
	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

func main() {
	args := os.Args

	switch {
	case len(args) > 1 && args[1] == "open":
		// open runs before the application is built: see opener.
		os.Exit(runOpen(args[2:], os.Stdout, os.Stderr))
	case len(args) == 1:
		// SDD-003 section 5.1: no arguments lists the commands and starts nothing.
		cli.List(os.Stdout)
		os.Exit(cli.ExitOK)
	case cli.IsCommand(args[1]):
		os.Exit(runProduct(args[1], args[2:]))
	}

	os.Exit(vukem.Main(bootstrap.New))
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
