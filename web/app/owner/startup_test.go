package owner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// SDD-005 section 7 step 2: a workspace written by a newer version is refused with the reason, in
// process as well as by serve, and the reason is not hidden behind E_INTERNAL.
func TestOpenRefusesANewerWorkspaceWithItsReason(t *testing.T) {
	getenv := envOf(map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws")})

	o, err := Open(context.Background(), Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	_, err = o.Store.DB().ExecContext(
		context.Background(),
		"INSERT INTO migrations (id, batch, checksum, ran_at) VALUES ('29990101000000_from_the_future', 9, 'x', ?)",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(context.Background(), Config{Getenv: getenv, Inline: true})

	se, ok := errors.AsType[*service.Error](err)
	if !ok {
		t.Fatalf("err = %v, want a *service.Error", err)
	}

	if se.Code == command.CodeInternal || se.Code != command.CodeInvalidState || se.Retryable {
		t.Errorf("code = %s retryable = %v, want %s and not retryable", se.Code, se.Retryable, command.CodeInvalidState)
	}

	if !strings.Contains(se.Message, "newer version") ||
		!strings.Contains(se.Message, "29990101000000_from_the_future") {
		t.Errorf("message = %q, want the newer version reason naming the migration", se.Message)
	}

	if se.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", se.ExitCode())
	}
}

// A loose token file is refused with a fixed reason: the raw error names the file, the answer does not.
func TestStartupErrorShowsTheTokenReasonWithoutThePath(t *testing.T) {
	raw := fmt.Errorf("%w: C:/Users/someone/ws/token has access; fix it", workspace.ErrTokenPermissions)

	se, ok := errors.AsType[*service.Error](StartupError(raw))
	if !ok {
		t.Fatal("want a *service.Error")
	}

	if se.Code != command.CodeInvalidState || se.Retryable {
		t.Errorf("code = %s retryable = %v", se.Code, se.Retryable)
	}

	if !strings.Contains(se.Message, "token file") || !strings.Contains(se.Message, "too loose") {
		t.Errorf("message = %q, want the token permissions reason", se.Message)
	}

	for _, s := range []string{se.Message, se.Hint} {
		if strings.Contains(s, "someone") || strings.Contains(s, `\`) || strings.Contains(s, "/") {
			t.Errorf("%q shows a path", s)
		}
	}
}

func TestStartupErrorLeavesOtherErrorsAlone(t *testing.T) {
	if StartupError(nil) != nil {
		t.Error("nil became an error")
	}

	other := errors.New("disk full")
	if got := StartupError(other); !errors.Is(got, other) {
		t.Errorf("got %v, want the error itself", got)
	}

	locked := &service.Error{Code: service.CodeWorkspaceLocked, Retryable: true}
	if got := StartupError(locked); !errors.Is(got, locked) {
		t.Errorf("got %v, want the same error", got)
	}
}

// A workspace path with a character that breaks the database URL is refused as a state error with a
// fixed reason: the raw error names the path, the answer does not.
func TestStartupErrorShowsTheUnusablePathReasonWithoutThePath(t *testing.T) {
	raw := fmt.Errorf("%w: C:/Users/someone/ws?x/db.sqlite", workspace.ErrUnusablePath)

	se, ok := errors.AsType[*service.Error](StartupError(raw))
	if !ok {
		t.Fatal("want a *service.Error")
	}

	if se.Code != command.CodeInvalidState || se.Retryable {
		t.Errorf("code = %s retryable = %v", se.Code, se.Retryable)
	}

	for _, s := range []string{se.Message, se.Hint} {
		if strings.Contains(s, "someone") || strings.Contains(s, `\`) || strings.Contains(s, "C:") {
			t.Errorf("%q shows a path", s)
		}
	}

	if !strings.Contains(se.Hint, workspace.EnvHome) {
		t.Errorf("hint = %q, want it to name %s", se.Hint, workspace.EnvHome)
	}
}

func TestOpenRefusesAWorkspacePathThatBreaksTheDatabaseURL(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ws#1")

	_, err := Open(context.Background(), Config{
		Getenv: envOf(map[string]string{workspace.EnvHome: home}),
		Inline: true,
	})

	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != command.CodeInvalidState {
		t.Fatalf("err = %v, want E_INVALID_STATE", err)
	}

	if strings.Contains(se.Message+se.Hint, "ws#1") {
		t.Errorf("answer shows the path: %q %q", se.Message, se.Hint)
	}
}
