package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	vukem "github.com/AlexGalhardo/vukem"

	"github.com/AlexGalhardo/vukem-video/service/config"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/agentdocs"
	"github.com/AlexGalhardo/vukem-video/web/app/owner"
	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

// exitServe is the exit code of a server that could not start or that failed
// (SDD-005 section 6.3 step 2 uses 2 for a locked workspace).
const (
	exitServeFailed = 1
	exitLocked      = 2
)

// runServe is `vukem-video serve`: the owner of the workspace (SDD-005 section
// 6.3). It takes the lock, opens the store, builds the Service and serves the
// API; runtime.json is written once the listener is bound and removed, with
// the lock released, on a graceful stop. The application key is the stopgap of
// ensureAppKey (VF-030).
func runServe(ctx context.Context, args []string, stderr io.Writer) int {
	resolved, ok := bootConfig(os.Getenv, stderr)
	if !ok {
		return config.ExitStatus
	}

	log, closeLog := openLog(os.Getenv, stderr)
	defer func() { _ = closeLog.Close() }()

	ow, err := owner.Open(
		ctx,
		owner.Config{Version: agentdocs.Version, Log: log, Resolved: resolved, Telemetry: owner.OTelProvider},
	)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "vukem-video serve:", err)

		if isLocked(err) {
			return exitLocked
		}

		return exitServeFailed
	}

	defer func() { _ = ow.Close() }()

	if err := serveOwner(ctx, ow, args, vukem.WithStderr(stderr), vukem.WithLogger(log)); err != nil {
		_, _ = fmt.Fprintln(stderr, "vukem-video serve:", err)

		return exitServeFailed
	}

	return 0
}

// bootConfig is the boot validation of SDD-007 section 4.5: every problem is
// printed, and the process does not start while there is one. It runs before
// the workspace is locked, so a refused configuration leaves nothing behind.
func bootConfig(getenv func(string) string, stderr io.Writer) (config.Resolved, bool) {
	dir, err := workspace.Resolve(getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "vukem-video serve:", err)

		return config.Resolved{}, false
	}

	res, problems := config.Validate(config.Env{Getenv: getenv, Workspace: string(dir)})
	if config.Report(stderr, problems) != 0 {
		return config.Resolved{}, false
	}

	return res, true
}

// serveOwner runs the listener, the queue worker and the announcement of an
// owner until ctx ends. It is what `serve` and the stdio MCP server share, so
// both become the owner the same way. extra are options for the application.
func serveOwner(ctx context.Context, ow *owner.Owner, args []string, extra ...vukem.Option) error {
	lookup := addrLookup(os.LookupEnv)

	// Network mode never gets a generated key: APP_KEY is required and was validated.
	if os.Getenv("APP_KEY") == "" && os.Getenv("APP_ENV") == "" && config.ModeOf(os.Getenv) != config.ModeNetwork {
		key, err := ensureAppKey(ow.Dir)
		if err != nil {
			return err
		}

		inner := lookup
		lookup = func(k string) (string, bool) {
			if k == "APP_KEY" {
				return key, true
			}

			return inner(k)
		}
	}

	ready := make(chan string, 1)

	build := func(opts ...vukem.Option) (*vukem.App, error) {
		opts = append(opts, vukem.WithReady(ready))
		if p := ow.TelemetryProvider(); p != nil {
			// First, so that its middleware wraps the whole stack (drivers/otel).
			opts = append([]vukem.Option{vukem.WithProviders(p)}, opts...)
		}

		app, err := bootstrap.NewWith(ow.Deps(), opts...)
		if err != nil {
			return nil, err
		}

		app.AddRunner("queue", vukem.RunnerFunc(ow.Work))
		app.AddRunner("announce", vukem.RunnerFunc(func(ctx context.Context) error {
			select {
			case addr := <-ready:
				return ow.Announce(addr)
			case <-ctx.Done():
				return nil
			}
		}))

		return app, nil
	}

	return vukem.RunMain(ctx, append([]string{"vukem-video", "serve"}, args...), lookup, build, extra...)
}

// addrLookup makes VUKEM_VIDEO_ADDR the bind address: vukem reads HOST and
// PORT, so the product variable, when set, answers for both.
func addrLookup(next func(string) (string, bool)) func(string) (string, bool) {
	addr, ok := next(config.EnvAddr)
	if !ok || addr == "" {
		return next
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return next
	}

	return func(k string) (string, bool) {
		switch k {
		case "HOST":
			return host, true
		case "PORT":
			return port, true
		}

		return next(k)
	}
}

func isLocked(err error) bool {
	we, ok := errors.AsType[*workspace.Error](err)

	return ok && we.Code == workspace.CodeWorkspaceLocked
}
