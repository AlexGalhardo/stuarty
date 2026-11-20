package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The operations of revert, the diff of two revisions and checkpoints (SDD-003 section 2.5.1).

type revertIn struct {
	ID           string `json:"id"                      validate:"required"`
	TimelineID   string `json:"timeline_id,omitempty"                       doc:"The timeline of the project; required when the project has several."`
	Revision     int64  `json:"revision,omitempty"                          doc:"The revision to go back to; or checkpoint."`
	Checkpoint   string `json:"checkpoint,omitempty"                        doc:"The name of a checkpoint to go back to; or revision."`
	BaseRevision int64  `json:"base_revision,omitempty"                     doc:"The revision the caller read; default the current one."`
	DryRun       any    `json:"dry_run,omitempty"`
}

type diffIn struct {
	ID             string `json:"id"                        validate:"required"`
	TimelineID     string `json:"timeline_id,omitempty"                         doc:"The timeline of the project; required when the project has several."`
	From           int64  `json:"from,omitempty"                                doc:"The revision the diff starts from; or from_checkpoint."`
	FromCheckpoint string `json:"from_checkpoint,omitempty"                     doc:"The name of the checkpoint the diff starts from; or from."`
	Until          int64  `json:"until,omitempty"                               doc:"The revision the diff ends at; default the current one."`
}

type checkpointListIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
}

type checkpointCreateIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Name       string `json:"name"                  validate:"required" doc:"1 to 60 characters, no slash."`
	Note       string `json:"note,omitempty"                            doc:"Up to 300 characters."`
	Revision   int64  `json:"revision,omitempty"                        doc:"The revision to name; default the current one."`
}

type checkpointRenameIn struct {
	ID         string  `json:"id"                    validate:"required"`
	TimelineID string  `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Name       string  `json:"name"                  validate:"required" doc:"The checkpoint to rename."`
	NewName    string  `json:"new_name"              validate:"required" doc:"The new name."`
	Note       *string `json:"note,omitempty"                            doc:"Replaces the note when present."`
}

type checkpointRemoveIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Name       string `json:"name"                  validate:"required" doc:"The checkpoint to remove."`
}

// registerHistoryOps binds the revert, diff and checkpoint routes to the Service.
func (h *apiHandler) registerHistoryOps() {
	register(h, "projects.revert", func(c *web.Ctx, in revertIn) (service.HistoryRevertResult, error) {
		dryRun, err := readDryRun(c, in.DryRun)
		if err != nil {
			return service.HistoryRevertResult{}, err
		}

		res, err := h.Service.HistoryRevert(c.Context(), service.HistoryRevertRequest{
			ProjectID: in.ID, TimelineID: in.TimelineID, Revision: in.Revision, Checkpoint: in.Checkpoint,
			BaseRevision: in.BaseRevision, DryRun: dryRun,
		})
		if res != nil {
			revisionHeader(c, res.Revision)
		}

		return deref(res), err
	})
	register(h, "projects.diff", func(c *web.Ctx, in diffIn) (service.HistoryDiffResult, error) {
		res, err := h.Service.HistoryDiff(c.Context(), service.HistoryDiffRequest{
			ProjectID:      in.ID,
			TimelineID:     in.TimelineID,
			From:           in.From,
			FromCheckpoint: in.FromCheckpoint,
			Until:          in.Until,
		})

		return deref(res), err
	})
	register(
		h,
		"projects.checkpoints.list",
		func(c *web.Ctx, in checkpointListIn) (service.CheckpointListResult, error) {
			res, err := h.Service.CheckpointList(
				c.Context(),
				service.CheckpointListRequest{ProjectID: in.ID, TimelineID: in.TimelineID},
			)

			return deref(res), err
		},
	)
	register(h, "projects.checkpoints.create", func(c *web.Ctx, in checkpointCreateIn) (service.Checkpoint, error) {
		res, err := h.Service.CheckpointCreate(c.Context(), service.CheckpointCreateRequest{
			ProjectID: in.ID, TimelineID: in.TimelineID, Name: in.Name, Note: in.Note, Revision: in.Revision,
		})

		return deref(res), err
	})
	register(h, "projects.checkpoints.rename", func(c *web.Ctx, in checkpointRenameIn) (service.Checkpoint, error) {
		res, err := h.Service.CheckpointRename(c.Context(), service.CheckpointRenameRequest{
			ProjectID: in.ID, TimelineID: in.TimelineID, Name: in.Name, NewName: in.NewName, Note: in.Note,
		})

		return deref(res), err
	})
	register(h, "projects.checkpoints.remove", func(c *web.Ctx, in checkpointRemoveIn) (service.Checkpoint, error) {
		res, err := h.Service.CheckpointRemove(
			c.Context(),
			service.CheckpointRemoveRequest{ProjectID: in.ID, TimelineID: in.TimelineID, Name: in.Name},
		)

		return deref(res), err
	})
}
