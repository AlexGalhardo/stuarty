package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"

	"github.com/AlexGalhardo/vukem/web"
)

type approvalListIn struct {
	State     string `json:"state,omitempty"      doc:"pending (the default), approved, rejected, expired or all."`
	ProjectID string `json:"project_id,omitempty" doc:"Only the requests of one project."`
	pageIn
}

type approvalRefIn struct {
	ID string `json:"id" validate:"required" doc:"An approval request id, apr_ and 10 characters; or a rule id, rul_ and 10 characters."`
}

type approvalDecideIn struct {
	ID       string `json:"id"               validate:"required" doc:"An approval request id, apr_ and 10 characters."`
	Decision string `json:"decision"         validate:"required" doc:"approve or reject."`
	Reason   string `json:"reason,omitempty"                     doc:"Why a request is rejected, at most 500 characters of plain text."`
}

type approvalRuleSetIn struct {
	ID            string `json:"id,omitempty"              doc:"The rule to replace; leave out to create one."`
	Kind          string `json:"kind"                      doc:"The kind of action the rule covers, for example publish.upload (publish now) or publish.scheduled."                                                validate:"required"`
	ConnectionID  string `json:"connection_id,omitempty"   doc:"Only this destination; leave out for any."`
	ProjectID     string `json:"project_id,omitempty"      doc:"Only this project; leave out for any."`
	Currency      string `json:"currency,omitempty"        doc:"Three upper-case letters; needed with a limit."`
	PerItemMicros *int64 `json:"per_item_micros,omitempty" doc:"The most one action may cost for the rule to approve it, in millionths of the currency unit. A rule with no limit never approves a costly action."`
	PerDayMicros  *int64 `json:"per_day_micros,omitempty"  doc:"The most the rule may approve in one UTC day, in millionths of the currency unit."`
	Enabled       *bool  `json:"enabled,omitempty"         doc:"false keeps the rule and stops it; the default is true."`
}

type costCeilingIn struct {
	ProjectID     string `json:"project_id,omitempty"      doc:"The project the ceiling is for; leave out for the whole workspace."`
	Currency      string `json:"currency"                  doc:"Three upper-case letters, for example USD."                                                                                 validate:"required"`
	PerItemMicros *int64 `json:"per_item_micros,omitempty" doc:"The most one action may cost, in millionths of the currency unit."`
	PerDayMicros  *int64 `json:"per_day_micros,omitempty"  doc:"The most that may be spent in one UTC day, in millionths of the currency unit. With neither limit, the ceiling is removed."`
}

func (h *apiHandler) approvalsList(c *web.Ctx, in approvalListIn) (service.ApprovalListResult, error) {
	res, err := h.Service.ApprovalList(c.Context(), service.ApprovalListRequest{
		State: in.State, ProjectID: in.ProjectID, Page: page(in.pageIn),
	})

	return deref(res), err
}

func (h *apiHandler) approvalsGet(c *web.Ctx, in approvalRefIn) (approval.Request, error) {
	res, err := h.Service.ApprovalGet(c.Context(), service.ApprovalRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) approvalsDecide(c *web.Ctx, in approvalDecideIn) (approval.Request, error) {
	res, err := h.Service.ApprovalDecide(c.Context(), service.ApprovalDecideRequest{
		ID: in.ID, Decision: in.Decision, Reason: in.Reason,
	})

	return deref(res), err
}

func (h *apiHandler) approvalRulesList(c *web.Ctx, _ noIn) (service.ApprovalRuleListResult, error) {
	res, err := h.Service.ApprovalRuleList(c.Context(), service.ApprovalRuleListRequest{})

	return deref(res), err
}

func (h *apiHandler) approvalRulesSet(c *web.Ctx, in approvalRuleSetIn) (approval.RuleView, error) {
	res, err := h.Service.ApprovalRuleSet(c.Context(), service.ApprovalRuleSetRequest(in))

	return deref(res), err
}

func (h *apiHandler) approvalRulesRemove(c *web.Ctx, in approvalRefIn) (approval.RuleView, error) {
	res, err := h.Service.ApprovalRuleRemove(c.Context(), service.ApprovalRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) costGet(c *web.Ctx, _ noIn) (service.CostSummaryResult, error) {
	res, err := h.Service.CostSummary(c.Context(), service.CostSummaryRequest{})

	return deref(res), err
}

func (h *apiHandler) costCeilingSet(c *web.Ctx, in costCeilingIn) (service.CostSummaryResult, error) {
	res, err := h.Service.CostCeilingSet(c.Context(), service.CostCeilingSetRequest(in))

	return deref(res), err
}
