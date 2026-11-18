package service

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/command"
)

// MediaTrash moves an asset of a project to the trash (SDD-005 section 13.2).
// An active asset leaves the document through the batch asset.remove issued as
// the caller, so the log records it and undo reaches the same state; a pending
// one has no document to edit and its row changes directly. No file is removed.
func (s *svc) MediaTrash(ctx context.Context, req MediaTrashRequest) (*MediaStateResult, error) {
	c, err := authorize(ctx, "MediaTrash", req.ProjectID)
	if err != nil {
		return nil, err
	}

	project, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	pa, aerr := s.projectAsset(ctx, req.ProjectID, req.AssetID)
	if aerr != nil {
		return nil, aerr
	}

	res := &MediaStateResult{AssetID: pa.AssetID, State: string(ProjectAssetTrashed), Text: pa.AssetID + " trashed"}

	switch pa.State {
	case ProjectAssetTrashed:
		return res, nil
	case ProjectAssetActive:
		args, merr := json.Marshal(map[string]string{"asset_id": pa.AssetID})
		if merr != nil {
			return nil, internalError()
		}

		if err := s.assetBatch(ctx, c, &project, "asset.remove", args, req.DryRun); err != nil {
			return nil, err
		}

		if aerr := s.auditAssetAction(ctx, c.Actor, "media.trash", project.ID, pa.AssetID, req.DryRun); aerr != nil {
			return nil, aerr
		}

		return res, nil
	case ProjectAssetPending:
		// No document holds it yet: the row changes below, without a batch.
	}

	if project.State == ProjectTrashed {
		return nil, projectInTrash(project.ID)
	}

	if req.DryRun {
		return res, nil
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		if err := tx.Assets().
			SetProjectAssetState(ctx, project.ID, pa.AssetID, ProjectAssetTrashed, c.Actor, now, nil); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "media.trash", project.ID, pa.AssetID)
	})
	if err != nil {
		return nil, asError(err)
	}

	s.emitAssetFor(project.ID, pa.AssetID, IngestPending, 0, nil)

	return res, nil
}

// MediaRestore brings a trashed asset back. A row that was in the document
// returns through the internal asset.add with its stored snapshot, so the same
// identifier comes back; one that never was goes back to pending, and is added
// at once when its file is already ingested.
func (s *svc) MediaRestore(ctx context.Context, req MediaTrashRequest) (*MediaStateResult, error) {
	c, err := authorize(ctx, "MediaRestore", req.ProjectID)
	if err != nil {
		return nil, err
	}

	project, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	pa, aerr := s.projectAsset(ctx, req.ProjectID, req.AssetID)
	if aerr != nil {
		return nil, aerr
	}

	if pa.State != ProjectAssetTrashed {
		return &MediaStateResult{
			AssetID: pa.AssetID, State: string(pa.State), Text: pa.AssetID + " " + string(pa.State),
		}, nil
	}

	if project.State == ProjectTrashed {
		return nil, projectInTrash(project.ID)
	}

	res := &MediaStateResult{AssetID: pa.AssetID, State: string(ProjectAssetActive), Text: pa.AssetID + " restored"}

	if len(pa.Snapshot) > 0 && string(pa.Snapshot) != "null" {
		args, merr := json.Marshal(map[string]json.RawMessage{"asset": pa.Snapshot})
		if merr != nil {
			return nil, internalError()
		}

		if err := s.assetBatch(ctx, c, &project, "asset.add", args, req.DryRun); err != nil {
			return nil, err
		}

		if aerr := s.auditAssetAction(ctx, c.Actor, "media.restore", project.ID, pa.AssetID, req.DryRun); aerr != nil {
			return nil, aerr
		}

		return res, nil
	}

	res.State = string(ProjectAssetPending)
	if req.DryRun {
		return res, nil
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		if err := tx.Assets().
			SetProjectAssetState(ctx, project.ID, pa.AssetID, ProjectAssetPending, c.Actor, now, nil); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "media.restore", project.ID, pa.AssetID)
	})
	if err != nil {
		return nil, asError(err)
	}

	ar, gerr := s.store.Assets().Get(ctx, pa.AssetHash)
	if gerr != nil {
		return nil, asError(gerr)
	}

	s.emitAssetFor(project.ID, pa.AssetID, ar.IngestState, ar.Progress, nil)

	if ar.IngestState == IngestReady {
		pa.State = ProjectAssetPending
		if aerr := s.addAsset(ctx, &project, pa, ar, ""); aerr != nil {
			return nil, aerr
		}

		res.State = string(ProjectAssetActive)
	}

	return res, nil
}

// auditAssetAction records a trash or restore that went through a batch. The
// batch commits in its own transaction (the commit of SDD-005 section 9), so
// the row follows in a second one; the log row of the batch stays the record of
// the edit itself.
func (s *svc) auditAssetAction(ctx context.Context, actor, action, projectID, assetID string, dryRun bool) error {
	if dryRun {
		return nil
	}

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, s.nowMs(), actor, action, projectID, assetID)
	})

	return asError(err)
}

func (s *svc) projectAsset(ctx context.Context, projectID, assetID string) (ProjectAssetRow, error) {
	pa, err := s.store.Assets().GetProjectAsset(ctx, projectID, assetID)
	if err != nil {
		if !isNotFound(err) {
			return ProjectAssetRow{}, asError(err)
		}

		return ProjectAssetRow{}, newError(command.CodeNotFound, "the project has no asset %s.", assetID).
			withHint("list the assets with media_inspect and no asset_id")
	}

	return pa, nil
}

// assetBatch issues one asset command as the caller. The method already
// checked the media scope that the command needs; the edit scope of Edit is
// lifted for this one batch.
func (s *svc) assetBatch(
	ctx context.Context,
	c Caller,
	project *ProjectRow,
	op string,
	args json.RawMessage,
	dryRun bool,
) error {
	head, err := s.store.Projects().Get(ctx, project.ID)
	if err != nil {
		return asError(err)
	}

	if !slices.Contains(c.Scopes, ScopeEdit) {
		c.Scopes = append(slices.Clone(c.Scopes), ScopeEdit)
	}

	_, err = s.edit(WithCaller(ctx, c), EditRequest{
		ProjectID: project.ID, BaseRevision: head.Revision, DryRun: dryRun,
		Operations: []command.Operation{{Op: op, Args: args}},
	}, true)

	return err
}

func projectInTrash(id string) *Error {
	return newError(command.CodeInvalidState, "the project %s is in the trash.", id).withHint("%s", hintRestore)
}

// reconcilePending adds the assets that waited for the project to leave the
// trash: rows still pending whose file finished ingesting meanwhile.
func (s *svc) reconcilePending(ctx context.Context, projectID string) {
	pending, err := s.store.Assets().ListProjectAssets(ctx, projectID, ProjectAssetPending)
	if err != nil {
		return
	}

	head, err := s.store.Projects().Get(ctx, projectID)
	if err != nil || head.State == ProjectTrashed {
		return
	}

	for i := range pending {
		ar, gerr := s.store.Assets().Get(ctx, pending[i].AssetHash)
		if gerr != nil || ar.IngestState != IngestReady {
			continue
		}

		// A refusal leaves the row pending; the next import or restore retries.
		_ = s.addAsset(ctx, &head, pending[i], ar, "")
	}
}
