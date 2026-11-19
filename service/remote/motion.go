package remote

import (
	"context"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service"
)

// MotionBrief implements service.Service.
func (c *Client) MotionBrief(ctx context.Context, req service.MotionBriefRequest) (*service.MotionBriefResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "from", req.From)
	setIf(q, "until", req.Until)
	setIf(q, "track_id", req.TrackID)

	return get[service.MotionBriefResult](ctx, c, projectPath(req.ProjectID, "/motion/brief"), q)
}

// MotionReplace implements service.Service.
func (c *Client) MotionReplace(
	ctx context.Context,
	req service.MotionReplaceRequest,
) (*service.MotionReplaceResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	body := struct {
		AssetID   string `json:"asset_id"`
		RenderKey string `json:"render_key,omitempty"`
		DryRun    bool   `json:"dry_run,omitempty"`
	}{req.AssetID, req.RenderKey, req.DryRun}

	return post[service.MotionReplaceResult](ctx, c, projectPath(req.ProjectID, "/motion/replace"), body)
}

// MotionCheck implements service.Service.
func (c *Client) MotionCheck(ctx context.Context, req service.MotionCheckRequest) (*service.MotionCheckResult, error) {
	ctx = withTimeline(ctx, req.TimelineID)
	q := url.Values{}
	setIf(q, "asset_id", req.AssetID)
	setIf(q, "from", req.From)
	setIf(q, "until", req.Until)
	setIf(q, "track_id", req.TrackID)

	return get[service.MotionCheckResult](ctx, c, projectPath(req.ProjectID, "/motion/check"), q)
}
