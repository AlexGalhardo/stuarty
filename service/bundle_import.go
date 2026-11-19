package service

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	"github.com/AlexGalhardo/stuarty/core/writing"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	assetlib "github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// The limits of a bundle (SDD-005 section 16.3 step 1, SDD-007 section 7.6).
const (
	bundleEntriesMax     = 1024
	bundleTotalMax       = 32 << 30
	bundleManifestMax    = 1 << 20
	bundleDocumentMax    = 64 << 20
	bundleOperationsMax  = 2 << 30
	bundleLineMax        = 64 << 20
	bundleBombRatio      = 1000
	bundleBombFloor      = 64 << 10
	bundleDiskMarginPerc = 10
)

// bundleEntryName is every name a bundle may hold. The names are flat and lower case, so an entry
// with a path, a drive, a dot segment, a control character or a differently cased twin is not in it.
var bundleEntryName = regexp.MustCompile(
	`^(manifest\.json|document\.json|operations\.ndjson|timeline\.t[0-9]{1,3}\.(json|ndjson)|item\.[0-9]{1,3}\.md|(media|font)\.[0-9a-f]{64})$`,
)

var bundleHashRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ProjectBundleImportRequest reads a .vvbundle from the machine of the server (SDD-005 section 16.3).
type ProjectBundleImportRequest struct {
	Path string `json:"path"` // absolute
	// AsNew false keeps the project id (E_ID_EXISTS when it exists, even trashed); true makes a
	// new id and leaves the log out.
	AsNew  bool `json:"as_new,omitempty"`
	DryRun bool `json:"dry_run,omitempty"`
}

func bundleRefused(reason string) *Error {
	return newError(command.CodeValidation, "the bundle is refused: %s.", reason).
		withHint("export the project again with bundle:export").withDetail("reason", reason)
}

func bundleOverLimit(limit, reason string, value, ceiling uint64) *Error {
	return newError("E_LIMIT_EXCEEDED", "the bundle is over a limit: %s.", reason).
		withHint("export a smaller project, or ask the person running the server to raise the import limit").
		withLimit(limit, value, ceiling).withDetail("reason", reason)
}

// bundleParts is a bundle after the policy and the checks, still unwritten.
type bundleParts struct {
	zr       *zip.Reader
	entries  map[string]*zip.File
	manifest BundleManifest
	document []byte
	doc      *timeline.Document
	migrated bool
	hasLog   bool
	// files are the media and font entries to copy: entry name to hash and size.
	files []bundleFile
	// fromVersion is the schema version of document.json before the migration.
	fromVersion int
	logSum      string
	// main is the first timeline as a whole and extra the others (version 2); main is set when every check passed.
	main  *bundleTimeline
	extra []*bundleTimeline
	// texts are the Markdown of the items of version 2, by index of the item; an item with no entry has none.
	texts map[int]string
}

// ProjectBundleImport reads a bundle, checks all of it and then stores the project in one transaction.
func (s *svc) ProjectBundleImport(ctx context.Context, req ProjectBundleImportRequest) (*ProjectCreateResult, error) {
	c, err := authorizeScope(ctx, ScopeEdit, "")
	if err != nil {
		return nil, err
	}

	if _, err = authorizeScope(ctx, ScopeMedia, ""); err != nil {
		return nil, err
	}

	if c.ProjectID != "" {
		return nil, newError(CodeForbidden, "the key is limited to one project and cannot create projects.").
			withHint("use a key without a project limit")
	}

	if c.OwnerUserID != "" && !req.AsNew {
		return nil, newError(command.CodeValidation, "the id of a project is chosen by the server.").
			withPath("as_new").withHint("set as_new to true")
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	if s.media.Guard == nil || s.media.Content == nil {
		return nil, internalError()
	}

	file, gerr := s.media.Guard.OpenExported(req.Path)
	if gerr != nil {
		return nil, fromEngine(gerr)
	}

	defer func() { _ = file.Close() }()

	st, serr := file.Stat()
	if serr != nil || !st.Mode().IsRegular() {
		return nil, newError("E_FILE_NOT_FOUND", "the file cannot be read.").withPath("path")
	}

	zr, zerr := zip.NewReader(file, st.Size())
	if zerr != nil {
		return nil, bundleRefused("it is not a readable zip")
	}

	parts, perr := s.readBundle(ctx, c, zr)
	if perr != nil {
		return nil, forCaller(c, perr)
	}

	projectID := parts.doc.ID
	if req.AsNew {
		projectID = s.newIDs(timeline.PrefixProject)
	} else if _, herr := s.store.Projects().Head(ctx, projectID); herr == nil {
		return nil, newError(command.CodeIDExists, "a project with the identifier %s already exists.", projectID).
			withHint("import it with as_new to get a new id")
	} else if !isNotFound(herr) {
		return nil, asError(herr)
	}

	doc := parts.doc
	doc.ID = projectID

	if err := s.numberImportedTimelines(ctx, parts, req.AsNew); err != nil {
		return nil, err
	}

	revision := parts.main.newRevision(req.AsNew)

	res := &ProjectCreateResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       projectID, Name: doc.Name, Revision: revision, Width: doc.Settings.Width,
		Height: doc.Settings.Height, FPS: fpsString(doc.Settings.FPS.Num, doc.Settings.FPS.Den),
		Text: textview.Created(projectID, doc.Name, doc.Settings.Width, doc.Settings.Height,
			timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}),
	}

	if req.DryRun {
		return res, nil
	}

	// The read side of the store lock covers the copies and the rows that name them, so an emptying
	// cannot delete a file this import found (SDD-005 section 13.4).
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()

	sizes, cerr := s.copyBundleFiles(ctx, parts)
	if cerr != nil {
		return nil, forCaller(c, cerr)
	}

	crashpoint.Hit("bundle_import.after_copy")

	now := s.nowMs()

	for _, t := range parts.timelines() {
		t.row = importedRow(c, t, req.AsNew, now)
		t.original = t.document
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return s.writeImportedProject(ctx, tx, c, parts, sizes, req.AsNew, now)
	})
	if err != nil {
		if errors.Is(err, ErrExists) {
			return nil, newError(command.CodeIDExists, "a project with the identifier %s already exists.", projectID).
				withHint("import it with as_new to get a new id")
		}

		if e, ok := errors.AsType[*Error](err); ok {
			return nil, forCaller(c, e)
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return nil, asError(err)
	}

	crashpoint.Hit("bundle_import.after_commit")

	res.Created = true

	return res, nil
}

// bundleTimeline is a timeline of a bundle after the checks: the first one is made from the parts of the bundle
// (mainTimeline), the others by readBundleTimelines.
type bundleTimeline struct {
	key         string
	id          string // the id in the bundle
	document    []byte
	doc         *timeline.Document
	migrated    bool
	fromVersion int
	revision    int64 // the revision in the bundle
	logEntry    *zip.File
	logInfo     *BundleLog
	hasLog      bool
	logSum      string

	// row and original are set by the import once the id of the new project is known.
	row      ProjectRow
	original []byte
}

// mainTimeline is the first timeline of the bundle.
func (p *bundleParts) mainTimeline() *bundleTimeline {
	return &bundleTimeline{
		key: bundleMainKey, id: p.manifest.Project.ID, document: p.document, doc: p.doc, migrated: p.migrated,
		fromVersion: p.fromVersion, revision: p.manifest.Project.Revision, logEntry: p.entries[bundleOperationsName],
		logInfo: p.manifest.Operations, hasLog: p.hasLog, logSum: p.logSum,
	}
}

// timelines are every timeline of the bundle, the first first.
func (p *bundleParts) timelines() []*bundleTimeline {
	return append([]*bundleTimeline{p.main}, p.extra...)
}

// newRevision is the revision a timeline has once imported: that of the bundle, one more when the import
// migrated the document, and 1 when the log is left out.
func (t *bundleTimeline) newRevision(asNew bool) int64 {
	switch {
	case asNew:
		return 1
	case t.migrated:
		return t.revision + 1
	}

	return t.revision
}

// numberImportedTimelines gives the timelines after the first the ids they will have: their own, or new ones for
// a copy. Keeping an id that exists is E_ID_EXISTS, as it is for the first.
func (s *svc) numberImportedTimelines(ctx context.Context, p *bundleParts, asNew bool) error {
	for _, t := range p.extra {
		id := t.id
		if asNew {
			id = s.newIDs(timeline.PrefixProject)
		} else if _, err := s.store.Projects().Head(ctx, id); err == nil {
			return newError(command.CodeIDExists, "a project with the identifier %s already exists.", id).
				withHint("import it with as_new to get a new id")
		} else if !isNotFound(err) {
			return asError(err)
		}

		t.doc.ID = id
	}

	return nil
}

// importedRow is the row of a timeline of an import.
func importedRow(c Caller, t *bundleTimeline, asNew bool, now int64) ProjectRow {
	canon := timeline.Canonical(t.doc)
	revision := t.newRevision(asNew)

	row := ProjectRow{
		ID: t.doc.ID, Name: t.doc.Name, State: ProjectActive, Revision: revision, SchemaVersion: t.doc.SchemaVersion,
		Summary: summarize(t.doc), CreatedMs: now, UpdatedMs: now, CreatedBy: c.Actor, ResyncFloor: revision,
		Document: canon, DocSHA256: sha256Hex(canon),
	}

	if c.OwnerUserID != "" {
		owner := c.OwnerUserID
		row.OwnerUserID = &owner
	}

	return row
}

// writeImportedProject is the one Write of an import (SDD-005 section 16.3 step 7): the project with its
// items and timelines, the log and the asset links of every timeline, the font owners and the audit row.
func (s *svc) writeImportedProject(
	ctx context.Context, tx Tx, c Caller, p *bundleParts, sizes map[string]int64, asNew bool, now int64,
) error {
	if err := s.putImportedProduction(ctx, tx, p); err != nil {
		return err
	}

	for _, t := range p.timelines() {
		if err := s.writeImportedTimeline(ctx, tx, c, t, asNew, now); err != nil {
			return err
		}

		if err := s.linkBundleAssets(ctx, tx, c, t.doc, sizes, now); err != nil {
			return err
		}
	}

	// The importer brought these bytes, and the hash was checked against the manifest, so they own them.
	if err := recordFontOwner(ctx, tx, c, now, bundleFontHashes(p)...); err != nil {
		return err
	}

	if err := appendAudit(ctx, tx, now, c.Actor, "project.import", p.main.row.ID, ""); err != nil {
		return err
	}

	crashpoint.Hit("bundle_import.before_commit")

	return nil
}

// putImportedProduction writes the project, its items and its timeline rows. A bundle of version 1 has one
// timeline and no other item; one of version 2 brings its items, each timeline item pointing at the row of
// the timeline the item names.
func (s *svc) putImportedProduction(ctx context.Context, tx Tx, p *bundleParts) error {
	row := &p.main.row
	if p.manifest.Production == nil {
		return s.putProduction(ctx, tx, row)
	}

	head := ProductionHead{
		ID: row.ID, Name: row.Name, State: ProjectActive, CreatedMs: row.CreatedMs, UpdatedMs: row.UpdatedMs,
		CreatedBy: row.CreatedBy, OwnerUserID: row.OwnerUserID,
	}
	if err := tx.Productions().Create(ctx, head); err != nil {
		return err
	}

	byKey := map[string]*bundleTimeline{}
	for _, t := range p.timelines() {
		byKey[t.key] = t
		if err := tx.Projects().Create(ctx, t.row); err != nil {
			return err
		}
	}

	for i, bi := range p.manifest.Production.Items {
		it := s.newItem(row.ID, bi.Kind, bi.Title, i, row.CreatedBy, row.CreatedMs)
		it.Body, it.Format = p.texts[i], bi.Format

		if bi.Kind == string(project.Text) {
			it.TextKind = bi.TextKind
		}

		if bi.Kind == string(project.Timeline) {
			it.Ref = byKey[bi.Timeline].row.ID
		}

		if err := tx.Productions().CreateItem(ctx, it); err != nil {
			return err
		}
	}

	return nil
}

// writeImportedTimeline writes what belongs to one timeline of the import: its log, and the migration row and
// the backup when the document was migrated or the import is a copy.
func (s *svc) writeImportedTimeline(
	ctx context.Context,
	tx Tx,
	c Caller,
	t *bundleTimeline,
	asNew bool,
	now int64,
) error {
	id, row := t.row.ID, &t.row

	if t.hasLog && !asNew {
		sum, err := s.streamLog(ctx, t.logEntry, func(op *bundleOperation) error {
			// An imported row is history, not a step of this workspace: nobody here made it, so nobody
			// may undo it (SDD-007 section 6.9).
			closed := op.ClosedBy
			if closed == nil {
				closed = &op.Revision
			}

			return tx.Operations().Append(ctx, OperationRow{
				ProjectID: id, Revision: op.Revision, Kind: op.Kind, Actor: importedActor(op.Actor),
				BaseRevision: op.BaseRevision, TargetRevision: op.TargetRevision, AtMs: op.AtMs, Ops: op.Ops,
				Inverse: op.Inverse, Diff: op.Diff, Skipped: op.Skipped, SchemaVersion: op.SchemaVersion,
				DocSHA256: op.DocSHA256, ClosedBy: closed,
			})
		})
		if err != nil {
			return err
		}

		if sum.sha256 != t.logSum {
			return bundleRefused("the file changed while it was read")
		}
	}

	if !asNew && !t.migrated {
		return nil
	}

	kind, reason := OpMigrate, fmt.Sprintf("migrate v%d to v%d", t.fromVersion, row.SchemaVersion)
	base := t.revision
	actor := migrateActor

	if asNew {
		kind, reason, base, actor = OpImport, "import", 0, c.Actor
	}

	last := row.Revision
	if err := tx.Backups().Put(ctx, BackupRow{
		OwnerKind: "project", OwnerID: id, Revision: base, SchemaVersion: t.fromVersion,
		Document: t.original, Reason: reason, CreatedMs: now,
	}); err != nil {
		return err
	}

	if err := tx.Operations().Append(ctx, OperationRow{
		ProjectID: id, Revision: last, Kind: kind, Actor: actor, BaseRevision: base, AtMs: now,
		Ops: json.RawMessage("[]"), Inverse: json.RawMessage("[]"), Diff: json.RawMessage("[]"),
		SchemaVersion: row.SchemaVersion, DocSHA256: row.DocSHA256, ClosedBy: &last,
	}); err != nil {
		return err
	}

	return tx.Operations().CloseAllOpen(ctx, id, last)
}

// linkBundleAssets inserts the assets rows of new hashes as pending, the project_assets rows as
// active (the document already holds them) and an ingest job for each hash without artifacts.
func (s *svc) linkBundleAssets(
	ctx context.Context,
	tx Tx,
	c Caller,
	doc *timeline.Document,
	sizes map[string]int64,
	now int64,
) error {
	id := doc.ID

	for _, assetID := range sortedKeys(doc.Assets) {
		a := doc.Assets[assetID]

		ar, _, err := tx.Assets().Upsert(ctx, AssetRow{
			Hash: a.Hash, SizeBytes: sizes[a.Hash], OriginalName: a.Name, IngestState: IngestPending,
			CreatedMs: now, UpdatedMs: now,
		})
		if err != nil {
			return err
		}

		if ar.IngestState == IngestFailed {
			if err := tx.Assets().SetIngest(ctx, a.Hash, IngestPending, 0, nil, now); err != nil {
				return err
			}

			ar.IngestState = IngestPending
		}

		err = tx.Assets().PutProjectAsset(ctx, ProjectAssetRow{
			ProjectID: id, AssetID: a.ID, AssetHash: a.Hash, State: ProjectAssetActive, Name: a.Name,
			ImportedBy: c.Actor, ImportedMs: now,
		})
		if err != nil {
			return err
		}

		if ar.IngestState != IngestReady {
			if _, err := s.ensureIngestJob(ctx, tx, a.Hash, c.Actor, now); err != nil {
				return err
			}
		}

		// The library side belongs to the owner who imports the bundle, which also makes the file found by name.
		if err := assetlib.RecordImport(
			ctx, tx.AssetLibrary(), c.OwnerUserID, a.Hash, asset.Origin{Kind: asset.Imported}, assetlib.ImportInfo{},
			c.Actor, now,
		); err != nil {
			return err
		}
	}

	return nil
}

// readBundle applies the policy and every check of SDD-005 section 16.3 steps 1 to 5 and writes nothing.
func (s *svc) readBundle(ctx context.Context, c Caller, zr *zip.Reader) (*bundleParts, *Error) {
	p := &bundleParts{zr: zr, entries: map[string]*zip.File{}}

	if e := s.scanBundleEntries(p); e != nil {
		return nil, e
	}

	if e := s.readBundleManifest(p); e != nil {
		return nil, e
	}

	if e := s.readBundleDocument(p); e != nil {
		return nil, e
	}

	if e := s.readBundleTimelines(p); e != nil {
		return nil, e
	}

	if e := readBundleItemTexts(p); e != nil {
		return nil, e
	}

	if e := s.matchBundleFiles(ctx, c, p); e != nil {
		return nil, e
	}

	if e := s.checkBundleLog(ctx, p); e != nil {
		return nil, e
	}

	if e := s.hashBundleFiles(ctx, p); e != nil {
		return nil, e
	}

	p.main = p.mainTimeline()

	return p, nil
}

// readBundleTimelines reads the document of every timeline after the first (version 2), with the checks of
// the first: its hash and size against the manifest, depth, migration and validation. An entry named for a
// timeline that the manifest does not list is refused.
func (s *svc) readBundleTimelines(p *bundleParts) *Error {
	declared := map[string]bool{}

	for i := range p.manifest.Timelines {
		t := &p.manifest.Timelines[i]
		declared[bundleTimelineDocEntry(t.Key)] = true
		declared[bundleTimelineLogEntry(t.Key)] = true

		en := p.entries[bundleTimelineDocEntry(t.Key)]
		if en == nil {
			return bundleRefused("a timeline listed in the manifest is not in the bundle")
		}

		body, err := readEntry(en, bundleDocumentMax)
		if err != nil {
			return bundleRefused("a timeline document is larger than it declares or cannot be read")
		}

		if sha256Hex(body) != t.Document.SHA256 || int64(len(body)) != t.Document.Bytes {
			return bundleRefused("a timeline document does not match the manifest")
		}

		if err := checkJSONDepth(body); err != nil {
			return bundleRefused("a timeline document is nested too deeply or is not valid JSON")
		}

		doc, from, verr := migrateBundleDocument(body)
		if verr != nil {
			return verr
		}

		if doc.ID != t.ID {
			return bundleRefused("the manifest names another timeline than the document")
		}

		if utf8.RuneCountInString(doc.Name) > maxProjectName {
			return bundleRefused("the project name is too long")
		}

		p.extra = append(p.extra, &bundleTimeline{
			key: t.Key, id: t.ID, document: body, doc: doc, migrated: from < timeline.CurrentSchemaVersion,
			fromVersion: from, revision: t.Revision, logEntry: p.entries[bundleTimelineLogEntry(t.Key)],
			logInfo: t.Operations,
		})
	}

	for name := range p.entries {
		if strings.HasPrefix(name, bundleTimelinePrefix) && !declared[name] {
			return bundleRefused("it holds a timeline that the manifest does not list")
		}
	}

	return nil
}

// readBundleItemTexts reads the Markdown of the items of version 2: each entry item.<n>.md that an item
// declares, checked against its hash and size and by the rule of a text of core/project. An entry that no item
// declares is refused.
func readBundleItemTexts(p *bundleParts) *Error {
	declared := map[string]bool{}

	if p.manifest.Production != nil {
		for i := range p.manifest.Production.Items {
			meta := p.manifest.Production.Items[i].Text
			if meta == nil {
				continue
			}

			name := bundleItemTextEntry(i)
			declared[name] = true

			en := p.entries[name]
			if en == nil {
				return bundleRefused("the text of an item listed in the manifest is not in the bundle")
			}

			body, err := readEntry(en, project.MaxText)
			if err != nil {
				return bundleRefused("the text of an item is larger than it declares or cannot be read")
			}

			if sha256Hex(body) != meta.SHA256 || int64(len(body)) != meta.Bytes ||
				project.CheckText(string(body)) != nil {
				return bundleRefused("the text of an item does not match the manifest or is not valid")
			}

			if p.texts == nil {
				p.texts = map[int]string{}
			}

			p.texts[i] = string(body)
		}
	}

	for name := range p.entries {
		if strings.HasPrefix(name, bundleItemPrefix) && !declared[name] {
			return bundleRefused("it holds a text that the manifest does not list")
		}
	}

	return nil
}

// entryCap is the cap of one entry by its name.
func (s *svc) entryCap(name string) int64 {
	switch name {
	case bundleManifestEntry:
		return bundleManifestMax
	case bundleDocumentEntry:
		return bundleDocumentMax
	case bundleOperationsName:
		return bundleOperationsMax
	}

	if strings.HasPrefix(name, bundleItemPrefix) {
		return project.MaxText
	}

	if strings.HasPrefix(name, bundleTimelinePrefix) {
		if strings.HasSuffix(name, ".ndjson") {
			return bundleOperationsMax
		}

		return bundleDocumentMax
	}

	return s.limits.MaxImportBytes
}

// scanBundleEntries judges the central directory before any byte is read: names, kinds, flags,
// declared sizes, ratio and the free disk space.
func (s *svc) scanBundleEntries(p *bundleParts) *Error {
	if len(p.zr.File) > bundleEntriesMax {
		return bundleOverLimit("bundle_entries", "it holds too many entries", uint64(len(p.zr.File)), bundleEntriesMax)
	}

	var total uint64

	for _, f := range p.zr.File {
		if f.FileInfo().IsDir() || f.Mode()&fs.ModeSymlink != 0 || f.Mode()&fs.ModeType != 0 {
			return bundleRefused("it holds an entry that is not a regular file")
		}

		if f.Flags&0x1 != 0 {
			return bundleRefused("it holds an encrypted entry")
		}

		if !bundleEntryName.MatchString(f.Name) {
			return bundleRefused("it holds an entry that a bundle may not hold")
		}

		if _, dup := p.entries[f.Name]; dup {
			return bundleRefused("it holds the same entry twice")
		}

		p.entries[f.Name] = f

		if f.UncompressedSize64 > uint64(s.entryCap(f.Name)) { //nolint:gosec // the cap is positive
			return bundleOverLimit("bundle_entry_bytes", "an entry is larger than allowed", f.UncompressedSize64,
				uint64(s.entryCap(f.Name))) //nolint:gosec // the cap is positive
		}

		total += f.UncompressedSize64
		if total > bundleTotalMax {
			return bundleOverLimit("bundle_total_bytes", "the total is larger than allowed", total, bundleTotalMax)
		}

		if f.UncompressedSize64 > bundleBombFloor && f.UncompressedSize64 > bundleBombRatio*max(f.CompressedSize64, 1) {
			return bundleOverLimit("bundle_ratio", "an entry has an implausible compression ratio",
				f.UncompressedSize64/max(f.CompressedSize64, 1), bundleBombRatio)
		}
	}

	if p.entries[bundleManifestEntry] == nil || p.entries[bundleDocumentEntry] == nil {
		return bundleRefused("manifest.json and document.json are required")
	}

	return s.checkBundleDisk(total)
}

// checkBundleDisk wants the declared total plus 10% and the free-space floor available.
func (s *svc) checkBundleDisk(total uint64) *Error {
	if s.workspace == "" || s.media.FreeBytes == nil {
		return nil
	}

	free, err := s.media.FreeBytes(s.workspace)
	need := total + total/100*bundleDiskMarginPerc + uint64(
		max(s.limits.MinFreeBytes, 0),
	)
	if err == nil && free < need {
		return newError("E_LIMIT_EXCEEDED", "The disk of the workspace has too little free space for this bundle.").
			withHint("free space on the disk of the workspace").
			withLimit("free_bytes", free, need).withDetail("reason", "disk_low").
			withDetail("variable", "STUARTY_MIN_FREE_BYTES")
	}

	return nil
}

func (s *svc) readBundleManifest(p *bundleParts) *Error {
	body, err := readEntry(p.entries[bundleManifestEntry], bundleManifestMax)
	if err != nil {
		return bundleRefused("manifest.json is larger than it declares or cannot be read")
	}

	if err := checkJSONDepth(body); err != nil {
		return bundleRefused("manifest.json is not valid JSON or is nested too deeply")
	}

	m := &p.manifest
	if err := json.Unmarshal(body, m); err != nil {
		return bundleRefused("manifest.json is not valid")
	}

	if (m.Format != BundleFormat && m.Format != formerBundleFormat) || m.BundleVersion < 1 {
		return bundleRefused("manifest.json is not a stuarty bundle manifest")
	}

	if m.BundleVersion > BundleVersion {
		return newError(command.CodeValidation, "the bundle has version %d; this build reads up to %d.",
			m.BundleVersion, BundleVersion).
			withHint("update stuarty, then import it again").withDetail("code", "V_BUNDLE_VERSION")
	}

	return checkBundleProduction(m)
}

// The most timelines and items a bundle may bring.
const (
	bundleTimelinesMax = project.MaxTimelines
	bundleItemsMax     = project.MaxItems
)

var bundleKeyRe = regexp.MustCompile(`^t\d{1,3}$`)

// checkBundleProduction judges the members of version 2 of the manifest before any entry is read: every key and
// id is of the right form and once, every timeline is named by exactly one item, and the items obey the rules of
// core/project.
func checkBundleProduction(m *BundleManifest) *Error {
	if m.Production == nil {
		if len(m.Timelines) > 0 {
			return bundleRefused("manifest.json lists timelines but no production")
		}

		return nil
	}

	if len(m.Timelines) >= bundleTimelinesMax || len(m.Production.Items) > bundleItemsMax {
		return bundleOverLimit("bundle_items", "it holds too many timelines or items",
			uint64(len(m.Timelines)+len(m.Production.Items)), bundleTimelinesMax+bundleItemsMax)
	}

	keys := map[string]bool{bundleMainKey: true}
	ids := map[string]bool{m.Project.ID: true}

	for i := range m.Timelines {
		t := &m.Timelines[i]
		if !bundleKeyRe.MatchString(t.Key) || keys[t.Key] || !timeline.ValidID(timeline.PrefixProject, t.ID) ||
			ids[t.ID] || t.Revision < 0 || t.SchemaVersion < 1 || !isHex64(t.Document.SHA256) || t.Document.Bytes < 0 {
			return bundleRefused("manifest.json lists a timeline that is not valid")
		}

		if t.Operations != nil && (t.Operations.Count < 0 || !isHex64(t.Operations.SHA256)) {
			return bundleRefused("manifest.json lists a log that is not valid")
		}

		keys[t.Key], ids[t.ID] = true, true
	}

	used := map[string]bool{}
	briefs := 0

	for i := range m.Production.Items {
		it := &m.Production.Items[i]
		if v := checkBundleItem(it, keys, used); v != "" {
			return bundleRefused(v)
		}

		if it.Kind == string(project.Brief) {
			briefs++
		}
	}

	if briefs > 1 || len(used) != len(keys) {
		return bundleRefused("the items do not name every timeline once, or hold two briefs")
	}

	return nil
}

// checkBundleItem is why an item of a bundle is refused, or empty.
func checkBundleItem(it *BundleItem, keys, used map[string]bool) string {
	if !project.Kind(it.Kind).Allowed() || project.CheckTitle(it.Title) != nil || len(it.Format) > 16 {
		return "an item of the project is not valid"
	}

	// A text kind belongs to a text, and is one of the three (a bundle of before epic 17 has none: it is a copy).
	if it.TextKind != "" {
		if _, v := writing.CheckKind(it.TextKind); v != nil || it.Kind != string(project.Text) {
			return "an item declares a text kind that is not valid"
		}
	}

	// Only a brief or a text has a text, and its entry is at most the limit of a text.
	if t := it.Text; t != nil && (it.Kind != string(project.Brief) && it.Kind != string(project.Text) ||
		!isHex64(t.SHA256) || t.Bytes < 1 || t.Bytes > project.MaxText) {
		return "an item declares a text that is not valid"
	}

	if it.Kind != string(project.Timeline) {
		if it.Timeline != "" {
			return "an item that is not a timeline names a timeline"
		}

		return ""
	}

	if !keys[it.Timeline] || used[it.Timeline] {
		return "an item names a timeline that is not in the bundle, or names one twice"
	}

	used[it.Timeline] = true

	return ""
}

func (s *svc) readBundleDocument(p *bundleParts) *Error {
	m := &p.manifest

	body, err := readEntry(p.entries[bundleDocumentEntry], bundleDocumentMax)
	if err != nil {
		return bundleRefused("document.json is larger than it declares or cannot be read")
	}

	if sha256Hex(body) != m.Document.SHA256 || int64(len(body)) != m.Document.Bytes {
		return bundleRefused("document.json does not match the manifest")
	}

	if err := checkJSONDepth(body); err != nil {
		return bundleRefused("document.json is nested too deeply or is not valid JSON")
	}

	doc, from, verr := migrateBundleDocument(body)
	if verr != nil {
		return verr
	}

	if doc.ID != m.Project.ID {
		return bundleRefused("the manifest names another project than the document")
	}

	if utf8.RuneCountInString(doc.Name) > maxProjectName {
		return bundleRefused("the project name is too long")
	}

	p.document, p.doc, p.fromVersion, p.migrated = body, doc, from, from < timeline.CurrentSchemaVersion

	return nil
}

// migrateBundleDocument parses, migrates and validates document.json (step 3). It reports whether the
// version it had before the migration.
func migrateBundleDocument(data []byte) (*timeline.Document, int, *Error) {
	bad := newError(command.CodeValidation, "document.json is not a valid document.").
		withHint("export the project again with bundle:export")

	raw, err := timeline.DecodeRaw(data)
	if err != nil {
		return nil, 0, bad
	}

	from := schemaVersionOf(raw)
	if from > timeline.CurrentSchemaVersion {
		return nil, 0, newError(command.CodeValidation, "the bundle was made by a newer version of the editor.").
			withHint("update stuarty, then import it again").withDetail("code", "V_SCHEMA_VERSION")
	}

	if from < 1 {
		return nil, 0, bad
	}

	raw, err = timeline.Migrate(raw)
	if err != nil {
		return nil, 0, newError(command.CodeValidation, "document.json cannot be migrated to this version.").
			withHint("export the project again with bundle:export")
	}

	doc, err := timeline.FromRaw(raw)
	if err != nil {
		return nil, 0, bad
	}

	if vs := timeline.Validate(doc); len(vs) > 0 {
		lines := make([]string, len(vs))
		for i := range vs {
			lines[i] = vs[i].String()
		}

		return nil, 0, newError(command.CodeValidation, "document.json is not valid: %s", strings.Join(lines, "; ")).
			withDetail("code", vs[0].Code)
	}

	return doc, from, nil
}

// matchBundleFiles pairs the media and fonts of the document with the manifest and the entries, and
// decides which hashes come from the zip and which are already in the store (step 4).
func (s *svc) matchBundleFiles(ctx context.Context, c Caller, p *bundleParts) *Error {
	listed, media, fonts, e := listManifestFiles(&p.manifest)
	if e != nil {
		return e
	}

	needed := map[string]string{} // hash to entry name

	var owned fontScope

	for _, doc := range p.documents() {
		for _, id := range sortedKeys(doc.Assets) {
			h := doc.Assets[id].Hash
			if media[h] {
				needed[h] = bundleMediaEntry(h)
			} else if !s.unshippedAllowed(ctx, c, h) {
				return bundleRefused("a file of the document is not in the bundle")
			}
		}

		for _, family := range sortedKeys(doc.Fonts) {
			for _, f := range doc.Fonts[family].Files {
				if fonts[f.Hash] {
					needed[f.Hash] = bundleFontEntry(f.Hash)
				} else if !owned.allowed(ctx, s.store, s, f.Hash) {
					return bundleRefused("a file of the document is not in the bundle")
				}
			}
		}
	}

	return pairBundleEntries(p, needed, listed)
}

// documents are the documents of every timeline of the bundle, the first first.
func (p *bundleParts) documents() []*timeline.Document {
	docs := []*timeline.Document{p.doc}
	for _, t := range p.extra {
		docs = append(docs, t.doc)
	}

	return docs
}

// listManifestFiles reads the media and fonts the manifest lists: their declared sizes by hash and
// the set of hashes of each kind.
func listManifestFiles(m *BundleManifest) (map[string]int64, map[string]bool, map[string]bool, *Error) {
	listed := map[string]int64{}
	media, fonts := map[string]bool{}, map[string]bool{}

	add := func(hash string, size int64) *Error {
		if !bundleHashRe.MatchString(hash) || size < 0 {
			return bundleRefused("the manifest lists a file with a bad hash or size")
		}

		if prev, ok := listed[hash]; ok && prev != size {
			return bundleRefused("the manifest lists one hash with two sizes")
		}

		listed[hash] = size

		return nil
	}

	for i := range m.Media {
		if e := add(m.Media[i].Hash, m.Media[i].Bytes); e != nil {
			return nil, nil, nil, e
		}

		media[m.Media[i].Hash] = true
	}

	for i := range m.Fonts {
		if e := add(m.Fonts[i].Hash, m.Fonts[i].Bytes); e != nil {
			return nil, nil, nil, e
		}

		fonts[m.Fonts[i].Hash] = true
	}

	return listed, media, fonts, nil
}

// pairBundleEntries checks that every file entry of the zip is needed by the document, so that
// nothing rides along, and that each needed one is present with the size the manifest declares.
func pairBundleEntries(p *bundleParts, needed map[string]string, listed map[string]int64) *Error {
	used := map[string]bool{}
	for _, entry := range needed {
		used[entry] = true
	}

	for name := range p.entries {
		if name == bundleManifestEntry || name == bundleDocumentEntry || name == bundleOperationsName ||
			strings.HasPrefix(name, bundleTimelinePrefix) || strings.HasPrefix(name, bundleItemPrefix) {
			continue
		}

		if !used[name] {
			return bundleRefused("it holds a file that the manifest does not list or the document does not use")
		}
	}

	for _, h := range sortedKeys(needed) {
		entry := needed[h]

		en := p.entries[entry]
		if en == nil {
			return bundleRefused("a file listed in the manifest is not in the bundle").withDetail("hash", h)
		}

		if int64(en.UncompressedSize64) != listed[h] { //nolint:gosec // bounded by the entry cap
			return bundleRefused("a file is not the size the manifest declares").withDetail("hash", h)
		}

		if listed[h] < enginestore.MinFileSize {
			return fromEngine(&enginestore.Error{
				Code:    enginestore.CodeUnsupportedMedia,
				Reason:  "too_small",
				Message: "A file of the bundle is too small.",
			})
		}

		p.files = append(p.files, bundleFile{entry: entry, hash: h, size: listed[h]})
	}

	return nil
}

// checkBundleLog is step 5 for every timeline of the bundle.
func (s *svc) checkBundleLog(ctx context.Context, p *bundleParts) *Error {
	m := &p.manifest

	var e *Error

	p.hasLog, p.logSum, e = s.checkTimelineLog(ctx, p.entries[bundleOperationsName], m.Operations,
		m.Project.Revision, m.Document.SHA256, p.migrated)
	if e != nil {
		return e
	}

	for _, t := range p.extra {
		var sha string

		for i := range m.Timelines {
			if m.Timelines[i].Key == t.key {
				sha = m.Timelines[i].Document.SHA256
			}
		}

		if t.hasLog, t.logSum, e = s.checkTimelineLog(
			ctx,
			t.logEntry,
			t.logInfo,
			t.revision,
			sha,
			t.migrated,
		); e != nil {
			return e
		}
	}

	return nil
}

// checkTimelineLog checks the log of one timeline: contiguous revisions, the hash of the file, and the last
// document hash. It returns whether there is a log and its hash.
func (s *svc) checkTimelineLog(
	ctx context.Context, en *zip.File, info *BundleLog, revision int64, docSHA string, migrated bool,
) (bool, string, *Error) {
	if (en != nil) != (info != nil) {
		return false, "", bundleRefused("a log and the manifest disagree about it")
	}

	if en == nil {
		return false, "", nil
	}

	var last string

	sum, err := s.streamLog(ctx, en, func(op *bundleOperation) error {
		last = op.DocSHA256

		return nil
	})
	if err != nil {
		if e, ok := errors.AsType[*Error](err); ok {
			return false, "", e
		}

		if ctx.Err() != nil {
			return false, "", asErr(ctx.Err())
		}

		return false, "", bundleRefused("operations.ndjson cannot be read")
	}

	if sum.count != info.Count || sum.sha256 != info.SHA256 {
		return false, "", bundleRefused("operations.ndjson does not match the manifest")
	}

	if sum.count != revision {
		return false, "", bundleRefused("the log does not end at the revision of the manifest")
	}

	if !migrated && sum.count > 0 && last != docSHA {
		return false, "", bundleRefused("the last revision of the log is not the document of the bundle")
	}

	return true, sum.sha256, nil
}

// streamLog reads operations.ndjson once, line by line, holding it to its cap and checking that the
// revisions are 1, 2, 3 ... . visit sees each parsed line.
func (s *svc) streamLog(ctx context.Context, en *zip.File, visit func(*bundleOperation) error) (bundleLog, error) {
	rc, err := en.Open()
	if err != nil {
		return bundleLog{}, err
	}

	defer func() { _ = rc.Close() }()

	h := sha256.New()
	limited := io.LimitReader(ctxReader{ctx, io.TeeReader(rc, h)}, bundleOperationsMax+1)

	sc := bufio.NewScanner(limited)
	sc.Buffer(make([]byte, 0, 64<<10), bundleLineMax)

	var count int64

	for sc.Scan() {
		var op bundleOperation

		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		if derr := dec.Decode(&op); derr != nil || dec.More() {
			return bundleLog{}, bundleRefused("a line of operations.ndjson is not valid")
		}

		count++

		if verr := checkBundleOperation(&op, count); verr != nil {
			return bundleLog{}, verr
		}

		if err := visit(&op); err != nil {
			return bundleLog{}, err
		}
	}

	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return bundleLog{}, bundleOverLimit("bundle_line_bytes", "a line of the log is larger than allowed",
				bundleLineMax+1, bundleLineMax)
		}

		return bundleLog{}, err
	}

	return bundleLog{count: count, sha256: hex.EncodeToString(h.Sum(nil))}, nil
}

func checkBundleOperation(op *bundleOperation, want int64) *Error {
	if op.Revision != want {
		return bundleRefused(fmt.Sprintf("the log has a gap or a repeat at revision %d", want))
	}

	switch op.Kind {
	case OpEdit, OpUndo, OpRedo, OpMigrate, OpImport:
	default:
		return bundleRefused(fmt.Sprintf("revision %d of the log has an unknown kind", want))
	}

	if op.Actor == "" || len(op.Actor) > 256 || !utf8.ValidString(op.Actor) ||
		op.SchemaVersion < 1 || op.BaseRevision < 0 || op.BaseRevision >= op.Revision ||
		!isHex64(op.DocSHA256) || (op.ClosedBy != nil && *op.ClosedBy < 1) ||
		(op.TargetRevision != nil && (*op.TargetRevision < 1 || *op.TargetRevision >= op.Revision)) {
		return bundleRefused(fmt.Sprintf("revision %d of the log is not valid", want))
	}

	if len(op.Ops) == 0 || len(op.Inverse) == 0 || len(op.Diff) == 0 {
		return bundleRefused(fmt.Sprintf("revision %d of the log lacks a required column", want))
	}

	return nil
}

// importedActorPrefix marks the actor of a log row that came from a bundle.
const importedActorPrefix = "imported:"

// importedActor attributes a foreign row to nobody who is live: an actor from an archive could be
// "local:ui" or the label of an agent of this workspace.
func importedActor(actor string) string {
	if strings.HasPrefix(actor, importedActorPrefix) {
		return actor
	}

	return importedActorPrefix + actor
}

func isHex64(s string) bool {
	return len(s) == 64 && bundleHashRe.MatchString("sha256:"+s)
}

// hashBundleFiles reads every media and font entry once and compares its SHA-256 and size with the
// manifest, so that a bundle that lies is refused before anything is stored.
func (s *svc) hashBundleFiles(ctx context.Context, p *bundleParts) *Error {
	for _, f := range p.files {
		sum, n, err := hashEntry(ctx, p.entries[f.entry], s.bundleFileLimit(f))
		if err != nil {
			if ctx.Err() != nil {
				return asErr(ctx.Err())
			}

			return bundleRefused("a file is larger than it declares or cannot be read").withDetail("hash", f.hash)
		}

		if "sha256:"+sum != f.hash || n != f.size {
			return bundleRefused("the bytes of a file do not match the hash in the manifest").withDetail("hash", f.hash)
		}
	}

	return nil
}

func hashEntry(ctx context.Context, en *zip.File, limit int64) (string, int64, error) {
	rc, err := en.Open()
	if err != nil {
		return "", 0, err
	}

	defer func() { _ = rc.Close() }()

	h := sha256.New()

	n, err := io.Copy(h, io.LimitReader(ctxReader{ctx, rc}, limit+1))
	if err != nil {
		return "", 0, err
	}

	if n > limit || n != int64(en.UncompressedSize64) { //nolint:gosec // bounded by the entry cap
		return "", 0, errors.New("size mismatch")
	}

	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// copyBundleFiles puts each file into its store, unless it is there already. Fonts go first and go
// through the font pipeline (validation, then the font store), so a crafted font is refused before
// any media is copied. The caller holds the read side of storeMu. A file copied before a failed
// transaction is a content-addressed orphan that the sweep removes (SDD-005 section 16.3).
func (s *svc) copyBundleFiles(ctx context.Context, p *bundleParts) (map[string]int64, *Error) {
	sizes := map[string]int64{}

	for _, f := range p.files {
		if f.isFont() {
			if e := s.copyBundleFont(p, f); e != nil {
				return nil, e
			}
		}
	}

	for _, f := range p.files {
		sizes[f.hash] = f.size

		if f.isFont() || s.media.Content.Has(strings.TrimPrefix(f.hash, "sha256:")) {
			continue
		}

		rc, err := p.entries[f.entry].Open()
		if err != nil {
			return nil, bundleRefused("a file cannot be read").withDetail("hash", f.hash)
		}

		put, err := s.media.Content.Put(ctx, rc, s.limits.MaxImportBytes)
		_ = rc.Close()

		if err != nil {
			if ctx.Err() != nil {
				return nil, asErr(ctx.Err())
			}

			return nil, fromEngine(err)
		}

		if "sha256:"+put.Hash != f.hash {
			return nil, bundleRefused("the file changed while it was read").withDetail("hash", f.hash)
		}
	}

	// The assets of the document that the store already held.
	for _, doc := range p.documents() {
		for _, id := range sortedKeys(doc.Assets) {
			a := doc.Assets[id]
			if _, ok := sizes[a.Hash]; ok {
				continue
			}

			size, e := s.storedSize(a.Hash)
			if e != nil {
				return nil, bundleRefused("a file of the store is missing").withDetail("hash", a.Hash)
			}

			sizes[a.Hash] = size
		}
	}

	return sizes, nil
}

// unshippedAllowed tells whether a document may name a file the bundle does not carry. In local mode
// the store is the caller's own, so a file there is enough. In network mode the store is shared by
// every user: naming a hash must not link another user's media (nor reveal that it exists), so the
// hash has to be linked already to a project the caller can see. A font follows the rule of fontScope
// (recorded owner, or used by a project of the caller). The refusal is the same for a hash that is
// stored and one that never was.
func (s *svc) unshippedAllowed(ctx context.Context, c Caller, hash string) bool {
	if !s.media.Content.Has(strings.TrimPrefix(hash, "sha256:")) {
		return false
	}

	if c.OwnerUserID == "" {
		return true
	}

	ids, err := s.store.Assets().ProjectsReferencing(ctx, hash)
	if err != nil {
		return false
	}

	for _, id := range ids {
		if h, err := s.store.Projects().Head(ctx, id); err == nil && visible(c, h.OwnerUserID) {
			return true
		}
	}

	return false
}

// bundleFontHashes are the hashes of the font entries the bundle carries.
func bundleFontHashes(p *bundleParts) []string {
	var out []string

	for _, f := range p.files {
		if f.isFont() {
			out = append(out, f.hash)
		}
	}

	return out
}
