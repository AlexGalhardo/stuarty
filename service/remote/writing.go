package remote

import (
	"context"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

func textPath(projectID, itemID, suffix string) string {
	return projectPath(projectID, "/texts/"+url.PathEscape(itemID)+suffix)
}

func setInt(q url.Values, key string, n int) {
	if n != 0 {
		q.Set(key, strconv.Itoa(n))
	}
}

// TextList implements service.Service.
func (c *Client) TextList(ctx context.Context, req service.TextListRequest) (*service.TextListResult, error) {
	q := url.Values{}
	setIf(q, "project_id", req.ProjectID)
	setIf(q, "text_kind", req.TextKind)
	setIf(q, "state", req.State)
	setIf(q, "cursor", req.Cursor)
	setInt(q, "limit", req.Limit)

	return get[service.TextListResult](ctx, c, "/texts", q)
}

// TextGet implements service.Service.
func (c *Client) TextGet(ctx context.Context, req service.TextGetRequest) (*service.TextGetResult, error) {
	q := url.Values{}
	setInt(q, "revision", req.Revision)

	return get[service.TextGetResult](ctx, c, textPath(req.ProjectID, req.ItemID, ""), q)
}

// TextWrite implements service.Service.
func (c *Client) TextWrite(ctx context.Context, req service.TextWriteRequest) (*service.TextWriteResult, error) {
	body := struct {
		Title        string `json:"title,omitempty"`
		TextKind     string `json:"text_kind,omitempty"`
		Text         string `json:"text"`
		BaseRevision *int   `json:"base_revision,omitempty"`
		DryRun       bool   `json:"dry_run,omitempty"`
	}{req.Title, req.TextKind, req.Text, req.BaseRevision, req.DryRun}

	if req.ItemID == "" {
		return post[service.TextWriteResult](ctx, c, projectPath(req.ProjectID, "/texts"), body)
	}

	return put[service.TextWriteResult](ctx, c, textPath(req.ProjectID, req.ItemID, ""), body)
}

// TextRevisions implements service.Service.
func (c *Client) TextRevisions(
	ctx context.Context, req service.TextRevisionsRequest,
) (*service.TextRevisionsResult, error) {
	q := url.Values{}
	setInt(q, "before", req.Before)
	setInt(q, "limit", req.Limit)
	setInt(q, "from", req.From)
	setInt(q, "to", req.To)

	return get[service.TextRevisionsResult](ctx, c, textPath(req.ProjectID, req.ItemID, "/revisions"), q)
}

// TextRevert implements service.Service.
func (c *Client) TextRevert(ctx context.Context, req service.TextRevertRequest) (*service.TextWriteResult, error) {
	body := struct {
		Revision int  `json:"revision"`
		DryRun   bool `json:"dry_run,omitempty"`
	}{req.Revision, req.DryRun}

	return post[service.TextWriteResult](ctx, c, textPath(req.ProjectID, req.ItemID, "/revert"), body)
}

// TextDraft implements service.Service.
func (c *Client) TextDraft(ctx context.Context, req service.TextDraftRequest) (*service.Job, error) {
	body := struct {
		ConnectionID string   `json:"connection_id"`
		Model        string   `json:"model"`
		Instruction  string   `json:"instruction"`
		Network      string   `json:"network,omitempty"`
		MaxTokens    int      `json:"max_tokens,omitempty"`
		Temperature  *float64 `json:"temperature,omitempty"`
		WaitMS       int      `json:"wait_ms,omitempty"`
		DryRun       bool     `json:"dry_run,omitempty"`
	}{req.ConnectionID, req.Model, req.Instruction, req.Network, req.MaxTokens, req.Temperature, req.WaitMS, req.DryRun}

	return jobText(post[service.Job](ctx, c, textPath(req.ProjectID, req.ItemID, "/draft"), body))
}

// CaptionValidate implements service.Service.
func (c *Client) CaptionValidate(
	ctx context.Context, req service.CaptionValidateRequest,
) (*service.CaptionValidateResult, error) {
	return post[service.CaptionValidateResult](ctx, c, "/captions/validate", req)
}

// ScriptEstimate implements service.Service.
func (c *Client) ScriptEstimate(
	ctx context.Context, req service.ScriptEstimateRequest,
) (*service.ScriptEstimateResult, error) {
	return post[service.ScriptEstimateResult](ctx, c, "/scripts/estimate", req)
}
