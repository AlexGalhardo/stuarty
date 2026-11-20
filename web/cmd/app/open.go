package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// Defaults of the local server (SDD-000 section 10).
const (
	defaultHost = "127.0.0.1"
	defaultPort = "7878"

	// envNoBrowser makes open print the address instead of starting a browser. The installer
	// smoke tests of SDD-009 use it on runners with no desktop.
	envNoBrowser = "STUARTY_NO_BROWSER"
	envAddr      = "STUARTY_ADDR"

	// startTimeout is how long open waits for a server it started to answer /up.
	startTimeout = 30 * time.Second
	startPoll    = 200 * time.Millisecond
)

// opener is `stuarty open` (SDD-003 5.4, SDD-007 3.4): make sure the local server runs, then
// open the UI in the default browser. The seams are fields so the tests need no browser and no
// server process.
//
// It asks the running server for a launch code (POST /api/v1/system/launch, SDD-007 3.4 item 3), whose
// session has no browser-only privileges because only the desktop shell holds the launcher secret
// (3.4 item 7). It runs
// before the application is built because every command of the application needs APP_KEY
// (vukem-feedback.md VF-030), and a shortcut must work on a machine that has none.
type opener struct {
	getenv   func(string) string
	stdout   io.Writer
	stderr   io.Writer
	client   *http.Client //nolint:forbidigo // loopback only: the probe talks to the local server (SDD-007 3.8)
	self     func() (string, error)
	startSrv func(self string, host, port string, env []string, log string) (exited <-chan error, err error)
	browse   func(rawURL string) error
	timeout  time.Duration
	// ownerWait bounds the wait for an owner that holds the workspace lock; poll is the pause
	// between its probes. Zero means the defaults (workspace.WaitLimit, startPoll).
	ownerWait time.Duration
	poll      time.Duration
}

func newOpener(stdout, stderr io.Writer) *opener {
	return &opener{
		getenv: os.Getenv, stdout: stdout, stderr: stderr,
		client:   &http.Client{Timeout: 2 * time.Second}, //nolint:forbidigo // loopback only
		self:     os.Executable,
		startSrv: startServer,
		browse:   openBrowser,
		timeout:  startTimeout,
	}
}

// runOpen is the entry of the command; it returns the exit code.
func runOpen(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		_, _ = fmt.Fprintln(stderr, "usage: stuarty open")
		return 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := newOpener(stdout, stderr).run(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, "stuarty open:", err)
		return runExitCode(err)
	}
	return 0
}

// runExitCode is 2 for a retryable error of the workspace (E_WORKSPACE_LOCKED), 1 otherwise, as the
// exit codes of SDD-003 section 5.4.
func runExitCode(err error) int {
	var werr *workspace.Error
	if errors.As(err, &werr) && werr.Retryable {
		return 2
	}
	return 1
}

// resolveAddr returns the host and port to reach: STUARTY_ADDR, else HOST and PORT (the
// variables vukem reads), else 127.0.0.1:7878. Only a loopback address is accepted: open starts
// and opens a local server, never a remote one.
func resolveAddr(getenv func(string) string) (host, port string, err error) {
	host, port = defaultHost, defaultPort
	if a := getenv(envAddr); a != "" {
		h, p, serr := net.SplitHostPort(a)
		if serr != nil {
			return "", "", fmt.Errorf("%s=%q is not host:port", envAddr, a)
		}
		host, port = h, p
	} else {
		if h := getenv("HOST"); h != "" {
			host = h
		}
		if p := getenv("PORT"); p != "" {
			port = p
		}
	}
	if host == "" {
		host = defaultHost
	}
	switch strings.Trim(host, "[]") {
	case "127.0.0.1", "localhost", "::1":
	default:
		return "", "", fmt.Errorf("%q is not a loopback address: open works with the local server only", host)
	}
	if n := net.JoinHostPort(host, port); n == "" || strings.ContainsAny(port, "/ ?#") {
		return "", "", fmt.Errorf("%q is not a port", port)
	}
	return strings.Trim(host, "[]"), port, nil
}

func baseURL(host, port string) string {
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}).String()
}

// isUp asks /up (vukem's health route) and requires {"status":"ok"}.
func (o *opener) isUp(ctx context.Context, base string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/up", http.NoBody)
	if err != nil {
		return false
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Status string `json:"status"`
	}
	return resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) == nil &&
		body.Status == "ok"
}

func (o *opener) run(ctx context.Context) error {
	host, port, err := resolveAddr(o.getenv)
	if err != nil {
		return err
	}
	base := baseURL(host, port)

	// The server that owns the workspace says where it listens in runtime.json: it wins over the
	// default address, so open never starts a second server beside it (VV-079).
	if rb, ok := o.runningServer(ctx); ok {
		base = rb
	} else if rb, ok, err := o.waitForLockOwner(ctx, base); err != nil {
		return err
	} else if ok {
		base = rb
	} else if !o.isUp(ctx, base) {
		if err := o.start(ctx, host, port, base); err != nil {
			return err
		}
	}

	target := o.launchURL(ctx, base)

	if o.getenv(envNoBrowser) != "" {
		_, _ = fmt.Fprintln(o.stdout, target)
		return nil
	}
	_, _ = fmt.Fprintln(o.stderr, "Opening", base+"/")
	if err := o.browse(target); err != nil {
		_, _ = fmt.Fprintln(o.stdout, target)
		return fmt.Errorf("the server is running but the browser could not be started: %w", err)
	}
	return nil
}

// lockHeld reports whether another process holds the workspace lock, that is, whether an owner
// lives, whatever it answers on /up (VV-258). A lock that cannot be tried says nothing: not held.
func lockHeld(ws workspace.Dir) bool {
	l, err := ws.TryLock()
	if err != nil {
		return errors.Is(err, workspace.ErrLocked)
	}
	_ = l.Release()
	return false
}

// waitForLockOwner is for an owner that holds the lock but is not answering /up yet (busy, starting
// or stuck). It never lets open start a second server beside it: it waits up to ownerWait for the
// owner's /up, at the address of runtime.json or the default one, and then gives up with
// E_WORKSPACE_LOCKED. When the lock is free, or is released while waiting, it answers false with no
// error and open goes on as with no owner.
func (o *opener) waitForLockOwner(ctx context.Context, defaultBase string) (string, bool, error) {
	ws, err := workspace.Resolve(o.getenv)
	if err != nil || !lockHeld(ws) {
		return "", false, nil
	}
	limit, poll := o.ownerWait, o.poll
	if limit <= 0 {
		limit = workspace.WaitLimit
	}
	if poll <= 0 {
		poll = startPoll
	}
	_, _ = fmt.Fprintln(o.stderr, "Another Stuarty owns this workspace; waiting for it to answer")
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		if rb, ok := o.runningServer(ctx); ok {
			return rb, true, nil
		}
		if o.isUp(ctx, defaultBase) {
			return defaultBase, true, nil
		}
		if !lockHeld(ws) {
			return "", false, nil
		}
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-deadline.C:
			return "", false, &workspace.Error{
				Code: workspace.CodeWorkspaceLocked,
				Message: fmt.Sprintf(
					"another Stuarty is running and not answering within %s; wait a few seconds and run open again, "+
						"or stop it (%s)", limit, ownerHint(ws)),
				Retryable: true,
				Err:       workspace.ErrLocked,
			}
		case <-tick.C:
		}
	}
}

func ownerHint(ws workspace.Dir) string {
	if rt, err := ws.ReadRuntime(); err == nil {
		return fmt.Sprintf("pid %d, %s", rt.PID, rt.Addr)
	}
	return "its process holds " + ws.Path("lock")
}

var launchCode = regexp.MustCompile(`^[0-9a-f]{64}$`)

// runningServer is the address in runtime.json when its process is alive, it is a loopback address
// and it answers /up.
func (o *opener) runningServer(ctx context.Context) (string, bool) {
	ws, err := workspace.Resolve(o.getenv)
	if err != nil {
		return "", false
	}
	rt, err := ws.ReadRuntime()
	if err != nil || !workspace.ProcessAlive(rt.PID) || rt.Mode == workspace.ModeNetwork {
		return "", false
	}
	host, port, err := net.SplitHostPort(rt.Addr)
	if err != nil {
		return "", false
	}
	switch strings.Trim(host, "[]") {
	case "127.0.0.1", "localhost", "::1":
	default:
		return "", false
	}
	base := baseURL(strings.Trim(host, "[]"), port)
	return base, o.isUp(ctx, base)
}

// launchURL asks the server for a one-time launch code (SDD-007 3.4 item 3) and returns the address
// that signs the browser in. Without the local token (a server that is still starting) it returns the
// plain address; the editor then shows its sign-in page.
func (o *opener) launchURL(ctx context.Context, base string) string {
	plain := base + "/"
	ws, err := workspace.Resolve(o.getenv)
	if err != nil {
		return plain
	}
	token, err := ws.LoadToken()
	if err != nil {
		return plain
	}
	if err := workspace.Handshake(ctx, o.client, base, token); err != nil {
		_, _ = fmt.Fprintln(o.stderr, "stuarty open: the server did not prove it knows the token:", err)
		return plain
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/system/launch", strings.NewReader("{}"))
	if err != nil {
		return plain
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return plain
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Code string `json:"code"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) != nil ||
		!launchCode.MatchString(body.Code) {
		return plain
	}
	return base + "/auth/launch?code=" + body.Code
}

// start launches `stuarty serve` in the background and waits for /up.
func (o *opener) start(ctx context.Context, host, port, base string) error {
	self, err := o.self()
	if err != nil {
		return fmt.Errorf("cannot find the program to start: %w", err)
	}
	ws, err := workspace.Resolve(o.getenv)
	if err != nil {
		return err
	}
	if err := ws.EnsureLayout(); err != nil {
		return err
	}
	env := os.Environ()
	if o.getenv("APP_KEY") == "" && o.getenv("APP_ENV") == "" {
		key, err := ensureAppKey(ws)
		if err != nil {
			return err
		}
		env = append(env, "APP_KEY="+key)
	}
	env = append(env, "HOST="+host, "PORT="+port)

	_, _ = fmt.Fprintln(o.stderr, "Starting the local server on", net.JoinHostPort(host, port))
	exited, err := o.startSrv(self, host, port, env, filepath.Join(ws.Path("logs"), "server.log"))
	if err != nil {
		return fmt.Errorf("the server could not be started: %w", err)
	}
	deadline := time.NewTimer(o.timeout)
	defer deadline.Stop()
	tick := time.NewTicker(startPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exited:
			return fmt.Errorf(
				"the server stopped right after it started (%v); see %s",
				errOrExit(err),
				ws.Path("logs", "server.log"),
			)
		case <-deadline.C:
			return fmt.Errorf("the server did not answer within %s; see %s", o.timeout, ws.Path("logs", "server.log"))
		case <-tick.C:
			if o.isUp(ctx, base) {
				return nil
			}
		}
	}
}

func errOrExit(err error) string {
	if err == nil {
		return "exit status 0"
	}
	return err.Error()
}

// ensureAppKey returns the application key kept in the workspace, creating it on first use. This is
// a stopgap until the server makes its own key (P1-SRV-001, vukem-feedback.md VF-030): without
// APP_KEY the server refuses to boot.
func ensureAppKey(ws workspace.Dir) (string, error) {
	path := ws.Path("app.key")
	if b, err := os.ReadFile(path); err == nil {
		if k := strings.TrimSpace(string(b)); k != "" {
			return k, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := "base64:" + base64.StdEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("cannot write the application key: %w", err)
	}
	return key, nil
}

// startServer runs `<self> serve` detached from this process, its output going to the log file, and
// reports an early exit on the returned channel.
func startServer(self, _, _ string, env []string, logPath string) (<-chan error, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, err
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(
		context.Background(),
		self,
		"serve",
	)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = logf, logf
	detach(cmd)
	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		return nil, err
	}
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = logf.Close()
		exited <- err
	}()
	return exited, nil
}

// openBrowser starts the default browser with argv, never through a shell (SDD-007 3.4).
func openBrowser(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" {
		return errors.New("not an http address")
	}
	var name string
	var args []string
	switch runtime.GOOS {
	case "windows":
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		name, args = filepath.Join(root, "System32", "rundll32.exe"), []string{"url.dll,FileProtocolHandler", rawURL}
	case "darwin":
		name, args = "/usr/bin/open", []string{rawURL}
	default:
		path, err := exec.LookPath("xdg-open")
		if err != nil {
			return errors.New("xdg-open was not found")
		}
		name, args = path, []string{rawURL}
	}
	cmd := exec.CommandContext( //nolint:gosec // G702: a fixed program per system, an address built above
		context.Background(), name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
