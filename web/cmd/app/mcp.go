package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	vukem "github.com/AlexGalhardo/vukem"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/agentdocs"
	"github.com/AlexGalhardo/vukem-video/web/app/cli"
	"github.com/AlexGalhardo/vukem-video/web/app/owner"
)

// announceTimeout bounds the wait for the listener of a stdio MCP server that
// became the owner.
const announceTimeout = 30 * time.Second

// runMCP is `vukem-video mcp`. With no owner running, this process becomes the
// owner of the workspace (SDD-003 section 4.7, SDD-005 section 6.3): it serves
// the HTTP API and the UI on a loopback port, runs the workers and writes
// runtime.json, so the browser and the other commands reach the same project
// the agent edits. With an owner running, the tools talk to it over HTTP.
// Standard output carries only protocol messages: the server logs to standard
// error.
func runMCP(ctx context.Context, args []string, stderr io.Writer) int {
	resolver := cli.LocalResolver{Become: becomeOwner(stderr)}

	return cli.NewRuntime(resolver).Execute(ctx, "mcp", args)
}

// becomeOwner returns the Become hook of the resolver: it opens the owner on
// the workspace the resolver already locked, starts serveOwner in the
// background and waits until runtime.json names this process.
func becomeOwner(
	stderr io.Writer,
) func(context.Context, *workspace.Session, func(string) string) (*cli.Connection, error) {
	return func(ctx context.Context, sess *workspace.Session, getenv func(string) string) (*cli.Connection, error) {
		log, closeLog := openLog(getenv, stderr)

		// An agent's server has no business failing because the usual port is
		// taken: runtime.json tells everyone else where it listens.
		if addr := freePortIfBusy(ctx, getenv, net.JoinHostPort(defaultHost, defaultPort)); addr != "" {
			if err := os.Setenv(envAddr, addr); err != nil {
				_ = closeLog.Close()

				return nil, err
			}
		}

		ow, err := owner.Open(
			ctx,
			owner.Config{
				Getenv:    getenv,
				Version:   agentdocs.Version,
				Log:       log,
				Session:   sess,
				Telemetry: owner.OTelProvider,
			},
		)
		if err != nil {
			_ = closeLog.Close()

			return nil, err
		}

		serveCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)

		go func() {
			done <- serveOwner(serveCtx, ow, nil, vukem.WithStdout(io.Discard), vukem.WithStderr(stderr), vukem.WithLogger(log))
		}()

		stop := func() error {
			cancel()

			serveErr := <-done

			return errors.Join(serveErr, ow.Close(), closeLog.Close())
		}

		if stopped, err := awaitAnnounce(ctx, ow, done); err != nil {
			cancel()

			if !stopped {
				err = errors.Join(err, <-done)
			}

			_ = ow.Close()
			_ = closeLog.Close()

			return nil, err
		}

		conn := &cli.Connection{Service: ow.Service, InProcess: true, Workspace: string(ow.Dir), Close: stop}

		if rt, err := ow.Dir.ReadRuntime(); err == nil {
			conn.Server = "http://" + rt.Addr
		}

		return conn, nil
	}
}

// freePortIfBusy returns an address on any free loopback port when the address
// was not chosen (VUKEM_VIDEO_ADDR unset) and the default one is taken; empty
// means keep what is configured.
func freePortIfBusy(ctx context.Context, getenv func(string) string, def string) string {
	if getenv(envAddr) != "" {
		return ""
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", def)
	if err != nil {
		return net.JoinHostPort(defaultHost, "0")
	}

	_ = ln.Close()

	return ""
}

// awaitAnnounce waits for runtime.json to name this process, or for the server
// to stop first; stopped says that done was already read.
func awaitAnnounce(ctx context.Context, ow *owner.Owner, done <-chan error) (stopped bool, err error) {
	deadline := time.NewTimer(announceTimeout)
	defer deadline.Stop()

	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()

	for {
		if rt, err := ow.Dir.ReadRuntime(); err == nil && rt.PID == os.Getpid() {
			return false, nil
		}

		select {
		case err := <-done:
			if err == nil {
				err = errors.New("the server stopped before it was ready")
			}

			return true, fmt.Errorf("serving the workspace: %w", err)
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, errors.New("the server did not announce itself in time")
		case <-tick.C:
		}
	}
}
