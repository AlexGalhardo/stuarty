package mcp

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

// briefingTool is project_item with action briefing: with answers it writes the brief from them, without it
// lists the questions and what the brief already answers (SDD-005 section 5.12.1).
func briefingTool(
	ctx context.Context,
	svc service.Service,
	projectID string,
	kind string,
	answers map[string]string,
) (*toolOutput, error) {
	if answers == nil {
		res, err := svc.BriefingQuestions(ctx, service.BriefingQuestionsRequest{ProjectID: projectID, Kind: kind})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	res, err := svc.BriefingAnswer(
		ctx,
		service.BriefingAnswerRequest{ProjectID: projectID, Kind: kind, Answers: answers},
	)
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: briefTextOf(res)}, nil
}
