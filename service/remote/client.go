// Package remote is service.Service over the HTTP API of a running owner
// (SDD-003 section 5.2, ADR-0008 item 5): the CLI and the stdio MCP server use
// it when a server owns the workspace or STUARTY_URL is set. It encodes the
// request structs as the routes of SDD-003 section 6 and decodes the error
// envelope of section 1.4 back into *service.Error. It holds no rule.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// Doer is the part of an HTTP client the package needs. The caller builds the
// client through internal/outbound, the only place that may.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	apiBase = "/api/v1"
	// ClientHeader carries the actor label of a local client (SDD-007 section 3.7).
	ClientHeader = "Stuarty-Client"
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

type timelineKey struct{}

// withTimeline carries the timeline_id of a request to do, which sends it as the query member of that name
// (SDD-003 section 1.7); the server reads it on every route that addresses a timeline.
func withTimeline(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}

	return context.WithValue(ctx, timelineKey{}, id)
}

// do sends one request and decodes a 2xx body into out, or the error envelope.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	if id, ok := ctx.Value(timelineKey{}).(string); ok && id != "" {
		query = cloneQuery(query)
		query.Set("timeline_id", id)
	}

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

		// A write that timed out after it was sent may have been applied, so a
		// blind retry could duplicate it. A refused connection never reached the
		// server, and a read is safe to repeat.
		var ne net.Error

		var oe *net.OpError

		isDial := errors.As(err, &oe) && oe.Op == "dial"

		if method != http.MethodGet && method != http.MethodHead && errors.As(err, &ne) && ne.Timeout() && !isDial {
			return &service.Error{
				Code:    command.CodeInternal,
				Message: "the server did not answer in time; the operation may have been applied.",
				Hint:    "check with list or get before running it again",
				Details: map[string]any{"reason": "timeout", "outcome": "unknown"},
			}
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
		// The HTTP server itself answers an oversized header (431) or body (413) before the app runs.
		if status == http.StatusRequestHeaderFieldsTooLarge || status == http.StatusRequestEntityTooLarge {
			return &service.Error{
				Code: service.CodePayloadTooLarge, Message: "the request is too large for the server.",
				Hint: "send a shorter cursor, actor or body",
			}
		}

		return &service.Error{
			Code:    command.CodeInternal,
			Message: fmt.Sprintf("the server answered %d without an error envelope.", status),
		}
	}

	return &service.Error{
		Code: p.Code, Message: p.Message, Hint: service.CanonicalHint(p.Hint), Retryable: p.Retryable,
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
	ctx = withTimeline(ctx, req.TimelineID)
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
	ctx = withTimeline(ctx, req.TimelineID)
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
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "actor", req.Actor)

	return get[service.HistoryListResult](ctx, c, projectPath(req.ProjectID, "/history"), pageQuery(q, req.Page))
}

// Undo implements service.Service.
func (c *Client) Undo(ctx context.Context, req service.UndoRequest) (*service.UndoResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	return post[service.UndoResult](ctx, c, projectPath(req.ProjectID, "/undo"), dryBody{req.DryRun})
}

// Redo implements service.Service.
func (c *Client) Redo(ctx context.Context, req service.UndoRequest) (*service.UndoResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	return post[service.UndoResult](ctx, c, projectPath(req.ProjectID, "/redo"), dryBody{req.DryRun})
}

// Lint implements service.Service.
func (c *Client) Lint(ctx context.Context, req service.LintRequest) (*service.LintResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "severity", req.Severity)

	return get[service.LintResult](ctx, c, projectPath(req.ProjectID, "/lint"), pageQuery(q, req.Page))
}

// MediaImport implements service.Service.
func (c *Client) MediaImport(ctx context.Context, req service.MediaImportRequest) (*service.MediaImportResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		Files  []service.ImportFile `json:"files"`
		WaitMS int                  `json:"wait_ms,omitempty"`
		DryRun bool                 `json:"dry_run,omitempty"`
	}{req.Files, req.WaitMS, req.DryRun}

	return post[service.MediaImportResult](ctx, c, projectPath(req.ProjectID, "/media"), body)
}

// MediaList implements service.Service.
func (c *Client) MediaList(ctx context.Context, req service.MediaListRequest) (*service.MediaListResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	return get[service.MediaListResult](ctx, c, projectPath(req.ProjectID, "/media"), pageQuery(url.Values{}, req.Page))
}

// MediaInspect implements service.Service.
func (c *Client) MediaInspect(
	ctx context.Context,
	req service.MediaInspectRequest,
) (*service.MediaInspectResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "sections", strings.Join(req.Sections, ","))
	setIf(q, "min_silence", req.MinSilence)

	path := projectPath(req.ProjectID, "/media/"+url.PathEscape(req.AssetID))

	return get[service.MediaInspectResult](ctx, c, path, pageQuery(q, req.Page))
}

// MediaTrash implements service.Service.
func (c *Client) MediaTrash(ctx context.Context, req service.MediaTrashRequest) (*service.MediaStateResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	return post[service.MediaStateResult](
		ctx, c, projectPath(req.ProjectID, "/media/"+url.PathEscape(req.AssetID)+"/trash"), dryBody{req.DryRun},
	)
}

// MediaRestore implements service.Service.
func (c *Client) MediaRestore(ctx context.Context, req service.MediaTrashRequest) (*service.MediaStateResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	return post[service.MediaStateResult](
		ctx, c, projectPath(req.ProjectID, "/media/"+url.PathEscape(req.AssetID)+"/restore"), dryBody{req.DryRun},
	)
}

// JobStart implements service.Service.
func (c *Client) JobStart(ctx context.Context, req service.JobStartRequest) (*service.Job, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		Kind   string          `json:"kind"`
		Args   json.RawMessage `json:"args,omitempty"`
		WaitMS int             `json:"wait_ms,omitempty"`
		DryRun bool            `json:"dry_run,omitempty"`
	}{req.Kind, req.Args, req.WaitMS, req.DryRun}

	return jobText(post[service.Job](ctx, c, projectPath(req.ProjectID, "/jobs"), body))
}

// ExportPresetList implements service.Service.
func (c *Client) ExportPresetList(
	ctx context.Context,
	req service.ExportPresetListRequest,
) (*service.ExportPresetListResult, error) {
	q := url.Values{}
	if req.Codecs {
		q.Set("codecs", "true")
	}

	return get[service.ExportPresetListResult](ctx, c, "/export-presets", q)
}

// ExportCheck implements service.Service.
func (c *Client) ExportCheck(
	ctx context.Context,
	req service.ExportCheckRequest,
) (*service.ExportCheckResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "preset", req.Preset)
	setIf(q, "start", req.Start)
	setIf(q, "end", req.End)
	setIf(q, "codec", req.Codec)

	return get[service.ExportCheckResult](ctx, c, projectPath(req.ProjectID, "/export-check"), q)
}

// ExportBatch implements service.Service.
func (c *Client) ExportBatch(ctx context.Context, req service.ExportBatchRequest) (*service.Job, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		Items  []service.ExportBatchItem `json:"items"`
		WaitMS int                       `json:"wait_ms,omitempty"`
	}{req.Items, req.WaitMS}

	return jobText(post[service.Job](ctx, c, projectPath(req.ProjectID, "/export-batches"), body))
}

// jobText turns the HTTP form of the next call in the text of a job ("call GET /api/v1/jobs/{id}")
// back to the tool names, so the surface that relays the job names its own commands (VV-200).
func jobText(j *service.Job, err error) (*service.Job, error) {
	if j != nil {
		j.Text = service.CanonicalHint(j.Text)
	}

	return j, err
}

// JobGet implements service.Service.
func (c *Client) JobGet(ctx context.Context, req service.JobGetRequest) (*service.Job, error) {
	q := url.Values{}
	if req.WaitMS > 0 {
		q.Set("wait_ms", strconv.Itoa(req.WaitMS))
	}

	return jobText(get[service.Job](ctx, c, "/jobs/"+url.PathEscape(req.JobID), q))
}

// JobCancel implements service.Service.
func (c *Client) JobCancel(ctx context.Context, req service.JobCancelRequest) (*service.Job, error) {
	return jobText(post[service.Job](ctx, c, "/jobs/"+url.PathEscape(req.JobID)+"/cancel", struct{}{}))
}

// TemplateList implements service.Service.
func (c *Client) TemplateList(
	ctx context.Context,
	req service.TemplateListRequest,
) (*service.TemplateListResult, error) {
	return get[service.TemplateListResult](ctx, c, "/templates", pageQuery(url.Values{}, req.Page))
}

// TemplateSave implements service.Service.
func (c *Client) TemplateSave(ctx context.Context, req service.TemplateSaveRequest) (*service.TemplateResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	return post[service.TemplateResult](ctx, c, "/templates", req)
}

// TemplateInstantiate implements service.Service.
func (c *Client) TemplateInstantiate(
	ctx context.Context,
	req service.TemplateInstantiateRequest,
) (*service.TemplateInstantiateResult, error) {
	body := struct {
		Name        string                       `json:"name"`
		Fill        map[string]service.SlotValue `json:"fill,omitempty"`
		ID          string                       `json:"id,omitempty"`
		DryRun      bool                         `json:"dry_run,omitempty"`
		BrandColors []string                     `json:"brand_colors,omitempty"`
		BrandFonts  []string                     `json:"brand_fonts,omitempty"`
	}{req.Name, req.Fill, req.ID, req.DryRun, req.BrandColors, req.BrandFonts}

	return post[service.TemplateInstantiateResult](
		ctx, c, "/templates/"+url.PathEscape(req.TemplateID)+"/instantiate", body,
	)
}

// TemplateExport implements service.Service.
func (c *Client) TemplateExport(ctx context.Context, req service.TemplateRef) (*service.FileResult, error) {
	return post[service.FileResult](ctx, c, "/templates/"+url.PathEscape(req.TemplateID)+"/export", struct{}{})
}

// TemplateImport implements service.Service.
func (c *Client) TemplateImport(
	ctx context.Context,
	req service.TemplateImportRequest,
) (*service.TemplateResult, error) {
	return post[service.TemplateResult](ctx, c, "/templates/import", req)
}

// TrashEmpty implements service.Service.
func (c *Client) TrashEmpty(ctx context.Context, req service.TrashEmptyRequest) (*service.TrashEmptyResult, error) {
	return post[service.TrashEmptyResult](ctx, c, "/trash/empty", req)
}

// ToolsInstall implements service.Service.
func (c *Client) ToolsInstall(
	ctx context.Context,
	req service.ToolsInstallRequest,
) (*service.ToolsInstallResult, error) {
	return post[service.ToolsInstallResult](ctx, c, "/system/tools/install", req)
}

// ToolsStatus implements service.Service.
func (c *Client) ToolsStatus(ctx context.Context, _ service.ToolsStatusRequest) (*service.ToolsStatusResult, error) {
	return get[service.ToolsStatusResult](ctx, c, "/system/tools", url.Values{})
}

// JobList implements service.Service.
func (c *Client) JobList(ctx context.Context, req service.JobListRequest) (*service.JobListResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "state", req.State)

	return get[service.JobListResult](ctx, c, projectPath(req.ProjectID, "/jobs"), pageQuery(q, req.Page))
}

// PlanWindow implements service.Service.
func (c *Client) PlanWindow(ctx context.Context, req service.PlanWindowRequest) (*service.PlanWindowResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	q.Set("from", strconv.Itoa(req.From))
	q.Set("until", strconv.Itoa(req.Until))

	if req.Width != 0 {
		q.Set("width", strconv.Itoa(req.Width))
	}

	return get[service.PlanWindowResult](ctx, c, projectPath(req.ProjectID, "/plan"), q)
}

// ViewFrame implements service.Service; the server answers the ViewResult as
// JSON with the image in image_base64 (Accept: application/json).
func (c *Client) ViewFrame(ctx context.Context, req service.ViewFrameRequest) (*service.ViewResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "at", req.At)

	if req.MaxTokens > 0 {
		q.Set("max_tokens", strconv.Itoa(req.MaxTokens))
	}

	if req.Width > 0 {
		q.Set("width", strconv.Itoa(req.Width))
	}

	return get[service.ViewResult](ctx, c, projectPath(req.ProjectID, "/frame"), q)
}

// ViewSheet implements service.Service.
func (c *Client) ViewSheet(ctx context.Context, req service.ViewSheetRequest) (*service.ViewResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "times", strings.Join(req.Times, ","))
	setIf(q, "from", req.From)
	setIf(q, "until", req.Until)

	for k, v := range map[string]int{"count": req.Count, "cols": req.Cols, "max_tokens": req.MaxTokens} {
		if v > 0 {
			q.Set(k, strconv.Itoa(v))
		}
	}

	return get[service.ViewResult](ctx, c, projectPath(req.ProjectID, "/sheet"), q)
}

// Doctor implements service.Service. The sweep and the bundle act on the
// machine of the server and run only there.
func (c *Client) Doctor(ctx context.Context, req service.DoctorRequest) (*service.DoctorResult, error) {
	if req.GC || req.Bundle {
		return nil, &service.Error{
			Code:    command.CodeValidation,
			Message: "the sweep and the bundle run on the machine of the server.",
			Hint:    "run doctor --gc or doctor --bundle there",
		}
	}

	q := url.Values{}

	if req.Full {
		q.Set("full", "true")
	}

	if req.Verify {
		q.Set("verify", "true")
	}

	return get[service.DoctorResult](ctx, c, "/system", q)
}

// SnapPoints implements service.Service.
func (c *Client) SnapPoints(ctx context.Context, req service.SnapPointsRequest) (*service.SnapPointsResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}

	if req.Revision != 0 {
		q.Set("rev", strconv.FormatInt(req.Revision, 10))
	}

	if req.Words {
		q.Set("words", "1")
	}

	return get[service.SnapPointsResult](ctx, c, projectPath(req.ProjectID, "/snap-points"), q)
}

// FontList implements service.Service.
func (c *Client) FontList(ctx context.Context, _ service.FontListRequest) (*service.FontListResult, error) {
	return get[service.FontListResult](ctx, c, "/fonts", nil)
}

// AssetList implements service.Service.
func (c *Client) AssetList(ctx context.Context, req service.AssetListRequest) (*service.AssetListResult, error) {
	q := url.Values{}
	if req.Kind != "" {
		q.Set("kind", req.Kind)
	}

	return get[service.AssetListResult](ctx, c, "/assets", q)
}

// AuditList implements service.Service.
func (c *Client) AuditList(ctx context.Context, req service.AuditListRequest) (*service.AuditListResult, error) {
	q := url.Values{}
	setIf(q, "project_id", req.ProjectID)

	return get[service.AuditListResult](
		ctx,
		c,
		"/audit",
		pageQuery(q, service.Page{Cursor: req.Cursor, Limit: req.Limit}),
	)
}

// ExportFile implements service.Service. The file is a path in the workspace of
// the server; a client reads the bytes from GET /jobs/{job_id}/file instead.
func (*Client) ExportFile(context.Context, service.ExportFileRequest) (*service.File, error) {
	return nil, &service.Error{
		Code:    command.CodeInternal,
		Message: "stored files are not opened over HTTP.",
		Hint:    "read the file from /jobs/{job_id}/file",
	}
}

// OpenFile implements service.Service. A stored file is a path inside the
// workspace of the server; a client reads the bytes from its URL instead.
func (*Client) OpenFile(context.Context, service.FileRef) (*service.File, error) {
	return nil, &service.Error{
		Code:    command.CodeInternal,
		Message: "stored files are not opened over HTTP.",
		Hint:    "read the file from its URL, such as /projects/{id}/media/{asset_id}/proxy",
	}
}

// KeyCreate implements service.Service. The server refuses it for a key, which
// is the only credential a remote client holds; it is here so that the client
// is a complete Service.
func (c *Client) KeyCreate(ctx context.Context, req service.KeyCreateRequest) (*service.KeyCreateResult, error) {
	return post[service.KeyCreateResult](ctx, c, "/keys", req)
}

// KeyList implements service.Service.
func (c *Client) KeyList(ctx context.Context, _ service.KeyListRequest) (*service.KeyListResult, error) {
	return get[service.KeyListResult](ctx, c, "/keys", url.Values{})
}

// KeyRevoke implements service.Service.
func (c *Client) KeyRevoke(ctx context.Context, req service.KeyRevokeRequest) (*service.Key, error) {
	out := new(service.Key)
	if err := c.do(ctx, http.MethodDelete, "/keys/"+url.PathEscape(req.KeyID), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// ProjectBundleExport implements service.Service.
func (c *Client) ProjectBundleExport(
	ctx context.Context,
	req service.ProjectBundleExportRequest,
) (*service.Job, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		IncludeLog *bool `json:"include_log,omitempty"`
		WaitMS     int   `json:"wait_ms,omitempty"`
	}{req.IncludeLog, req.WaitMS}

	return post[service.Job](ctx, c, projectPath(req.ProjectID, "/bundle"), body)
}

// ProjectBundleImport implements service.Service.
func (c *Client) ProjectBundleImport(
	ctx context.Context,
	req service.ProjectBundleImportRequest,
) (*service.ProjectCreateResult, error) {
	return post[service.ProjectCreateResult](ctx, c, "/projects/import", req)
}

// FontImport implements service.Service.
func (c *Client) FontImport(ctx context.Context, req service.FontImportRequest) (*service.FontImportResult, error) {
	return post[service.FontImportResult](ctx, c, "/fonts", req)
}

// BrandKitGet implements service.Service.
func (c *Client) BrandKitGet(ctx context.Context, _ service.BrandKitGetRequest) (*service.BrandKit, error) {
	return get[service.BrandKit](ctx, c, "/brand-kit", nil)
}

// BrandKitSet implements service.Service.
func (c *Client) BrandKitSet(ctx context.Context, req service.BrandKitSetRequest) (*service.BrandKit, error) {
	out := new(service.BrandKit)
	if err := c.do(ctx, http.MethodPut, "/brand-kit", nil, req, out); err != nil {
		return nil, err
	}

	return out, nil
}

// ProjectOtioExport implements service.Service.
func (c *Client) ProjectOtioExport(
	ctx context.Context,
	req service.ProjectOtioExportRequest,
) (*service.ProjectOtioExportResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	return get[service.ProjectOtioExportResult](ctx, c, projectPath(req.ProjectID, "/otio"), url.Values{})
}

// ProjectOtioImport implements service.Service.
func (c *Client) ProjectOtioImport(
	ctx context.Context,
	req service.ProjectOtioImportRequest,
) (*service.ProjectOtioImportResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		BaseRevision int64  `json:"base_revision"`
		Path         string `json:"path"`
		DryRun       bool   `json:"dry_run,omitempty"`
	}{req.BaseRevision, req.Path, req.DryRun}

	return post[service.ProjectOtioImportResult](ctx, c, projectPath(req.ProjectID, "/otio"), body)
}

func cloneQuery(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = slices.Clone(v)
	}

	return out
}

// ConnectionList implements service.Service.
func (c *Client) ConnectionList(
	ctx context.Context, _ service.ConnectionListRequest,
) (*service.ConnectionListResult, error) {
	return get[service.ConnectionListResult](ctx, c, "/connections", nil)
}

// ConnectionGet implements service.Service.
func (c *Client) ConnectionGet(ctx context.Context, req service.ConnectionRef) (*connections.Connection, error) {
	return get[connections.Connection](ctx, c, "/connections/"+url.PathEscape(req.ID), nil)
}

// ConnectionCreate implements service.Service.
func (c *Client) ConnectionCreate(
	ctx context.Context, req service.ConnectionCreateRequest,
) (*connections.Connection, error) {
	return post[connections.Connection](ctx, c, "/connections", req)
}

// ConnectionRemove implements service.Service.
func (c *Client) ConnectionRemove(ctx context.Context, req service.ConnectionRef) (*connections.Connection, error) {
	out := new(connections.Connection)
	if err := c.do(ctx, http.MethodDelete, "/connections/"+url.PathEscape(req.ID), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// ConnectionTest implements service.Service.
func (c *Client) ConnectionTest(ctx context.Context, req service.ConnectionRef) (*service.ConnectionTestResult, error) {
	return post[service.ConnectionTestResult](ctx, c, "/connections/"+url.PathEscape(req.ID)+"/test", struct{}{})
}
