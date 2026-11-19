package service

import (
	"context"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// BriefingService is the guided briefing of a production (SDD-005 section 5.12.1, RF-301): the questions, and
// the answers that become the brief of the project. The questions and the rules are in core/project; a person
// at a form, the CLI, the HTTP API and an agent in a chat all use the same ones.
type BriefingService interface {
	BriefingQuestions(ctx context.Context, req BriefingQuestionsRequest) (*BriefingQuestionsResult, error)
	BriefingAnswer(ctx context.Context, req BriefingAnswerRequest) (*BriefResult, error)
}

var _ BriefingService = (*svc)(nil)

// BriefingQuestionsRequest asks for the questions. With a project id the result also carries what the brief of
// that project already answers.
type BriefingQuestionsRequest struct {
	ProjectID string `json:"project_id,omitempty"`
}

// BriefingQuestion is one question of the briefing.
type BriefingQuestion struct {
	ID       string   `json:"id"`
	Heading  string   `json:"heading"`
	Text     string   `json:"text"`
	Kind     string   `json:"kind"` // text, choice, seconds or date
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	MaxChars int      `json:"max_chars,omitempty"`
	Min      int      `json:"min,omitempty"`
	Max      int      `json:"max,omitempty"`
}

// BriefingAnswer is an answer a brief holds.
type BriefingAnswer struct {
	ID    string `json:"id"`
	Value string `json:"value" untrusted:"true"`
}

// BriefingQuestionsResult is the questions and, for a project, the state of its brief.
type BriefingQuestionsResult struct {
	Envelope
	ProjectID string             `json:"project_id,omitempty"`
	Questions []BriefingQuestion `json:"questions"`
	// HasBrief is true when the project has a brief that is not empty. Guided is true when that brief was
	// written by the briefing, and Answers then holds its answers; a brief written by hand is Free and is
	// replaced only with replace true.
	HasBrief bool             `json:"has_brief"`
	Guided   bool             `json:"guided"`
	Answers  []BriefingAnswer `json:"answers,omitempty"`
	Text     string           `json:"text"`
}

// BriefingAnswerRequest answers the briefing of a project: the answers replace the brief.
type BriefingAnswerRequest struct {
	ProjectID string            `json:"project_id"`
	Answers   map[string]string `json:"answers"`           // question id to answer; see BriefingQuestions
	Replace   bool              `json:"replace,omitempty"` // allow replacing a brief written by hand
	DryRun    bool              `json:"dry_run,omitempty"`
}

// BriefingQuestions returns the questions of the briefing, and what the brief of a project answers so far.
func (s *svc) BriefingQuestions(ctx context.Context, req BriefingQuestionsRequest) (*BriefingQuestionsResult, error) {
	c, err := authorize(ctx, "BriefingQuestions", req.ProjectID)
	if err != nil {
		return nil, err
	}

	res := &BriefingQuestionsResult{UntrustedNotice: UntrustedNotice, ProjectID: req.ProjectID}

	for _, q := range project.BriefingQuestions() {
		res.Questions = append(res.Questions, BriefingQuestion{
			ID: q.ID, Heading: q.Heading, Text: q.Text, Kind: string(q.Kind), Required: q.Required,
			Options: q.Options, MaxChars: q.MaxChars, Min: q.Min, Max: q.Max,
		})
	}

	var sb strings.Builder

	sb.WriteString(textview.Note + "\n")

	for _, q := range res.Questions {
		sb.WriteString(questionLine(q) + "\n")
	}

	if req.ProjectID != "" {
		head, err := s.loadProduction(ctx, s.store.Productions(), c, req.ProjectID)
		if err != nil {
			return nil, err
		}

		rows, err := s.store.Productions().Items(ctx, head.ID)
		if err != nil {
			return nil, asError(err)
		}

		if b := briefOf(rows); b != nil && strings.TrimSpace(b.Body) != "" {
			res.HasBrief = true

			if answers, ok := project.ParseBriefing(b.Body); ok {
				res.Guided = true

				for _, q := range project.BriefingQuestions() {
					if v, found := answers[q.ID]; found {
						res.Answers = append(res.Answers, BriefingAnswer{ID: q.ID, Value: v})
					}
				}
			}

			sb.WriteString("The project has a brief. " + map[bool]string{
				true:  "It was written by the briefing; answer again to change it.",
				false: "It was written by hand; answer with replace true to replace it, or clear it first.",
			}[res.Guided] + "\n" + untrusted.Block(
				untrusted.SourceProjectText, untrusted.Attrs{"project": head.ID}, b.Body, 0))
		}
	}

	res.Text = strings.TrimRight(sb.String(), "\n")

	return res, nil
}

func questionLine(q BriefingQuestion) string {
	need := "optional"
	if q.Required {
		need = "required"
	}

	line := q.ID + " (" + q.Kind + ", " + need + "): " + q.Text
	if len(q.Options) > 0 {
		line += " one of " + strings.Join(q.Options, ", ")
	}

	return line
}

// BriefingAnswer checks the answers and writes them as the brief of the project (the write of BriefSet, so the
// audit row, the dry run and the item rules are the same). A brief written by hand is not replaced unless
// replace is true (project.CheckBriefingReplace, checked in the same transaction as the write), so a briefing
// never destroys text a person wrote. It needs the edit scope only.
func (s *svc) BriefingAnswer(ctx context.Context, req BriefingAnswerRequest) (*BriefResult, error) {
	c, err := authorize(ctx, "BriefingAnswer", req.ProjectID)
	if err != nil {
		return nil, err
	}

	answers, v := project.CheckAnswers(req.Answers)
	if v != nil {
		return nil, violation(v)
	}

	keep := func(current string) *project.RuleError { return project.CheckBriefingReplace(current, req.Replace) }

	return s.writeBrief(ctx, c, BriefSetRequest{
		ProjectID: req.ProjectID, Text: project.BriefingMarkdown(answers), DryRun: req.DryRun,
	}, keep)
}
