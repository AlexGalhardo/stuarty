package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// MCPServerName is the name the agent sees for Stuarty's own MCP server.
const MCPServerName = "stuarty"

type pendingPerm struct {
	id       string
	options  []chat.Option
	title    string
	toolKind string
	callID   string
	ch       chan permResult
	answered bool
}

type permResult struct {
	optionID  string
	cancelled bool
}

// liveSession is one agent running for one project: the process, the protocol connection, the
// token, the recorder of the transcript and the state machine of core/chat.
type liveSession struct {
	h   *Host
	req StartRequest
	rec *recorder

	ctx    context.Context
	cancel context.CancelFunc

	proc  *Process
	conn  *Conn
	acpID string

	flood floodMeter

	mu        sync.Mutex
	state     chat.State
	endReason chat.EndReason
	title     string
	firstSent bool
	turn      int
	perms     map[string]*pendingPerm
	revoke    func()
	idle      *time.Timer
	life      *time.Timer
	created   time.Time
	updated   time.Time
	ended     *time.Time

	endOnce sync.Once
	done    chan struct{}
}

func newLive(h *Host, req StartRequest) *liveSession {
	ctx, cancel := context.WithCancel(context.Background())
	now := h.o.Now().UTC()

	return &liveSession{
		h: h, req: req, ctx: ctx, cancel: cancel,
		rec:     newRecorder(h.o.Store, req.SessionID, h.o.Now, h.o.Log),
		flood:   floodMeter{now: h.o.Now},
		state:   chat.StateStarting,
		perms:   map[string]*pendingPerm{},
		created: now, updated: now, done: make(chan struct{}),
	}
}

// row is the session as the store keeps it.
func (s *liveSession) row() Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.rowLocked()
}

func (s *liveSession) rowLocked() Session {
	return Session{
		ID: s.req.SessionID, ProjectID: s.req.ProjectID, Profile: s.req.Profile.Name,
		OwnerUserID: s.req.OwnerUserID, StartedBy: s.req.StartedBy, State: s.state, EndReason: s.endReason,
		Title: s.title, CreatedAt: s.created, UpdatedAt: s.updated, EndedAt: s.ended, EventCount: s.rec.count(),
	}
}

// save writes the row; the caller holds no lock.
func (s *liveSession) save() {
	s.mu.Lock()
	row := s.rowLocked()
	s.mu.Unlock()

	if err := s.h.o.Store.SessionUpdate(context.Background(), row); err != nil {
		s.h.o.Log.Warn("a chat session was not saved", "session", row.ID, "error", err)
	}
}

// open starts the process and opens the protocol (SDD-017 sections 2 and 4).
func (s *liveSession) open(ctx context.Context) error {
	o := &s.h.o

	row := s.row()
	if err := o.Store.SessionCreate(ctx, row); err != nil {
		s.abandon()

		return &StartError{Reason: ReasonStartFailed, Detail: "the session could not be stored"}
	}

	if err := o.Store.SessionsPrune(ctx, s.req.ProjectID, chat.MaxSessionsPerProj); err != nil {
		o.Log.WarnContext(ctx, "old chat sessions were not pruned", "error", err)
	}

	dir := s.h.Dir(s.req.ProjectID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return s.failStart(&StartError{Reason: ReasonStartFailed, Detail: "the working directory could not be made"})
	}

	proc, err := startProcess(StartSpec{
		Command: s.req.Profile.Command, Args: s.req.Profile.Args, Dir: dir, Env: Environment(o.Environ()),
	})
	if err != nil {
		return s.failStart(&StartError{Reason: startReason(err), Detail: "the program could not be started"})
	}

	s.proc = proc
	s.conn = NewConn(proc.Stdout(), proc.Stdin(), s)

	sctx, cancel := context.WithTimeout(ctx, s.h.lim.Start)
	defer cancel()

	// A child that exits before it answers ends the wait at once.
	go func() {
		select {
		case <-proc.Exited():
			s.conn.Close()
		case <-s.ctx.Done():
		}
	}()

	init, err := s.conn.Initialize(sctx, o.Version)
	if err != nil {
		return s.failStart(s.startFailure(err))
	}

	if init.ProtocolVersion != ProtocolVersion {
		return s.failStart(&StartError{Reason: ReasonProtocolVersion})
	}

	servers, err := s.mcpServers(init)
	if err != nil {
		return s.failStart(err)
	}

	id, err := s.conn.NewSession(sctx, dir, servers)
	if err != nil {
		return s.failStart(s.startFailure(err))
	}

	s.acpID = id

	s.mu.Lock()
	next, _ := chat.Next(s.state, chat.InputStarted)
	s.state = next
	s.mu.Unlock()

	s.rec.State(chat.StateInfo{State: chat.StateIdle})
	s.save()

	s.mu.Lock()
	s.idle = time.AfterFunc(s.h.lim.Idle, func() { s.end(chat.EndIdle) })
	s.life = time.AfterFunc(s.h.lim.Lifetime, func() { s.end(chat.EndTimeout) })
	s.mu.Unlock()

	go s.watch()

	return nil
}

// startFailure explains why the opening of the protocol failed, with the diagnostic of the
// child when it exited.
func (s *liveSession) startFailure(err error) *StartError {
	reason := startReason(err)
	if reason == ReasonExited || reason == ReasonProtocol {
		select {
		case <-s.proc.Exited():
			reason = ReasonExited
		default:
		}
	}

	return &StartError{Reason: reason}
}

// mcpServers builds the one MCP entry of the session: Stuarty's own server, reached over HTTP
// with the token of this session (SDD-017 section 6.1).
func (s *liveSession) mcpServers(init *InitializeResult) ([]MCPServer, error) {
	o := &s.h.o

	if o.Tokens == nil || o.MCPURL == nil {
		return nil, nil
	}

	url := o.MCPURL()
	if url == "" {
		return nil, nil
	}

	if !init.AgentCapabilities.MCP.HTTP {
		return nil, &StartError{Reason: ReasonNoHTTPMCP}
	}

	secret, revoke, err := o.Tokens.Issue(Grant{
		SessionID: s.req.SessionID, ProjectID: s.req.ProjectID, Owner: s.req.OwnerUserID, Scopes: GrantScopes(),
	}, o.Now().Add(s.h.lim.Lifetime))
	if err != nil {
		return nil, &StartError{Reason: ReasonStartFailed, Detail: "the session token could not be issued"}
	}

	s.mu.Lock()
	s.revoke = revoke
	s.mu.Unlock()

	s.rec.AddSecret(secret)

	return []MCPServer{{
		Type: "http", Name: MCPServerName, URL: url,
		Headers: []Header{{Name: "Authorization", Value: "Bearer " + secret}},
	}}, nil
}

// failStart ends a session that never opened: nothing is left running.
func (s *liveSession) failStart(err error) error {
	var se *StartError
	if !errors.As(err, &se) {
		se = &StartError{Reason: ReasonStartFailed}
	}

	if s.proc != nil && se.Detail == "" {
		if diag := s.proc.Diagnostic(); diag != "" {
			s.h.o.Log.Debug("the agent failed to start", "session", s.req.SessionID, "stderr", diag)
		}
	}

	s.end(chat.EndStartFailed)

	return se
}

// abandon forgets a session whose row could not be written.
func (s *liveSession) abandon() {
	s.cancel()
	s.h.remove(s)
	s.h.wg.Done()
	close(s.done)
}

// watch ends the session when its process or its connection ends.
func (s *liveSession) watch() {
	select {
	case <-s.ctx.Done():
	case <-s.proc.Exited():
		s.end(s.endingReason())
	case <-s.conn.Done():
		s.end(s.endingReason())
	}
}

func (s *liveSession) endingReason() chat.EndReason {
	if err := s.conn.Err(); errors.Is(err, ErrProtocol) {
		return chat.EndProtocol
	}

	return chat.EndExited
}

// end ends the session once: it stops the process tree, revokes the token, writes the last
// events and closes the subscriptions. It returns when the process tree is gone.
func (s *liveSession) end(reason chat.EndReason) {
	s.endOnce.Do(func() { s.finish(reason) })
	<-s.done
}

func (s *liveSession) finish(reason chat.EndReason) {
	defer s.h.wg.Done()
	defer close(s.done)

	s.mu.Lock()
	if s.idle != nil {
		s.idle.Stop()
	}

	if s.life != nil {
		s.life.Stop()
	}

	next, err := chat.Next(s.state, chat.InputEnd)
	if err == nil {
		s.state = next
	}

	s.endReason = reason
	now := s.h.o.Now().UTC()
	s.updated, s.ended = now, &now
	revoke := s.revoke
	s.revoke = nil

	for _, p := range s.perms {
		if !p.answered {
			p.answered = true
			p.ch <- permResult{cancelled: true}
		}
	}
	s.mu.Unlock()

	if revoke != nil {
		revoke()
	}

	s.cancel()

	s.rec.State(chat.StateInfo{State: chat.StateEnded, EndReason: reason})

	if s.proc != nil {
		s.proc.Stop(s.h.lim.InputClose)
	}

	if s.conn != nil {
		s.conn.Close()
	}

	s.rec.Close()
	s.save()
	s.h.remove(s)
}

// Notify handles a notification of the agent: session/update becomes events.
func (s *liveSession) Notify(method string, params json.RawMessage) {
	if s.flood.Hit() {
		go s.end(chat.EndFlood)
		return
	}

	if method != MethodSessionUpdate {
		return
	}

	u, ok := ParseUpdate(params)
	if !ok || u.SessionID != s.acpID {
		return
	}

	var kept bool

	switch u.Kind {
	case chat.KindToolCall:
		kept = s.rec.Tool(u)
	default:
		kept = s.rec.Chunk(u.Kind, u.MessageID, u.Text)
	}

	if !kept {
		go s.end(chat.EndFlood)
	}
}

// Request handles a request of the agent: a permission request waits for a person; the
// capabilities the client did not offer are refused.
func (s *liveSession) Request(ctx context.Context, method string, params json.RawMessage) (any, *RPCError) {
	if method != MethodRequestPermission {
		return nil, MethodNotFound(method)
	}

	req, err := ParsePermission(params)
	if err != nil || req.SessionID != s.acpID {
		return nil, InvalidParams()
	}

	p := &pendingPerm{
		id: s.h.newID(chat.PermissionPrefix), options: req.Options, title: req.Title, toolKind: req.ToolKind,
		callID: req.CallID, ch: make(chan permResult, 1),
	}

	s.mu.Lock()
	if len(s.perms) >= 100 {
		s.mu.Unlock()

		return PermissionOutcome("", true), nil
	}

	s.perms[p.id] = p

	if next, err := chat.Next(s.state, chat.InputPermissionAsked); err == nil {
		s.state = next
	}
	s.mu.Unlock()

	s.rec.Permission(chat.Permission{
		RequestID: p.id, CallID: p.callID, Title: p.title, ToolKind: p.toolKind, Options: p.options,
	})
	s.rec.State(chat.StateInfo{State: chat.StateAwaitingPermission})

	timeout := time.NewTimer(s.h.lim.Permission)
	defer timeout.Stop()

	select {
	case r := <-p.ch:
		return PermissionOutcome(r.optionID, r.cancelled), nil
	case <-timeout.C:
		s.settle(p, permResult{cancelled: true}, chat.AnswerCancel)

		return PermissionOutcome("", true), nil
	case <-ctx.Done():
		return PermissionOutcome("", true), nil
	}
}

// settle answers a request once, and returns the session to running when it waited on it.
func (s *liveSession) settle(p *pendingPerm, r permResult, answer chat.Answer) bool {
	s.mu.Lock()
	if p.answered {
		s.mu.Unlock()
		return false
	}

	p.answered = true

	resumed := false
	if next, err := chat.Next(s.state, chat.InputPermissionAnswered); err == nil {
		s.state, resumed = next, true
	}
	s.mu.Unlock()

	p.ch <- r

	s.rec.PermissionAnswer(chat.Permission{RequestID: p.id, CallID: p.callID, Title: p.title, Answer: answer})

	if resumed {
		s.rec.State(chat.StateInfo{State: chat.StateRunning})
	}

	return true
}

func (s *liveSession) answer(requestID string, a chat.Answer) error {
	s.mu.Lock()
	p, ok := s.perms[requestID]
	s.mu.Unlock()

	if !ok {
		return ErrNoRequest
	}

	id, cancelled, err := chat.PickOption(p.options, a)
	if err != nil {
		return err
	}

	if !s.settle(p, permResult{optionID: id, cancelled: cancelled}, a) {
		return ErrAnswered
	}

	return nil
}

func (s *liveSession) send(text string) error {
	s.mu.Lock()

	next, err := chat.Next(s.state, chat.InputSend)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	s.state = next
	s.turn++
	gen := s.turn
	blocks := []string{text}

	if !s.firstSent {
		s.firstSent = true

		if s.req.Context != "" {
			blocks = []string{s.req.Context, text}
		}

		s.title = chat.Title(text)
	}

	s.updated = s.h.o.Now().UTC()

	if s.idle != nil {
		s.idle.Stop()
	}
	s.mu.Unlock()

	s.rec.UserMessage(text)
	s.rec.State(chat.StateInfo{State: chat.StateRunning})
	s.save()

	go s.runTurn(gen, blocks)

	return nil
}

func (s *liveSession) runTurn(gen int, blocks []string) {
	timer := time.AfterFunc(s.h.lim.Turn, func() { s.stopTurn(gen) })
	defer timer.Stop()

	stop, err := s.conn.Prompt(s.ctx, s.acpID, blocks...)

	s.finishTurn(gen, stop, err)
}

// stopTurn asks the agent to stop a turn that went on too long or that a person cancelled, and
// ends the session when the agent does not stop within the grace time.
func (s *liveSession) stopTurn(gen int) {
	s.mu.Lock()
	current := s.turn == gen && (s.state == chat.StateRunning || s.state == chat.StateAwaitingPermission)
	s.mu.Unlock()

	if !current {
		return
	}

	_ = s.conn.Cancel(s.acpID)
	s.cancelPermissions()

	time.AfterFunc(s.h.lim.CancelGrace, func() {
		s.mu.Lock()
		still := s.turn == gen && (s.state == chat.StateRunning || s.state == chat.StateAwaitingPermission)
		s.mu.Unlock()

		if still {
			s.end(chat.EndTimeout)
		}
	})
}

func (s *liveSession) cancelPermissions() {
	s.mu.Lock()

	var open []*pendingPerm

	for _, p := range s.perms {
		if !p.answered {
			open = append(open, p)
		}
	}
	s.mu.Unlock()

	for _, p := range open {
		s.settle(p, permResult{cancelled: true}, chat.AnswerCancel)
	}
}

func (s *liveSession) cancelTurn(end bool) error {
	if end {
		s.end(chat.EndUser)
		return nil
	}

	s.mu.Lock()
	state, turn := s.state, s.turn
	_, err := chat.Next(state, chat.InputCancel)
	s.mu.Unlock()

	if err != nil {
		return err
	}

	if state != chat.StateIdle {
		s.stopTurn(turn)
	}

	return nil
}

func (s *liveSession) finishTurn(gen int, stop string, err error) {
	s.mu.Lock()

	if s.turn != gen || s.state == chat.StateEnded {
		s.mu.Unlock()
		return
	}

	var rpc *RPCError

	switch {
	case err == nil:
	case errors.As(err, &rpc):
	default:
		// The connection broke or the session ended: the watcher ends the session.
		s.mu.Unlock()

		return
	}

	if next, nerr := chat.Next(s.state, chat.InputTurnEnded); nerr == nil {
		s.state = next
	}

	s.updated = s.h.o.Now().UTC()

	if s.idle != nil {
		s.idle.Reset(s.h.lim.Idle)
	}
	s.mu.Unlock()

	s.rec.Flush()

	switch {
	case rpc != nil:
		s.rec.Notice("The agent reported an error: " + clean(rpc.Message, 500))
	case stop == "cancelled":
		s.rec.Notice("The turn was cancelled.")
	case stop != "" && stop != "end_turn":
		s.rec.Notice("The agent stopped: " + clean(stop, 64))
	}

	s.rec.State(chat.StateInfo{State: chat.StateIdle})
	s.save()
}
