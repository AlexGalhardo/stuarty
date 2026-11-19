package remote

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

// ProjectDescribe implements service.Service.
func (c *Client) ProjectDescribe(
	ctx context.Context,
	req service.ProjectDescribeRequest,
) (*service.ProjectDescribeResult, error) {
	q := url.Values{}
	if req.MaxTokens > 0 {
		q.Set("max_tokens", strconv.Itoa(req.MaxTokens))
	}

	return get[service.ProjectDescribeResult](ctx, c, projectPath(req.ProjectID, "/describe"), q)
}

// ProjectDuplicate implements service.Service.
func (c *Client) ProjectDuplicate(
	ctx context.Context,
	req service.ProjectDuplicateRequest,
) (*service.ProjectCreateResult, error) {
	body := struct {
		Name   string `json:"name,omitempty"`
		DryRun bool   `json:"dry_run,omitempty"`
	}{req.Name, req.DryRun}

	return post[service.ProjectCreateResult](ctx, c, projectPath(req.ProjectID, "/duplicate"), body)
}

// ProjectItemAdd implements service.Service.
func (c *Client) ProjectItemAdd(
	ctx context.Context,
	req service.ProjectItemAddRequest,
) (*service.ProjectItemResult, error) {
	body := struct {
		Kind     string `json:"kind"`
		Title    string `json:"title"`
		Position *int   `json:"position,omitempty"`
		Text     string `json:"text,omitempty"`
		TextKind string `json:"text_kind,omitempty"`
		Preset   string `json:"preset,omitempty"`
		FPS      string `json:"fps,omitempty"`
		JobID    string `json:"job_id,omitempty"`
		DryRun   bool   `json:"dry_run,omitempty"`
	}{req.Kind, req.Title, req.Position, req.Text, req.TextKind, req.Preset, req.FPS, req.JobID, req.DryRun}

	return post[service.ProjectItemResult](ctx, c, projectPath(req.ProjectID, "/items"), body)
}

func itemPath(projectID, itemID, suffix string) string {
	return projectPath(projectID, "/items/"+url.PathEscape(itemID)+suffix)
}

// ProjectItemMove implements service.Service.
func (c *Client) ProjectItemMove(
	ctx context.Context,
	req service.ProjectItemMoveRequest,
) (*service.ProjectItemResult, error) {
	body := struct {
		Position int  `json:"position"`
		DryRun   bool `json:"dry_run,omitempty"`
	}{req.Position, req.DryRun}

	return post[service.ProjectItemResult](ctx, c, itemPath(req.ProjectID, req.ItemID, "/move"), body)
}

// ProjectItemRename implements service.Service.
func (c *Client) ProjectItemRename(
	ctx context.Context,
	req service.ProjectItemRenameRequest,
) (*service.ProjectItemResult, error) {
	body := struct {
		Title  string `json:"title"`
		DryRun bool   `json:"dry_run,omitempty"`
	}{req.Title, req.DryRun}

	return post[service.ProjectItemResult](ctx, c, itemPath(req.ProjectID, req.ItemID, "/rename"), body)
}

// ProjectItemTrash implements service.Service.
func (c *Client) ProjectItemTrash(
	ctx context.Context,
	req service.ProjectItemTrashRequest,
) (*service.ProjectItemResult, error) {
	return post[service.ProjectItemResult](ctx, c, itemPath(req.ProjectID, req.ItemID, "/trash"), dryBody{req.DryRun})
}

// ProjectItemRestore implements service.Service.
func (c *Client) ProjectItemRestore(
	ctx context.Context,
	req service.ProjectItemTrashRequest,
) (*service.ProjectItemResult, error) {
	return post[service.ProjectItemResult](ctx, c, itemPath(req.ProjectID, req.ItemID, "/restore"), dryBody{req.DryRun})
}

// BriefGet implements service.Service.
func (c *Client) BriefGet(ctx context.Context, req service.BriefGetRequest) (*service.BriefResult, error) {
	return get[service.BriefResult](ctx, c, projectPath(req.ProjectID, "/brief"), nil)
}

// BriefSet implements service.Service.
func (c *Client) BriefSet(ctx context.Context, req service.BriefSetRequest) (*service.BriefResult, error) {
	body := struct {
		Text   string `json:"text"`
		DryRun bool   `json:"dry_run,omitempty"`
	}{req.Text, req.DryRun}

	out := new(service.BriefResult)
	if err := c.do(ctx, http.MethodPut, projectPath(req.ProjectID, "/brief"), nil, body, out); err != nil {
		return nil, err
	}

	return out, nil
}
