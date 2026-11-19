package remote

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// The library operations (SDD-018 section 14) over the HTTP API.

func libraryPath(kind, name, suffix string) string {
	p := "/library/" + kind
	if name != "" {
		p += "/" + url.PathEscape(name)
	}

	return p + suffix
}

func setBool(q url.Values, key string, v bool) {
	if v {
		q.Set(key, "true")
	}
}

func setList(q url.Values, key string, items []string) {
	if len(items) > 0 {
		q.Set(key, strings.Join(items, ","))
	}
}

func put[T any](ctx context.Context, c *Client, path string, body any) (*T, error) {
	out := new(T)
	if err := c.do(ctx, http.MethodPut, path, nil, body, out); err != nil {
		return nil, err
	}

	return out, nil
}

// MemoryList implements service.Service.
func (c *Client) MemoryList(ctx context.Context, req service.MemoryListRequest) (*service.LibraryListResult, error) {
	q := url.Values{}
	setIf(q, "scope", req.Scope)
	setList(q, "tags", req.Tags)
	setBool(q, "include_unreviewed", req.IncludeUnreviewed)
	setBool(q, "agent_view", req.AgentView)

	return get[service.LibraryListResult](ctx, c, libraryPath("memory", "", ""), q)
}

// MemoryGet implements service.Service.
func (c *Client) MemoryGet(ctx context.Context, req service.LibraryRef) (*service.MemoryGetResult, error) {
	q := url.Values{}
	setBool(q, "agent_view", req.AgentView)

	return get[service.MemoryGetResult](ctx, c, libraryPath("memory", req.Name, ""), q)
}

// MemoryWrite implements service.Service.
func (c *Client) MemoryWrite(ctx context.Context, req service.MemoryWriteRequest) (*service.MemoryWriteResult, error) {
	return post[service.MemoryWriteResult](ctx, c, libraryPath("memory", "", ""), req)
}

// MemoryRemove implements service.Service.
func (c *Client) MemoryRemove(ctx context.Context, req service.LibraryRef) (*service.LibraryRemoveResult, error) {
	out := new(service.LibraryRemoveResult)
	if err := c.do(ctx, http.MethodDelete, libraryPath("memory", req.Name, ""), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// SkillList implements service.Service.
func (c *Client) SkillList(ctx context.Context, req service.SkillListRequest) (*service.LibraryListResult, error) {
	q := url.Values{}
	setIf(q, "origin", req.Origin)
	setBool(q, "include_unreviewed", req.IncludeUnreviewed)
	setBool(q, "agent_view", req.AgentView)

	return get[service.LibraryListResult](ctx, c, libraryPath("skills", "", ""), q)
}

// SkillGet implements service.Service.
func (c *Client) SkillGet(ctx context.Context, req service.LibraryRef) (*service.SkillGetResult, error) {
	q := url.Values{}
	setBool(q, "agent_view", req.AgentView)

	return get[service.SkillGetResult](ctx, c, libraryPath("skills", req.Name, ""), q)
}

// SkillWrite implements service.Service.
func (c *Client) SkillWrite(ctx context.Context, req service.SkillWriteRequest) (*service.SkillWriteResult, error) {
	body := req
	body.Name = ""

	return put[service.SkillWriteResult](ctx, c, libraryPath("skills", req.Name, ""), body)
}

// SkillImport implements service.Service.
func (c *Client) SkillImport(ctx context.Context, req service.SkillImportRequest) (*service.SkillImportResult, error) {
	return post[service.SkillImportResult](ctx, c, "/library/skills/import", req)
}

// SkillMarkReviewed implements service.Service.
func (c *Client) SkillMarkReviewed(ctx context.Context, req service.LibraryRef) (*service.SkillReviewResult, error) {
	return post[service.SkillReviewResult](ctx, c, libraryPath("skills", req.Name, "/review"), nil)
}

// SkillLint implements service.Service.
func (c *Client) SkillLint(ctx context.Context, req service.LibraryRef) (*service.SkillLintResult, error) {
	return get[service.SkillLintResult](ctx, c, libraryPath("skills", req.Name, "/lint"), nil)
}

// SkillExport implements service.Service.
func (c *Client) SkillExport(ctx context.Context, req service.SkillExportRequest) (*service.SkillExportResult, error) {
	return post[service.SkillExportResult](ctx, c, "/library/skills/export", req)
}

// PromptList implements service.Service.
func (c *Client) PromptList(ctx context.Context, req service.PromptListRequest) (*service.LibraryListResult, error) {
	q := url.Values{}
	setList(q, "tags", req.Tags)
	setBool(q, "agent_view", req.AgentView)

	return get[service.LibraryListResult](ctx, c, libraryPath("prompts", "", ""), q)
}

// PromptGet implements service.Service.
func (c *Client) PromptGet(ctx context.Context, req service.LibraryRef) (*service.PromptGetResult, error) {
	q := url.Values{}
	setBool(q, "agent_view", req.AgentView)

	return get[service.PromptGetResult](ctx, c, libraryPath("prompts", req.Name, ""), q)
}

// PromptWrite implements service.Service.
func (c *Client) PromptWrite(ctx context.Context, req service.PromptWriteRequest) (*service.PromptWriteResult, error) {
	body := req
	body.Name = ""

	return put[service.PromptWriteResult](ctx, c, libraryPath("prompts", req.Name, ""), body)
}

// PromptRender implements service.Service.
func (c *Client) PromptRender(
	ctx context.Context,
	req service.PromptRenderRequest,
) (*service.PromptRenderResult, error) {
	body := struct {
		Values    map[string]string `json:"values,omitempty"`
		AgentView bool              `json:"agent_view,omitempty"`
	}{req.Values, req.AgentView}

	return post[service.PromptRenderResult](ctx, c, libraryPath("prompts", req.Name, "/render"), body)
}

// LibrarySearch implements service.Service.
func (c *Client) LibrarySearch(
	ctx context.Context,
	req service.LibrarySearchRequest,
) (*service.LibrarySearchResult, error) {
	q := url.Values{}
	q.Set("query", req.Query)
	setList(q, "kinds", req.Kinds)
	setIf(q, "scope", req.Scope)
	setList(q, "tags", req.Tags)
	setIf(q, "origin", req.Origin)
	setBool(q, "include_unreviewed", req.IncludeUnreviewed)
	setBool(q, "agent_view", req.AgentView)

	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	if req.Budget > 0 {
		q.Set("budget", strconv.Itoa(req.Budget))
	}

	return get[service.LibrarySearchResult](ctx, c, "/library/search", q)
}

// LibraryReindex implements service.Service.
func (c *Client) LibraryReindex(
	ctx context.Context,
	_ service.LibraryReindexRequest,
) (*service.LibraryReindexResult, error) {
	return post[service.LibraryReindexResult](ctx, c, "/library/reindex", struct{}{})
}
