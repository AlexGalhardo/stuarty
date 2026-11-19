package mcp

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/service"
)

// approval lists the approval queue, or reads one request. It only reads: an agent submits through
// job_start and never decides, writes a rule or sets a ceiling (RF-136, SDD-015 section 10).
func approval(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.ID != "" {
		res, gerr := svc.ApprovalGet(ctx, service.ApprovalRef{ID: in.ID})
		if gerr != nil {
			return nil, gerr
		}

		return &toolOutput{value: res, text: res.Line()}, nil
	}

	res, err := svc.ApprovalList(ctx, service.ApprovalListRequest{State: in.State})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}
