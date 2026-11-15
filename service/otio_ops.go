package service

import (
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/otio"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// otioFileMax is the size of the largest .otio file an import reads (SDD-001 section 9).
const otioFileMax = 16 << 20

// ProjectOtioExportRequest asks for the OpenTimelineIO file of the head revision (SDD-001 section 9).
type ProjectOtioExportRequest struct {
	ProjectID string `json:"project_id"`
}

// ProjectOtioExportResult is the .otio text and what it could not carry.
type ProjectOtioExportResult struct {
	ProjectID string      `json:"project_id"`
	Revision  int64       `json:"revision"`
	FileName  string      `json:"file_name"` // a safe name, ending in .otio
	Content   string      `json:"content"`   // the file, deterministic JSON
	Lost      []otio.Lost `json:"lost"`
	Text      string      `json:"text"`
}

// ProjectOtioImportRequest reads an .otio file from the machine of the server and applies it to a
// project as one batch.
type ProjectOtioImportRequest struct {
	ProjectID    string `json:"project_id"`
	BaseRevision int64  `json:"base_revision"`
	Path         string `json:"path"` // absolute
	DryRun       bool   `json:"dry_run,omitempty"`
}

// ProjectOtioImportResult is the batch the file became and what it left out.
type ProjectOtioImportResult struct {
	ProjectID string `json:"project_id"`
	Revision  int64  `json:"revision"`
	Applied   bool   `json:"applied"`
	// Edit is the result of the batch applied through Edit: one undo step.
	Edit *EditResult `json:"edit"`
	// UnresolvedMedia is every reference of the file that no asset of the project answers.
	UnresolvedMedia []otio.Media `json:"unresolved_media"`
	// Lost is what the batch cannot carry, and the clips it left out.
	Lost []otio.Lost `json:"lost"`
	Text string      `json:"text"`
}

// ProjectOtioExport converts the head revision of a project to OpenTimelineIO. It writes nothing.
func (s *svc) ProjectOtioExport(ctx context.Context, req ProjectOtioExportRequest) (*ProjectOtioExportResult, error) {
	c, err := authorize(ctx, "ProjectOtioExport", req.ProjectID)
	if err != nil {
		return nil, err
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	data, lost, xerr := otio.Export(doc)
	if xerr != nil {
		return nil, internalError()
	}

	if lost == nil {
		lost = []otio.Lost{}
	}

	return &ProjectOtioExportResult{
		ProjectID: row.ID, Revision: row.Revision, FileName: otioFileName(doc.Name), Content: string(data), Lost: lost,
		Text: "exported " + row.ID + " r" + strconv.FormatInt(row.Revision, 10) + " to OpenTimelineIO; " +
			strconv.Itoa(len(lost)) + " lost item(s)",
	}, nil
}

// ProjectOtioImport parses an .otio file and applies what it describes through Edit, so the
// import is one undo step by the actor of the caller and every rule of the command core holds.
// No media reference is opened: a reference is matched by name to an asset the project has.
func (s *svc) ProjectOtioImport(ctx context.Context, req ProjectOtioImportRequest) (*ProjectOtioImportResult, error) {
	c, err := authorize(ctx, "ProjectOtioImport", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	if s.media.Guard == nil {
		return nil, internalError()
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if row.State == ProjectTrashed {
		return nil, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	im, ierr := s.readOtio(req.Path)
	if ierr != nil {
		return nil, ierr
	}

	if im.FPS != doc.Settings.FPS {
		return nil, newError(command.CodeValidation, "the file is at %s fps and the project at %s fps.",
			fpsString(im.FPS.Num, im.FPS.Den), fpsString(doc.Settings.FPS.Num, doc.Settings.FPS.Den)).
			withPath("path").withHint("set the frame rate of the project to the one of the file, then import again")
	}

	assets, unresolved := matchOtioMedia(doc, im.Media)

	ops, dropped := im.Operations(assets, s.newIDs)
	lost := append(slices.Clone(im.Lost), dropped...)

	if len(ops) == 0 {
		return nil, newError(command.CodeValidation, "the file has nothing this server can import.").
			withPath("path").withDetail("lost", lost).withDetail("unresolved_media", unresolved)
	}

	if len(ops) > maxOperationsPerBatch {
		return nil, newError(CodePayloadTooLarge, "the file needs %d operations; a batch has at most %d.",
			len(ops), maxOperationsPerBatch).withPath("path").withHint("split the timeline into smaller files")
	}

	edit, eerr := s.edit(ctx, EditRequest{
		ProjectID: req.ProjectID, BaseRevision: req.BaseRevision, Operations: ops, DryRun: req.DryRun,
	}, false)
	if eerr != nil {
		return nil, eerr
	}

	return &ProjectOtioImportResult{
		ProjectID: row.ID, Revision: edit.Revision, Applied: edit.Applied, Edit: edit, UnresolvedMedia: unresolved,
		Lost: lost,
		Text: "imported " + strconv.Itoa(len(ops)) + " operation(s) into " + row.ID + " r" +
			strconv.FormatInt(edit.Revision, 10) + "; " + strconv.Itoa(len(unresolved)) + " unresolved media, " +
			strconv.Itoa(len(lost)) + " lost item(s)",
	}, nil
}

// readOtio opens the file with the import path rules and parses it.
func (s *svc) readOtio(path string) (*otio.Import, error) {
	file, gerr := s.media.Guard.OpenImport(path)
	if gerr != nil {
		return nil, fromEngine(gerr)
	}

	defer func() { _ = file.Close() }()

	data, rerr := io.ReadAll(io.LimitReader(file, otioFileMax+1))
	if rerr != nil {
		return nil, newError("E_FILE_NOT_FOUND", "the file cannot be read.").withPath("path")
	}

	if len(data) > otioFileMax {
		return nil, newError(CodePayloadTooLarge, "the OpenTimelineIO file is over %d bytes.", otioFileMax).
			withPath("path").withHint("export a smaller timeline")
	}

	im, perr := otio.Parse(data)
	if perr != nil {
		if errors.Is(perr, otio.ErrInvalid) {
			return nil, newError(command.CodeValidation, "the file is not an OpenTimelineIO timeline this server reads.").
				withPath("path").withHint("export a Timeline with one Stack of Tracks")
		}

		return nil, internalError()
	}

	return im, nil
}

// matchOtioMedia maps each candidate reference to the asset of the document with that name: the
// reference as written or its last path segment, in the form displayName gives an imported file.
// The lowest id wins when two assets share a name. A reference with no asset is unresolved.
func matchOtioMedia(doc *timeline.Document, media []otio.Media) (map[string]string, []otio.Media) {
	ids := make([]string, 0, len(doc.Assets))
	for id := range doc.Assets {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	byName := make(map[string]string, len(ids))

	for _, id := range ids {
		if _, ok := byName[doc.Assets[id].Name]; !ok {
			byName[doc.Assets[id].Name] = id
		}
	}

	assets := map[string]string{}
	unresolved := []otio.Media{}

	for _, m := range media {
		if m.Status != otio.Candidate {
			unresolved = append(unresolved, m)

			continue
		}

		id, ok := byName[m.Ref]
		if !ok {
			id, ok = byName[displayName(m.Ref)]
		}

		if !ok {
			unresolved = append(unresolved, otio.Media{
				Ref: m.Ref, Status: otio.Unresolved,
				Reason: "no asset of the project has this name; import the file with media:import, then import again",
			})

			continue
		}

		assets[m.Ref] = id
	}

	return assets, unresolved
}

// otioFileName is the project name reduced to letters, digits, dash and underscore.
func otioFileName(name string) string {
	var b strings.Builder

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}

		if b.Len() >= 60 {
			break
		}
	}

	base := strings.Trim(b.String(), "_")
	if base == "" {
		base = "project"
	}

	return base + ".otio"
}
