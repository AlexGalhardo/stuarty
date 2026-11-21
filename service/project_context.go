package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// ProjectGoal is the SMART goal of a project (SDD-005 section 5.12.2). The text is the creator's: an agent reads
// it as data, never as an instruction.
type ProjectGoal struct {
	Specific   string `json:"specific"   untrusted:"true"`
	Measurable string `json:"measurable" untrusted:"true"`
	Achievable string `json:"achievable" untrusted:"true"`
	Relevant   string `json:"relevant"   untrusted:"true"`
	TimeBound  string `json:"time_bound" untrusted:"true"`
	Due        string `json:"due"` // YYYY-MM-DD
}

// ProjectContext is what a project says about itself, and what an agent reads first. Complete is false for a
// project made before epic 56, which has neither text.
type ProjectContext struct {
	Description string      `json:"description" untrusted:"true"`
	Goal        ProjectGoal `json:"goal"`
	Complete    bool        `json:"complete"`
}

func contextOf(h *ProductionHead) ProjectContext {
	c := project.Context{Name: h.Name, Description: h.Description, Goal: h.Goal}

	return ProjectContext{Description: h.Description, Goal: ProjectGoal(h.Goal), Complete: c.Complete()}
}

// contextText is the description and the goal as the lines an agent reads: the ones that are set, and nothing
// when the project has neither (a project made before epic 56 that nobody has completed).
func contextText(h *ProductionHead) string {
	g := h.Goal
	lines := []struct{ label, text string }{
		{"description", h.Description},
		{"goal specific", g.Specific},
		{"goal measurable", g.Measurable},
		{"goal achievable", g.Achievable},
		{"goal relevant", g.Relevant},
		{"goal time-bound", g.TimeBound},
		{"goal due", g.Due},
	}

	var out []string

	for _, l := range lines {
		if l.text != "" {
			out = append(out, l.label+": "+l.text)
		}
	}

	return strings.Join(out, "\n")
}

// ProjectUpdateRequest changes the name, the description or the goal of a project. A member that is left out is
// kept; a goal is sent whole, because a goal with a part missing is not a goal.
type ProjectUpdateRequest struct {
	ProjectID   string       `json:"project_id"`
	Name        *string      `json:"name,omitempty"`
	Description *string      `json:"description,omitempty"`
	Goal        *ProjectGoal `json:"goal,omitempty"`
	DryRun      bool         `json:"dry_run,omitempty"`
}

// ProjectUpdateResult is the project after the change.
type ProjectUpdateResult struct {
	Envelope
	ProjectID   string      `json:"project_id"`
	Name        string      `json:"name"        untrusted:"true"`
	Description string      `json:"description" untrusted:"true"`
	Goal        ProjectGoal `json:"goal"`
	Complete    bool        `json:"complete"`
	Updated     bool        `json:"updated"` // false on a dry run
	Text        string      `json:"text"`
}

// checkContextError is the E_VALIDATION of the first broken rule, with every broken rule in details.fields.
func checkContextError(errs []*project.RuleError) *Error {
	if len(errs) == 0 {
		return nil
	}

	fields := make([]map[string]string, len(errs))
	for i, e := range errs {
		fields[i] = map[string]string{"path": e.Path, "message": e.Message, "hint": e.Hint}
	}

	first := errs[0]

	return newError(command.CodeValidation, "%s.", first.Message).
		withPath(first.Path).withHint("%s", first.Hint).withDetail("fields", fields)
}

// today is the day of the service clock.
func (s *svc) today() string { return s.now().Format(project.DateLayout) }

// ProjectUpdate edits the name, the description and the goal of a project (SDD-005 section 5.12.2).
func (s *svc) ProjectUpdate(ctx context.Context, req ProjectUpdateRequest) (*ProjectUpdateResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectUpdate", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.Name == nil && req.Description == nil && req.Goal == nil {
		return nil, newError(command.CodeValidation, "the request changes nothing.").
			withPath("name").withHint("send name, description or goal")
	}

	var (
		res       *ProjectUpdateResult
		renameDoc bool
		base      int64
	)

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		head, _, werr := s.writableProduction(ctx, tx, c, req.ProjectID)
		if werr != nil {
			return werr
		}

		next := project.Context{Name: head.Name, Description: head.Description, Goal: head.Goal}
		checkDue := false

		if req.Name != nil {
			next.Name = *req.Name
		}

		if req.Description != nil {
			next.Description = *req.Description
		}

		if req.Goal != nil {
			next.Goal = project.Goal(*req.Goal)
			checkDue = next.Goal.Due != head.Goal.Due
		}

		if verr := checkContextError(s.changedProblems(next, &req, checkDue)); verr != nil {
			return verr
		}

		res = &ProjectUpdateResult{
			UntrustedNotice: UntrustedNotice, ProjectID: head.ID, Name: next.Name,
			Description: next.Description, Goal: ProjectGoal(next.Goal), Complete: next.Complete(),
			Text: textview.UpdatedProject(head.ID, next.Name, next.Complete()),
		}

		if req.DryRun {
			return nil
		}

		now := s.nowMs()
		if err := tx.Productions().SetContext(ctx, head.ID, next, now); err != nil {
			return err
		}

		if err := appendAudit(ctx, tx, now, c.Actor, "project.update", head.ID, ""); err != nil {
			return err
		}

		tx.AfterCommit(s.emitProject(&head, head.State, c.Actor, now))

		res.Updated = true

		if next.Name != head.Name && head.Timeline.ID == head.ID && head.Timeline.Name == head.Name {
			renameDoc, base = true, head.Timeline.Revision
		}

		return nil
	})
	if err != nil {
		if se, ok := errors.AsType[*Error](err); ok {
			return nil, se
		}

		return nil, projectNotFound(req.ProjectID, err)
	}

	if renameDoc {
		s.renameFirstTimeline(ctx, res.ProjectID, base, res.Name)
	}

	return res, nil
}

// renameFirstTimeline gives the first timeline the new name of its project, as an edit, so that the editor shows
// it and an undo brings both back. The project is already renamed: a timeline that cannot take the name (a
// concurrent change) keeps its own, which is a label, and the rename of the project stands.
func (s *svc) renameFirstTimeline(ctx context.Context, id string, base int64, name string) {
	args, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return
	}

	if _, err := s.Edit(ctx, EditRequest{
		ProjectID: id, TimelineID: id, BaseRevision: base,
		Operations: []command.Operation{{Op: "project.rename", Args: args}},
	}); err != nil {
		slog.WarnContext(ctx, "the first timeline kept its name", "project", id, "error", err)
	}
}

// applyEdit applies a batch and holds a rename that reaches the project to the rule of a project name.
func (s *svc) applyEdit(
	ctx context.Context,
	tx Tx,
	g *productionGraph,
	row *ProjectRow,
	doc *timeline.Document,
	ops []command.Operation,
	ids *idRecorder,
) (command.BatchResult, error) {
	out, err := s.applyChecked(g, doc, ops, ids)
	if err != nil {
		return out, err
	}

	return out, checkFollowedName(ctx, tx, row, out.Doc.Name)
}

// checkFollowedName holds a rename to the rule of a project name when it renames the project too: the timeline is
// the first of its project (it has its id) and carries its name. Any other timeline name is a label of 1 to 120
// characters, which the core already checked.
func checkFollowedName(ctx context.Context, tx Tx, row *ProjectRow, next string) error {
	if next == row.Name {
		return nil
	}

	head, err := tx.Productions().Get(ctx, row.ID)
	if errors.Is(err, ErrNotFound) {
		return nil // a later timeline: its id names no project
	}

	if err != nil {
		return err
	}

	if head.Name != row.Name {
		return nil
	}

	if e := project.CheckName(next); e != nil {
		return checkContextError([]*project.RuleError{e})
	}

	return nil
}

// changedProblems are the broken rules of the members the request sends: a project made before epic 56 can be
// renamed before its description is written, so the members that are left out are not checked.
func (s *svc) changedProblems(next project.Context, req *ProjectUpdateRequest, checkDue bool) []*project.RuleError {
	var out []*project.RuleError

	for _, e := range project.CheckContext(next, project.CheckOptions{Today: s.today(), CheckDue: checkDue}) {
		switch {
		case e.Path == "name" && req.Name != nil,
			e.Path == "description" && req.Description != nil,
			strings.HasPrefix(e.Path, "goal") && req.Goal != nil:
			out = append(out, e)
		}
	}

	return out
}

// projectScopeRow is the row of a project for work that belongs to the project and not to a timeline document (a
// generation and the file it attaches, SDD-005 section 5.12.3): the timeline row that has the id when there is
// one, and a row with no document for a project that has no timeline yet. The id names a project or a timeline.
func projectScopeRow(
	ctx context.Context,
	timelines ProjectRepo,
	projects ProductionRepo,
	id string,
) (ProjectRow, error) {
	row, err := timelines.Get(ctx, id)
	if !errors.Is(err, ErrNotFound) {
		return row, err
	}

	head, perr := projects.Get(ctx, id)
	if perr != nil {
		return ProjectRow{}, err
	}

	var synth ProjectRow

	synth.ID, synth.Name, synth.State = head.ID, head.Name, head.State
	synth.CreatedMs, synth.UpdatedMs, synth.CreatedBy = head.CreatedMs, head.UpdatedMs, head.CreatedBy
	synth.OwnerUserID, synth.TrashedMs, synth.TrashedBy = head.OwnerUserID, head.TrashedMs, head.TrashedBy

	return synth, nil
}

func (s *svc) scopeRow(ctx context.Context, id string) (ProjectRow, error) {
	return projectScopeRow(ctx, s.store.Projects(), s.store.Productions(), id)
}

// loadProjectScope is loadProject for work that belongs to the project: it also finds a project with no timeline.
func (s *svc) loadProjectScope(ctx context.Context, c Caller, id string) (ProjectRow, error) {
	row, err := s.scopeRow(ctx, id)
	if err != nil || !visible(c, row.OwnerUserID) {
		return ProjectRow{}, projectNotFound(id, err)
	}

	return row, nil
}

// useTimelineOrProject is useTimeline for an operation that is about the project and its jobs: a project that has
// no active timeline keeps its own id instead of being refused.
func (s *svc) useTimelineOrProject(ctx context.Context, c Caller, id *string, timelineID string) error {
	if timelineID == "" && *id != "" {
		if tls, err := s.store.Productions().ActiveTimelines(ctx, *id); err == nil && len(tls) == 0 {
			if head, gerr := s.store.Productions().Get(ctx, *id); gerr == nil && visible(c, head.OwnerUserID) {
				return nil
			}
		}
	}

	return s.useTimeline(ctx, c, id, timelineID)
}
