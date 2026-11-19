package service

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
)

// ProjectBundleExportRequest asks for a .vvbundle of the current revision of a project
// (SDD-005 section 16, RF-006).
type ProjectBundleExportRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	// IncludeLog false leaves operations.ndjson out, to share without history. Default true.
	IncludeLog *bool `json:"include_log,omitempty"`
	WaitMS     int   `json:"wait_ms,omitempty"`
}

// resolveBundleSource decides what a bundle is of. With a timeline_id it is that timeline (a bundle of version
// 1); with a project it is the whole project (version 2), and the job is kept on the first active timeline,
// which the request then names; an id that is no project is a timeline, as it always was. others are the
// other active timelines of a whole project.
func (s *svc) resolveBundleSource(
	ctx context.Context,
	c Caller,
	req *ProjectBundleExportRequest,
) (bool, []string, error) {
	if req.TimelineID != "" {
		return false, nil, s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID)
	}

	tls, err := s.store.Productions().ActiveTimelines(ctx, req.ProjectID)
	if err != nil {
		return false, nil, asError(err)
	}

	if len(tls) == 0 {
		return false, nil, nil
	}

	req.ProjectID = tls[0].TimelineID

	others := make([]string, 0, len(tls)-1)
	for _, t := range tls[1:] {
		others = append(others, t.TimelineID)
	}

	return true, others, nil
}

// ProjectBundleExport starts the export job with input.format "bundle" (SDD-005 section 16).
func (s *svc) ProjectBundleExport(ctx context.Context, req ProjectBundleExportRequest) (*Job, error) {
	c, err := authorizeScope(ctx, ScopeRender, req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	whole, others, err := s.resolveBundleSource(ctx, c, &req)
	if err != nil {
		return nil, err
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if row.State == ProjectTrashed {
		return nil, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	if s.media.Content == nil {
		return nil, newError(command.CodeInternal, "exporting is not available in this build.")
	}

	if ferr := s.checkExportFonts(ctx, c, &row); ferr != nil {
		return nil, ferr
	}

	for _, id := range others {
		other, oerr := s.loadProject(ctx, c, id)
		if oerr != nil {
			return nil, oerr
		}

		if ferr := s.checkExportFonts(ctx, c, &other); ferr != nil {
			return nil, ferr
		}
	}

	if derr := s.checkDisk(); derr != nil {
		return nil, forCaller(c, derr)
	}

	include := req.IncludeLog == nil || *req.IncludeLog
	projectID := row.ID
	job := JobRow{
		ID: s.newIDs("job"), Kind: JobExport, ProjectID: &projectID, Actor: c.Actor, State: JobQueued,
		Input: marshalOrNull(exportInput{Format: bundleExportFormat, IncludeLog: &include, Whole: whole}),
	}
	lim := &JobLimit{
		Kind: JobExport, Max: s.limits.MaxExports, Actor: c.Actor,
		MaxPerActor: s.limits.MaxExportsPerActor + queuedPerActor,
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		job.CreatedMs, job.UpdatedMs = now, now

		if err := tx.Jobs().CreateLimited(ctx, job, lim); err != nil {
			return err
		}

		if err := tx.Audit().Append(ctx, AuditRow{
			AtMs: now, Actor: c.Actor, Action: "job.start", ProjectID: &projectID, SubjectID: &job.ID,
		}); err != nil {
			return err
		}

		queued := job
		tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

		if s.enqueue != nil {
			return s.enqueue.Enqueue(ctx, tx, queueKindFor(job.Kind), job.ID)
		}

		return nil
	})
	if err != nil {
		return nil, callerError(c, limitError(err, s.limits))
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, job.ID, time.Duration(req.WaitMS)*time.Millisecond)
	}

	cur, gerr := s.store.Jobs().Get(ctx, job.ID)
	if gerr != nil {
		return nil, asError(gerr)
	}

	return s.jobView(c, cur, req.WaitMS > 0), nil
}

func bundleFailed(reason string) *Error {
	return newError(CodeExportFailed, "The bundle could not be written.").
		withHint("run doctor to check the content store").withDetail("reason", reason)
}

// bundleFile is a store file that goes into the bundle: its entry, hash and size.
type bundleFile struct {
	entry string
	hash  string // sha256:<hex>
	size  int64
}

// bundleLog summarizes the operations.ndjson a snapshot wrote.
type bundleLog struct {
	count  int64
	sha256 string
}

// bundleSnapshot is one timeline as the export read it: its row, and its log in a temporary file.
type bundleSnapshot struct {
	key  string
	row  ProjectRow
	doc  *timeline.Document
	logf *os.File
	log  bundleLog
}

func closeSnapshots(snaps []*bundleSnapshot) {
	for _, sn := range snaps {
		if sn.logf != nil {
			_ = sn.logf.Close()
			_ = os.Remove(sn.logf.Name())
		}
	}
}

// snapshotTimeline reads the row of a timeline and, when asked, writes its log to a temporary file in dir.
func snapshotTimeline(
	ctx context.Context, tx ProjectTx, id, dir string, includeLog bool,
) (*bundleSnapshot, error) {
	row, err := tx.Projects().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	sn := &bundleSnapshot{row: row}
	if !includeLog {
		return sn, nil
	}

	if sn.logf, err = os.CreateTemp(dir, ".operations-*.ndjson"); err != nil {
		return nil, err
	}

	sn.log, err = writeOperationsLog(ctx, tx.Operations(), row.ID, row.Revision, sn.logf)

	return sn, err
}

// snapshotBundle reads, in one transaction, the first timeline and, for a whole project, the other active
// ones with the items of the project. The caller closes the snapshots it gets, also on an error.
func (s *svc) snapshotBundle(
	ctx context.Context, job *JobRow, dir string, in *exportInput,
) (snaps []*bundleSnapshot, items []ItemRow, err error) {
	includeLog := in.IncludeLog == nil || *in.IncludeLog

	err = s.store.WriteProject(ctx, *job.ProjectID, func(ctx context.Context, tx ProjectTx) error {
		main, err := snapshotTimeline(ctx, tx, *job.ProjectID, dir, includeLog)
		if main != nil {
			main.key = bundleMainKey
			snaps = append(snaps, main)
		}

		if err != nil || !in.Whole {
			return err
		}

		owner, err := tx.Productions().ProjectOfTimeline(ctx, *job.ProjectID)
		if err != nil {
			return err
		}

		if items, err = tx.Productions().Items(ctx, owner); err != nil {
			return err
		}

		for i := range items {
			it := &items[i]
			if it.Kind != string(project.Timeline) || it.State != ProjectActive || it.Ref == *job.ProjectID {
				continue
			}

			sn, err := snapshotTimeline(ctx, tx, it.Ref, dir, includeLog)
			if sn != nil {
				sn.key = "t" + strconv.Itoa(len(snaps))
				snaps = append(snaps, sn)
			}

			if err != nil {
				return err
			}
		}

		return nil
	})

	return snaps, items, err
}

// runBundleExport writes exports/<project id>/bundle-r<R>-<job id>.vvbundle (SDD-005 section 16.2).
func (s *svc) runBundleExport(ctx context.Context, job *JobRow, pw *progress, in exportInput) (json.RawMessage, error) {
	if job.ProjectID == nil || s.media.Content == nil {
		return nil, internalError()
	}

	if err := s.checkDisk(); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.workspace, "exports", *job.ProjectID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, internalError()
	}

	// The snapshot: the rows and their logs are read in one transaction and nothing else is.
	snaps, items, err := s.snapshotBundle(ctx, job, dir, &in)
	defer closeSnapshots(snaps)

	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return nil, bundleFailed("snapshot")
	}

	for _, sn := range snaps {
		doc, derr := decodeRow(&sn.row)
		if derr != nil {
			return nil, derr
		}

		sn.doc = doc
	}

	manifest, files, ferr := s.bundleManifest(snaps, items, in.Whole)
	if ferr != nil {
		return nil, ferr
	}

	main := snaps[0]
	if main.logf != nil {
		manifest.Operations = &BundleLog{Count: main.log.count, SHA256: main.log.sha256}
	}

	final := filepath.Join(dir, bundleFileName(main.row.Revision, job.ID))
	tmp := final + ".tmp"

	size, sum, werr := s.writeBundleFile(ctx, pw, tmp, manifest, snaps, files)
	if werr != nil {
		_ = os.Remove(tmp)

		return nil, werr
	}

	crashpoint.Hit("bundle_export.after_write")

	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)

		return nil, internalError()
	}

	crashpoint.Hit("bundle_export.after_rename")

	abs, aerr := filepath.Abs(final)
	if aerr != nil {
		return nil, internalError()
	}

	raw, merr := json.Marshal(JobResult{Export: &ExportResult{
		Revision:    main.row.Revision,
		Path:        abs,
		DownloadURL: "/api/v1/jobs/" + job.ID + "/file",
		Bytes:       size,
		SHA256:      sum,
		Checks:      []VerifyCheck{},
	}})
	if merr != nil {
		_ = os.Remove(final)

		return nil, internalError()
	}

	return raw, nil
}

// writeOperationsLog writes revisions 1..head as one JSON object per line and rewinds f.
func writeOperationsLog(
	ctx context.Context, ops OperationRepo, projectID string, head int64, f *os.File,
) (bundleLog, error) {
	h := sha256.New()
	bw := bufio.NewWriter(io.MultiWriter(f, h))
	enc := json.NewEncoder(bw)

	var count, after int64

	for after < head {
		rows, err := ops.Since(ctx, projectID, after, OperationFilter{})
		if err != nil {
			return bundleLog{}, err
		}

		if len(rows) == 0 {
			break
		}

		for i := range rows {
			r := &rows[i]
			if r.Revision != after+1 {
				return bundleLog{}, fmt.Errorf("service: the log of %s skips revision %d", projectID, after+1)
			}

			line := bundleOperation{
				Revision: r.Revision, Kind: r.Kind, Actor: r.Actor, BaseRevision: r.BaseRevision,
				TargetRevision: r.TargetRevision, AtMs: r.AtMs, Ops: r.Ops, Inverse: r.Inverse, Diff: r.Diff,
				Skipped: r.Skipped, SchemaVersion: r.SchemaVersion, DocSHA256: r.DocSHA256, ClosedBy: r.ClosedBy,
			}
			if err := enc.Encode(line); err != nil { // Encode ends the line with \n
				return bundleLog{}, err
			}

			after, count = r.Revision, count+1
		}
	}

	if err := bw.Flush(); err != nil {
		return bundleLog{}, err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return bundleLog{}, err
	}

	return bundleLog{count: count, sha256: hex.EncodeToString(h.Sum(nil))}, nil
}

// bundleManifest lists the media and fonts of the documents, in a stable order, after checking that every file
// is in the store (missing_media). The first snapshot is the main timeline; the others, and the items, are
// described when the bundle is of a whole project.
func (s *svc) bundleManifest(
	snaps []*bundleSnapshot, items []ItemRow, whole bool,
) (*BundleManifest, []bundleFile, *Error) {
	row := &snaps[0].row
	docSum := sha256.Sum256(row.Document)
	m := &BundleManifest{
		Format: BundleFormat, BundleVersion: BundleVersion, AppVersion: s.version,
		CreatedAt: s.now().UTC().Format("2006-01-02T15:04:05Z"),
		Project:   BundleProject{ID: row.ID, Name: row.Name, SchemaVersion: row.SchemaVersion, Revision: row.Revision},
		Document:  BundleDocument{SHA256: hex.EncodeToString(docSum[:]), Bytes: int64(len(row.Document))},
		Media:     []BundleMedia{}, Fonts: []BundleFontFile{},
	}

	if !whole {
		m.BundleVersion = 1
	}

	var files []bundleFile

	index := map[string]int{} // hash to position in m.Media
	seen := map[string]bool{}

	for _, sn := range snaps {
		more, err := s.addDocumentFiles(m, sn.doc, index, seen)
		if err != nil {
			return nil, nil, err
		}

		files = append(files, more...)
	}

	if whole {
		describeProduction(m, snaps, items)
	}

	return m, files, nil
}

// addDocumentFiles adds the media and fonts of one document to the manifest, once each, and returns the files
// that go into the zip.
func (s *svc) addDocumentFiles(
	m *BundleManifest, doc *timeline.Document, index map[string]int, seen map[string]bool,
) ([]bundleFile, *Error) {
	var files []bundleFile

	for _, id := range sortedKeys(doc.Assets) {
		a := doc.Assets[id]

		if i, ok := index[a.Hash]; ok {
			if !slices.Contains(m.Media[i].AssetIDs, a.ID) {
				m.Media[i].AssetIDs = append(m.Media[i].AssetIDs, a.ID)
			}

			continue
		}

		size, err := s.storedSize(a.Hash)
		if err != nil {
			return nil, err
		}

		index[a.Hash] = len(m.Media)
		m.Media = append(m.Media,
			BundleMedia{Hash: a.Hash, Kind: string(a.Kind), Name: a.Name, Bytes: size, AssetIDs: []string{a.ID}})
		files = append(files, bundleFile{entry: bundleMediaEntry(a.Hash), hash: a.Hash, size: size})
	}

	for _, family := range sortedKeys(doc.Fonts) {
		for _, f := range doc.Fonts[family].Files {
			if seen[family+"/"+f.Hash] {
				continue
			}

			size, err := s.fontSize(f.Hash)
			if err != nil {
				return nil, err
			}

			seen[family+"/"+f.Hash] = true
			m.Fonts = append(m.Fonts, BundleFontFile{Hash: f.Hash, Family: family, Weight: f.Weight, Bytes: size})

			if !seen[f.Hash] {
				seen[f.Hash] = true
				files = append(files, bundleFile{entry: bundleFontEntry(f.Hash), hash: f.Hash, size: size})
			}
		}
	}

	return files, nil
}

// describeProduction writes the members of version 2: the timelines after the first, and the active items of
// the project in order with their timelines named by key.
func describeProduction(m *BundleManifest, snaps []*bundleSnapshot, items []ItemRow) {
	keys := map[string]string{} // timeline id to key

	for _, sn := range snaps {
		keys[sn.row.ID] = sn.key

		if sn.key == bundleMainKey {
			continue
		}

		sum := sha256.Sum256(sn.row.Document)
		t := BundleTimeline{
			Key: sn.key, ID: sn.row.ID, Name: sn.row.Name, SchemaVersion: sn.row.SchemaVersion,
			Revision: sn.row.Revision,
			Document: BundleDocument{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(sn.row.Document))},
		}

		if sn.logf != nil {
			t.Operations = &BundleLog{Count: sn.log.count, SHA256: sn.log.sha256}
		}

		m.Timelines = append(m.Timelines, t)
	}

	p := &BundleProduction{Name: snaps[0].row.Name, Items: []BundleItem{}}

	for i := range items {
		it := &items[i]
		if it.State != ProjectActive {
			continue
		}

		bi := BundleItem{Kind: it.Kind, Title: it.Title, Position: len(p.Items)}

		switch it.Kind {
		case string(project.Timeline):
			key, ok := keys[it.Ref]
			if !ok {
				continue
			}

			bi.Timeline = key
		case string(project.Brief), string(project.Text):
			if it.Body != "" {
				sum := sha256.Sum256([]byte(it.Body))
				bi.Text = &BundleDocument{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(it.Body))}

				if m.texts == nil {
					m.texts = map[string]string{}
				}

				m.texts[bundleItemTextEntry(len(p.Items))] = it.Body
			}
		case string(project.Output):
			bi.Format = it.Format
		}

		p.Items = append(p.Items, bi)
	}

	m.Production = p
}

func (s *svc) storedSize(hash string) (int64, *Error) {
	f, err := s.media.Content.Open(strings.TrimPrefix(hash, "sha256:"))
	if err != nil {
		return 0, bundleFailed("missing_media").withDetail("hash", hash)
	}

	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return 0, bundleFailed("missing_media").withDetail("hash", hash)
	}

	return st.Size(), nil
}

// ctxReader stops a copy when the context ends, so a cancelled export does not finish a 3 GiB file.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}

	return c.r.Read(p)
}

// countWriter counts and hashes what the zip writes to the file.
type countWriter struct {
	w io.Writer
	n int64
	h io.Writer
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	_, _ = c.h.Write(p[:n])

	return n, err
}

// writeBundleFile streams the zip to path: manifest first, then the document and log of the first timeline,
// those of the others, media and fonts. Media is copied while its SHA-256 is recomputed; a mismatch is
// store_corrupt.
func (s *svc) writeBundleFile(
	ctx context.Context, pw *progress, path string, m *BundleManifest, snaps []*bundleSnapshot,
	files []bundleFile,
) (int64, string, error) {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, "", internalError()
	}

	defer func() { _ = out.Close() }()

	h := sha256.New()
	buffered := bufio.NewWriterSize(out, 1<<20)
	cw := &countWriter{w: buffered, h: h}
	zw := zip.NewWriter(cw)

	manifest, merr := json.MarshalIndent(m, "", "\t")
	if merr != nil {
		return 0, "", internalError()
	}

	text := func(name string, r io.Reader) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return err
		}

		_, err = io.Copy(w, ctxReader{ctx, r})

		return err
	}

	if err := text(bundleManifestEntry, strings.NewReader(string(manifest))); err != nil {
		return 0, "", copyError(ctx, err)
	}

	for _, name := range sortedKeys(m.texts) {
		if err := text(name, strings.NewReader(m.texts[name])); err != nil {
			return 0, "", copyError(ctx, err)
		}
	}

	for _, sn := range snaps {
		docEntry, logEntry := bundleDocumentEntry, bundleOperationsName
		if sn.key != bundleMainKey {
			docEntry, logEntry = bundleTimelineDocEntry(sn.key), bundleTimelineLogEntry(sn.key)
		}

		if err := text(docEntry, strings.NewReader(string(sn.row.Document))); err != nil {
			return 0, "", copyError(ctx, err)
		}

		if sn.logf != nil {
			if err := text(logEntry, sn.logf); err != nil {
				return 0, "", copyError(ctx, err)
			}
		}
	}

	var total, done int64
	for _, f := range files {
		total += f.size
	}

	for _, f := range files {
		if err := s.copyMedia(ctx, zw, f, func(n int64) {
			done += n
			if total > 0 {
				pw.report(ctx, float64(done)/float64(total), "Writing the bundle")
			}
		}); err != nil {
			return 0, "", err
		}
	}

	if err := zw.Close(); err != nil {
		return 0, "", copyError(ctx, err)
	}

	if err := buffered.Flush(); err != nil {
		return 0, "", internalError()
	}

	if err := out.Sync(); err != nil {
		return 0, "", internalError()
	}

	return cw.n, hex.EncodeToString(h.Sum(nil)), nil
}

// copyMedia copies one store file into the zip with io.Copy and checks the hash it saw.
func (s *svc) copyMedia(ctx context.Context, zw *zip.Writer, f bundleFile, progress func(int64)) error {
	src, err := s.openBundleSource(f)
	if err != nil {
		return bundleFailed("missing_media").withDetail("hash", f.hash)
	}

	defer func() { _ = src.Close() }()

	// Media is already compressed: stored, not deflated.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: f.entry, Method: zip.Store})
	if err != nil {
		return internalError()
	}

	h := sha256.New()

	if _, err := io.Copy(io.MultiWriter(w, h, progressWriter(progress)), ctxReader{ctx, src}); err != nil {
		return copyError(ctx, err)
	}

	if "sha256:"+hex.EncodeToString(h.Sum(nil)) != f.hash {
		return bundleFailed("store_corrupt").withDetail("hash", f.hash)
	}

	return nil
}

type progressWriter func(int64)

func (p progressWriter) Write(b []byte) (int, error) {
	p(int64(len(b)))

	return len(b), nil
}

// copyError keeps a cancellation as the context error, which the job runner settles as cancelled,
// and turns any other write failure into a typed export error.
func copyError(ctx context.Context, err error) error {
	if ctx.Err() != nil && (errors.Is(err, ctx.Err()) || errors.Is(err, context.Canceled)) {
		return ctx.Err()
	}

	return bundleFailed("write_failed")
}

// isFont tells a font entry of a bundle from a media one: fonts live in the font store, not in the
// content store.
func (f bundleFile) isFont() bool { return strings.HasPrefix(f.entry, "font.") }

// checkExportFonts keeps a bundle from copying out a font file the caller may not use: the font
// store is shared in network mode, and a document that names a hash does not prove the caller
// holds the file. Each font must be one the caller owns or uses in another project (SDD-007
// section 7.7 item 6). The message names the family, never the hash.
func (s *svc) checkExportFonts(ctx context.Context, c Caller, row *ProjectRow) *Error {
	if c.OwnerUserID == "" {
		return nil
	}

	doc, err := decodeRow(row)
	if err != nil {
		return internalError()
	}

	family, bad := s.unusableFont(ctx, s.store, doc, row.ID)
	if !bad {
		return nil
	}

	return newError(CodeForbidden, "the font family %q of the project is a file you did not import or bring.", family).
		withHint("import the font file with font:import, or remove the family with font.remove, then export again").
		withPath("fonts")
}
