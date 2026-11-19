package workspace

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

// Force selects how Open behaves (SDD-005 section 6.4).
type Force int

const (
	// Auto uses a running owner, else becomes the in-process owner.
	Auto Force = iota
	// Server never falls back to in-process: E_WORKSPACE_LOCKED when no owner answers.
	Server
	// InProcess never uses a server: E_WORKSPACE_LOCKED when one exists, and E_INVALID_STATE naming
	// STUARTY_URL when that names a remote one.
	InProcess
)

// Environment variables of SDD-000 section 10 read by Open.
const (
	EnvURL = "STUARTY_URL"
	EnvKey = "STUARTY_KEY"
)

const (
	// WaitLimit is how long Open waits for an owner that is starting or for
	// another in-process command to finish (SDD-005 section 6.4 step 4).
	WaitLimit = 10 * time.Second
	// PollEvery is the pause between attempts in that wait.
	PollEvery = 100 * time.Millisecond
	// ProbeTimeout bounds each GET /up and handshake request.
	ProbeTimeout = 500 * time.Millisecond

	handshakePath    = "/api/v1/handshake"
	handshakeContext = "stuarty-handshake-v1"
)

// Options configures Open. The zero value is Auto with the process environment.
type Options struct {
	Force   Force
	Getenv  func(string) string // default os.Getenv
	KeyFile string              // --key-file; used with STUARTY_URL
	// Test seams; zero values mean the constants above.
	Wait   time.Duration
	Poll   time.Duration
	Client *http.Client
	OnPoll func() // called before every attempt of the wait, so a test can act deterministically
}

// Mode tells how the caller reaches the workspace.
type Mode int

const (
	// ModeInProcess means this process owns the workspace and holds its lock.
	ModeInProcess Mode = iota
	// ModeHTTP means an owner process answers at BaseURL; send Token as a bearer.
	ModeHTTP
)

// Session is the result of Open. The adapters of P1-SRV-003 and P1-SRV-005
// turn it into a service.Service: an in-process one over the store, or the
// remote client.
type Session struct {
	Mode Mode
	Dir  Dir // empty when STUARTY_URL was used: no local file is read
	// Lock is held until Close when Mode is ModeInProcess.
	Lock *Lock
	// BaseURL and Token are set when Mode is ModeHTTP.
	BaseURL string
	Token   string
}

// Close releases the lock of an in-process session.
func (s *Session) Close() error { return s.Lock.Release() }

// Open picks in-process or HTTP for a command, in the order of SDD-003 section
// 5.2 and SDD-005 section 6.4.
func Open(ctx context.Context, o Options) (*Session, error) {
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	if o.Wait == 0 {
		o.Wait = WaitLimit
	}
	if o.Poll == 0 {
		o.Poll = PollEvery
	}
	if o.Client == nil {
		o.Client = &http.Client{
			Timeout: ProbeTimeout,
		}
	}

	if raw := o.Getenv(EnvURL); raw != "" {
		// A command that must own the workspace has no folder in a remote session: answering with one let
		// it write the workspace into the current directory (VV-272).
		if o.Force == InProcess {
			return nil, &Error{
				Code: CodeInvalidState,
				Message: "this command writes the local workspace and " + EnvURL + " names a server: unset it " +
					"and run the command on the machine of the workspace, with no server running",
			}
		}
		return remoteSession(raw, o)
	}

	d, err := Resolve(o.Getenv)
	if err != nil {
		return nil, err
	}

	if o.Force != InProcess {
		if s, err := tryServer(ctx, d, o); err != nil || s != nil {
			return s, err
		}
	}

	if o.Force == Server {
		return waitForServer(ctx, d, o)
	}

	if s, err := tryOwn(d); err != nil || s != nil {
		return s, err
	}
	if o.Force == InProcess {
		return nil, d.LockedError()
	}
	return waitAndRetry(ctx, d, o)
}

func remoteSession(raw string, o Options) (*Session, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("workspace: %s must be an http or https URL", EnvURL)
	}
	// A key never crosses the network in clear (SDD-007 section 4.3): plain HTTP
	// is for the loopback name of a container on the user's own machine.
	if u.Scheme == "http" && !loopbackName(u.Hostname()) {
		return nil, fmt.Errorf("workspace: %s must be https (http is accepted only for localhost): "+
			"the API key would cross the network in clear", EnvURL)
	}
	key := o.Getenv(EnvKey)
	if o.KeyFile != "" {
		data, err := os.ReadFile(o.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("workspace: key file: %w", err)
		}
		key = strings.TrimSpace(string(data))
	}
	if key == "" {
		return nil, unauthorizedNoKey()
	}
	return &Session{Mode: ModeHTTP, BaseURL: strings.TrimRight(raw, "/"), Token: key}, nil
}

func loopbackName(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

func unauthorizedNoKey() error {
	return &Error{
		Code:    CodeUnauthorized,
		Message: "this server needs an API key: set " + EnvKey + " or pass --key-file",
	}
}

// tryOwn takes the lock if it is free. A stale runtime.json is removed.
func tryOwn(d Dir) (*Session, error) {
	l, err := d.TryLock()
	if errors.Is(err, ErrLocked) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := d.RemoveRuntime(); err != nil {
		_ = l.Release()
		return nil, err
	}
	return &Session{Mode: ModeInProcess, Dir: d, Lock: l}, nil
}

// tryServer returns a session when a live owner answers, nil when there is
// none (yet), and an error when the answering process must not be trusted.
func tryServer(ctx context.Context, d Dir, o Options) (*Session, error) {
	rt, err := d.ReadRuntime()
	if err != nil || !ProcessAlive(rt.PID) {
		return nil, nil //nolint:nilerr // absent, unreadable or stale: no owner to use
	}
	base, err := baseURL(rt.Addr)
	if err != nil {
		return nil, nil //nolint:nilerr // a malformed address is a stale file
	}
	if !probeUp(ctx, o.Client, base) {
		return nil, nil
	}

	if rt.Mode == ModeNetwork {
		// The owner proves itself with the per-run secret before the key is sent: a process that
		// took the advertised address cannot read a file private to the user (SDD-007 section 3.8).
		secret, err := d.LoadRuntimeSecret()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil // the owner is still starting
		}
		if err != nil {
			return nil, err
		}
		if err := Handshake(ctx, o.Client, base, secret); err != nil {
			if errors.Is(err, errHandshakeTransport) {
				return nil, nil
			}
			return nil, err
		}
		key := o.Getenv(EnvKey)
		if o.KeyFile != "" {
			data, err := os.ReadFile(o.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("workspace: key file: %w", err)
			}
			key = strings.TrimSpace(string(data))
		}
		if key == "" {
			return nil, unauthorizedNoKey()
		}
		return &Session{Mode: ModeHTTP, Dir: d, BaseURL: base, Token: key}, nil
	}

	if !isLoopback(rt.Addr) {
		return nil, &Error{
			Code:    CodeUnauthorized,
			Message: "runtime.json advertises a local owner on a non-loopback address; refusing to send the token",
		}
	}
	token, err := d.LoadToken()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // the owner is still starting
	}
	if err != nil {
		return nil, err
	}
	if err := Handshake(ctx, o.Client, base, token); err != nil {
		if errors.Is(err, errHandshakeTransport) {
			return nil, nil
		}
		return nil, err
	}
	return &Session{Mode: ModeHTTP, Dir: d, BaseURL: base, Token: token}, nil
}

// waitForServer is the wait of Force == Server: it never takes the lock, and
// gives up at once when nobody holds it (there is no owner to wait for).
func waitForServer(ctx context.Context, d Dir, o Options) (*Session, error) {
	if l, err := d.TryLock(); err == nil {
		_ = l.Release()
		return nil, &Error{
			Code:      CodeWorkspaceLocked,
			Message:   "no server owns this workspace; start one with serve",
			Retryable: true,
		}
	} else if !errors.Is(err, ErrLocked) {
		return nil, err
	}
	return poll(ctx, d, o, func() (*Session, error) { return tryServer(ctx, d, o) })
}

// waitAndRetry is step 4 of SDD-005 section 6.4: retry TryLock first, so a
// waiting command becomes the owner as soon as the previous one exits.
func waitAndRetry(ctx context.Context, d Dir, o Options) (*Session, error) {
	return poll(ctx, d, o, func() (*Session, error) {
		if s, err := tryOwn(d); err != nil || s != nil {
			return s, err
		}
		return tryServer(ctx, d, o)
	})
}

func poll(ctx context.Context, d Dir, o Options, attempt func() (*Session, error)) (*Session, error) {
	deadline := time.NewTimer(o.Wait)
	defer deadline.Stop()
	tick := time.NewTicker(o.Poll)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, d.LockedError()
		case <-tick.C:
			if o.OnPoll != nil {
				o.OnPoll()
			}
			if s, err := attempt(); err != nil || s != nil {
				return s, err
			}
		}
	}
}

// baseURL turns an owner address into an http URL, replacing an unspecified
// host with the loopback address.
func baseURL(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsUnspecified() {
		if ip.Is4() {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback() && ip.Zone() == ""
}

func probeUp(
	ctx context.Context,
	c *http.Client,
	base string,
) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/up", http.NoBody)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode == http.StatusOK
}

var errHandshakeTransport = errors.New("workspace: handshake did not complete")

// HandshakeProof is the proof a server that knows the token returns for a
// nonce: HMAC-SHA256 keyed by the token over a fixed context and the nonce
// (SDD-007 section 3.8). The server side of P1-SRV-005 uses it too.
func HandshakeProof(token, nonce string) string {
	m := hmac.New(sha256.New, []byte(token))
	m.Write([]byte(handshakeContext + nonce))
	return hex.EncodeToString(m.Sum(nil))
}

// Handshake checks that the server at base knows the token before the token
// is sent to it. A transport failure wraps errHandshakeTransport (retryable);
// a wrong answer is E_UNAUTHORIZED and must not be retried.
func Handshake(
	ctx context.Context,
	c *http.Client,
	base, token string,
) error {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	nonce := hex.EncodeToString(raw[:])

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+handshakePath+"?nonce="+nonce, http.NoBody)
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", errHandshakeTransport, err)
	}
	defer resp.Body.Close()

	var body struct {
		Proof string `json:"proof"`
	}
	if resp.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) != nil ||
		!hmac.Equal([]byte(body.Proof), []byte(HandshakeProof(token, nonce))) {
		return &Error{
			Code:    CodeUnauthorized,
			Message: "the process at " + base + " did not prove that it knows the workspace token; the token was not sent",
		}
	}
	return nil
}
