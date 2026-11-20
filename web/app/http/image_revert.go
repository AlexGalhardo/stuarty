package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The revert, the diff and the checkpoints of an image document (SDD-003 section 2.5.1, SDD-021 section 4): the
// operations of a timeline's, with the id of the image in the path.

type imageRevertIn struct {
	ID           string `json:"id"                      validate:"required"`
	Revision     int64  `json:"revision,omitempty"                          doc:"The revision to go back to; or checkpoint."`
	Checkpoint   string `json:"checkpoint,omitempty"                        doc:"The name of a checkpoint to go back to; or revision."`
	BaseRevision int64  `json:"base_revision,omitempty"                     doc:"The revision the caller read; default the current one."`
	DryRun       any    `json:"dry_run,omitempty"`
}

type imageDiffIn struct {
	ID             string `json:"id"                        validate:"required"`
	From           int64  `json:"from,omitempty"                                doc:"The revision the diff starts from; or from_checkpoint."`
	FromCheckpoint string `json:"from_checkpoint,omitempty"                     doc:"The name of the checkpoint the diff starts from; or from."`
	Until          int64  `json:"until,omitempty"                               doc:"The revision the diff ends at; default the current one."`
}

type imageCheckpointListIn struct {
	ID string `json:"id" validate:"required"`
}

type imageCheckpointCreateIn struct {
	ID       string `json:"id"                 validate:"required"`
	Name     string `json:"name"               validate:"required" doc:"1 to 60 characters, no slash."`
	Note     string `json:"note,omitempty"                         doc:"Up to 300 characters."`
	Revision int64  `json:"revision,omitempty"                     doc:"The revision to name; default the current one."`
}

type imageCheckpointRenameIn struct {
	ID      string  `json:"id"             validate:"required"`
	Name    string  `json:"name"           validate:"required" doc:"The checkpoint to rename."`
	NewName string  `json:"new_name"       validate:"required" doc:"The new name."`
	Note    *string `json:"note,omitempty"                     doc:"Replaces the note when present."`
}

type imageCheckpointRemoveIn struct {
	ID   string `json:"id"   validate:"required"`
	Name string `json:"name" validate:"required" doc:"The checkpoint to remove."`
}

// registerImageHistoryOps binds the revert, diff and checkpoint routes of an image to the Service.
func (h *apiHandler) registerImageHistoryOps() {
	register(h, "images.revert", func(c *web.Ctx, in imageRevertIn) (service.ImageRevertResult, error) {
		dryRun, err := readDryRun(c, in.DryRun)
		if err != nil {
			return service.ImageRevertResult{}, err
		}

		res, err := h.Service.ImageRevert(c.Context(), service.ImageRevertRequest{
			ImageID: in.ID, Revision: in.Revision, Checkpoint: in.Checkpoint, BaseRevision: in.BaseRevision,
			DryRun: dryRun,
		})
		if res != nil {
			revisionHeader(c, res.Revision)
		}

		return deref(res), err
	})
	register(h, "images.diff", func(c *web.Ctx, in imageDiffIn) (service.ImageDiffResult, error) {
		res, err := h.Service.ImageDiff(c.Context(), service.ImageDiffRequest{
			ImageID: in.ID, From: in.From, FromCheckpoint: in.FromCheckpoint, Until: in.Until,
		})

		return deref(res), err
	})
	register(
		h,
		"images.checkpoints.list",
		func(c *web.Ctx, in imageCheckpointListIn) (service.ImageCheckpointListResult, error) {
			res, err := h.Service.ImageCheckpointList(c.Context(), service.ImageCheckpointListRequest{ImageID: in.ID})

			return deref(res), err
		},
	)
	register(
		h,
		"images.checkpoints.create",
		func(c *web.Ctx, in imageCheckpointCreateIn) (service.Checkpoint, error) {
			res, err := h.Service.ImageCheckpointCreate(c.Context(), service.ImageCheckpointCreateRequest{
				ImageID: in.ID, Name: in.Name, Note: in.Note, Revision: in.Revision,
			})

			return deref(res), err
		},
	)
	register(
		h,
		"images.checkpoints.rename",
		func(c *web.Ctx, in imageCheckpointRenameIn) (service.Checkpoint, error) {
			res, err := h.Service.ImageCheckpointRename(c.Context(), service.ImageCheckpointRenameRequest{
				ImageID: in.ID, Name: in.Name, NewName: in.NewName, Note: in.Note,
			})

			return deref(res), err
		},
	)
	register(
		h,
		"images.checkpoints.remove",
		func(c *web.Ctx, in imageCheckpointRemoveIn) (service.Checkpoint, error) {
			res, err := h.Service.ImageCheckpointRemove(
				c.Context(),
				service.ImageCheckpointRemoveRequest{ImageID: in.ID, Name: in.Name},
			)

			return deref(res), err
		},
	)
}
