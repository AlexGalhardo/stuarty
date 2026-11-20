package remote

import (
	"context"
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service"
)

// BriefingQuestions implements service.Service.
func (c *Client) BriefingQuestions(
	ctx context.Context,
	req service.BriefingQuestionsRequest,
) (*service.BriefingQuestionsResult, error) {
	q := url.Values{}
	setIf(q, "project_id", req.ProjectID)
	setIf(q, "kind", req.Kind)

	return get[service.BriefingQuestionsResult](ctx, c, "/briefing/questions", q)
}

// BriefingAnswer implements service.Service.
func (c *Client) BriefingAnswer(ctx context.Context, req service.BriefingAnswerRequest) (*service.BriefResult, error) {
	body := struct {
		Kind    string            `json:"kind,omitempty"`
		Answers map[string]string `json:"answers"`
		Replace bool              `json:"replace,omitempty"`
		DryRun  bool              `json:"dry_run,omitempty"`
	}{req.Kind, req.Answers, req.Replace, req.DryRun}

	out := new(service.BriefResult)
	if err := c.do(ctx, http.MethodPut, projectPath(req.ProjectID, "/briefing"), nil, body, out); err != nil {
		return nil, err
	}

	return out, nil
}
