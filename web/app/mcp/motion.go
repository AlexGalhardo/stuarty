package mcp

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

// motionTool is view with mode motion: what an external render of a range must fit (SDD-011 section 15). The tool
// list has no room for another tool (NFR-020), and none for the check of a file (MotionCheck is on HTTP and in the
// CLI; SDD-011 section 15.5). It returns no image block: the reference frames are the files in their paths.
func motionTool(
	ctx context.Context,
	svc service.Service,
	projectID, timelineID, from, until string,
) (*toolOutput, error) {
	res, err := svc.MotionBrief(ctx, service.MotionBriefRequest{
		ProjectID: projectID, TimelineID: timelineID, From: from, Until: until,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}
