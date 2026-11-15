// Package mcp is the MCP adapter of service.Service (ADR-0015, SDD-003 section
// 4): the tools of the Prototype over stdio and over stateless Streamable HTTP.
// A tool parses its arguments against the schema of its definition, calls one
// Service method and formats the result as text plus structured content; it
// holds no rule about timelines (ADR-0008).
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// Options configure the adapter.
type Options struct {
	// Version is the server version; default the module version of the binary.
	Version string
	// Actor overrides the label of the actor of the stdio server (--actor).
	Actor string
	// Registry is the command registry of the build, which decides the operations
	// edit offers; default command.Default().
	Registry *command.Registry
}

// Pending names the task that delivers each tool of the Prototype that has no
// Service method yet. A pending tool is not in tools/list: the list shows the
// tools whose operations exist in the build (SDD-003 section 4.1).
var Pending = map[string]string{}

const (
	defaultActor = "mcp"
	// listTTLMs is how long a client may cache tools/list: the list changes only
	// when the binary is upgraded (SDD-003 section 4.1).
	listTTLMs = 3_600_000
)

// New builds the MCP server over a Service.
func New(svc service.Service, o Options) (*sdk.Server, error) {
	if o.Registry == nil {
		o.Registry = command.Default()
	}

	if o.Version == "" {
		o.Version = moduleVersion()
	}

	defs, err := loadDefinitions(o.Registry)
	if err != nil {
		return nil, err
	}

	srv := sdk.NewServer(&sdk.Implementation{Name: "vukem-video", Version: o.Version}, &sdk.ServerOptions{
		Instructions: strings.TrimSpace(instructionsText),
		SetCacheable: func(_ context.Context, req sdk.Request, c *sdk.Cacheable) {
			if req.GetParams() != nil && isToolsList(req) {
				c.TTLMs, c.CacheScope = listTTLMs, "public"
			}
		},
	})

	srv.AddReceivingMiddleware(inTableOrder)

	a := &adapter{svc: svc, opts: o}

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

	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
	}), nil
}

// RunStdio serves one client on standard input and output until it closes them.
// Logs go to standard error, never to standard output (SDD-003 section 4.5 rule 2).
func RunStdio(ctx context.Context, svc service.Service, o Options) error {
	srv, err := New(svc, o)
	if err != nil {
		return err
	}

	return srv.Run(ctx, &sdk.StdioTransport{})
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

	label := a.opts.Actor
	if label == "" {
		if ci := req.ClientInfo(); ci != nil {
			label = ci.Name
		}
	}

	return service.Caller{Actor: "local:" + SanitizeLabel(label), Scopes: service.AllScopes}, nil
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
