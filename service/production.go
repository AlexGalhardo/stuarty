package service

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

const (
	defaultDescribeTokens = 1500
	minDescribeTokens     = 200
	maxDescribeTokens     = 6000
)

func timelineNotFound(id string, err error) error {
	if err == nil || errors.Is(err, ErrNotFound) {
		return newError(command.CodeNotFound, "there is no timeline %s.", id).
			withHint("read the project with project_get to list its timelines")
	}

	return asError(err)
}

// resolveTimeline is the rule of SDD-003 section 1.7: it turns the project_id and the optional timeline_id of a
// request into the id of the timeline document the operation works on.
//
//   - With a timeline_id, the timeline must exist and, when a project_id is given too, belong to that project; a
//     key limited to another project is refused.
//   - With only a project_id, the active timeline of the project when it has one, an error that names the
//     choices when it has several, and the id unchanged when it names no project (it may be a timeline id, or
//     nothing; the operation then answers as it always did).
//
// No answer tells a caller whether an id it may not reach exists: a timeline of a project the caller may not see
// is not found as a missing one is, a key limited to one project is refused every timeline outside it whether
// it exists or not (as authorize refuses a project id), and the id of a timeline is named only to a caller who
// may see its project.
func (s *svc) resolveTimeline(ctx context.Context, c Caller, projectID, timelineID string) (string, error) {
	repo := s.store.Productions()

	if timelineID != "" {
		owner, err := repo.ProjectOfTimeline(ctx, timelineID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return "", asError(err)
		}

		if c.ProjectID != "" && (err != nil || c.ProjectID != owner) {
			return "", newError(CodeForbidden, "the key does not cover the timeline %s.", timelineID).
				withHint("use a key that covers its project")
		}

		// The timeline id may stand in the place of the project id as well, which is how the editor, whose
		// address is the id of a timeline, names it in both.
		if err != nil || (projectID != "" && projectID != owner && projectID != timelineID) {
			return "", timelineNotFound(timelineID, nil)
		}

		if head, gerr := repo.Get(ctx, owner); gerr != nil || !visible(c, head.OwnerUserID) {
			return "", timelineNotFound(timelineID, gerr)
		}

		return timelineID, nil
	}

	if projectID == "" {
		return "", nil
	}

	tls, err := repo.ActiveTimelines(ctx, projectID)
	if err != nil {
		return "", asError(err)
	}

	// The usual case, a project whose one timeline has its id, needs no other read: the operation checks the
	// row of that id as it always did.
	if len(tls) == 0 || (len(tls) == 1 && tls[0].TimelineID == projectID) {
		return projectID, nil
	}

	// A project the caller may not see does not tell its timelines.
	if head, gerr := repo.Get(ctx, projectID); gerr != nil || !visible(c, head.OwnerUserID) {
		return "", projectNotFound(projectID, gerr)
	}

	if len(tls) == 1 {
		return tls[0].TimelineID, nil
	}

	ids := make([]string, len(tls))
	for i := range tls {
		ids[i] = tls[i].TimelineID
	}

	return "", newError(
		command.CodeValidation,
		"the project has %d timelines and the request does not say which.",
		len(ids),
	).
		withPath("timeline_id").
		withHint("pass timeline_id: %s", strings.Join(ids, ", ")).
		withDetail("timelines", ids)
}

// useTimeline resolves the document an operation works on and writes it into id, so that the rest of the
// operation, which was written when a project was a document, keeps using the one name.
func (s *svc) useTimeline(ctx context.Context, c Caller, id *string, timelineID string) error {
	got, err := s.resolveTimeline(ctx, c, *id, timelineID)
	if err != nil {
		return err
	}

	*id = got

	return nil
}

// loadProduction reads a project the caller may see. An id that is not a project but is a timeline stands for
// the project that holds it, so that every URL that carries a timeline id in the place of a project id works.
func (s *svc) loadProduction(ctx context.Context, repo ProductionRepo, c Caller, id string) (ProductionHead, error) {
	head, err := repo.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		if owner, oerr := repo.ProjectOfTimeline(ctx, id); oerr == nil {
			head, err = repo.Get(ctx, owner)
		}
	}

	// A key limited to one project reaches that project only, whatever id names it. An id that resolves to
	// another project and one that resolves to nothing are refused alike, as resolveTimeline refuses them, so
	// the answer does not say whether the id exists.
	if c.ProjectID != "" && (err != nil && id != c.ProjectID || err == nil && head.ID != c.ProjectID) {
		return ProductionHead{}, newError(CodeForbidden, "the key is limited to another project.").
			withHint("use a key that covers this project")
	}

	if err != nil || !visible(c, head.OwnerUserID) {
		return ProductionHead{}, projectNotFound(id, err)
	}

	return head, nil
}

func coreItems(rows []ItemRow) []project.Item {
	out := make([]project.Item, len(rows))

	for i := range rows {
		r := &rows[i]
		out[i] = project.Item{
			ID: r.ID, Kind: project.Kind(r.Kind), Position: r.Position, Trashed: r.State == ProjectTrashed,
		}

		if r.Kind == string(project.Brief) || r.Kind == string(project.Text) {
			out[i].Text = r.Body
		}

		// The status of a publication is read from its targets (withPublications keeps it in Ref).
		if r.Kind == string(project.Publication) {
			out[i].Status = r.Ref
		}
	}

	return out
}

// withPublications adds the publications of a project to its items (ADR-0039, SDD-020 section 23): a
// publication is an item of the project, kept in its own tables, so it is read from them. Its Ref holds the
// status the core rolls up from its targets and its Format the networks they go to. Nothing is added when the
// publications cannot be read: the overview is still the production.
func (s *svc) withPublications(ctx context.Context, projectID string, rows []ItemRow) []ItemRow {
	list, err := s.store.Publications().OfProject(ctx, projectID)
	if err != nil {
		s.log.WarnContext(
			ctx,
			"the publications of a project were not read",
			"project",
			projectID,
			"error",
			err.Error(),
		)

		return rows
	}

	for i := range list {
		p := &list[i]
		states := make([]corepub.TargetState, 0, len(p.Targets))
		networks := make([]string, 0, len(p.Targets))

		for j := range p.Targets {
			states = append(states, p.Targets[j].State)

			if !slices.Contains(networks, p.Targets[j].Network) {
				networks = append(networks, p.Targets[j].Network)
			}
		}

		rows = append(rows, ItemRow{
			ID: p.ID, ProjectID: projectID, Kind: string(project.Publication), Title: p.Title, Position: len(rows),
			State: ProjectActive, Ref: corepub.ItemStatus(states), Format: strings.Join(networks, ","),
			CreatedMs: p.CreatedAt.UnixMilli(), UpdatedMs: p.UpdatedAt.UnixMilli(), CreatedBy: p.CreatedBy,
		})
	}

	return rows
}

func itemView(r *ItemRow, withText bool) ProjectItem {
	it := ProjectItem{
		ID: r.ID, Kind: r.Kind, Title: r.Title, Position: r.Position, State: stateString(r.State),
		Format: r.Format, UpdatedAt: msTime(r.UpdatedMs),
	}

	switch r.Kind {
	case string(project.Timeline):
		it.TimelineID = r.Ref
	case string(project.Output):
		it.JobID = r.Ref
	case string(project.Publication):
		it.Status = r.Ref
	}

	if withText {
		it.Text = r.Body
	}

	return it
}

func itemViews(rows []ItemRow) []ProjectItem {
	out := make([]ProjectItem, len(rows))
	for i := range rows {
		out[i] = itemView(&rows[i], false)
	}

	return out
}

// violation turns a rule broken in core/project into the error of the service.
func violation(v *project.RuleError) *Error {
	e := newError(v.Code, "%s.", v.Message)
	e.Path, e.Hint = v.Path, v.Hint

	return e
}

// newItem is an item row with the common members filled in.
func (s *svc) newItem(projectID, kind, title string, position int, actor string, now int64) ItemRow {
	return ItemRow{
		ID: s.newIDs(project.PrefixItem), ProjectID: projectID, Kind: kind, Title: title, Position: position,
		State: ProjectActive, CreatedMs: now, UpdatedMs: now, CreatedBy: actor,
	}
}

// putProduction writes the three rows that a new project is made of: the project, its timeline item and the
// timeline itself (SDD-005 section 5.12). The project takes the id of the timeline.
func (s *svc) putProduction(ctx context.Context, tx Tx, row *ProjectRow) error {
	head := ProductionHead{
		ID: row.ID, Name: row.Name, State: row.State, CreatedMs: row.CreatedMs, UpdatedMs: row.UpdatedMs,
		CreatedBy: row.CreatedBy, OwnerUserID: row.OwnerUserID, TrashedMs: row.TrashedMs, TrashedBy: row.TrashedBy,
	}
	if err := tx.Productions().Create(ctx, head); err != nil {
		return err
	}

	// The item of the timeline that gives the project its id takes the characters of that id, so that making a
	// project draws no extra identifier and the migration of an old workspace writes the same one.
	var it ItemRow

	if rest, ok := strings.CutPrefix(
		row.ID,
		timeline.PrefixProject+"_",
	); ok &&
		timeline.ValidID(timeline.PrefixProject, row.ID) {
		it = ItemRow{
			ID: project.PrefixItem + "_" + rest, ProjectID: row.ID, Kind: string(project.Timeline), Title: row.Name,
			State: ProjectActive, CreatedMs: row.CreatedMs, UpdatedMs: row.CreatedMs, CreatedBy: row.CreatedBy,
		}
	} else {
		it = s.newItem(row.ID, string(project.Timeline), row.Name, 0, row.CreatedBy, row.CreatedMs)
	}

	it.Ref = row.ID

	if err := tx.Productions().CreateItem(ctx, it); err != nil {
		return err
	}

	return tx.Projects().Create(ctx, *row)
}

// productionState reads the items of a project and what they say about its production.
func productionOf(rows []ItemRow) ProductionState {
	return project.ProductionState(coreItems(rows))
}

func (s *svc) emitProject(head *ProductionHead, state ProjectState, actor string, now int64) func() {
	ev := Event{
		Type: "project.updated", ProjectID: head.ID, Revision: head.Timeline.Revision, Actor: actor,
		State: string(state), At: msTime(now),
	}

	return func() { s.emit(ev) }
}

// writableProduction loads, inside tx, the project and its items for a change: the project must be visible and
// not in the trash.
func (s *svc) writableProduction(
	ctx context.Context, tx Tx, c Caller, id string,
) (ProductionHead, []ItemRow, error) {
	head, err := s.loadProduction(ctx, tx.Productions(), c, id)
	if err != nil {
		return ProductionHead{}, nil, err
	}

	if head.State == ProjectTrashed {
		return ProductionHead{}, nil, projectInTrash(head.ID)
	}

	rows, err := tx.Productions().Items(ctx, head.ID)
	if err != nil {
		return ProductionHead{}, nil, asError(err)
	}

	return head, rows, nil
}

func findItem(rows []ItemRow, id string) *ItemRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}

	return nil
}

func itemNotFound(id string) *Error {
	return newError(command.CodeNotFound, "there is no item %s in this project.", id).
		withHint("read the project with project_get to list its items")
}

func (s *svc) itemResult(head *ProductionHead, rows []ItemRow, id string) *ProjectItemResult {
	res := &ProjectItemResult{UntrustedNotice: UntrustedNotice, ProjectID: head.ID, Production: productionOf(rows)}

	if it := findItem(rows, id); it != nil {
		res.Item = itemView(it, false)
		res.Text = textview.Note + "\n" + it.ID + " " + it.Kind + " " + textview.Quote(it.Title, 60) + " " +
			stateString(it.State) + " at " + strconv.Itoa(it.Position)
	}

	return res
}

// BriefGet reads the brief of a project.
func (s *svc) BriefGet(ctx context.Context, req BriefGetRequest) (*BriefResult, error) {
	c, err := s.authorizeProject(ctx, "BriefGet", req.ProjectID)
	if err != nil {
		return nil, err
	}

	head, err := s.loadProduction(ctx, s.store.Productions(), c, req.ProjectID)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.Productions().Items(ctx, head.ID)
	if err != nil {
		return nil, asError(err)
	}

	return briefResult(&head, rows), nil
}

func briefOf(rows []ItemRow) *ItemRow {
	for i := range rows {
		if rows[i].Kind == string(project.Brief) && rows[i].State == ProjectActive {
			return &rows[i]
		}
	}

	return nil
}

func briefResult(head *ProductionHead, rows []ItemRow) *BriefResult {
	res := &BriefResult{UntrustedNotice: UntrustedNotice, ProjectID: head.ID}

	if b := briefOf(rows); b != nil {
		at := msTime(b.UpdatedMs)
		res.ItemID, res.Text, res.UpdatedAt = b.ID, b.Body, &at
	}

	return res
}

// ProjectDescribe is the text view of a production, for an agent that has never seen it (RF-303).
func (s *svc) ProjectDescribe(ctx context.Context, req ProjectDescribeRequest) (*ProjectDescribeResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectDescribe", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.MaxTokens != 0 && (req.MaxTokens < minDescribeTokens || req.MaxTokens > maxDescribeTokens) {
		return nil, newError(command.CodeValidation, "max_tokens is %d; it must be %d to %d.",
			req.MaxTokens, minDescribeTokens, maxDescribeTokens).
			withPath("max_tokens").withHint("omit max_tokens for %d", defaultDescribeTokens)
	}

	head, err := s.loadProduction(ctx, s.store.Productions(), c, req.ProjectID)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.Productions().Items(ctx, head.ID)
	if err != nil {
		return nil, asError(err)
	}

	rows = s.withPublications(ctx, head.ID, rows)

	res := &ProjectDescribeResult{
		UntrustedNotice: UntrustedNotice, ProjectID: head.ID, Name: head.Name, State: stateString(head.State),
		Production: productionOf(rows), Items: itemViews(rows),
	}
	res.Text = s.describeText(ctx, &head, rows, res.Production, req.MaxTokens)

	return res, nil
}

func (s *svc) describeText(
	ctx context.Context, head *ProductionHead, rows []ItemRow, p ProductionState, maxTokens int,
) string {
	if maxTokens == 0 {
		maxTokens = defaultDescribeTokens
	}

	v := textview.ProductionView{
		ID: head.ID, Name: head.Name, State: stateString(head.State), HasBrief: p.HasBrief, HasScript: p.HasScript,
		Timelines: p.Timelines, Outputs: p.Outputs, Pending: p.PendingApprovals, Published: p.Published,
	}

	if b := briefOf(rows); b != nil {
		v.Brief = b.Body
	}

	for i := range rows {
		r := &rows[i]
		l := textview.ProductionLine{ID: r.ID, Kind: r.Kind, Title: r.Title, Trashed: r.State == ProjectTrashed}

		switch r.Kind {
		case string(project.Timeline):
			l.Detail = s.timelineDetail(ctx, r.Ref)
		case string(project.Output):
			l.Detail = "format=" + r.Format + " job=" + r.Ref
		case string(project.Publication):
			l.Detail = "networks=" + r.Format + " status=" + r.Ref
		}

		v.Lines = append(v.Lines, l)
	}

	return textview.Production(&v, maxTokens)
}

// timelineDetail is the part of a timeline line that the head of its row gives.
func (s *svc) timelineDetail(ctx context.Context, id string) string {
	h, err := s.store.Projects().Head(ctx, id)
	if err != nil {
		return "timeline=" + id
	}

	fps := timeval.FPS{Num: int64(h.Summary.FPSNum), Den: int64(h.Summary.FPSDen)}

	return "timeline=" + id + " " + strconv.Itoa(
		h.Summary.Width,
	) + "x" + strconv.Itoa(
		h.Summary.Height,
	) + " " + fps.String() + "fps " +
		textview.Cell(
			fps,
			h.Summary.DurationFrames,
		) + " r" + strconv.FormatInt(
		h.Revision,
		10,
	)
}

// ProjectDuplicate copies a project: its brief and texts and every active timeline, each timeline as a new
// document at revision 0 with no log, using the same assets. Outputs are not copied.
func (s *svc) ProjectDuplicate(ctx context.Context, req ProjectDuplicateRequest) (*ProjectCreateResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectDuplicate", req.ProjectID)
	if err != nil {
		return nil, err
	}

	// A key limited to one project cannot create another (SDD-007 section 12, item 9).
	if c.ProjectID != "" {
		return nil, newError(CodeForbidden, "the key is limited to one project and cannot create projects.").
			withHint("use a key without a project limit")
	}

	if req.Name != "" {
		if v := project.CheckTitle(req.Name); v != nil {
			return nil, violation(v).withPath("name")
		}
	}

	var res *ProjectCreateResult

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		src, rows, werr := s.readableForCopy(ctx, tx, c, req.ProjectID)
		if werr != nil {
			return werr
		}

		name := req.Name
		if name == "" {
			name = copyName(src.Name)
		}

		plan, perr := s.planDuplicate(ctx, tx, c, &src, rows, name)
		if perr != nil {
			return perr
		}

		first := plan.timelines[0]
		res = &ProjectCreateResult{
			UntrustedNotice: UntrustedNotice, ProjectID: first.ID, Name: first.Name,
			Width: first.Summary.Width, Height: first.Summary.Height,
			FPS: fpsString(first.Summary.FPSNum, first.Summary.FPSDen),
			Text: textview.Created(first.ID, first.Name, first.Summary.Width, first.Summary.Height,
				timeval.FPS{Num: int64(first.Summary.FPSNum), Den: int64(first.Summary.FPSDen)}),
		}

		if req.DryRun {
			return nil
		}

		if werr := s.writeDuplicate(ctx, tx, c, &src, plan); werr != nil {
			return werr
		}

		res.Created = true

		return nil
	})
	if err != nil {
		return nil, asError(err)
	}

	return res, nil
}

// copyName is the default name of a copy: the name and " copy", cut so that the result is a valid name.
func copyName(name string) string {
	const suffix = " copy"

	if utf8.RuneCountInString(name)+len(suffix) <= maxProjectName {
		return name + suffix
	}

	runes := []rune(name)

	return string(runes[:maxProjectName-len(suffix)]) + suffix
}

// readableForCopy loads a project the caller may see, with its items. A project in the trash may be copied: the
// copy is a reading of it, and the copy itself is active.
func (s *svc) readableForCopy(
	ctx context.Context, tx Tx, c Caller, id string,
) (ProductionHead, []ItemRow, error) {
	head, err := s.loadProduction(ctx, tx.Productions(), c, id)
	if err != nil {
		return ProductionHead{}, nil, err
	}

	rows, err := tx.Productions().Items(ctx, head.ID)
	if err != nil {
		return ProductionHead{}, nil, asError(err)
	}

	return head, rows, nil
}

// duplicatePlan is a copy ready to be written.
type duplicatePlan struct {
	timelines []ProjectRow
	items     []ItemRow
	assets    []ProjectAssetRow
}

// planDuplicate builds the rows of the copy of the project src; nothing is written. The first active timeline
// of the copy has the id of the copy, as a new project always does.
func (s *svc) planDuplicate(
	ctx context.Context, tx Tx, c Caller, src *ProductionHead, rows []ItemRow, name string,
) (*duplicatePlan, error) {
	now := s.nowMs()
	plan := &duplicatePlan{}

	var owner *string

	if c.OwnerUserID != "" {
		o := c.OwnerUserID
		owner = &o
	}

	newProject := s.newIDs(timeline.PrefixProject)

	for i := range rows {
		r := &rows[i]
		if r.State != ProjectActive {
			continue
		}

		it := s.newItem(newProject, r.Kind, r.Title, len(plan.items), c.Actor, now)

		switch r.Kind {
		case string(project.Brief), string(project.Text):
			it.Body = r.Body
		case string(project.Timeline):
			trow, terr := tx.Projects().Get(ctx, r.Ref)
			if terr != nil {
				return nil, asError(terr)
			}

			doc, derr := decodeRow(&trow)
			if derr != nil {
				return nil, derr
			}

			id := newProject
			if len(plan.timelines) > 0 {
				id = s.newIDs(timeline.PrefixProject)
			} else {
				it.Title = name
			}

			doc.ID = id
			if len(plan.timelines) == 0 {
				doc.Name = name
			}

			plan.timelines = append(plan.timelines, s.timelineRow(doc, c, owner, now))

			it.Ref = id

			held, aerr := tx.Assets().ListProjectAssets(ctx, r.Ref, ProjectAssetActive)
			if aerr != nil {
				return nil, asError(aerr)
			}

			for j := range held {
				a := held[j]
				a.ProjectID, a.ImportedBy, a.ImportedMs = id, c.Actor, now
				plan.assets = append(plan.assets, a)
			}
		default:
			continue // an output belongs to the job that made it
		}

		plan.items = append(plan.items, it)
	}

	if len(plan.timelines) == 0 {
		return nil, newError(command.CodeInvalidState, "the project %s has no active timeline to copy.", src.ID).
			withHint("restore a timeline first")
	}

	return plan, nil
}

func (s *svc) writeDuplicate(ctx context.Context, tx Tx, c Caller, src *ProductionHead, plan *duplicatePlan) error {
	first := plan.timelines[0]
	head := ProductionHead{
		ID: first.ID, Name: first.Name, State: ProjectActive, CreatedMs: first.CreatedMs, UpdatedMs: first.UpdatedMs,
		CreatedBy: c.Actor, OwnerUserID: first.OwnerUserID,
	}

	if err := tx.Productions().Create(ctx, head); err != nil {
		return err
	}

	for i := range plan.timelines {
		if err := tx.Projects().Create(ctx, plan.timelines[i]); err != nil {
			return err
		}
	}

	for i := range plan.items {
		if err := tx.Productions().CreateItem(ctx, plan.items[i]); err != nil {
			return err
		}
	}

	indexed := map[string]bool{}

	for i := range plan.assets {
		if err := tx.Assets().PutProjectAsset(ctx, plan.assets[i]); err != nil {
			return err
		}

		// The copy may belong to an owner with no search row for the file yet (a local copy of a user's
		// project): the file is found by the names the copy gives it (VV-254).
		if h := plan.assets[i].AssetHash; !indexed[h] {
			indexed[h] = true

			if err := tx.AssetLibrary().Reindex(ctx, ownerOf(head.OwnerUserID), h); err != nil {
				return err
			}
		}
	}

	if err := appendAudit(ctx, tx, first.CreatedMs, c.Actor, "project.create", first.ID, ""); err != nil {
		return err
	}

	return appendAudit(ctx, tx, first.CreatedMs, c.Actor, "project.duplicate", first.ID, src.ID)
}
