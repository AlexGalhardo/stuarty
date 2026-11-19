package service

import (
	"context"
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

// libraryRecord is what an import writes to the library side of the files it brings (SDD-011 section 14.4),
// checked and normalized before any file is read.
type libraryRecord struct {
	origin asset.Origin
	info   assets.ImportInfo
}

// checkImportBlocks validates the origin and the info of a MediaImport call, and that every file is a path or a
// hash and not both. A bad block refuses the whole call and nothing is written (SDD-011 section 14.2).
func checkImportBlocks(req *MediaImportRequest) (libraryRecord, error) {
	for i, f := range req.Files {
		switch {
		case f.Hash != "" && f.Path != "":
			return libraryRecord{}, newError(command.CodeValidation, "file %d has both a path and a hash.", i).
				withPath(fmt.Sprintf("files[%d]", i)).withHint("give a path, or the hash of a file of the library")
		case f.Hash == "" && f.Path == "":
			return libraryRecord{}, newError(command.CodeValidation, "file %d has neither a path nor a hash.", i).
				withPath(fmt.Sprintf("files[%d].path", i)).
				withHint("give an absolute path, or the hash of a file of the library")
		case f.Hash != "":
			if _, err := parseHash(f.Hash); err != nil {
				return libraryRecord{}, err
			}
		}
	}

	var info *assets.ImportInfo

	if req.Info != nil {
		info = &assets.ImportInfo{
			Tags: req.Info.Tags, Reusable: req.Info.Reusable, Description: req.Info.Description,
		}
	}

	origin, in, err := assets.CheckImport(req.Origin, info)
	if err != nil {
		return libraryRecord{}, ruleError(err)
	}

	return libraryRecord{origin: origin, info: in}, nil
}

// reuseFile is the import of a file that the library already holds, given by its hash: the use is linked to
// the project with its own identifier and, when the asset is ready, added to the document at once. Nothing is
// copied. In network mode, and for a key limited to a project, the hash must be held by a project the caller can
// see; a hash that is absent and one that is not the caller's answer the same (SDD-011 section 14.3).
func (s *svc) reuseFile(
	ctx context.Context,
	c Caller,
	project *ProjectRow,
	f ImportFile,
	dryRun bool,
	rec *libraryRecord,
) ImportedAsset {
	out := ImportedAsset{State: string(IngestPending), Deduplicated: true}

	refuse := func(err error) ImportedAsset {
		out.State = string(IngestFailed)
		out.Error = fromEngine(err)

		return out
	}

	hash, perr := parseHash(f.Hash)
	if perr != nil {
		return refuse(perr)
	}

	out.Hash = hash

	if f.AssetID != "" && !timeline.ValidID(timeline.PrefixAsset, f.AssetID) {
		return refuse(newError(command.CodeValidation, "the asset id is not valid.").withPath("asset_id").
			withHint("an asset id is ast_ and 10 characters from 0-9 and a-z, without i, l, o and u"))
	}

	notThere := newError(command.CodeValidation, "the library has no file with this hash.").
		withPath("hash").withHint("list the files with asset:list")

	if c.OwnerUserID != "" || c.ProjectID != "" {
		scan, serr := s.libraryScan(ctx, c)
		if serr != nil {
			return refuse(serr)
		}

		if _, held := scan[hash]; !held {
			return refuse(notThere)
		}
	}

	ar, gerr := s.store.Assets().Get(ctx, hash)
	if isNotFound(gerr) {
		return refuse(notThere)
	}

	if gerr != nil {
		return refuse(gerr)
	}

	out.Name, out.Kind = ar.OriginalName, ar.Kind

	if dryRun {
		return out
	}

	s.storeMu.RLock()
	defer s.storeMu.RUnlock()

	var att attached

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()

		row, rerr := tx.Assets().Get(ctx, hash)
		if rerr != nil {
			return rerr
		}

		var aerr error
		if att, aerr = s.attach(ctx, tx, project, row, ar.OriginalName, f.AssetID, c.Actor, now); aerr != nil {
			return aerr
		}

		return assets.RecordImport(ctx, tx.Library(), hash, asset.Origin{Kind: asset.Imported}, rec.info, c.Actor, now)
	})
	if werr != nil {
		if isNotFound(werr) {
			return refuse(notThere)
		}

		return refuse(werr)
	}

	out.AssetID, out.JobID, out.Kind = att.pa.AssetID, att.jobID, att.asset.Kind
	out.newInProject = att.fresh

	if att.addNow != nil {
		if aerr := s.addAsset(ctx, project, *att.addNow, att.asset, ""); aerr != nil {
			out.Error = aerr
		}
	}

	return out
}
