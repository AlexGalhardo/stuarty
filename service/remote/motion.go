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
