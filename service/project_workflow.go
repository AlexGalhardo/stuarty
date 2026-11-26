package service

import (
	"context"
	"errors"
	"slices"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service/library"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// effectiveKind is the kind a surface shows for a project (SDD-005 section 5.12.4).
func effectiveKind(head *ProductionHead, rows []ItemRow) string {
	return string(project.EffectiveKind(head.Kind, coreItems(rows)))
}

// workflowOf is the workflow of a project. rows are its items, its publications among them when known says they
// could be read; the other facts are read here, and one that cannot be read leaves its step unknown or its cost out.
func (s *svc) workflowOf(ctx context.Context, head *ProductionHead, rows []ItemRow, known bool) project.Workflow {
	facts := workflowFacts(head, rows, known)
	facts.Costs = s.approvalCosts(ctx, head.ID)
	facts.Skills = s.offeredSkills(ctx)
	facts.Spent = s.settledSpend(ctx, head.ID)

	return project.ComputeWorkflow(head.Kind, &facts)
}

// workflowFacts are the facts of the workflow that the items and the head of a project give, which is all a list
// needs to say where each project stands.
func workflowFacts(head *ProductionHead, rows []ItemRow, known bool) project.Facts {
	return project.Facts{
		Items:             coreItems(rows),
		Skipped:           head.Skipped,
		PublicationsKnown: known,
		TimelineAssets:    head.Timeline.Summary.Assets,
		TimelineItems:     head.Timeline.Summary.Items,
	}
}

// offeredSkills are the names of the skills the library offers, the ones a step may name: the reviewed ones. A
// server with no library offers none.
func (s *svc) offeredSkills(ctx context.Context) []string {
	if s.lib == nil {
		return nil
	}

	rows, err := s.lib.List(ctx, core.KindSkill, library.Filter{})
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(rows))

	for i := range rows {
		// The filter lets a skill whose file cannot be read through, reviewed or not; such a skill is not offered.
		if rows[i].Reviewed && rows[i].State == library.StateOK {
			names = append(names, rows[i].Name)
		}
	}

	return names
}

// settledSpend is what the settled jobs of a project cost, by kind of job: real cost only. It is empty when the
// ledger cannot be read: a cost the server does not have is left out, never invented.
func (s *svc) settledSpend(ctx context.Context, projectID string) []project.Spend {
	rows, err := s.store.Approvals().SettledByKind(ctx, projectID)
	if err != nil {
		s.log.WarnContext(ctx, "the spending of a project was not read", "project", projectID, "error", err.Error())

		return nil
	}

	out := make([]project.Spend, len(rows))
	for i := range rows {
		out[i] = project.Spend{JobKind: rows[i].Kind, Currency: rows[i].Currency, Micros: rows[i].Micros}
	}

	return out
}

// nextPost is the instant of the next post of a project, nil when none goes out or the publications cannot be read.
func (s *svc) nextPost(ctx context.Context, projectID string) *time.Time {
	list, ok := s.publicationsOf(ctx, projectID)
	if !ok {
		return nil
	}

	return s.nextPostOf(list)
}

// approvalCosts is the sum, by currency, of the estimates of the approval requests of a project that are pending
// or approved. It is empty when there are none, and when the requests cannot be read: a cost the server does not
// have is left out, never invented.
func (s *svc) approvalCosts(ctx context.Context, projectID string) []project.Cost {
	rows, err := s.store.Approvals().PendingApprovedCosts(ctx, projectID)
	if err != nil {
		s.log.WarnContext(
			ctx,
			"the approvals of a project were not read",
			"project",
			projectID,
			"error",
			err.Error(),
		)

		return nil
	}

	if len(rows) == 0 {
		return nil
	}

	sums := make([]project.Cost, 0, len(rows))
	for i := range rows {
		if rows[i].Micros > 0 && rows[i].Currency != "" {
			sums = append(sums, project.Cost{Micros: rows[i].Micros, Currency: rows[i].Currency})
		}
	}

	return sums
}

func addCost(sums []project.Cost, currency string, micros int64) []project.Cost {
	if micros <= 0 || currency == "" {
		return sums
	}

	for i := range sums {
		if sums[i].Currency == currency {
			sums[i].Micros += micros

			return sums
		}
	}

	return append(sums, project.Cost{Micros: micros, Currency: currency})
}

// workflowLine is the line of the text of a project that says where it stands.
func workflowLine(w *project.Workflow) string {
	steps := make([]textview.StepLine, len(w.Steps))
	for i := range w.Steps {
		steps[i] = textview.StepLine{ID: string(w.Steps[i].ID), State: string(w.Steps[i].State)}
	}

	return textview.Workflow(string(w.Kind), steps, string(w.Current), string(w.Next))
}

// textSummaryOf is what a list says about the text of a project: its first text that has words, a post before a
// copy before a script (core/writing). nil when the project holds none.
func textSummaryOf(rows []ItemRow) *TextSummary {
	var (
		candidates []writing.Candidate
		at         []int
	)

	for i := range rows {
		r := &rows[i]
		if r.Kind != string(project.Text) || r.State == ProjectTrashed {
			continue
		}

		candidates = append(candidates, writing.Candidate{Kind: r.TextKind, Body: r.Body})
		at = append(at, i)
	}

	pick, ok := writing.SummaryText(candidates)
	if !ok {
		return nil
	}

	r := &rows[at[pick]]
	sum := writing.Summarize(r.TextKind, r.Body)

	return &TextSummary{
		ItemID: r.ID, TextKind: textKindOf(r.TextKind), Words: sum.Words, ReadingMinutes: sum.ReadingMinutes,
		Excerpt: sum.Excerpt,
	}
}

// textKindOf is the kind of a text item; a text that has none is a copy.
func textKindOf(kind string) string {
	if kind == "" {
		return string(writing.DefaultKind)
	}

	return kind
}

// ProjectStepSkip skips an optional step of the workflow of a project, or with Restore puts it back (SDD-005
// section 5.12.4).
func (s *svc) ProjectStepSkip(ctx context.Context, req ProjectStepSkipRequest) (*ProjectStepSkipResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectStepSkip", req.ProjectID)
	if err != nil {
		return nil, err
	}

	var res *ProjectStepSkipResult

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		head, rows, werr := s.writableProduction(ctx, tx, c, req.ProjectID)
		if werr != nil {
			return werr
		}

		// The state of a step is read from the publications too, and a write holds its transaction while it does.
		rows, known := s.withPublications(ctx, head.ID, rows)
		w := s.workflowOf(ctx, &head, rows, known)

		id, v := project.CheckStep(&w, req.Step, req.Restore)
		if v != nil {
			return violation(v)
		}

		skipped := slices.Contains(head.Skipped, id)
		next := slices.Clone(head.Skipped)

		switch {
		case req.Restore && skipped:
			next = slices.DeleteFunc(next, func(x project.StepID) bool { return x == id })
		case !req.Restore && !skipped:
			next = append(next, id)
		}

		changed := req.Restore == skipped

		head.Skipped = next
		res = &ProjectStepSkipResult{
			UntrustedNotice: UntrustedNotice, ProjectID: head.ID,
			Workflow: s.workflowOf(ctx, &head, rows, known),
			Text:     textview.StepSkipped(head.ID, string(id), req.Restore, changed, req.DryRun),
		}

		if req.DryRun || !changed {
			return nil
		}

		now, action := s.nowMs(), "project.step.skip"
		if req.Restore {
			action = "project.step.restore"
		}

		if err := tx.Productions().SetSkippedSteps(ctx, head.ID, next, now); err != nil {
			return err
		}

		if err := appendAudit(ctx, tx, now, c.Actor, action, head.ID, string(id)); err != nil {
			return err
		}

		tx.AfterCommit(s.emitProject(&head, head.State, c.Actor, now))

		res.Changed = true

		return nil
	})
	if err != nil {
		if se, ok := errors.AsType[*Error](err); ok {
			return nil, se
		}

		return nil, projectNotFound(req.ProjectID, err)
	}

	return res, nil
}
