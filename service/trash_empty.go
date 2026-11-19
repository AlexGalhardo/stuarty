package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
)

// jobWait is how long TrashEmpty waits for the jobs of a trashed project to
// reach a final state after asking them to stop (SDD-005 section 13.3).
const jobWait = 15 * time.Second

// TrashEmptyRequest limits what is emptied: one trashed project, and/or only
// what has been in the trash for that many days.
type TrashEmptyRequest struct {
	ProjectID     string `json:"project_id,omitempty"`
	OlderThanDays int    `json:"older_than_days,omitempty"`
}

// TrashEmptyResult counts what was deleted.
type TrashEmptyResult struct {
	Projects      int    `json:"projects"`
	ProjectAssets int    `json:"project_assets"`
	Assets        int    `json:"assets"`
	BytesFreed    int64  `json:"bytes_freed"`
	Text          string `json:"text"`
}

// humanOrAdminKey is the caller class that may empty the trash (SDD-007 section
// 5.3): a browser session, or an admin key. The local token, the in-process
// CLI and the stdio MCP server are none of them.
func humanOrAdminKey(c Caller) bool {
	return c.Browser || strings.HasPrefix(c.Actor, "key:")
}

// doomed is what a committed emptying leaves to delete from the disk.
type doomed struct {
	hashes   []string // hex of the content-store files and artifact directories
	projects []string // ids whose exports/ and frames/ directories go
}

// TrashEmpty deletes what is in the trash for good. It is the only operation
// that removes data, and no agent surface reaches it (RF-131, T-28).
func (s *svc) TrashEmpty(ctx context.Context, req TrashEmptyRequest) (*TrashEmptyResult, error) {
	c, err := s.authorizeProject(ctx, "TrashEmpty", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if !humanOrAdminKey(c) {
		return nil, newError(CodeForbidden, "only a person in the browser or an admin key can empty the trash.").
			withHint("open the trash in the editor; the local token and the agent surfaces cannot do it")
	}

	if req.OlderThanDays < 0 {
		return nil, newError(command.CodeValidation, "older_than_days cannot be negative.").
			withPath("older_than_days")
	}

	// A key limited to one project empties that project and nothing else.
	if c.ProjectID != "" && req.ProjectID == "" {
		req.ProjectID = c.ProjectID
	}

	// Import holds the read side from the copy into store/ through the row of
	// the asset; holding the write side keeps a concurrent import from finding
	// a file that is about to go.
	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	crashpoint.Hit("trash_empty.after_lock")

	targets, terr := s.trashTargets(ctx, c, &req)
	if terr != nil {
		return nil, terr
	}

	for _, id := range targets {
		for _, tl := range s.timelineIDs(ctx, id) {
			s.stopProjectJobs(ctx, tl)
		}
	}

	res := &TrashEmptyResult{}

	var plan doomed

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := s.deleteInTx(ctx, tx, c, &req, targets, res, &plan); err != nil {
			return err
		}

		crashpoint.Hit("trash_empty.before_commit")

		return nil
	})
	if err != nil {
		return nil, asError(err)
	}

	crashpoint.Hit("trash_empty.after_commit")

	res.BytesFreed = s.removeFiles(&plan)
	res.Text = fmt.Sprintf("emptied the trash: %d projects, %d project assets, %d assets, %d bytes freed",
		res.Projects, res.ProjectAssets, res.Assets, res.BytesFreed)

	return res, nil
}

// trashTargets lists the trashed projects the request selects and the caller
// may see.
func (s *svc) trashTargets(ctx context.Context, c Caller, req *TrashEmptyRequest) ([]string, error) {
	cutoff := int64(0)
	if req.OlderThanDays > 0 {
		cutoff = s.nowMs() - int64(req.OlderThanDays)*24*int64(time.Hour/time.Millisecond)
	}

	matches := func(h *ProductionHead) bool {
		return h.State == ProjectTrashed && visible(c, h.OwnerUserID) &&
			(c.ProjectID == "" || h.ID == c.ProjectID) &&
			(cutoff == 0 || h.TrashedMs != nil && *h.TrashedMs <= cutoff)
	}

	if req.ProjectID != "" {
		h, err := s.store.Productions().Get(ctx, req.ProjectID)
		if err != nil || !matches(&h) {
			return nil, newError(command.CodeNotFound, "there is no trashed project %s.", req.ProjectID).
				withHint("a project must be in the trash before it can be emptied")
		}

		return []string{h.ID}, nil
	}

	var ids []string

	after := ""

	for {
		heads, next, err := s.store.Productions().List(ctx, ProductionQuery{
			State: ProjectTrashed, OwnerUserID: c.OwnerUserID, After: after, Limit: 200,
		})
		if err != nil {
			return nil, asError(err)
		}

		for i := range heads {
			if matches(&heads[i]) {
				ids = append(ids, heads[i].ID)
			}
		}

		if next == "" {
			return ids, nil
		}

		after = next
	}
}

// timelineIDs lists every timeline of a project, trashed ones included: they are all deleted with it.
func (s *svc) timelineIDs(ctx context.Context, projectID string) []string {
	rows, err := s.store.Productions().Items(ctx, projectID)
	if err != nil {
		return nil
	}

	var ids []string

	for i := range rows {
		if rows[i].Kind == "timeline" {
			ids = append(ids, rows[i].Ref)
		}
	}

	return ids
}

// stopProjectJobs asks the queued and running jobs of a project to stop and
// waits for them, up to jobWait.
func (s *svc) stopProjectJobs(ctx context.Context, projectID string) {
	rows, err := s.store.Jobs().List(ctx, projectID, []string{JobQueued, JobRunning}, 0)
	if err != nil {
		return
	}

	for i := range rows {
		if s.cancelJob(ctx, rows[i].ID) == nil {
			s.waitJob(ctx, rows[i].ID, s.jobWait)
		}
	}
}

// deleteProduction deletes a trashed project: its timelines first (their log, assets and jobs cascade), then the
// project and its items. A project with a job that outlived the wait of stopProjectJobs is kept: the cascade
// would delete the job without a final state, and the next emptying takes the project.
func (s *svc) deleteProduction(ctx context.Context, tx Tx, id string) (timelines []string, deleted bool, err error) {
	items, err := tx.Productions().Items(ctx, id)
	if err != nil {
		return nil, false, err
	}

	for i := range items {
		if items[i].Kind != "timeline" {
			continue
		}

		timelines = append(timelines, items[i].Ref)

		active, lerr := tx.Jobs().List(ctx, items[i].Ref, []string{JobQueued, JobRunning}, 1)
		if lerr != nil {
			return nil, false, lerr
		}

		if len(active) > 0 {
			s.log.LogAttrs(ctx, slog.LevelWarn, "trash empty: project kept, a job is still active",
				slog.String("project", id))

			return nil, false, nil
		}
	}

	for _, tl := range timelines {
		if derr := tx.Projects().Delete(ctx, tl); derr != nil && !isNotFound(derr) {
			return nil, false, derr
		}
	}

	if err := tx.Productions().Delete(ctx, id); err != nil {
		return nil, false, err
	}

	return timelines, true, nil
}

// deleteInTx is the transaction of section 13.3: the projects (which cascade),
// the trashed references of the other projects, then every asset nothing
// references any more.
func (s *svc) deleteInTx(
	ctx context.Context,
	tx Tx,
	c Caller,
	req *TrashEmptyRequest,
	targets []string,
	res *TrashEmptyResult,
	plan *doomed,
) error {
	for _, id := range targets {
		timelines, deleted, err := s.deleteProduction(ctx, tx, id)
		if err != nil {
			return err
		}

		if deleted {
			res.Projects++

			plan.projects = append(plan.projects, timelines...)
		}
	}

	// With neither filter the trashed assets of every visible active project go
	// too; a filter limits the emptying to projects.
	if req.ProjectID == "" && req.OlderThanDays == 0 && c.ProjectID == "" {
		if err := s.deleteTrashedAssetRows(ctx, tx, c, res); err != nil {
			return err
		}
	}

	orphans, err := tx.Assets().Unreferenced(ctx)
	if err != nil {
		return err
	}

	for i := range orphans {
		// An ingest still reads and writes this file; the next emptying takes it.
		if _, aerr := tx.Jobs().ActiveIngestForHash(ctx, orphans[i].Hash); aerr == nil {
			continue
		} else if !isNotFound(aerr) {
			return aerr
		}

		if err := tx.Assets().Delete(ctx, orphans[i].Hash); err != nil {
			return err
		}

		res.Assets++

		plan.hashes = append(plan.hashes, strings.TrimPrefix(orphans[i].Hash, "sha256:"))
	}

	details, merr := json.Marshal(map[string]int{
		"projects": res.Projects, "project_assets": res.ProjectAssets, "assets": res.Assets,
	})
	if merr != nil {
		return merr
	}

	return tx.Audit().Append(ctx, AuditRow{
		AtMs: s.nowMs(), Actor: c.Actor, Action: "trash.empty", Details: details,
	})
}

func (s *svc) deleteTrashedAssetRows(ctx context.Context, tx Tx, c Caller, res *TrashEmptyResult) error {
	after := ""

	for {
		heads, next, err := tx.Projects().List(ctx, ProjectQuery{
			OwnerUserID: c.OwnerUserID, ProjectID: c.ProjectID, After: after, Limit: 200,
		})
		if err != nil {
			return err
		}

		for i := range heads {
			n, derr := tx.Assets().DeleteTrashedProjectAssets(ctx, heads[i].ID)
			if derr != nil {
				return derr
			}

			res.ProjectAssets += int(n)
		}

		if next == "" {
			return nil
		}

		after = next
	}
}

// removeFiles deletes, after the commit, what the emptying left behind and
// returns the bytes freed. A failure leaves an orphan that the sweep removes.
func (s *svc) removeFiles(plan *doomed) int64 {
	var freed int64

	if s.workspace == "" {
		return 0
	}

	for _, hex := range plan.hashes {
		if s.media.Content != nil {
			if p, err := s.media.Content.Path(hex); err == nil {
				if st, serr := os.Stat(p); serr == nil {
					freed += st.Size()
				}
			}

			_ = s.media.Content.Remove(hex)
		}

		freed += removeTree(filepath.Join(s.workspace, "artifacts", hex))
	}

	for _, id := range plan.projects {
		freed += removeTree(filepath.Join(s.workspace, "exports", id))
		freed += removeTree(filepath.Join(s.workspace, "frames", id))
	}

	return freed
}

// removeTree deletes a directory and returns the size of the files in it.
func removeTree(dir string) int64 {
	var size int64

	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // a vanished entry is not a failure of the walk
		}

		if info, ierr := d.Info(); ierr == nil {
			size += info.Size()
		}

		return nil
	})

	_ = os.RemoveAll(dir)

	return size
}
