package cli

import (
	"context"
	"errors"
	"os"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

// LocalResolver is the Resolver of the binary: it finds the workspace owner
// with workspace.Open (SDD-005 section 6.4) and, when no server owns the
// workspace, runs the Service in this process over the SQLite store.
//
// Until the remote client of P1-SRV-005 exists, a workspace owned by a running
// server (or a remote server in VUKEM_VIDEO_URL) is reported as an error
// that says so, instead of being opened a second time.
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

	if sess.Mode != workspace.ModeInProcess {
		return nil, &service.Error{
			Code:    command.CodeInternal,
			Message: "a server owns this workspace and the CLI client for a server is not available yet.",
			Hint:    "stop the server, or wait for task P1-SRV-005",
		}
	}

	conn, err := openInProcess(ctx, sess)
	if err != nil {
		_ = sess.Close()
		return nil, err
	}

	return conn, nil
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
