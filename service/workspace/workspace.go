// Package workspace is the data folder every surface shares (ADR-0008,
// SDD-005 section 6): where it is, the lock that makes one process its owner,
// runtime.json that advertises the owner, and the local token file that
// authenticates local clients (SDD-007 section 3).
package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Dir is the absolute path of a workspace directory.
type Dir string

// Subdirectories lists the directories of SDD-000 section 9 that the owner
// creates at start-up. Files (db.sqlite, lock, runtime.json, token) are not here.
var Subdirectories = []string{
	"store", "artifacts", "exports", "frames", "bitmaps",
	"fonts", "tools", "tmp", "backups", "logs",
}

const (
	// EnvHome overrides the workspace location (SDD-000 section 10).
	EnvHome    = "STUARTY_HOME"
	defaultDir = ".stuarty"
)

// Notice is where Resolve says that it adopted the workspace of the former name, or that it found one it
// did not adopt. It is the standard error unless a test replaces it.
var Notice io.Writer = os.Stderr

// Resolve returns the workspace directory: STUARTY_HOME or ~/.stuarty.
// It creates the directory (mode 0700 on Unix) and refuses a path that holds
// ?, # or %, which break the file: DSN of the database (SDD-005 section 6.2).
//
// With STUARTY_HOME unset and no ~/.stuarty, a workspace left by Vukem Video in ~/.vukem-video is
// adopted first, after a backup (SDD-005 section 6.6), and Resolve says so once on Notice.
func Resolve(getenv func(string) string) (Dir, error) {
	path := getenv(EnvHome)
	adopt := ""

	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("workspace: cannot find the home directory: %w", err)
		}
		path = filepath.Join(home, defaultDir)
		adopt = filepath.Join(home, formerDir)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}
	if strings.ContainsAny(abs, "?#%") {
		return "", fmt.Errorf("%w: %q holds ?, # or %%; choose another with %s", ErrUnusablePath, abs, EnvHome)
	}

	if adopt != "" {
		if err := adoptFormer(adopt, abs); err != nil {
			return "", err
		}
	}

	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}
	return Dir(abs), nil
}

// EnsureLayout creates the missing directories of SDD-000 section 9.
func (d Dir) EnsureLayout() error {
	for _, name := range Subdirectories {
		if err := os.MkdirAll(d.Path(name), 0o700); err != nil {
			return fmt.Errorf("workspace: %w", err)
		}
	}
	return nil
}

// Path joins elements under the workspace.
func (d Dir) Path(elem ...string) string {
	return filepath.Join(append([]string{string(d)}, elem...)...)
}

// Error codes of SDD-000 section 9 that this package raises.
const (
	CodeWorkspaceLocked = "E_WORKSPACE_LOCKED"
	CodeUnauthorized    = "E_UNAUTHORIZED"
)

// Error carries an SDD-000 error code so adapters can map it to an exit code
// and an HTTP status without parsing text.
type Error struct {
	Code      string
	Message   string
	Retryable bool
	Err       error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// ErrUnusablePath is returned when the workspace path holds a character that breaks the database
// URL (?, # or %). The wrapped text names the path; an answer to a person must not.
var ErrUnusablePath = errors.New("workspace: the path breaks the database URL")

// ErrLocked is returned by TryLock when another process holds the lock.
var ErrLocked = errors.New("workspace: locked by another process")

// LockedError is the E_WORKSPACE_LOCKED of SDD-005 section 6.3 step 2: it names
// the owner pid when runtime.json is readable.
func (d Dir) LockedError() error {
	msg := "another process owns this workspace"
	if rt, err := d.ReadRuntime(); err == nil {
		msg = fmt.Sprintf("another process owns this workspace (pid %d, %s)", rt.PID, rt.Addr)
	}
	return &Error{Code: CodeWorkspaceLocked, Message: msg, Retryable: true, Err: ErrLocked}
}
