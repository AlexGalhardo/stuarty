package agenthost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// StartSpec is what a child is started from (SDD-017 section 4): an executable and an argument
// array that a person wrote in a profile, a working directory inside the workspace, and the
// environment of Environment. Nothing in it is joined into a command line.
type StartSpec struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
}

// Process is one running agent: the child, its standard streams and the handle that kills its
// whole tree.
type Process struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *os.File
	tree   treeKiller

	stderr *ring

	exited  chan struct{}
	waitErr error

	stopOnce sync.Once
}

// ErrStart is a child that could not be started.
var ErrStart = errors.New("agenthost: the agent could not be started")

// startProcess starts the child with pipes of its own. The standard streams are plain files
// handed to the child, so no goroutine of the standard library copies them and no wait is held
// by a grandchild that keeps a pipe open.
func startProcess(spec StartSpec) (*Process, error) {
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStart, err)
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		closeAll(inR, inW)
		return nil, fmt.Errorf("%w: %w", ErrStart, err)
	}

	errR, errW, err := os.Pipe()
	if err != nil {
		closeAll(inR, inW, outR, outW)
		return nil, fmt.Errorf("%w: %w", ErrStart, err)
	}

	// The program and its arguments are an array: exec never goes through a shell.
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...) //nolint:gosec // a profile only a person wrote
	cmd.Dir, cmd.Env = spec.Dir, spec.Env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, errW
	prepare(cmd)

	if err := cmd.Start(); err != nil {
		closeAll(inR, inW, outR, outW, errR, errW)
		return nil, fmt.Errorf("%w: %w", ErrStart, err)
	}

	// The child holds its ends now.
	closeAll(inR, outW, errW)

	tree, err := attach(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		closeAll(inW, outR, errR)

		return nil, fmt.Errorf("%w: %w", ErrStart, err)
	}

	p := &Process{cmd: cmd, stdin: inW, stdout: outR, tree: tree, stderr: &ring{}, exited: make(chan struct{})}

	go func() {
		_, _ = io.Copy(p.stderr, errR)
		_ = errR.Close()
	}()

	go func() {
		p.waitErr = cmd.Wait()
		close(p.exited)
	}()

	return p, nil
}

func closeAll(files ...*os.File) {
	for _, f := range files {
		_ = f.Close()
	}
}

// Stdin is where the protocol messages to the child are written.
func (p *Process) Stdin() io.Writer { return p.stdin }

// Stdout is where the protocol messages of the child are read.
func (p *Process) Stdout() io.Reader { return p.stdout }

// Exited is closed when the child itself has exited.
func (p *Process) Exited() <-chan struct{} { return p.exited }

// Diagnostic is the tail of what the child wrote on its standard error, for the diagnostic of a
// failed start. It is never shown as chat text.
func (p *Process) Diagnostic() string { return p.stderr.String() }

// PID is the process id of the child, for the tests.
func (p *Process) PID() int { return p.cmd.Process.Pid }

// Stop ends the child: it closes the child's input, waits grace for it to exit by itself, then
// kills the whole tree and waits for it. It returns once nothing of the child is left running.
func (p *Process) Stop(grace time.Duration) {
	p.stopOnce.Do(func() {
		_ = p.stdin.Close()

		select {
		case <-p.exited:
		case <-time.After(grace):
		}

		// The tree is killed even when the child exited: a grandchild outlives its parent
		// otherwise.
		p.tree.Kill()

		select {
		case <-p.exited:
		case <-time.After(10 * time.Second):
		}

		p.tree.Close()
		_ = p.stdout.Close()
	})
}

// Kill ends the whole tree at once.
func (p *Process) Kill() {
	p.tree.Kill()
}

// ring keeps the last bytes written to it.
type ring struct {
	mu  sync.Mutex
	buf []byte
}

const ringSize = 8 << 10

func (r *ring) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf = append(r.buf, b...)
	if len(r.buf) > ringSize {
		r.buf = r.buf[len(r.buf)-ringSize:]
	}

	return len(b), nil
}

func (r *ring) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return chat.CleanText(string(r.buf))
}

// allowedEnv is the environment a program needs to find its own configuration and sign-in
// (SDD-017 section 4). Names are compared in upper case, which is how Windows treats them.
var allowedEnv = map[string]bool{
	"PATH": true, "HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true,
	"SYSTEMROOT": true, "COMSPEC": true, "PATHEXT": true, "TEMP": true, "TMP": true, "TMPDIR": true,
	"LANG": true, "USER": true, "USERNAME": true, "LOGNAME": true, "SHELL": true, "TERM": true,
	"HOMEDRIVE": true, "HOMEPATH": true, "PROGRAMDATA": true, "PROGRAMFILES": true,
	"PROGRAMFILES(X86)": true, "ALLUSERSPROFILE": true, "SYSTEMDRIVE": true, "WINDIR": true,
}

// Environment keeps from environ only the names of the allowlist and the LC_ and XDG_ families.
// No STUARTY_ variable, no key, no token and no master key can pass it.
func Environment(environ []string) []string {
	var out []string

	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}

		up := strings.ToUpper(name)

		if allowedEnv[up] || strings.HasPrefix(up, "LC_") || strings.HasPrefix(up, "XDG_") {
			out = append(out, kv)
		}
	}

	return out
}
