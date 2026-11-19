package agenthost

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Limits are the times and counts of SDD-017 section 4; a zero member is the default of
// core/chat. Tests shorten them.
type Limits struct {
	Start       time.Duration
	Turn        time.Duration
	CancelGrace time.Duration
	InputClose  time.Duration
	Permission  time.Duration
	Idle        time.Duration
	Lifetime    time.Duration
	MaxLive     int
}

func (l Limits) withDefaults() Limits {
	def := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}

	def(&l.Start, chat.StartTimeout)
	def(&l.Turn, chat.TurnTimeout)
	def(&l.CancelGrace, chat.CancelGrace)
	def(&l.InputClose, chat.InputCloseGrace)
	def(&l.Permission, chat.PermissionTimeout)
	def(&l.Idle, chat.IdleTimeout)
	def(&l.Lifetime, chat.SessionLifetime)

	if l.MaxLive <= 0 {
		l.MaxLive = chat.MaxLiveSessions
	}

	return l
}

// Options configure New. Store is required.
type Options struct {
	Store Store
	// Tokens issues the credential of the MCP server; nil starts agents with no MCP entry.
	Tokens TokenIssuer
	// MCPURL is the address of Stuarty's own MCP endpoint; empty (or nil) means there is none yet.
	MCPURL func() string
	// Workspace holds chat/<project id>/, the working directory of an agent.
	Workspace string
	Now       func() time.Time
	// Environ is the environment the allowlist is applied to; default os.Environ.
	Environ func() []string
	Version string
	Log     *slog.Logger
	Limits  Limits
	// NewID makes ids; default timeline.NewID (cryptographic).
	NewID func(prefix string) string
}

// StartRequest is what a session is started from.
type StartRequest struct {
	SessionID string
	ProjectID string
	Profile   Profile
	// OwnerUserID is the person who started it, in network mode.
	OwnerUserID string
	// StartedBy is the actor of that person.
	StartedBy string
	// Context is the text sent ahead of the first message: the fixed sentence that names the
	// project (SDD-017 section 6.2).
	Context string
}

// Host owns the live sessions: their processes, their protocol connections and their tokens.
type Host struct {
	o   Options
	lim Limits

	mu      sync.Mutex
	live    map[string]*liveSession
	project map[string]string
	closing bool
	wg      sync.WaitGroup
}

// New returns a host. It marks the sessions that a dead server left as ended.
func New(o Options) *Host {
	if o.Now == nil {
		o.Now = time.Now
	}

	if o.NewID == nil {
		o.NewID = timeline.NewID
	}

	if o.Environ == nil {
		o.Environ = os.Environ
	}

	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}

	h := &Host{o: o, lim: o.Limits.withDefaults(), live: map[string]*liveSession{}, project: map[string]string{}}

	if n, err := o.Store.SessionsEndLive(context.Background(), o.Now()); err != nil {
		o.Log.Warn("the chat sessions of the last run were not closed", "error", err)
	} else if n > 0 {
		o.Log.Info("chat sessions left by the last run were closed", "count", n)
	}

	return h
}

func (h *Host) newID(prefix string) string { return h.o.NewID(prefix) }

// Dir is the working directory of the agent of a project.
func (h *Host) Dir(projectID string) string {
	return filepath.Join(h.o.Workspace, "chat", projectID)
}

// Start launches the agent of a profile, opens the protocol and returns the session in the
// state idle. It fails with *StartError, and leaves nothing running when it does.
func (h *Host) Start(ctx context.Context, req StartRequest) (*Session, error) {
	h.mu.Lock()

	if h.closing {
		h.mu.Unlock()

		return nil, &StartError{Reason: ReasonLimit, Detail: "the server is shutting down"}
	}

	var previous *liveSession
	if id, ok := h.project[req.ProjectID]; ok {
		previous = h.live[id]
	}

	h.mu.Unlock()

	// One live session per project: the new one replaces the old.
	if previous != nil {
		previous.end(chat.EndUser)
	}

	h.mu.Lock()

	if len(h.live) >= h.lim.MaxLive {
		h.mu.Unlock()

		return nil, &StartError{Reason: ReasonLimit, Detail: "too many agents are running"}
	}

	s := newLive(h, req)
	h.live[req.SessionID] = s
	h.project[req.ProjectID] = req.SessionID
	h.wg.Add(1)
	h.mu.Unlock()

	if err := s.open(ctx); err != nil {
		return nil, err
	}

	row := s.row()

	return &row, nil
}

func (h *Host) get(id string) (*liveSession, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s, ok := h.live[id]
	if !ok {
		return nil, ErrNoSession
	}

	return s, nil
}

func (h *Host) remove(s *liveSession) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.live, s.req.SessionID)

	if h.project[s.req.ProjectID] == s.req.SessionID {
		delete(h.project, s.req.ProjectID)
	}
}

// Live reports whether a session has a process.
func (h *Host) Live(id string) bool {
	_, err := h.get(id)

	return err == nil
}

// LiveCount is how many sessions have a process.
func (h *Host) LiveCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return len(h.live)
}

// Send gives the agent the next message of the person. It returns when the turn has started;
// what the agent answers arrives as events.
func (h *Host) Send(id, text string) error {
	s, err := h.get(id)
	if err != nil {
		return err
	}

	return s.send(text)
}

// Cancel stops the turn in progress; with end it ends the session.
func (h *Host) Cancel(id string, end bool) error {
	s, err := h.get(id)
	if err != nil {
		return err
	}

	return s.cancelTurn(end)
}

// Answer gives the agent the answer of the person to a permission request.
func (h *Host) Answer(id, requestID string, answer chat.Answer) error {
	s, err := h.get(id)
	if err != nil {
		return err
	}

	return s.answer(requestID, answer)
}

// End ends a session and waits for its process tree to be gone. It is a no-op for a session
// that is not live.
func (h *Host) End(id string, reason chat.EndReason) {
	if s, err := h.get(id); err == nil {
		s.end(reason)
	}
}

// Pending is the event of a live session that is still being written, which the store does not
// hold yet; false when there is none. A read of the transcript adds it and changes nothing: the
// next chunk of the agent still continues it.
func (h *Host) Pending(id string) (chat.Event, bool) {
	s, err := h.get(id)
	if err != nil {
		return chat.Event{}, false
	}

	return s.rec.Pending()
}

// Subscribe follows a live session after the event numbered after.
func (h *Host) Subscribe(ctx context.Context, id string, after int64) (*Subscription, error) {
	s, err := h.get(id)
	if err != nil {
		return nil, err
	}

	return s.rec.Subscribe(ctx, after)
}

// Shutdown ends every session and returns when no child of the host is left running.
func (h *Host) Shutdown() {
	h.mu.Lock()
	h.closing = true

	sessions := make([]*liveSession, 0, len(h.live))
	for _, s := range h.live {
		sessions = append(sessions, s)
	}

	h.mu.Unlock()

	var wg sync.WaitGroup

	for _, s := range sessions {
		wg.Go(func() { s.end(chat.EndShutdown) })
	}

	wg.Wait()
	h.wg.Wait()
}

// startReason maps the failure of a start to the reason of E_AGENT_UNAVAILABLE.
func startReason(err error) string {
	switch {
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		return ReasonNotFound
	case errors.Is(err, ErrStart):
		return ReasonStartFailed
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonTimeout
	case errors.Is(err, ErrSignInRequired):
		return ReasonSignIn
	case errors.Is(err, ErrProtocol):
		return ReasonProtocol
	case errors.Is(err, ErrClosed):
		return ReasonExited
	}

	return ReasonStartFailed
}
