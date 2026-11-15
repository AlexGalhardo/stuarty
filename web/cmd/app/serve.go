package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	vukem "github.com/AlexGalhardo/vukem"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/owner"
	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

// version is set at build time with -X main.version.
var version = "dev"

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
	ow, err := owner.Open(ctx, owner.Config{Version: version, Log: slog.New(slog.NewTextHandler(stderr, nil))})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "vukem-video serve:", err)

		if isLocked(err) {
			return exitLocked
		}

		return exitServeFailed
	}

	defer func() { _ = ow.Close() }()

	lookup := os.LookupEnv

	if os.Getenv("APP_KEY") == "" && os.Getenv("APP_ENV") == "" {
		key, kerr := ensureAppKey(ow.Dir)
		if kerr != nil {
			_, _ = fmt.Fprintln(stderr, "vukem-video serve:", kerr)

			return exitServeFailed
		}

		lookup = func(k string) (string, bool) {
			if k == "APP_KEY" {
				return key, true
			}

			return os.LookupEnv(k)
		}
	}

	ready := make(chan string, 1)

	build := func(opts ...vukem.Option) (*vukem.App, error) {
		app, berr := bootstrap.NewWith(ow.Deps(), append(opts, vukem.WithReady(ready))...)
		if berr != nil {
			return nil, berr
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

	if err := vukem.RunMain(ctx, append([]string{"vukem-video", "serve"}, args...), lookup, build); err != nil {
		_, _ = fmt.Fprintln(stderr, "vukem-video serve:", err)

		return exitServeFailed
	}

	return 0
}

func isLocked(err error) bool {
	we, ok := errors.AsType[*workspace.Error](err)

	return ok && we.Code == workspace.CodeWorkspaceLocked
}
