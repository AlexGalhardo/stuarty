package remote

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

// HistoryRevert implements service.Service.
func (c *Client) HistoryRevert(
	ctx context.Context,
	req service.HistoryRevertRequest,
) (*service.HistoryRevertResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		Revision     int64  `json:"revision,omitempty"`
		Checkpoint   string `json:"checkpoint,omitempty"`
		BaseRevision int64  `json:"base_revision,omitempty"`
		DryRun       bool   `json:"dry_run,omitempty"`
	}{req.Revision, req.Checkpoint, req.BaseRevision, req.DryRun}

	return post[service.HistoryRevertResult](ctx, c, projectPath(req.ProjectID, "/revert"), body)
}

// HistoryDiff implements service.Service.
func (c *Client) HistoryDiff(ctx context.Context, req service.HistoryDiffRequest) (*service.HistoryDiffResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "from_checkpoint", req.FromCheckpoint)

	if req.From > 0 {
		q.Set("from", strconv.FormatInt(req.From, 10))
	}

	if req.Until > 0 {
		q.Set("until", strconv.FormatInt(req.Until, 10))
	}

	return get[service.HistoryDiffResult](ctx, c, projectPath(req.ProjectID, "/diff"), q)
}

// CheckpointCreate implements service.Service.
func (c *Client) CheckpointCreate(
	ctx context.Context,
	req service.CheckpointCreateRequest,
) (*service.Checkpoint, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		Name     string `json:"name"`
		Note     string `json:"note,omitempty"`
		Revision int64  `json:"revision,omitempty"`
	}{req.Name, req.Note, req.Revision}

	return post[service.Checkpoint](ctx, c, projectPath(req.ProjectID, "/checkpoints"), body)
}

// CheckpointList implements service.Service.
func (c *Client) CheckpointList(
	ctx context.Context,
	req service.CheckpointListRequest,
) (*service.CheckpointListResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)

	return get[service.CheckpointListResult](ctx, c, projectPath(req.ProjectID, "/checkpoints"), nil)
}

// CheckpointRename implements service.Service.
func (c *Client) CheckpointRename(
	ctx context.Context,
	req service.CheckpointRenameRequest,
) (*service.Checkpoint, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		NewName string  `json:"new_name"`
		Note    *string `json:"note,omitempty"`
	}{req.NewName, req.Note}

	out := new(service.Checkpoint)
	if err := c.do(ctx, http.MethodPatch, checkpointPath(req.ProjectID, req.Name), nil, body, out); err != nil {
		return nil, err
	}

	return out, nil
}

// CheckpointRemove implements service.Service.
func (c *Client) CheckpointRemove(
	ctx context.Context,
	req service.CheckpointRemoveRequest,
) (*service.Checkpoint, error) {
	ctx = withTimeline(ctx, req.TimelineID)

	out := new(service.Checkpoint)
	if err := c.do(ctx, http.MethodDelete, checkpointPath(req.ProjectID, req.Name), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

func checkpointPath(project, name string) string {
	return projectPath(project, "/checkpoints/"+url.PathEscape(name))
}
