package mcp

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

// approvalRead is job_get with approval_id (one request) or approvals (the queue, by state). It only reads: an
// agent asks for a decision with job_start and never decides, writes a rule or sets a ceiling (RF-136, SDD-015
// section 10). The service scopes what it returns to the caller's projects.
func approvalRead(ctx context.Context, svc service.Service, id, state, projectID string) (*toolOutput, error) {
	if id != "" {
		res, err := svc.ApprovalGet(ctx, service.ApprovalRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Line()}, nil
	}

	res, err := svc.ApprovalList(ctx, service.ApprovalListRequest{State: state, ProjectID: projectID})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}
