package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The routes of the guided briefing (SDD-005 section 5.12.1). They carry no rule: the Service validates and
// core/project holds the questions (ADR-0008).

type briefingQuestionsIn struct {
	ProjectID string `json:"project_id,omitempty" doc:"With a project: also the answers its brief holds."`
	Kind      string `json:"kind,omitempty"       doc:"The kind of production: short (the default), commercial or long."`
}

type briefingAnswerIn struct {
	ID      string            `json:"id"                validate:"required"`
	Kind    string            `json:"kind,omitempty"                        doc:"The kind of production of the answers: short (the default), commercial or long."`
	Answers map[string]string `json:"answers"                               doc:"Question id to answer; the questions are GET /briefing/questions. The answers replace the brief."`
	Replace bool              `json:"replace,omitempty"                     doc:"Replace a brief that was written by hand."`
	DryRun  any               `json:"dry_run,omitempty"`
}

func (h *apiHandler) briefingQuestions(c *web.Ctx, in briefingQuestionsIn) (service.BriefingQuestionsResult, error) {
	res, err := h.Service.BriefingQuestions(c.Context(), service.BriefingQuestionsRequest{ProjectID: in.ProjectID, Kind: in.Kind})

	return deref(res), err
}

func (h *apiHandler) briefingAnswer(c *web.Ctx, in briefingAnswerIn) (service.BriefResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.BriefResult{}, err
	}

	res, err := h.Service.BriefingAnswer(c.Context(), service.BriefingAnswerRequest{
		ProjectID: in.ID, Kind: in.Kind, Answers: in.Answers, Replace: in.Replace, DryRun: dryRun,
	})

	return deref(res), err
}
