package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// Environment variables of the server (SDD-000 section 10); the shell sets
// both so the child binds a known loopback port and writes into the workspace.
const (
	envAddr = "VUKEM_VIDEO_ADDR"
	envHome = workspace.EnvHome
)

const (
	defaultReadyTimeout = 30 * time.Second
	defaultStopGrace    = 5 * time.Second
	pollEvery           = 50 * time.Millisecond
	stderrTail          = 2048
)

// Reason says why a start failed.
type Reason string

// The reasons a StartupError carries.
const (
	ReasonPortInUse Reason = "port-in-use"
	ReasonSpawn     Reason = "spawn"
	ReasonExited    Reason = "exited"
	ReasonTimeout   Reason = "timeout"
	ReasonRuntime   Reason = "runtime"
)

// StartupError is a failed start. Message is plain text for a dialog.
type StartupError struct {
	Reason  Reason
	Message string
	Err     error
}

func (e *StartupError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *StartupError) Unwrap() error { return e.Err }

// Config describes the server to start.
type Config struct {
	// Binary is the path of vukem-video; it is run as `<Binary> serve`.
	Binary string
	// Workspace is the workspace directory the child uses.
	Workspace workspace.Dir
	// Port is the loopback port to bind; 0 picks a free one.
	Port int
	// Env is added to the child environment (after the variables of the shell).
	Env []string
	// ReadyTimeout bounds the wait for /up (default 30 s).
	ReadyTimeout time.Duration
	// StopGrace is how long Stop waits after the graceful signal before it kills (default 5 s).
	StopGrace time.Duration
}

// Server is a running child.
type Server struct {
	// Addr is the host:port from runtime.json.
	Addr string
	// Port is the numeric port of Addr.
	Port int
	// Token is the workspace local token.
	Token string

	cmd   *exec.Cmd
	grace time.Duration
	done  chan struct{}
	once  sync.Once
}

// URL is the base address of the server.
func (s *Server) URL() string { return "http://" + s.Addr }

// Done is closed when the child has exited.
func (s *Server) Done() <-chan struct{} { return s.done }

// Start runs the server and returns once /up answers 200, or fails when ctx,
// the deadline or the child ends the wait.
func Start(ctx context.Context, cfg Config) (*Server, error) {
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultReadyTimeout
	}
	if cfg.StopGrace <= 0 {
		cfg.StopGrace = defaultStopGrace
	}
	port, err := pickPort(cfg.Port)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	// A stale runtime.json of a dead owner must not be taken for ours.
	if err := cfg.Workspace.RemoveRuntime(); err != nil {
		return nil, &StartupError{Reason: ReasonRuntime, Message: "Cannot prepare the workspace", Err: err}
	}

	var errBuf tailBuffer
	// The path comes from the caller of the shell, never from a page.
	cmd := exec.Command(cfg.Binary, "serve") //nolint:gosec,noctx // a child that lives as long as the window
	cmd.Env = append(os.Environ(), envAddr+"="+addr, envHome+"="+string(cfg.Workspace))
	cmd.Env = append(cmd.Env, cfg.Env...)
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, &StartupError{Reason: ReasonSpawn, Message: "Cannot start " + cfg.Binary, Err: err}
	}
	s := &Server{cmd: cmd, grace: cfg.StopGrace, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()

	if err := s.waitReady(ctx, cfg, &errBuf); err != nil {
		s.Stop()
		return nil, err
	}
	return s, nil
}

func (s *Server) waitReady(ctx context.Context, cfg Config, errBuf *tailBuffer) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.ReadyTimeout)
	defer cancel()
	client := &http.Client{Timeout: time.Second} //nolint:forbidigo // loopback only: the readiness probe of the child
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()

	for {
		select {
		case <-s.done:
			return &StartupError{
				Reason:  ReasonExited,
				Message: "The server stopped while starting: " + errBuf.String(),
			}
		case <-ctx.Done():
			return &StartupError{
				Reason:  ReasonTimeout,
				Message: fmt.Sprintf("The server was not ready after %s", cfg.ReadyTimeout),
				Err:     ctx.Err(),
			}
		case <-tick.C:
		}
		rt, err := cfg.Workspace.ReadRuntime()
		if err != nil || rt.PID != s.cmd.Process.Pid {
			continue
		}
		if err := s.adopt(cfg.Workspace, rt); err != nil {
			return err
		}
		if up(ctx, client, s.URL()+"/up") {
			return nil
		}
	}
}

// adopt takes the address from runtime.json and reads the token, once.
func (s *Server) adopt(ws workspace.Dir, rt workspace.Runtime) error {
	if s.Port == 0 {
		_, p, err := net.SplitHostPort(rt.Addr)
		if err == nil {
			s.Port, err = strconv.Atoi(p)
		}
		if err != nil {
			return &StartupError{Reason: ReasonRuntime, Message: "runtime.json has a bad address", Err: err}
		}
		s.Addr = rt.Addr
	}
	if s.Token == "" {
		token, err := ws.LoadToken()
		if err != nil {
			return &StartupError{Reason: ReasonRuntime, Message: "Cannot read the local token", Err: err}
		}
		s.Token = token
	}
	return nil
}

func up(ctx context.Context, c *http.Client, target string) bool { //nolint:forbidigo // loopback only
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// Stop ends the child: a graceful signal, then a kill after the grace period.
// It returns once the process is gone and is safe to call more than once.
func (s *Server) Stop() {
	s.once.Do(func() {
		select {
		case <-s.done:
			return
		default:
		}
		_ = terminate(s.cmd.Process)
		select {
		case <-s.done:
		case <-time.After(s.grace):
			_ = s.cmd.Process.Kill()
			<-s.done
		}
	})
}

// pickPort checks that a wanted port is free, or finds a free one when want is
// 0. The port can still be taken before the child binds it; the child then
// exits and Start reports that.
func pickPort(want int) (int, error) {
	hostPort := net.JoinHostPort("127.0.0.1", strconv.Itoa(want))
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", hostPort)
	if err != nil {
		if want != 0 {
			return 0, &StartupError{
				Reason:  ReasonPortInUse,
				Message: fmt.Sprintf("Port %d is already in use", want),
				Err:     err,
			}
		}
		return 0, &StartupError{Reason: ReasonPortInUse, Message: "No free local port", Err: err}
	}
	defer func() { _ = ln.Close() }()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("shell: not a TCP address")
	}
	return tcp.Port, nil
}

// tailBuffer keeps the last stderrTail bytes the child wrote.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrTail {
		t.buf = t.buf[len(t.buf)-stderrTail:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(bytes.TrimSpace(t.buf))
}
