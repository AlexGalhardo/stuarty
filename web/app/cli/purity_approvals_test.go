package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

func (s stub) ApprovalList(context.Context, service.ApprovalListRequest) (*service.ApprovalListResult, error) {
	return reply[service.ApprovalListResult](s.err)
}

func (s stub) ApprovalGet(context.Context, service.ApprovalRef) (*approval.Request, error) {
	return reply[approval.Request](s.err)
}

func (s stub) ApprovalDecide(context.Context, service.ApprovalDecideRequest) (*approval.Request, error) {
	return reply[approval.Request](s.err)
}

func (s stub) ApprovalRuleList(
	context.Context,
	service.ApprovalRuleListRequest,
) (*service.ApprovalRuleListResult, error) {
	return reply[service.ApprovalRuleListResult](s.err)
}

func (s stub) ApprovalRuleSet(context.Context, service.ApprovalRuleSetRequest) (*approval.RuleView, error) {
	return reply[approval.RuleView](s.err)
}

func (s stub) ApprovalRuleRemove(context.Context, service.ApprovalRef) (*approval.RuleView, error) {
	return reply[approval.RuleView](s.err)
}

func (s stub) CostSummary(context.Context, service.CostSummaryRequest) (*service.CostSummaryResult, error) {
	return reply[service.CostSummaryResult](s.err)
}

func (s stub) CostCeilingSet(context.Context, service.CostCeilingSetRequest) (*service.CostSummaryResult, error) {
	return reply[service.CostSummaryResult](s.err)
}
