package cli

import (
	"context"
	"os"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/remote"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/owner"
)

// LocalResolver is the Resolver of the binary (SDD-003 section 5.2): it finds
// the workspace owner with workspace.Open (SDD-005 section 6.4). A remote
// server (STUARTY_URL) or a local owner becomes service/remote over HTTP;
// when no server owns the workspace, the Service runs in this process over the
// SQLite store.
type LocalResolver struct {
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
	// Become, when set, replaces the in-process Service of a command that finds
	// no owner: the process turns into the owner (listener, worker, runtime.json)
	// so that the UI and other commands reach it. The stdio MCP server uses it
	// (SDD-003 section 4.7). It receives the workspace that is already locked
	// and must release it through Connection.Close.
	Become func(ctx context.Context, sess *workspace.Session, getenv func(string) string) (*Connection, error)
}

// Resolve implements Resolver.
func (r LocalResolver) Resolve(ctx context.Context, t Target) (*Connection, error) {
	getenv := r.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	if t.Workspace != "" {
		base := getenv

		getenv = func(k string) string {
			if k == workspace.EnvHome {
				return t.Workspace
			}

			return base(k)
		}
	}

	sess, err := workspace.Open(ctx, workspace.Options{Getenv: getenv, KeyFile: t.KeyFile})
	if err != nil {
		return nil, err
	}

	if sess.Mode == workspace.ModeHTTP {
		return httpConnection(sess), nil
	}

	open := openInProcess
	if r.Become != nil {
		open = r.Become
	}

	conn, err := open(ctx, sess, getenv)
	if err != nil {
		_ = sess.Close()
		return nil, err
	}

	return conn, nil
}

// callTimeout bounds one HTTP call; a longer wait is the job of --wait.
const callTimeout = 2 * time.Minute

// httpConnection is steps 1 and 2 of SDD-003 section 5.2. Without a workspace
// directory the address came from STUARTY_URL and any host is allowed; a
// local owner is reached on loopback only.
func httpConnection(sess *workspace.Session) *Connection {
	hc := outbound.LocalServer(callTimeout)
	if sess.Dir == "" {
		hc = outbound.RemoteServer(callTimeout)
	}

	// A stream has no end, so it has no timeout either; the reconnect loop of
	// events rides over a server that closes it.
	sc := outbound.LocalServer(0)
	if sess.Dir == "" {
		sc = outbound.RemoteServer(0)
	}

	return &Connection{
		Service:   remote.New(sess.BaseURL, sess.Token, hc),
		Workspace: string(sess.Dir),
		Server:    sess.BaseURL,
		Events:    httpEvents(sess.BaseURL, sess.Token, sc),
		Download:  httpDownload(sess.BaseURL, sess.Token, sc),
	}
}

// openInProcess builds the Service with the wiring of serve (media, exporter,
// jobs run inside the call), so import and export work with no server.
func openInProcess(ctx context.Context, sess *workspace.Session, getenv func(string) string) (*Connection, error) {
	ow, err := owner.Open(ctx, owner.Config{Getenv: getenv, Session: sess, Inline: true})
	if err != nil {
		return nil, err
	}

	return &Connection{Service: ow.Service, InProcess: true, Workspace: string(sess.Dir), Close: ow.Close}, nil
}
