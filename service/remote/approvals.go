package remote

import (
	"context"
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

// ApprovalList implements service.Service.
func (c *Client) ApprovalList(
	ctx context.Context,
	req service.ApprovalListRequest,
) (*service.ApprovalListResult, error) {
	q := url.Values{}
	setIf(q, "state", req.State)
	setIf(q, "project_id", req.ProjectID)

	return get[service.ApprovalListResult](ctx, c, "/approvals", pageQuery(q, req.Page))
}

// ApprovalGet implements service.Service.
func (c *Client) ApprovalGet(ctx context.Context, req service.ApprovalRef) (*approval.Request, error) {
	return get[approval.Request](ctx, c, "/approvals/"+url.PathEscape(req.ID), nil)
}

// ApprovalDecide implements service.Service.
func (c *Client) ApprovalDecide(ctx context.Context, req service.ApprovalDecideRequest) (*approval.Request, error) {
	body := struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason,omitempty"`
	}{req.Decision, req.Reason}

	return post[approval.Request](ctx, c, "/approvals/"+url.PathEscape(req.ID)+"/decide", body)
}

// ApprovalRuleList implements service.Service.
func (c *Client) ApprovalRuleList(
	ctx context.Context, _ service.ApprovalRuleListRequest,
) (*service.ApprovalRuleListResult, error) {
	return get[service.ApprovalRuleListResult](ctx, c, "/approval-rules", nil)
}

// ApprovalRuleSet implements service.Service.
func (c *Client) ApprovalRuleSet(
	ctx context.Context, req service.ApprovalRuleSetRequest,
) (*approval.RuleView, error) {
	return post[approval.RuleView](ctx, c, "/approval-rules", req)
}

// ApprovalRuleRemove implements service.Service.
func (c *Client) ApprovalRuleRemove(ctx context.Context, req service.ApprovalRef) (*approval.RuleView, error) {
	out := new(approval.RuleView)
	if err := c.do(ctx, http.MethodDelete, "/approval-rules/"+url.PathEscape(req.ID), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// CostSummary implements service.Service.
func (c *Client) CostSummary(ctx context.Context, _ service.CostSummaryRequest) (*service.CostSummaryResult, error) {
	return get[service.CostSummaryResult](ctx, c, "/cost", nil)
}

// CostCeilingSet implements service.Service.
func (c *Client) CostCeilingSet(
	ctx context.Context,
	req service.CostCeilingSetRequest,
) (*service.CostSummaryResult, error) {
	out := new(service.CostSummaryResult)
	if err := c.do(ctx, http.MethodPut, "/cost/ceilings", nil, req, out); err != nil {
		return nil, err
	}

	return out, nil
}
