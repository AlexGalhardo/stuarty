// Package remote is service.Service over the HTTP API of a running owner
// (SDD-003 section 5.2, ADR-0008 item 5): the CLI and the stdio MCP server use
// it when a server owns the workspace or VUKEM_VIDEO_URL is set. It encodes the
// request structs as the routes of SDD-003 section 6 and decodes the error
// envelope of section 1.4 back into *service.Error. It holds no rule.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// Doer is the part of an HTTP client the package needs. The caller builds the
// client through internal/outbound, the only place that may.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	apiBase = "/api/v1"
	// ClientHeader carries the actor label of a local client (SDD-007 section 3.7).
	ClientHeader = "Vukem-Video-Client"
	// maxBody bounds what the client reads, so a wrong server cannot fill memory.
	maxBody = 64 << 20
)

// Client implements service.Service over HTTP.
type Client struct {
	base  string
	token string
	http  Doer
}

var _ service.Service = (*Client)(nil)

// New returns a Client for the server at base (scheme and host, no path) that
// sends token as a bearer credential. The actor label is read from the Caller
// of each call (service.WithCaller) and sent as ClientHeader.
func New(base, token string, hc Doer) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: hc}
}

// do sends one request and decodes a 2xx body into out, or the error envelope.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader io.Reader = http.NoBody

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return internal("encode the request: %v", err)
		}

		reader = bytes.NewReader(raw)
	}

	u := c.base + apiBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return internal("build the request: %v", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if caller, ok := service.CallerFrom(ctx); ok {
		req.Header.Set(ClientHeader, strings.TrimPrefix(caller.Actor, "local:"))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return &service.Error{
			Code: service.CodeWorkspaceLocked, Retryable: true,
			Message: "the server did not answer.",
			Hint:    "the owner of the workspace is not answering; retry in a few seconds",
		}
	}

	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return internal("read the answer: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeError(resp.StatusCode, raw)
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return internal("the server answered with a body that is not the expected JSON: %v", err)
	}

	return nil
}

// problem is the error envelope of SDD-003 section 1.4.
type problem struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint"`
	Retryable bool           `json:"retryable"`
	OpIndex   *int           `json:"op_index"`
	Path      string         `json:"path"`
	Details   map[string]any `json:"details"`
}

// decodeError turns an error answer into the *service.Error it carried. An
// answer that is not the envelope (a proxy page, a crash) is E_INTERNAL without
// its text.
func decodeError(status int, raw []byte) error {
	var p problem
	if json.Unmarshal(raw, &p) != nil || p.Code == "" {
		return &service.Error{
			Code:    command.CodeInternal,
			Message: fmt.Sprintf("the server answered %d without an error envelope.", status),
		}
	}

	return &service.Error{
		Code: p.Code, Message: p.Message, Hint: p.Hint, Retryable: p.Retryable,
		OpIndex: p.OpIndex, Path: p.Path, Details: p.Details,
	}
}

func internal(format string, args ...any) error {
	return &service.Error{Code: command.CodeInternal, Message: fmt.Sprintf(format, args...)}
}

func get[T any](ctx context.Context, c *Client, path string, q url.Values) (*T, error) {
	out := new(T)
	if err := c.do(ctx, http.MethodGet, path, q, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

func post[T any](ctx context.Context, c *Client, path string, body any) (*T, error) {
	out := new(T)
	if err := c.do(ctx, http.MethodPost, path, nil, body, out); err != nil {
		return nil, err
	}

	return out, nil
}

// pageQuery adds the paging members that are set.
func pageQuery(q url.Values, p service.Page) url.Values {
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}

	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	return q
}

func setIf(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

func projectPath(id, suffix string) string {
	return "/projects/" + url.PathEscape(id) + suffix
}

// ProjectList implements service.Service.
func (c *Client) ProjectList(ctx context.Context, req service.ProjectListRequest) (*service.ProjectListResult, error) {
	q := url.Values{}
	setIf(q, "state", req.State)

	return get[service.ProjectListResult](ctx, c, "/projects", pageQuery(q, req.Page))
}

// ProjectCreate implements service.Service.
func (c *Client) ProjectCreate(
	ctx context.Context,
	req service.ProjectCreateRequest,
) (*service.ProjectCreateResult, error) {
	return post[service.ProjectCreateResult](ctx, c, "/projects", req)
}

// ProjectGet implements service.Service.
func (c *Client) ProjectGet(ctx context.Context, req service.ProjectGetRequest) (*service.ProjectGetResult, error) {
	q := url.Values{}
	setIf(q, "detail", req.Detail)

	return get[service.ProjectGetResult](ctx, c, projectPath(req.ProjectID, ""), pageQuery(q, req.Page))
}

type dryBody struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// ProjectTrash implements service.Service.
func (c *Client) ProjectTrash(
	ctx context.Context,
	req service.ProjectTrashRequest,
) (*service.ProjectStateResult, error) {
	return post[service.ProjectStateResult](ctx, c, projectPath(req.ProjectID, "/trash"), dryBody{req.DryRun})
}

// ProjectRestore implements service.Service.
func (c *Client) ProjectRestore(
	ctx context.Context,
	req service.ProjectTrashRequest,
) (*service.ProjectStateResult, error) {
	return post[service.ProjectStateResult](ctx, c, projectPath(req.ProjectID, "/restore"), dryBody{req.DryRun})
}

type editBody struct {
	BaseRevision   int64               `json:"base_revision"`
	Operations     []command.Operation `json:"operations"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	DryRun         bool                `json:"dry_run,omitempty"`
}

// Edit implements service.Service.
func (c *Client) Edit(ctx context.Context, req service.EditRequest) (*service.EditResult, error) {
	return post[service.EditResult](ctx, c, projectPath(req.ProjectID, "/edit"), editBody{
		BaseRevision:   req.BaseRevision,
		Operations:     req.Operations,
		IdempotencyKey: req.IdempotencyKey,
		DryRun:         req.DryRun,
	})
}

// CommandHelp implements service.Service.
func (c *Client) CommandHelp(ctx context.Context, req service.CommandHelpRequest) (*service.CommandHelpResult, error) {
	if req.Name == "" {
		return get[service.CommandHelpResult](ctx, c, "/commands", nil)
	}

	return get[service.CommandHelpResult](ctx, c, "/commands/"+url.PathEscape(req.Name), nil)
}

// HistoryList implements service.Service.
func (c *Client) HistoryList(ctx context.Context, req service.HistoryListRequest) (*service.HistoryListResult, error) {
	q := url.Values{}
	setIf(q, "actor", req.Actor)

	return get[service.HistoryListResult](ctx, c, projectPath(req.ProjectID, "/history"), pageQuery(q, req.Page))
}

// Undo implements service.Service.
func (c *Client) Undo(ctx context.Context, req service.UndoRequest) (*service.UndoResult, error) {
	return post[service.UndoResult](ctx, c, projectPath(req.ProjectID, "/undo"), dryBody{req.DryRun})
}

// Redo implements service.Service.
func (c *Client) Redo(ctx context.Context, req service.UndoRequest) (*service.UndoResult, error) {
	return post[service.UndoResult](ctx, c, projectPath(req.ProjectID, "/redo"), dryBody{req.DryRun})
}

// Lint implements service.Service.
func (c *Client) Lint(ctx context.Context, req service.LintRequest) (*service.LintResult, error) {
	q := url.Values{}
	setIf(q, "severity", req.Severity)

	return get[service.LintResult](ctx, c, projectPath(req.ProjectID, "/lint"), pageQuery(q, req.Page))
}
