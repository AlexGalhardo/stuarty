package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/writing"
)

// productionChange is what the plan of an item operation decides: the items as they will be, the item the
// answer is about, and the writes that make it so (nil when nothing changes).
type productionChange struct {
	rows   []ItemRow
	itemID string
	write  func(ctx context.Context, tx Tx) error
}

type productionPlan func(ctx context.Context, tx Tx, head *ProductionHead, rows []ItemRow) (*productionChange, error)

// changeProduction is the frame of every operation that changes the items of a project: in one transaction it
// loads the project (visible, not in the trash) and its items, asks plan what to do, and, unless the request
// is a dry run or nothing changes, writes it with the touch of the project, the audit row and the event.
func (s *svc) changeProduction(
	ctx context.Context, c Caller, action, projectID string, dry bool, plan productionPlan,
) (ProductionHead, *productionChange, error) {
	var (
		head   ProductionHead
		change *productionChange
	)

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		var (
			rows []ItemRow
			err  error
		)

		if head, rows, err = s.writableProduction(ctx, tx, c, projectID); err != nil {
			return err
		}

		if change, err = plan(ctx, tx, &head, rows); err != nil {
			return err
		}

		if dry || change.write == nil {
			return nil
		}

		now := s.nowMs()

		if err := change.write(ctx, tx); err != nil {
			return err
		}

		if err := tx.Productions().Touch(ctx, head.ID, now); err != nil {
			return err
		}

		if err := appendAudit(ctx, tx, now, c.Actor, action, head.ID, change.itemID); err != nil {
			return err
		}

		tx.AfterCommit(s.emitProject(&head, head.State, c.Actor, now))

		return nil
	})
	if err != nil {
		return ProductionHead{}, nil, asError(err)
	}

	return head, change, nil
}

func idsOf(rows []ItemRow) []string {
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}

	return ids
}

// renumber sets the position of every row to its place in ids and sorts the rows by it.
func renumber(rows []ItemRow, ids []string) {
	for i := range rows {
		rows[i].Position = slices.Index(ids, rows[i].ID)
	}

	slices.SortFunc(rows, func(a, b ItemRow) int { return a.Position - b.Position })
}

// checkItemAdd is what ProjectItemAdd refuses before it touches the store; for a timeline it also builds the
// document.
func (s *svc) checkItemAdd(req *ProjectItemAddRequest) (*timeline.Document, error) {
	kind := project.Kind(req.Kind)
	if v := project.CheckAdd(kind); v != nil {
		return nil, violation(v)
	}

	if v := project.CheckTitle(req.Title); v != nil {
		return nil, violation(v)
	}

	if req.Position != nil {
		if _, v := project.Position(0, *req.Position); v != nil {
			return nil, violation(v)
		}
	}

	if req.TextKind != "" && kind != project.Text {
		return nil, newError(command.CodeValidation, "text_kind is for an item of kind text.").
			withPath("text_kind").withHint("leave text_kind out, or add a text")
	}

	switch kind {
	case project.Text:
		if v := project.CheckText(req.Text); v != nil {
			return nil, violation(v)
		}

		if _, v := writing.CheckKind(req.TextKind); v != nil {
			return nil, writingViolation(v)
		}
	case project.Timeline:
		// The id is a placeholder that passes the check: the first timeline of a project takes the id of the project
		// and any other draws its own when the item is written, so that a first timeline draws no identifier.
		doc, verr := s.newDocument(ProjectCreateRequest{
			Name: req.Title, Preset: req.Preset, FPS: req.FPS, ID: "prj_0000000000",
		})
		if verr != nil {
			return nil, verr
		}

		return doc, nil
	case project.Output:
		if req.JobID == "" {
			return nil, newError(command.CodeValidation, "an output item needs the export job that made the file.").
				withPath("job_id").withHint("pass job_id of a succeeded export job of this project")
		}
	case project.Brief, project.Image, project.Publication:
		// CheckAdd refused them.
	}

	return nil, nil //nolint:nilnil // only a timeline has a document
}

// ProjectItemAdd adds an item to a production.
func (s *svc) ProjectItemAdd(ctx context.Context, req ProjectItemAddRequest) (*ProjectItemResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectItemAdd", req.ProjectID)
	if err != nil {
		return nil, err
	}

	doc, err := s.checkItemAdd(&req)
	if err != nil {
		return nil, err
	}

	head, change, err := s.changeProduction(ctx, c, "project.item.add", req.ProjectID, req.DryRun,
		func(ctx context.Context, tx Tx, head *ProductionHead, rows []ItemRow) (*productionChange, error) {
			if v := project.CheckRoom(coreItems(rows), project.Kind(req.Kind)); v != nil {
				return nil, violation(v)
			}

			now := s.nowMs()
			firstTimeline := project.Kind(req.Kind) == project.Timeline &&
				!slices.ContainsFunc(rows, func(r ItemRow) bool { return r.Kind == string(project.Timeline) })

			it := s.itemRowFor(head, rows, &req, c, now, firstTimeline)

			switch project.Kind(req.Kind) {
			case project.Text:
				it.Body = req.Text
				it.TextKind = req.TextKind

				if it.TextKind == "" {
					it.TextKind = string(writing.DefaultKind)
				}
			case project.Timeline:
				// The first timeline of a project has the id of the project (SDD-005 section 5.12), also when it
				// is added after the project was made (epic 56): the rows, the URLs and the logs that name a
				// timeline by the id of its project keep working.
				if firstTimeline {
					doc.ID = head.ID
				} else {
					doc.ID = s.newIDs(timeline.PrefixProject)
				}

				it.Ref = doc.ID
			case project.Output:
				job, jerr := s.outputJob(ctx, tx, rows, req.JobID)
				if jerr != nil {
					return nil, jerr
				}

				it.Ref, it.Format = job.ID, outputFormat(job)
			case project.Brief, project.Image, project.Publication:
			}

			at := len(rows)
			if req.Position != nil {
				at, _ = project.Position(len(rows), *req.Position)
			}

			ids := slices.Insert(idsOf(rows), at, it.ID)
			rows = append(rows, it)
			renumber(rows, ids)

			return &productionChange{rows: rows, itemID: it.ID, write: func(ctx context.Context, tx Tx) error {
				if doc != nil {
					if err := s.createTimeline(ctx, tx, doc, &it, rows, firstTimeline, c, head, now); err != nil {
						return err
					}
				}

				it.Position = slices.Index(ids, it.ID)
				if err := tx.Productions().CreateItem(ctx, it); err != nil {
					return err
				}

				return tx.Productions().SetPositions(ctx, head.ID, ids)
			}}, nil
		})
	if err != nil {
		return nil, err
	}

	// The files a generation attached to the project before it had a timeline wait for it (SDD-005 section 5.12.3).
	s.reconcileFirstTimeline(ctx, &req, head.ID, findItem(change.rows, change.itemID))

	return s.itemResult(&head, change.rows, change.itemID), nil
}

// createTimeline writes the document of a new timeline. A row of another project can hold the id a first timeline
// takes from its project (VV-373): the project is not left without a timeline for it, and takes an id of its own.
func (s *svc) createTimeline(
	ctx context.Context, tx Tx, doc *timeline.Document, it *ItemRow, rows []ItemRow, first bool,
	c Caller, head *ProductionHead, now int64,
) error {
	err := tx.Projects().Create(ctx, s.timelineRow(doc, c, head.OwnerUserID, now))
	if !first || !errors.Is(err, ErrExists) {
		return err
	}

	doc.ID = s.newIDs(timeline.PrefixProject)
	it.Ref = doc.ID

	if shown := findItem(rows, it.ID); shown != nil {
		shown.Ref = doc.ID
	}

	if err := tx.Projects().Create(ctx, s.timelineRow(doc, c, head.OwnerUserID, now)); err != nil {
		return err
	}

	// The files waiting for the first timeline wait under the id of the project, which this one could not take.
	return tx.Assets().MovePendingProjectAssets(ctx, head.ID, doc.ID)
}

// timelineRow is the row of a new timeline document.
func (s *svc) timelineRow(doc *timeline.Document, c Caller, owner *string, now int64) ProjectRow {
	canon := timeline.Canonical(doc)

	return ProjectRow{
		ID: doc.ID, Name: doc.Name, State: ProjectActive, SchemaVersion: doc.SchemaVersion,
		Summary: summarize(doc), CreatedMs: now, UpdatedMs: now, CreatedBy: c.Actor, OwnerUserID: owner,
		Document: canon, DocSHA256: sha256Hex(canon),
	}
}

// outputJob finds the succeeded export job an output item records. The job must belong to one of the timelines
// of the project; a bundle export is not an output.
func (s *svc) outputJob(ctx context.Context, tx Tx, rows []ItemRow, jobID string) (*JobRow, error) {
	notFound := newError(command.CodeNotFound, "there is no export job %s in this project.", jobID).
		withPath("job_id").withHint("list the jobs of the project with job_list")

	job, err := tx.Jobs().Get(ctx, jobID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, notFound
		}

		return nil, asError(err)
	}

	own := false

	for i := range rows {
		if rows[i].Kind == string(project.Timeline) && job.ProjectID != nil && rows[i].Ref == *job.ProjectID {
			own = true
		}
	}

	if job.Kind != JobExport || !own {
		return nil, notFound
	}

	var in struct {
		Format string `json:"format"`
	}

	_ = json.Unmarshal(job.Input, &in)

	if in.Format == bundleExportFormat {
		return nil, newError(
			command.CodeValidation,
			"the job %s exported a bundle, which is not an output piece.",
			jobID,
		).
			withPath("job_id").
			withHint("use the job of a video, audio or image export")
	}

	if job.State != JobSucceeded {
		return nil, newError(command.CodeInvalidState, "the export job %s is %s.", jobID, job.State).
			withPath("job_id").withHint("wait for the export to succeed")
	}

	return &job, nil
}

func outputFormat(job *JobRow) string {
	var res JobResult
	if json.Unmarshal(job.Result, &res) == nil && res.Export != nil && res.Export.Format != "" {
		return res.Export.Format
	}

	var in struct {
		Format string `json:"format"`
	}

	if json.Unmarshal(job.Input, &in) == nil && in.Format != "" {
		return in.Format
	}

	return "mp4"
}

// ProjectItemMove moves an item to a position of the order of the production.
func (s *svc) ProjectItemMove(ctx context.Context, req ProjectItemMoveRequest) (*ProjectItemResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectItemMove", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if _, v := project.Position(0, req.Position); v != nil {
		return nil, violation(v)
	}

	head, change, err := s.changeProduction(ctx, c, "project.item.move", req.ProjectID, req.DryRun,
		func(_ context.Context, _ Tx, head *ProductionHead, rows []ItemRow) (*productionChange, error) {
			if findItem(rows, req.ItemID) == nil {
				return nil, itemNotFound(req.ItemID)
			}

			before := idsOf(rows)
			ordered, _ := project.Order(before, req.ItemID, req.Position)
			renumber(rows, ordered)

			ch := &productionChange{rows: rows, itemID: req.ItemID}
			if !slices.Equal(before, ordered) {
				ch.write = func(ctx context.Context, tx Tx) error {
					return tx.Productions().SetPositions(ctx, head.ID, ordered)
				}
			}

			return ch, nil
		})
	if err != nil {
		return nil, err
	}

	return s.itemResult(&head, change.rows, change.itemID), nil
}

// ProjectItemRename changes the title of an item; the document of a timeline is not touched.
func (s *svc) ProjectItemRename(ctx context.Context, req ProjectItemRenameRequest) (*ProjectItemResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectItemRename", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if v := project.CheckTitle(req.Title); v != nil {
		return nil, violation(v)
	}

	head, change, err := s.changeProduction(ctx, c, "project.item.rename", req.ProjectID, req.DryRun,
		func(_ context.Context, _ Tx, _ *ProductionHead, rows []ItemRow) (*productionChange, error) {
			it := findItem(rows, req.ItemID)
			if it == nil {
				return nil, itemNotFound(req.ItemID)
			}

			ch := &productionChange{rows: rows, itemID: req.ItemID}
			if it.Title != req.Title {
				it.Title, it.UpdatedMs = req.Title, s.nowMs()
				ch.write = func(ctx context.Context, tx Tx) error { return tx.Productions().UpdateItem(ctx, *it) }
			}

			return ch, nil
		})
	if err != nil {
		return nil, err
	}

	return s.itemResult(&head, change.rows, change.itemID), nil
}

// ProjectItemTrash moves an item to the trash of its project.
func (s *svc) ProjectItemTrash(ctx context.Context, req ProjectItemTrashRequest) (*ProjectItemResult, error) {
	return s.setItemState(ctx, "ProjectItemTrash", req, ProjectTrashed, "project.item.trash")
}

// ProjectItemRestore brings an item back from the trash of its project.
func (s *svc) ProjectItemRestore(ctx context.Context, req ProjectItemTrashRequest) (*ProjectItemResult, error) {
	return s.setItemState(ctx, "ProjectItemRestore", req, ProjectActive, "project.item.restore")
}

func (s *svc) setItemState(
	ctx context.Context, method string, req ProjectItemTrashRequest, to ProjectState, action string,
) (*ProjectItemResult, error) {
	c, err := s.authorizeProject(ctx, method, req.ProjectID)
	if err != nil {
		return nil, err
	}

	var tlID string

	head, change, err := s.changeProduction(ctx, c, action, req.ProjectID, req.DryRun,
		func(ctx context.Context, tx Tx, _ *ProductionHead, rows []ItemRow) (*productionChange, error) {
			it := findItem(rows, req.ItemID)
			if it == nil {
				return nil, itemNotFound(req.ItemID)
			}

			ch := &productionChange{rows: rows, itemID: req.ItemID}
			if it.State == to {
				return ch, nil
			}

			if it.Kind == string(project.Timeline) {
				if nerr := s.checkTimelineState(ctx, tx, it.Ref, to); nerr != nil {
					return nil, nerr
				}
			}

			check := project.CheckTrash(coreItems(rows), it.ID)
			if to == ProjectActive {
				check = project.CheckRoom(coreItems(rows), project.Kind(it.Kind))
			}

			if check != nil {
				return nil, violation(check).withPath("item_id")
			}

			now := s.nowMs()
			it.State, it.UpdatedMs, it.TrashedMs = to, now, nil

			if to == ProjectTrashed {
				it.TrashedMs = &now
			}

			ch.write = func(ctx context.Context, tx Tx) error {
				if err := tx.Productions().UpdateItem(ctx, *it); err != nil {
					return err
				}

				// A timeline in the trash rejects edits like a project in the trash does (SDD-005 section 13.1).
				if it.Kind != string(project.Timeline) {
					return nil
				}

				tlID = it.Ref

				return tx.Projects().SetState(ctx, it.Ref, to, c.Actor, now)
			}

			return ch, nil
		})
	if err != nil {
		return nil, err
	}

	switch {
	case tlID == "" || req.DryRun:
	case to == ProjectTrashed:
		s.cancelProjectJobs(ctx, tlID)
	default:
		s.reconcilePending(ctx, tlID)
	}

	return s.itemResult(&head, change.rows, change.itemID), nil
}

// BriefSet writes the brief of a project: the Markdown the answers of the briefing are kept in (RF-302). The
// first call makes the brief item, at the head of the production; an empty text clears it.
func (s *svc) BriefSet(ctx context.Context, req BriefSetRequest) (*BriefResult, error) {
	c, err := s.authorizeProject(ctx, "BriefSet", req.ProjectID)
	if err != nil {
		return nil, err
	}

	return s.writeBrief(ctx, c, req, nil)
}

// writeBrief is BriefSet for a caller already authorized. keep, when set, is given the brief there is inside the
// write transaction and may refuse the write, so that no other write lands between the check and the write.
func (s *svc) writeBrief(
	ctx context.Context, c Caller, req BriefSetRequest, keep func(current string) *project.RuleError,
) (*BriefResult, error) {
	if v := project.CheckText(req.Text); v != nil {
		return nil, violation(v)
	}

	head, change, err := s.changeProduction(ctx, c, "project.brief.set", req.ProjectID, req.DryRun,
		func(_ context.Context, _ Tx, head *ProductionHead, rows []ItemRow) (*productionChange, error) {
			b := briefOf(rows)

			if keep != nil {
				current := ""
				if b != nil {
					current = b.Body
				}

				if v := keep(current); v != nil {
					return nil, violation(v)
				}
			}

			if (b == nil && req.Text == "") || (b != nil && b.Body == req.Text) {
				return &productionChange{rows: rows}, nil
			}

			now := s.nowMs()

			if b != nil {
				b.Body, b.UpdatedMs = req.Text, now

				return &productionChange{rows: rows, itemID: b.ID, write: func(ctx context.Context, tx Tx) error {
					return tx.Productions().UpdateItem(ctx, *b)
				}}, nil
			}

			if v := project.CheckRoom(coreItems(rows), project.Brief); v != nil {
				return nil, violation(v).withPath("text")
			}

			it := s.newItem(head.ID, string(project.Brief), "Brief", 0, c.Actor, now)
			it.Body = req.Text
			rows = append([]ItemRow{it}, rows...)
			renumber(rows, idsOf(rows))

			return &productionChange{rows: rows, itemID: it.ID, write: func(ctx context.Context, tx Tx) error {
				if err := tx.Productions().CreateItem(ctx, rows[0]); err != nil {
					return err
				}

				return tx.Productions().SetPositions(ctx, head.ID, idsOf(rows))
			}}, nil
		})
	if err != nil {
		return nil, err
	}

	return briefResult(&head, change.rows), nil
}

// firstTimelineItemID is the id of the item of the first timeline of a project: it takes the characters of the id of
// the project, as it always did, so that the item that gives the project its id is known without a draw.
func (s *svc) firstTimelineItemID(projectID string) string {
	if rest, ok := strings.CutPrefix(projectID, timeline.PrefixProject+"_"); ok &&
		timeline.ValidID(timeline.PrefixProject, projectID) {
		return project.PrefixItem + "_" + rest
	}

	return s.newIDs(project.PrefixItem)
}

// itemRowFor is the row of the item that ProjectItemAdd writes. The item of the first timeline of a project takes
// the characters of the id of the project and draws nothing.
func (s *svc) itemRowFor(
	head *ProductionHead, rows []ItemRow, req *ProjectItemAddRequest, c Caller, now int64, firstTimeline bool,
) ItemRow {
	if !firstTimeline {
		return s.newItem(head.ID, req.Kind, req.Title, len(rows), c.Actor, now)
	}

	return ItemRow{
		ID: s.firstTimelineItemID(head.ID), ProjectID: head.ID, Kind: req.Kind, Title: req.Title,
		Position: len(rows), State: ProjectActive, CreatedMs: now, UpdatedMs: now, CreatedBy: c.Actor,
	}
}

// reconcileFirstTimeline puts the files that a generation attached to the project before it had a timeline into the
// document of the video just added, which is under the id of the project or, when another row holds that id, its own.
func (s *svc) reconcileFirstTimeline(
	ctx context.Context, req *ProjectItemAddRequest, projectID string, added *ItemRow,
) {
	if req.Kind != string(project.Timeline) || req.DryRun {
		return
	}

	s.reconcilePending(ctx, projectID)

	if added != nil && added.Ref != projectID {
		s.reconcilePending(ctx, added.Ref)
	}
}
