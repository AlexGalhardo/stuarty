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

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
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
	`^(manifest\.json|document\.json|operations\.ndjson|(media|font)\.[0-9a-f]{64})$`,
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

	original := parts.document
	doc := parts.doc
	doc.ID = projectID
	canon := timeline.Canonical(doc)

	revision := int64(1)
	if !req.AsNew {
		revision = parts.manifest.Project.Revision
		if parts.migrated {
			revision++
		}
	}

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
	row := ProjectRow{
		ID: projectID, Name: doc.Name, State: ProjectActive, Revision: revision, SchemaVersion: doc.SchemaVersion,
		Summary: summarize(doc), CreatedMs: now, UpdatedMs: now, CreatedBy: c.Actor, ResyncFloor: revision,
		Document: canon, DocSHA256: sha256Hex(canon),
	}

	if c.OwnerUserID != "" {
		owner := c.OwnerUserID
		row.OwnerUserID = &owner
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return s.writeImportedProject(ctx, tx, c, parts, &row, original, sizes, req.AsNew, now)
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

// writeImportedProject is the one Write of an import (SDD-005 section 16.3 step 7).
func (s *svc) writeImportedProject(
	ctx context.Context, tx Tx, c Caller, p *bundleParts, row *ProjectRow, original []byte, sizes map[string]int64,
	asNew bool, now int64,
) error {
	id := row.ID

	if err := tx.Projects().Create(ctx, *row); err != nil {
		return err
	}

	rev := p.manifest.Project.Revision

	if p.hasLog && !asNew {
		sum, err := s.streamLog(ctx, p.entries[bundleOperationsName], func(op *bundleOperation) error {
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

		if sum.sha256 != p.logSum {
			return bundleRefused("the file changed while it was read")
		}
	}

	if asNew || p.migrated {
		kind, reason := OpMigrate, fmt.Sprintf("migrate v%d to v%d", p.fromVersion, row.SchemaVersion)
		base := rev
		actor := migrateActor
		diff := json.RawMessage("[]")

		if asNew {
			kind, reason, base, actor = OpImport, "import", 0, c.Actor
		}

		last := row.Revision
		if err := tx.Backups().Put(ctx, BackupRow{
			OwnerKind: "project", OwnerID: id, Revision: base, SchemaVersion: p.fromVersion,
			Document: original, Reason: reason, CreatedMs: now,
		}); err != nil {
			return err
		}

		if err := tx.Operations().Append(ctx, OperationRow{
			ProjectID: id, Revision: last, Kind: kind, Actor: actor, BaseRevision: base, AtMs: now,
			Ops: json.RawMessage("[]"), Inverse: json.RawMessage("[]"), Diff: diff, SchemaVersion: row.SchemaVersion,
			DocSHA256: row.DocSHA256, ClosedBy: &last,
		}); err != nil {
			return err
		}

		if err := tx.Operations().CloseAllOpen(ctx, id, last); err != nil {
			return err
		}
	}

	if err := s.linkBundleAssets(ctx, tx, c, p, sizes, now); err != nil {
		return err
	}

	// The importer brought these bytes, and the hash was checked against the manifest, so they own them.
	if err := recordFontOwner(ctx, tx, c, now, bundleFontHashes(p)...); err != nil {
		return err
	}

	if err := appendAudit(ctx, tx, now, c.Actor, "project.import", id, ""); err != nil {
		return err
	}

	crashpoint.Hit("bundle_import.before_commit")

	return nil
}

// linkBundleAssets inserts the assets rows of new hashes as pending, the project_assets rows as
// active (the document already holds them) and an ingest job for each hash without artifacts.
func (s *svc) linkBundleAssets(
	ctx context.Context,
	tx Tx,
	c Caller,
	p *bundleParts,
	sizes map[string]int64,
	now int64,
) error {
	id := p.doc.ID

	for _, assetID := range sortedKeys(p.doc.Assets) {
		a := p.doc.Assets[assetID]

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

	if e := s.matchBundleFiles(ctx, c, p); e != nil {
		return nil, e
	}

	if e := s.checkBundleLog(ctx, p); e != nil {
		return nil, e
	}

	if e := s.hashBundleFiles(ctx, p); e != nil {
		return nil, e
	}

	return p, nil
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

	return nil
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

	for _, id := range sortedKeys(p.doc.Assets) {
		h := p.doc.Assets[id].Hash
		if media[h] {
			needed[h] = bundleMediaEntry(h)
		} else if !s.unshippedAllowed(ctx, c, h) {
			return bundleRefused("a file of the document is not in the bundle")
		}
	}

	var owned fontScope

	for _, family := range sortedKeys(p.doc.Fonts) {
		for _, f := range p.doc.Fonts[family].Files {
			if fonts[f.Hash] {
				needed[f.Hash] = bundleFontEntry(f.Hash)
			} else if !owned.allowed(ctx, s.store, s, f.Hash) {
				return bundleRefused("a file of the document is not in the bundle")
			}
		}
	}

	return pairBundleEntries(p, needed, listed)
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
		if name == bundleManifestEntry || name == bundleDocumentEntry || name == bundleOperationsName {
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

// checkBundleLog is step 5: contiguous revisions, operations.sha256 and the last document hash.
func (s *svc) checkBundleLog(ctx context.Context, p *bundleParts) *Error {
	m := &p.manifest
	en := p.entries[bundleOperationsName]

	p.hasLog = en != nil
	if (en != nil) != (m.Operations != nil) {
		return bundleRefused("operations.ndjson and the manifest disagree about the log")
	}

	if en == nil {
		return nil
	}

	var last string

	sum, err := s.streamLog(ctx, en, func(op *bundleOperation) error {
		last = op.DocSHA256

		return nil
	})
	if err != nil {
		if e, ok := errors.AsType[*Error](err); ok {
			return e
		}

		if ctx.Err() != nil {
			return asErr(ctx.Err())
		}

		return bundleRefused("operations.ndjson cannot be read")
	}

	if sum.count != m.Operations.Count || sum.sha256 != m.Operations.SHA256 {
		return bundleRefused("operations.ndjson does not match the manifest")
	}

	if sum.count != m.Project.Revision {
		return bundleRefused("the log does not end at the revision of the manifest")
	}

	if !p.migrated && sum.count > 0 && last != m.Document.SHA256 {
		return bundleRefused("the last revision of the log is not the document of the bundle")
	}

	p.logSum = sum.sha256

	return nil
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
	for _, id := range sortedKeys(p.doc.Assets) {
		a := p.doc.Assets[id]
		if _, ok := sizes[a.Hash]; ok {
			continue
		}

		size, e := s.storedSize(a.Hash)
		if e != nil {
			return nil, bundleRefused("a file of the store is missing").withDetail("hash", a.Hash)
		}

		sizes[a.Hash] = size
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
