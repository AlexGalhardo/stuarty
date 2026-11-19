// Package cli is the command line adapter of the Service (ADR-0016, SDD-003
// section 5). Each command parses its arguments, calls one service.Service
// method and prints the result; no editing rule lives here (ADR-0007).
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

const (
	// EnvProject names the project a command acts on when --project is absent.
	EnvProject = "STUARTY_PROJECT"
	// DefaultActor is the label of the caller when --actor is absent.
	DefaultActor = "cli"
)

// Target says how a command wants to reach the workspace.
type Target struct {
	// Workspace is the --workspace flag; empty means the environment, then the default.
	Workspace string
	// Actor is the label of the caller, already validated.
	Actor string
	// KeyFile is the --key-file flag.
	KeyFile string
	// Write is true for a command that writes or starts a job.
	Write bool
}

// Connection is a Service the CLI can call, and what to release afterwards.
type Connection struct {
	Service service.Service
	// InProcess is true when this process owns the workspace (SDD-003 section 5.2 step 3).
	InProcess bool
	// Workspace is the directory the human note names.
	Workspace string
	// Server is the address of the server the Service talks to; empty when this process is the owner.
	Server string
	// Close releases the workspace lock of an in-process connection; may be nil.
	Close func() error
	// Events opens the event stream of a project on the server; nil when no
	// server answers (the events live in the hub of the owner).
	Events EventOpener
	// Download opens the file at an API path of the server (a download URL); nil when the
	// workspace is local and the Service reports the path of the file.
	Download Downloader
}

// Resolver finds the Service for a command: a remote server, the local owner,
// or this process (SDD-003 section 5.2).
type Resolver interface {
	Resolve(ctx context.Context, t Target) (*Connection, error)
}

// Runtime is everything a command touches outside the Service, so tests can
// replace it.
type Runtime struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Getenv   func(string) string
	Getwd    func() (string, error)
	Resolver Resolver
	// Interactive is true when Stdin is a terminal, so a command may ask a question.
	Interactive bool
	// StderrTTY is true when Stderr is a terminal, so a long command may draw its progress there.
	StderrTTY bool
	// Manifest returns the pins of the helpers this build may install; the one
	// embedded in the binary when nil.
	Manifest func() (transcribe.Manifest, error)
	// Installer builds the install step over a local workspace directory, for tools:install while a
	// server owns it; owner.InstallTools when nil.
	Installer func(workspaceDir string) (service.ToolsInstaller, error)

	// resolved are the paths abs made absolute in this run, so that a missing
	// file is reported with the path that was looked for (VV-048 b).
	resolved []string
}

// NewRuntime is the Runtime of the real process.
func NewRuntime(r Resolver) *Runtime {
	return &Runtime{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Getenv: os.Getenv, Getwd: os.Getwd, Resolver: r, Interactive: isTerminal(os.Stdin),
		StderrTTY: isTerminal(os.Stderr),
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()

	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var actorLabel = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// actorFrom validates the --actor flag (SDD-003 section 5.1) and returns the label.
func actorFrom(flag string) (string, error) {
	if flag == "" {
		return DefaultActor, nil
	}

	label := strings.TrimPrefix(flag, "local:")

	if strings.HasPrefix(label, "user:") || strings.HasPrefix(label, "key:") {
		return "", validation("actor", "--actor cannot name a user or a key: %q.", flag)
	}

	if label == "ui" {
		return "", validation("actor", "the actor label ui is reserved for the browser.")
	}

	if !actorLabel.MatchString(label) {
		return "", validation("actor", "--actor must be 1 to 32 characters of a-z, 0-9 and -: %q.", flag)
	}

	return label, nil
}

// abs makes a path absolute against the current directory before it is sent
// to the service (SDD-003 section 5.3, ADR-0016 item 7). Against a remote server
// (STUARTY_URL) a rooted path belongs to the server's filesystem and goes as typed.
func (rt *Runtime) abs(path string) (string, error) {
	out, err := rt.absolute(expandPath(path, rt.Getenv))
	if err == nil {
		rt.resolved = append(rt.resolved, out)
	}

	return out, err
}

func (rt *Runtime) absolute(path string) (string, error) {
	if rt.Getenv != nil && rt.Getenv(workspace.EnvURL) != "" && strings.HasPrefix(path, "/") {
		return path, nil
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	wd, err := rt.Getwd()
	if err != nil {
		return "", fmt.Errorf("read the current directory: %w", err)
	}

	return filepath.Join(wd, path), nil
}
