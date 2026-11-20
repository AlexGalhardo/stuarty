// Package mcp is the MCP adapter of service.Service (ADR-0015, SDD-003 section
// 4): the tools of the Prototype over stdio and over stateless Streamable HTTP.
// A tool parses its arguments against the schema of its definition, calls one
// Service method and formats the result as text plus structured content; it
// holds no rule about timelines (ADR-0008).
package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
)

// Options configure the adapter.
type Options struct {
	// Version is the server version; default the module version of the binary.
	Version string
	// Actor overrides the label of the actor of the stdio server (--actor).
	Actor string
	// Session is the id of the MCP session in the actor label; empty makes a random one.
	// A stdio server serves one client for the life of the process, so this is that client.
	Session string
	// Description is shown in serverInfo: which workspace and server the tools act on.
	Description string
	// Registry is not read since the operations left the definition of edit (SDD-003
	// section 4.8): the Service names them, through command_help. It stays so that the
	// callers that set it still compile.
	Registry *command.Registry
}

// Pending names the task that delivers each tool of the Prototype that has no
// Service method yet. A pending tool is not in tools/list: the list shows the
// tools whose operations exist in the build (SDD-003 section 4.1).
var Pending = map[string]string{}

const (
	defaultActor = "mcp"
	// maxLabelLength is the longest actor label (SDD-000 section 3).
	maxLabelLength = 32
	// listTTLMs is how long a client may cache tools/list: the list changes only
	// when the binary is upgraded (SDD-003 section 4.1).
	listTTLMs = 3_600_000
)

// New builds the MCP server over a Service.
func New(svc service.Service, o Options) (*sdk.Server, error) {
	if o.Version == "" {
		o.Version = moduleVersion()
	}

	defs, err := loadDefinitions()
	if err != nil {
		return nil, err
	}

	srv := sdk.NewServer(
		&sdk.Implementation{Name: "stuarty", Version: o.Version, Description: o.Description},
		&sdk.ServerOptions{
			Instructions: agentdocs.Instructions(),
			SetCacheable: func(_ context.Context, req sdk.Request, c *sdk.Cacheable) {
				if req.GetParams() != nil && isToolsList(req) {
					c.TTLMs, c.CacheScope = listTTLMs, "public"
				}
			},
		},
	)

	srv.AddReceivingMiddleware(inTableOrder)

	if o.Session == "" {
		o.Session = newSessionID()
	}

	a := &adapter{svc: svc, opts: o, session: o.Session}

	for _, name := range order {
		call, ok := a.handlers()[name]
		if !ok {
			continue
		}

		d := defs[name]
		srv.AddTool(d.tool, a.handle(d, call))
	}

	return srv, nil
}

// inTableOrder puts the tools of tools/list in the order of the table of SDD-003
// section 4.1; the SDK lists them by name.
func inTableOrder(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		res, err := next(ctx, method, req)
		if err != nil {
			return res, err
		}

		if list, ok := res.(*sdk.ListToolsResult); ok {
			slices.SortStableFunc(list.Tools, func(a, b *sdk.Tool) int {
				return slices.Index(order, a.Name) - slices.Index(order, b.Name)
			})
		}

		return res, nil
	}
}

func isToolsList(req sdk.Request) bool {
	_, ok := req.GetParams().(*sdk.ListToolsParams)

	return ok
}

// Handler is the Streamable HTTP endpoint (POST /mcp): stateless, plain JSON
// answers, no session header. The caller must already have authenticated the
// request and carry the Caller in its context (service.WithCaller), as the
// guard of the API does; without one a tool call is E_UNAUTHORIZED.
func Handler(svc service.Service, o Options) (http.Handler, error) {
	srv, err := New(svc, o)
	if err != nil {
		return nil, err
	}

	return jsonRPCUnknownMethod(
		sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{
			Stateless: true, JSONResponse: true,
		}),
	), nil
}

// notHandledPrefix starts the text the SDK answers to a method it does not know.
const notHandledPrefix = "JSON RPC not handled"

// idCaptureLimit bounds what is kept of a request body to find its id.
const idCaptureLimit = 1 << 20

// jsonRPCUnknownMethod turns the plain text 400 that the SDK answers to a
// method it does not know into the JSON-RPC error -32601 with the id of the
// request (VV-195); the SDK does that only for protocol versions newer than
// the ones clients speak today.
func jsonRPCUnknownMethod(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		var seen bytes.Buffer

		r.Body = &teeBody{ReadCloser: r.Body, buf: &seen}
		rw := &unknownMethodWriter{ResponseWriter: w}

		next.ServeHTTP(rw, r)

		if rw.held && !strings.HasPrefix(rw.body.String(), notHandledPrefix) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(rw.body.Bytes())

			return
		}

		if rw.held {
			var req struct {
				ID json.RawMessage `json:"id"`
			}

			_ = json.Unmarshal(seen.Bytes(), &req)

			if len(req.ID) == 0 {
				req.ID = json.RawMessage("null")
			}

			h := w.Header()
			h.Del("Content-Length")
			h.Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			out, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]any{"code": -32601, "message": strings.TrimSpace(rw.body.String())},
			})
			_, _ = w.Write(out)
		}
	})
}

// teeBody keeps the first idCaptureLimit bytes that are read.
type teeBody struct {
	io.ReadCloser

	buf *bytes.Buffer
}

func (t *teeBody) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	if room := idCaptureLimit - t.buf.Len(); room > 0 {
		t.buf.Write(p[:min(n, room)])
	}

	return n, err
}

// unknownMethodWriter holds back the answer of the SDK to an unknown method
// and passes every other answer through.
type unknownMethodWriter struct {
	http.ResponseWriter

	started bool
	held    bool
	body    bytes.Buffer
}

func (u *unknownMethodWriter) WriteHeader(status int) {
	if u.started {
		return
	}

	u.started = true

	// Every plain text 400 is held until its text says what it is.
	if status == http.StatusBadRequest && strings.HasPrefix(u.Header().Get("Content-Type"), "text/plain") {
		u.held = true
		return
	}

	u.ResponseWriter.WriteHeader(status)
}

func (u *unknownMethodWriter) Write(b []byte) (int, error) {
	if !u.started {
		u.WriteHeader(http.StatusOK)
	}

	if u.held {
		return u.body.Write(b)
	}

	return u.ResponseWriter.Write(b)
}

func (u *unknownMethodWriter) Flush() {
	if f, ok := u.ResponseWriter.(http.Flusher); ok && !u.held {
		f.Flush()
	}
}

// RunStdio serves one client on standard input and output until it closes them.
// Logs go to standard error, never to standard output (SDD-003 section 4.5 rule 2).
func RunStdio(ctx context.Context, svc service.Service, o Options) error {
	srv, err := New(svc, o)
	if err != nil {
		return err
	}

	return srv.Run(ctx, &drainTransport{inner: &sdk.StdioTransport{}})
}

// runDraining is RunStdio over any transport.
func runDraining(ctx context.Context, svc service.Service, o Options, t sdk.Transport) error {
	srv, err := New(svc, o)
	if err != nil {
		return err
	}

	return srv.Run(ctx, &drainTransport{inner: t})
}

// drainTransport keeps the connection open after the input ends until every
// request already read has been answered, so `echo <request> | stuarty mcp`
// gets its response instead of exiting while the call is still running.
type drainTransport struct {
	inner sdk.Transport
}

func (t *drainTransport) Connect(ctx context.Context) (sdk.Connection, error) {
	c, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}

	return &drainConn{Connection: c, idle: make(chan struct{})}, nil
}

type drainConn struct {
	sdk.Connection

	mu      sync.Mutex
	pending map[string]struct{}
	idle    chan struct{} // closed-and-replaced whenever pending becomes empty
}

func (c *drainConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	msg, err := c.Connection.Read(ctx)
	if err != nil {
		c.waitIdle(ctx)

		return nil, err
	}

	if req, ok := msg.(*jsonrpc.Request); ok && req.IsCall() {
		c.mu.Lock()
		if c.pending == nil {
			c.pending = map[string]struct{}{}
		}
		c.pending[fmt.Sprint(req.ID.Raw())] = struct{}{}
		c.mu.Unlock()
	}

	return msg, nil
}

func (c *drainConn) Write(ctx context.Context, msg jsonrpc.Message) error {
	err := c.Connection.Write(ctx, msg)

	if resp, ok := msg.(*jsonrpc.Response); ok {
		c.mu.Lock()
		delete(c.pending, fmt.Sprint(resp.ID.Raw()))
		if len(c.pending) == 0 {
			close(c.idle)
			c.idle = make(chan struct{})
		}
		c.mu.Unlock()
	}

	return err
}

func (c *drainConn) waitIdle(ctx context.Context) {
	for {
		c.mu.Lock()
		if len(c.pending) == 0 {
			c.mu.Unlock()

			return
		}
		idle := c.idle
		c.mu.Unlock()

		select {
		case <-idle:
		case <-ctx.Done():
			return
		}
	}
}

func moduleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}

	return "devel"
}

// adapter holds what every tool shares.
type adapter struct {
	svc  service.Service
	opts Options
	// session tells this server from another with the same client name (VV-026).
	session string
}

// handle wraps a tool: validate the arguments against the schema, attach the
// Caller to the context, call, and build the result or the error result.
func (a *adapter) handle(d *definition, call callFunc) sdk.ToolHandler {
	bulk := a.bulkFields()

	return func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		caller, err := a.caller(ctx, req)
		if err != nil {
			return errorResult(err), nil
		}

		args, err := parseArguments(d, req.Params.Arguments)
		if err != nil {
			return errorResult(err), nil
		}

		out, err := call(service.WithCaller(ctx, caller), a.svc, args)
		if err != nil {
			return errorResult(err), nil
		}

		return successResult(out, bulk)
	}
}

// caller is the actor of a call (SDD-003 section 4.5): the Caller the HTTP guard
// put in the context, else, for stdio, local:<label> from --actor or from the
// clientInfo name the client sent.
func (a *adapter) caller(ctx context.Context, req *sdk.CallToolRequest) (service.Caller, error) {
	if c, ok := service.CallerFrom(ctx); ok {
		return c, nil
	}

	if req.GetExtra() != nil && req.GetExtra().Header != nil {
		// A request that came over HTTP and carries no Caller was not authenticated.
		return service.Caller{}, &service.Error{
			Code: service.CodeUnauthorized, Message: "the request carries no credentials.",
			Hint: "send the local token as Authorization: Bearer <token>",
		}
	}

	label := SanitizeLabel(a.opts.Actor)

	if a.opts.Actor == "" {
		name := ""
		if ci := req.ClientInfo(); ci != nil {
			name = ci.Name
		}

		label = sessionLabel(name, a.session)
	}

	return service.Caller{Actor: "local:" + label, Scopes: service.AllScopes}, nil
}

// sessionIDLength is the length of the session part of an actor label.
const sessionIDLength = 6

// crockford is the alphabet of the ids of the product (SDD-000 section 2), lower case.
const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

// newSessionID is a random id for one MCP session: a stdio server serves one
// client for the life of the process, so the id is made once per server.
func newSessionID() string {
	var raw [sessionIDLength]byte

	_, _ = rand.Read(raw[:])

	id := make([]byte, sessionIDLength)
	for i, b := range raw {
		id[i] = crockford[int(b)%len(crockford)]
	}

	return string(id)
}

// sessionLabel is the actor label of a client with no --actor: the sanitized
// client name and the id of the session, so two agents with the same client name
// are two actors and an undo never reaches the steps of another session or of an
// earlier connection (VV-026). The label stays inside [a-z0-9-]{1,32}.
func sessionLabel(clientName, session string) string {
	name := SanitizeLabel(clientName)
	if limit := maxLabelLength - 1 - len(session); len(name) > limit {
		name = name[:limit]
	}

	return name + "-" + session
}

// SanitizeLabel makes an actor label from a client name: lower-case, every
// character outside [a-z0-9-] becomes "-", cut at 32 characters; an empty
// result, or ui (reserved for the browser), becomes mcp.
func SanitizeLabel(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}

		if b.Len() == 32 {
			break
		}
	}

	label := b.String()
	if label == "" || label == "ui" {
		return defaultActor
	}

	return label
}

// asServiceError turns what a tool returned into the *service.Error it reports.
func asServiceError(err error) *service.Error {
	if se, ok := errors.AsType[*service.Error](err); ok {
		return se
	}

	if ce, ok := errors.AsType[*command.Error](err); ok {
		return &service.Error{
			Code: ce.Code, Message: ce.Message, Hint: ce.Hint, Retryable: ce.Retryable,
			OpIndex: ce.OpIndex, Path: ce.Path, Details: ce.Details,
		}
	}

	return &service.Error{
		Code:    command.CodeInternal,
		Message: "the operation failed because of an internal error.",
		Hint:    "Report this as a bug with the operation that was sent.",
	}
}

// rawOrEmpty is the arguments of a call, an empty object when there are none.
func rawOrEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}

	return raw
}
