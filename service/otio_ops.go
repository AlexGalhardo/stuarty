package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/otio"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// otioFileMax is the size of the largest .otio file an import reads (SDD-001 section 9).
const otioFileMax = 16 << 20

// ProjectOtioExportRequest asks for the OpenTimelineIO file of the head revision (SDD-001 section 9).
type ProjectOtioExportRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
}

// OtioLost is an item an OpenTimelineIO conversion could not carry. Its name and feature come from
// a project or from an imported file, so they are untrusted (SDD-007 section 8.1).
type OtioLost struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty" untrusted:"true"`
	Feature string `json:"feature"        untrusted:"true"`
}

// OtioMedia is a media reference of an imported file. The reference and the reason come from the
// file, so they are untrusted.
type OtioMedia struct {
	Ref    string `json:"ref"              untrusted:"true"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty" untrusted:"true"`
}

// OtioMatch is a media reference of an imported file answered by an asset of the project because
// the names are equal. The file carries no content hash, so the match is not proof of identity (VV-025).
type OtioMatch struct {
	Ref     string `json:"ref"      untrusted:"true"`
	AssetID string `json:"asset_id"`
	Matched string `json:"matched"` // always "by_name"
}

// otioText is the budget of a name, a feature or a reference of an OTIO result.
const otioText = untrusted.AssetNameFull

func otioLost(in []otio.Lost) []OtioLost {
	out := make([]OtioLost, len(in))
	for i, l := range in {
		out[i] = OtioLost{
			ID: l.ID, Name: untrusted.Value(l.Name, otioText), Feature: untrusted.Value(l.Feature, otioText),
		}
	}

	return out
}

func otioMedia(in []otio.Media) []OtioMedia {
	out := make([]OtioMedia, len(in))
	for i, m := range in {
		out[i] = OtioMedia{
			Ref: untrusted.Value(m.Ref, otioText), Status: m.Status, Reason: untrusted.Value(m.Reason, otioText),
		}
	}

	return out
}

// ProjectOtioExportResult is the .otio text and what it could not carry.
type ProjectOtioExportResult struct {
	Envelope
	ProjectID string     `json:"project_id"`
	Revision  int64      `json:"revision"`
	FileName  string     `json:"file_name"  untrusted:"true"` // a safe name, ending in .otio
	Content   string     `json:"content"`                     // the file, deterministic JSON
	Lost      []OtioLost `json:"lost"`
	Text      string     `json:"text"`
}

// ProjectOtioImportRequest reads an .otio file from the machine of the server and applies it to a
// project as one batch.
type ProjectOtioImportRequest struct {
	ProjectID    string `json:"project_id"`
	TimelineID   string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	BaseRevision int64  `json:"base_revision"`
	Path         string `json:"path"` // absolute
	DryRun       bool   `json:"dry_run,omitempty"`
	// AllowPartial imports what the project can take when the file refers to media the project does not have;
	// without it such a file is refused and nothing is applied (VV-133).
	AllowPartial bool `json:"allow_partial,omitempty"`
}

// ProjectOtioImportResult is the batch the file became and what it left out.
type ProjectOtioImportResult struct {
	Envelope
	ProjectID string `json:"project_id"`
	Revision  int64  `json:"revision"`
	Applied   bool   `json:"applied"`
	// Edit is the result of the batch applied through Edit: one undo step.
	Edit *EditResult `json:"edit"`
	// UnresolvedMedia is every reference of the file that no asset of the project answers.
	UnresolvedMedia []OtioMedia `json:"unresolved_media"`
	// MatchedMedia is every reference answered by an asset of the same name, not by content.
	MatchedMedia []OtioMatch `json:"matched_media"`
	// Lost is what the batch cannot carry, and the clips it left out.
	Lost []OtioLost `json:"lost"`
	// Warnings are notes about the import that are not a reason to refuse it.
	Warnings []string `json:"warnings"`
	Text     string   `json:"text"`
}

// ProjectOtioExport converts the head revision of a project to OpenTimelineIO. It writes nothing.
func (s *svc) ProjectOtioExport(ctx context.Context, req ProjectOtioExportRequest) (*ProjectOtioExportResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectOtioExport", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
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

	return &ProjectOtioExportResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       row.ID, Revision: row.Revision, FileName: untrusted.Value(otioFileName(doc.Name), otioText),
		Content: string(data), Lost: otioLost(lost),
		Text: "exported " + row.ID + " r" + strconv.FormatInt(row.Revision, 10) + " to OpenTimelineIO; " +
			strconv.Itoa(len(lost)) + " lost item(s)",
	}, nil
}

// ProjectOtioImport parses an .otio file and applies what it describes through Edit, so the
// import is one undo step by the actor of the caller and every rule of the command core holds.
// No media reference is opened: a reference is matched by name to an asset the project has.
func (s *svc) ProjectOtioImport(ctx context.Context, req ProjectOtioImportRequest) (*ProjectOtioImportResult, error) {
	c, err := s.authorizeProject(ctx, "ProjectOtioImport", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
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

	assets, unresolved, matched := matchOtioMedia(doc, im.Media)

	for family := range doc.Fonts {
		im.KnownFonts = append(im.KnownFonts, family)
	}

	for _, m := range doc.Markers {
		im.MarkedFrames = append(im.MarkedFrames, m.Frame)
	}

	if len(unresolved) > 0 && !req.AllowPartial && !req.DryRun && req.BaseRevision <= row.Revision {
		// The names are in details, cleaned and tagged as untrusted; the message carries none of them.
		return nil, newError(command.CodeInvalidState,
			"the file refers to %d media that the project does not have.", len(unresolved)).
			withPath("path").withDetail("unresolved_media", otioMedia(unresolved)).
			withHint("import the media with media:import (it is matched by file name) and import the file again, " +
				"or set allow_partial to import the rest now and place the clips yourself; a dry run lists what is missing")
	}

	ops, dropped := im.Operations(assets, s.newIDs)
	lost := append(slices.Clone(im.Lost), dropped...)

	if len(ops) == 0 {
		return nil, newError(command.CodeValidation, "the file has nothing this server can import.").
			withPath("path").withDetail("lost", otioLost(lost)).withDetail("unresolved_media", otioMedia(unresolved))
	}

	if len(ops) > maxOperationsPerBatch {
		return nil, newError(CodePayloadTooLarge, "the file needs %d operations; a batch has at most %d.",
			len(ops), maxOperationsPerBatch).withPath("path").withHint("split the timeline into smaller files").
			withLimit("batch_operations", len(ops), maxOperationsPerBatch)
	}

	edit, eerr := s.edit(ctx, EditRequest{
		ProjectID: req.ProjectID, TimelineID: req.ProjectID, BaseRevision: req.BaseRevision, Operations: ops,
		DryRun: req.DryRun,
	}, false)
	if eerr != nil {
		return nil, eerr
	}

	verb := "imported "
	if !edit.Applied {
		verb = "would import "
	}

	return &ProjectOtioImportResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       row.ID, Revision: edit.Revision, Applied: edit.Applied, Edit: edit,
		UnresolvedMedia: otioMedia(unresolved), MatchedMedia: otioMatches(matched), Lost: otioLost(lost),
		Warnings: otioWarnings(matched),
		Text: verb + strconv.Itoa(len(ops)) + " operation(s) into " + row.ID + " r" +
			strconv.FormatInt(edit.Revision, 10) + "; " + strconv.Itoa(len(unresolved)) + " unresolved media, " +
			strconv.Itoa(len(lost)) + " lost item(s)" + noClipsNote(ops, len(unresolved)),
	}, nil
}

// otioMatches turns the by-name matches into the members of the result, in the order of the file.
func otioMatches(in []otioMatch) []OtioMatch {
	out := make([]OtioMatch, len(in))
	for i, m := range in {
		out[i] = OtioMatch{Ref: untrusted.Value(m.ref, otioText), AssetID: m.asset, Matched: "by_name"}
	}

	return out
}

// otioWarnings says once that the media was matched by file name. It carries no name from the file.
func otioWarnings(matched []otioMatch) []string {
	if len(matched) == 0 {
		return []string{}
	}

	return []string{strconv.Itoa(len(matched)) + " media reference(s) were matched to assets of the project by " +
		"file name, not by content: the file carries no hash, so check matched_media that each asset is the " +
		"footage you meant"}
}

// otioMatch is a reference and the asset a name equality chose for it.
type otioMatch struct{ ref, asset string }

// noClipsNote explains an import that placed tracks but no clip because no media reference
// of the file matched an asset: media is matched by asset name, and nothing is opened.
func noClipsNote(ops []command.Operation, unresolved int) string {
	if unresolved == 0 ||
		slices.ContainsFunc(ops, func(o command.Operation) bool { return o.Op == "clip.add" || o.Op == "audio.add" }) {
		return ""
	}

	return "; no clip was placed: media is matched by asset name (the target_url of the file or its last path " +
		"segment), so import the media with media:import first, then import the file again"
}

// otioEnvelope is the answer of ProjectOtioExport: the file is its content member. A saved answer is accepted
// as the file, so the round trip needs no editing (VV-108).
func otioEnvelope(data []byte) []byte {
	var env struct {
		FileName string `json:"file_name"`
		Content  string `json:"content"`
	}

	if json.Unmarshal(data, &env) == nil && env.Content != "" && env.FileName != "" {
		return []byte(env.Content)
	}

	return data
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
			withPath("path").withHint("export a smaller timeline").withLimit("otio_bytes", len(data), otioFileMax)
	}

	im, perr := otio.Parse(otioEnvelope(data))
	if perr != nil {
		if errors.Is(perr, otio.ErrInvalid) {
			return nil, newError(
				command.CodeValidation,
				"the file is not an OpenTimelineIO timeline this server reads.",
			).
				withPath("path").
				withHint("export a Timeline with one Stack of Tracks")
		}

		return nil, internalError()
	}

	return im, nil
}

// matchOtioMedia maps each candidate reference to the asset of the document with that name: the
// reference as written or its last path segment, in the form displayName gives an imported file.
// The lowest id wins when two assets share a name. A reference with no asset is unresolved.
func matchOtioMedia(doc *timeline.Document, media []otio.Media) (map[string]string, []otio.Media, []otioMatch) {
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

	var matched []otioMatch

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

		if _, seen := assets[m.Ref]; !seen {
			matched = append(matched, otioMatch{ref: m.Ref, asset: id})
		}

		assets[m.Ref] = id
	}

	return assets, unresolved, matched
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
