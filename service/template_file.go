package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// templateFileDir is where the files of exported templates are kept, under the
// workspace. It is not under exports/, whose directories are named by project.
const templateFileDir = "templates"

// findTemplateByContent looks for a template the caller can see whose canonical document has the hash.
func (s *svc) findTemplateByContent(ctx context.Context, c Caller, sha string) (TemplateRow, bool) {
	var after string

	for range maxTemplateScanPages {
		rows, next, err := s.store.Templates().List(ctx, maxTemplateLimit, after, c.OwnerUserID)
		if err != nil {
			return TemplateRow{}, false
		}

		for i := range rows {
			if rows[i].DocSHA256 == sha {
				return rows[i], true
			}
		}

		if next == "" {
			break
		}

		after = next
	}

	return TemplateRow{}, false
}

// maxTemplateScanPages bounds the templates read to find a duplicate.
const maxTemplateScanPages = 20

// TemplateImportRequest reads a template file from the machine of the server.
type TemplateImportRequest struct {
	Path   string `json:"path"` // absolute
	DryRun bool   `json:"dry_run,omitempty"`
}

// FileResult names a file the caller can fetch.
type FileResult struct {
	Path        string `json:"path,omitempty"` // local mode only
	DownloadURL string `json:"download_url"`
	Bytes       int64  `json:"bytes"`
	Text        string `json:"text"`
}

func templateFileName(id string) string { return id + ".vukem-template.zip" }

// TemplateExport writes the template file of a template and says where to get
// it (SDD-005 section 15): manifest.json and template.json with flat names.
func (s *svc) TemplateExport(ctx context.Context, req TemplateRef) (*FileResult, error) {
	c, err := authorize(ctx, "TemplateExport", "")
	if err != nil {
		return nil, err
	}

	if err := templateRefused(c); err != nil {
		return nil, err
	}

	rel, size, err := s.writeTemplateFile(ctx, c, req.TemplateID)
	if err != nil {
		return nil, err
	}

	res := &FileResult{
		DownloadURL: "/api/v1/templates/" + req.TemplateID + "/file", Bytes: size,
		Text: "template file of " + req.TemplateID + ", " + formatBytes(size),
	}

	if c.OwnerUserID == "" && s.workspace != "" {
		res.Path = filepath.Join(s.workspace, filepath.FromSlash(rel))
	}

	return res, nil
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}

	return fmt.Sprintf("%d B", n)
}

// writeTemplateFile builds the archive of a visible template and stores it
// under the workspace; it returns the slash path relative to the workspace.
func (s *svc) writeTemplateFile(ctx context.Context, c Caller, id string) (string, int64, error) {
	row, err := s.visibleTemplate(ctx, c, id)
	if err != nil {
		return "", 0, err
	}

	if s.workspace == "" {
		return "", 0, internalError()
	}

	tdoc, derr := timeline.Decode(row.Document)
	if derr != nil {
		return "", 0, internalError()
	}

	fonts, ferr := s.templateFontFiles(ctx, c, tdoc)
	if ferr != nil {
		return "", 0, ferr
	}

	data, berr := buildTemplateArchive(templateManifest{
		Format: templateFormat, TemplateVersion: templateFileVersion, AppVersion: s.version,
		SchemaVersion: row.SchemaVersion, Name: row.Name,
	}, row.Document, nil, fonts)
	if berr != nil {
		return "", 0, internalError()
	}

	if len(data) > templateFileMax {
		return "", 0, newError(command.CodeValidation, "the template file would be over 64 MiB.")
	}

	dir := filepath.Join(s.workspace, templateFileDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", 0, internalError()
	}

	// Written to a temporary name and renamed, so a reader never sees half a file.
	tmp, terr := os.CreateTemp(dir, ".tmp-*")
	if terr != nil {
		return "", 0, internalError()
	}

	if _, werr := tmp.Write(data); werr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return "", 0, internalError()
	}

	if cerr := tmp.Close(); cerr != nil {
		_ = os.Remove(tmp.Name())

		return "", 0, internalError()
	}

	final := filepath.Join(dir, templateFileName(id))
	if rerr := os.Rename(tmp.Name(), final); rerr != nil {
		_ = os.Remove(tmp.Name())

		return "", 0, internalError()
	}

	return templateFileDir + "/" + templateFileName(id), int64(len(data)), nil
}

// openTemplateFile is OpenFile for the template file of a template.
func (s *svc) openTemplateFile(ctx context.Context, c Caller, id string) (*File, error) {
	if err := templateRefused(c); err != nil {
		return nil, err
	}

	rel, size, err := s.writeTemplateFile(ctx, c, id)
	if err != nil {
		return nil, err
	}

	row, gerr := s.store.Templates().Get(ctx, id)
	if gerr != nil {
		return nil, asError(gerr)
	}

	sum := sha256.Sum256(row.Document)

	return &File{
		Path: rel, ContentType: "application/zip", Size: size, ETag: `"tpl-` + hex.EncodeToString(sum[:8]) + `"`,
		ModTime: msTime(row.CreatedMs),
	}, nil
}

// TemplateImport reads a template file under the archive policy of SDD-007
// section 7.6 and stores its template. Nothing is written for a refused file.
func (s *svc) TemplateImport(ctx context.Context, req TemplateImportRequest) (*TemplateResult, error) {
	c, err := authorize(ctx, "TemplateImport", "")
	if err != nil {
		return nil, err
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	if err := templateRefused(c); err != nil {
		return nil, err
	}

	if s.media.Guard == nil {
		return nil, internalError()
	}

	file, gerr := s.media.Guard.OpenExported(req.Path)
	if gerr != nil {
		return nil, fromEngine(gerr)
	}

	defer func() { _ = file.Close() }()

	data, rerr := io.ReadAll(io.LimitReader(file, templateFileMax+1))
	if rerr != nil {
		return nil, newError("E_FILE_NOT_FOUND", "the file cannot be read.").withPath("path")
	}

	contents, aerr := readTemplateArchive(data)
	if aerr != nil {
		return nil, aerr
	}

	doc, verr := migrateTemplateDocument(contents.Document)
	if verr != nil {
		return nil, verr
	}

	if e := checkTemplateFontEntries(doc, contents.Fonts); e != nil {
		return nil, e
	}

	name := contents.Manifest.Name
	if n := utf8.RuneCountInString(name); n < 1 || n > maxProjectName {
		name = doc.Name
	}

	if utf8.RuneCountInString(doc.Template.Description) > maxTemplateText {
		return nil, newError(command.CodeValidation, "the description of the template is too long.")
	}

	doc.Name = name
	canon := timeline.Canonical(doc)

	slotsJSON, merr := json.Marshal(slotInfos(doc))
	if merr != nil {
		return nil, internalError()
	}

	now := s.nowMs()
	row := TemplateRow{
		ID: s.newIDs("tpl"), Name: name, Description: doc.Template.Description, SchemaVersion: doc.SchemaVersion,
		Document: canon, DocSHA256: sha256Hex(canon), Slots: slotsJSON, Summary: summarize(doc),
		CreatedMs: now, CreatedBy: c.Actor,
	}

	if c.OwnerUserID != "" {
		owner := c.OwnerUserID
		row.OwnerUserID = &owner
	}

	// The same file twice is the same template: the existing one is answered and no copy is made (VV-128).
	if existing, found := s.findTemplateByContent(ctx, c, row.DocSHA256); found {
		res := &TemplateResult{Template: templateSummary(&existing, doc), Deduplicated: true}
		res.UntrustedNotice = UntrustedNotice
		res.Text = "template " + existing.ID + " already imported (same content): " + templateLine(&res.Template)

		return res, nil
	}

	res := &TemplateResult{Template: templateSummary(&row, doc)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = "template " + row.ID + " imported: " + templateLine(&res.Template)

	// Fonts go through the font pipeline before the row is written; a refused font ends the import.
	fontHashes, fe := s.storeTemplateFonts(contents.Fonts, req.DryRun)
	if fe != nil {
		return nil, forCaller(c, fe)
	}

	if req.DryRun {
		return res, nil
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Templates().Create(ctx, row); err != nil {
			return err
		}

		// The importer brought these bytes, and each hash matched its entry name, so they own them.
		if err := recordFontOwner(ctx, tx, c, now, fontHashes...); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "template.import", "", row.ID)
	})
	if err != nil {
		return nil, asError(err)
	}

	return res, nil
}
