package cli

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/internal/outbound"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/remote"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

// LocalResolver is the Resolver of the binary (SDD-003 section 5.2): it finds
// the workspace owner with workspace.Open (SDD-005 section 6.4). A remote
// server (VUKEM_VIDEO_URL) or a local owner becomes service/remote over HTTP;
// when no server owns the workspace, the Service runs in this process over the
// SQLite store.
type LocalResolver struct {
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
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

	conn, err := openInProcess(ctx, sess)
	if err != nil {
		_ = sess.Close()
		return nil, err
	}

	return conn, nil
}

// callTimeout bounds one HTTP call; a longer wait is the job of --wait.
const callTimeout = 2 * time.Minute

// httpConnection is steps 1 and 2 of SDD-003 section 5.2. Without a workspace
// directory the address came from VUKEM_VIDEO_URL and any host is allowed; a
// local owner is reached on loopback only.
func httpConnection(sess *workspace.Session) *Connection {
	hc := outbound.LocalServer(callTimeout)
	if sess.Dir == "" {
		hc = outbound.RemoteServer(callTimeout)
	}

	return &Connection{Service: remote.New(sess.BaseURL, sess.Token, hc), Workspace: string(sess.Dir)}
}

func openInProcess(ctx context.Context, sess *workspace.Session) (*Connection, error) {
	if err := sess.Dir.EnsureLayout(); err != nil {
		return nil, err
	}

	st, err := store.Open(ctx, sess.Dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		return nil, err
	}

	svc, err := service.New(service.Options{Store: st, Registry: command.Default()})
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	return &Connection{
		Service: svc, InProcess: true, Workspace: string(sess.Dir),
		Close: func() error { return errors.Join(st.Close(), sess.Close()) },
	}, nil
}
