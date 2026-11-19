// Package agenthost starts the agent a user installed, speaks the Agent Client Protocol to it
// over its standard streams, and turns what it says into the events of a chat transcript
// (ADR-0037, SDD-017). It holds no rule of the chat: the states, the profile rules and the
// answers a person may give are in core/chat. It imports nothing of the service, so the
// service can host it behind the small interfaces of this package.
package agenthost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// ProtocolVersion is the version of the Agent Client Protocol this client speaks (SDD-017
// section 2). The client is this package, so a change of the protocol is a change of one place.
const ProtocolVersion = 1

// The JSON-RPC codes the client answers with.
const (
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// The errors of the connection. ErrProtocol wraps what the agent did wrong on the wire.
var (
	ErrClosed       = errors.New("agenthost: the connection is closed")
	ErrProtocol     = errors.New("agenthost: the agent broke the protocol")
	ErrLineTooLong  = fmt.Errorf("%w: a line over %d bytes", ErrProtocol, chat.MaxLineBytes)
	errUnknownReply = fmt.Errorf("%w: an answer to a request that was never made", ErrProtocol)
)

// RPCError is an error object of a JSON-RPC response from the agent.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("agent error %d: %s", e.Code, e.Message) }

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Handler receives what the agent sends on its own. Notify is called in order, on the reading
// goroutine, so it must not block; Request runs on a goroutine of its own and may wait for a
// person.
type Handler interface {
	Notify(method string, params json.RawMessage)
	// Request answers a request of the agent. A non-nil *RPCError is sent as the error of the
	// response; otherwise result is sent.
	Request(ctx context.Context, method string, params json.RawMessage) (result any, rpcErr *RPCError)
}

// Conn is the JSON-RPC connection to one agent: one message per line, no embedded newline.
type Conn struct {
	w  io.Writer
	wm sync.Mutex
	h  Handler

	next    atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan *message
	closed  bool
	err     error

	done   chan struct{}
	cancel context.CancelFunc
	ctx    context.Context
}

// NewConn starts reading r. The connection ends when r ends, when the agent breaks the protocol
// or when Close is called; Done is closed then and Err says why.
func NewConn(r io.Reader, w io.Writer, h Handler) *Conn {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Conn{w: w, h: h, pending: map[int64]chan *message{}, done: make(chan struct{}), ctx: ctx, cancel: cancel}

	go c.read(r)

	return c
}

// Done is closed when the connection ended.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err is why the connection ended, nil while it is open; io.EOF-like endings are ErrClosed.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.err
}

// Close ends the connection and fails every call in flight.
func (c *Conn) Close() { c.fail(ErrClosed) }

func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}

	c.closed, c.err = true, err

	pending := c.pending
	c.pending = map[int64]chan *message{}
	c.mu.Unlock()

	c.cancel()

	for _, ch := range pending {
		close(ch)
	}

	close(c.done)
}

// readLine returns one line without its newline, never more than chat.MaxLineBytes of it.
func readLine(br *bufio.Reader) ([]byte, error) {
	var line []byte

	for {
		chunk, err := br.ReadSlice('\n')
		line = append(line, chunk...)

		if len(line) > chat.MaxLineBytes+1 {
			return nil, ErrLineTooLong
		}

		switch {
		case err == nil:
			return bytes.TrimRight(line, "\r\n"), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			if len(line) > 0 && errors.Is(err, io.EOF) {
				return bytes.TrimRight(line, "\r\n"), nil
			}

			return nil, err
		}
	}
}

func (c *Conn) read(r io.Reader) {
	br := bufio.NewReaderSize(r, 64<<10)

	for {
		line, err := readLine(br)
		if err != nil {
			if errors.Is(err, ErrLineTooLong) {
				c.fail(err)
			} else {
				c.fail(ErrClosed)
			}

			return
		}

		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var m message
		if err := json.Unmarshal(line, &m); err != nil || m.JSONRPC != "2.0" {
			c.fail(fmt.Errorf("%w: a line that is not a JSON-RPC 2.0 message", ErrProtocol))
			return
		}

		if err := c.dispatch(&m); err != nil {
			c.fail(err)
			return
		}
	}
}

func (c *Conn) dispatch(m *message) error {
	switch {
	case m.Method != "" && len(m.ID) == 0:
		c.h.Notify(m.Method, m.Params)
	case m.Method != "":
		id, params, method := m.ID, m.Params, m.Method

		go c.answer(id, method, params)
	case len(m.ID) > 0:
		var id int64
		if err := json.Unmarshal(m.ID, &id); err != nil {
			return errUnknownReply
		}

		c.mu.Lock()
		ch, ok := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()

		if !ok {
			return errUnknownReply
		}

		ch <- m
	default:
		return fmt.Errorf("%w: a message with no id and no method", ErrProtocol)
	}

	return nil
}

// answer runs a request of the agent and sends the response.
func (c *Conn) answer(id json.RawMessage, method string, params json.RawMessage) {
	result, rpcErr := c.h.Request(c.ctx, method, params)

	resp := message{JSONRPC: "2.0", ID: id}

	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		raw, err := json.Marshal(result)
		if err != nil {
			resp.Error = &RPCError{Code: codeInternal, Message: "the answer could not be encoded"}
		} else {
			resp.Result = raw
		}
	}

	_ = c.write(&resp)
}

func (c *Conn) write(m *message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()

	if closed {
		return ErrClosed
	}

	c.wm.Lock()
	defer c.wm.Unlock()

	if _, err := c.w.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("%w: %w", ErrClosed, err)
	}

	return nil
}

// Call sends a request and waits for its response, for the context to end or for the connection
// to close. result may be nil.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	id := c.next.Add(1)
	ch := make(chan *message, 1)

	c.mu.Lock()
	if c.closed {
		err := c.err
		c.mu.Unlock()

		return err
	}

	c.pending[id] = ch
	c.mu.Unlock()

	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}

	idRaw := json.RawMessage(strconv.FormatInt(id, 10))

	if err := c.write(&message{JSONRPC: "2.0", ID: idRaw, Method: method, Params: raw}); err != nil {
		c.forget(id)
		return err
	}

	select {
	case m, ok := <-ch:
		if !ok {
			return c.Err()
		}

		if m.Error != nil {
			return m.Error
		}

		if result != nil && len(m.Result) > 0 {
			if err := json.Unmarshal(m.Result, result); err != nil {
				return fmt.Errorf("%w: the result of %s", ErrProtocol, method)
			}
		}

		return nil
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	}
}

func (c *Conn) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}

	return c.write(&message{JSONRPC: "2.0", Method: method, Params: raw})
}
