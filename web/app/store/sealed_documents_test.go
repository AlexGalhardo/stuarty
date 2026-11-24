package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

// The words of the second stage (ADR-0053): a caption and a text layer in a document, an operation of the edit log,
// the description and the rights note of a file, each unique, so a scan finds exactly what leaked.
const (
	docCaption   = "CANARY-CAPTION-hello-from-the-timeline"
	docTextLayer = "CANARY-TEXTLAYER-on-the-image"
	docTemplate  = "CANARY-TEMPLATE-text-of-a-template"
	docBrand     = "CANARY-BRAND-kit-voice"
	docOp        = "CANARY-OPERATION-an-edit-the-log-repeats"
	docInverse   = "CANARY-INVERSE-what-undo-restores"
	docDiff      = "CANARY-DIFF-the-change"
	docSkipped   = "CANARY-SKIPPED-reason"
	docResult    = "CANARY-RESULT-of-an-image-edit"
	docBackup    = "CANARY-BACKUP-an-older-version"
	docIdem      = "CANARY-IDEMPOTENT-answer"
	docAssetNote = "CANARY-ASSETDESC-what-the-file-shows"
	docRights    = "CANARY-RIGHTS-licence-note"
)

func canariesOfTheSecondStage() []string {
	return []string{
		docCaption, docTextLayer, docTemplate, docBrand, docOp, docInverse, docDiff, docSkipped, docResult, docBackup,
		docIdem, docAssetNote, docRights,
	}
}

func js(s string) json.RawMessage {
	b, err := json.Marshal(map[string]string{"text": s})
	if err != nil {
		panic(err)
	}

	return b
}

// writeDocuments writes one of every second-stage column.
func writeDocuments(t *testing.T, s *Store) {
	t.Helper()

	p := newProject("prj_9", 100)
	p.Document = js(docCaption)

	if err := s.Projects().Create(ctx, p); err != nil {
		t.Fatalf("timeline: %v", err)
	}

	if err := s.Operations().Append(ctx, service.OperationRow{
		ProjectID: "prj_9", Revision: 1, Kind: service.OpEdit, Actor: "local:ui", AtMs: 5, Ops: js(docOp),
		Inverse: js(docInverse), Diff: js(docDiff), Skipped: js(docSkipped), SchemaVersion: 1, DocSHA256: "sha",
	}); err != nil {
		t.Fatalf("operation: %v", err)
	}

	if err := s.Idempotency().Put(ctx, service.IdempotencyRow{
		ProjectID: "prj_9", Actor: "local:ui", Key: "k1", RequestHash: "h", Revision: 1, Result: js(docIdem),
		CreatedMs: 1, ExpiresMs: 1 << 40,
	}); err != nil {
		t.Fatalf("idempotency: %v", err)
	}

	if err := s.Backups().Put(ctx, service.BackupRow{
		OwnerKind: "project", OwnerID: "prj_9", Revision: 1, SchemaVersion: 1, Document: js(docBackup),
		Reason: "migrate", CreatedMs: 2,
	}); err != nil {
		t.Fatalf("backup: %v", err)
	}

	if err := s.Templates().Create(ctx, service.TemplateRow{
		ID:            "tpl_1",
		Name:          "t",
		SchemaVersion: 1,
		Document:      js(docTemplate),
		DocSHA256:     "sha",
		Slots:         json.RawMessage(`[]`),
		Summary:       service.DocSummary{Width: 1, Height: 1, FPSNum: 30, FPSDen: 1},
		CreatedMs:     1,
		CreatedBy:     "local:ui",
	}); err != nil {
		t.Fatalf("template: %v", err)
	}

	if err := s.BrandKit().Put(ctx, service.BrandKitRow{
		OwnerUserID: "", Document: js(docBrand), UpdatedMs: 1, UpdatedBy: "local:ui",
	}); err != nil {
		t.Fatalf("brand kit: %v", err)
	}

	image := service.ImageRow{Document: js(docTextLayer), DocSHA256: "sha"}
	image.ID, image.Name, image.Revision, image.SchemaVersion = "img_1", "i", 1, 1
	image.Width, image.Height, image.Layers = 1, 1, 1
	image.CreatedMs, image.UpdatedMs, image.CreatedBy = 1, 1, "local:ui"

	if err := s.Images().Create(ctx, image); err != nil {
		t.Fatalf("image: %v", err)
	}

	if err := s.Images().AppendOp(ctx, service.ImageOpRow{
		ImageID: "img_1", Revision: 1, Kind: service.OpEdit, Actor: "local:ui", AtMs: 5, Ops: js(docOp),
		Inverse: js(docInverse), Diff: js(docDiff), Skipped: js(docSkipped), DocSHA256: "sha", Result: js(docResult),
	}); err != nil {
		t.Fatalf("image op: %v", err)
	}

	if _, _, err := s.Assets().Upsert(ctx, service.AssetRow{
		Hash: "sha256:aa", SizeBytes: 1, OriginalName: "clip.mp4", IngestState: service.IngestReady, CreatedMs: 1,
		UpdatedMs: 1,
	}); err != nil {
		t.Fatalf("asset: %v", err)
	}

	if err := s.AssetLibrary().Put(ctx, assets.Row{
		Hash:        "sha256:aa",
		CreatedBy:   "agent",
		CreatedMs:   5,
		UpdatedMs:   6,
		Description: docAssetNote,
		Tags:        []string{"harbour"},
		Origin:      asset.Origin{Kind: asset.Imported, RightsNote: docRights},
	}); err != nil {
		t.Fatalf("asset info: %v", err)
	}
}

func readDocuments(t *testing.T, s *Store) {
	t.Helper()

	got, err := s.Projects().Get(ctx, "prj_9")
	if err != nil || !bytes.Equal(got.Document, js(docCaption)) || got.DocSHA256 != "sha-prj_9" {
		t.Fatalf("timeline: %q %v", got.Document, err)
	}

	op, err := s.Operations().Get(ctx, "prj_9", 1)
	if err != nil || string(op.Ops) != string(js(docOp)) || string(op.Inverse) != string(js(docInverse)) ||
		string(op.Diff) != string(js(docDiff)) || string(op.Skipped) != string(js(docSkipped)) {
		t.Fatalf("operation: %+v %v", op, err)
	}

	since, err := s.Operations().Since(ctx, "prj_9", 0, service.OperationFilter{})
	if err != nil || len(since) < 1 || string(since[0].Ops) != string(js(docOp)) {
		t.Fatalf("operations since: %+v %v", since, err)
	}

	idem, err := s.Idempotency().Get(ctx, "prj_9", "local:ui", "k1", 2)
	if err != nil || string(idem.Result) != string(js(docIdem)) {
		t.Fatalf("idempotency: %+v %v", idem, err)
	}

	list, err := s.Backups().List(ctx, "project", "prj_9")
	if err != nil || len(list) != 1 {
		t.Fatalf("backup list: %v %v", list, err)
	}

	b, err := s.Backups().Get(ctx, list[0].ID)
	if err != nil || string(b.Document) != string(js(docBackup)) {
		t.Fatalf("backup: %q %v", b.Document, err)
	}

	tpl, err := s.Templates().Get(ctx, "tpl_1")
	if err != nil || string(tpl.Document) != string(js(docTemplate)) {
		t.Fatalf("template: %q %v", tpl.Document, err)
	}

	kit, err := s.BrandKit().Get(ctx, "")
	if err != nil || string(kit.Document) != string(js(docBrand)) {
		t.Fatalf("brand kit: %q %v", kit.Document, err)
	}

	img, err := s.Images().Get(ctx, "img_1")
	if err != nil || string(img.Document) != string(js(docTextLayer)) {
		t.Fatalf("image: %q %v", img.Document, err)
	}

	iop, err := s.Images().OpsSince(ctx, "img_1", 0)
	if err != nil || len(iop) != 1 || string(iop[0].Ops) != string(js(docOp)) ||
		string(iop[0].Result) != string(js(docResult)) || string(iop[0].Skipped) != string(js(docSkipped)) {
		t.Fatalf("image op: %+v %v", iop, err)
	}

	info, err := s.AssetLibrary().Get(ctx, "", "sha256:aa")
	if err != nil || info.Description != docAssetNote || info.Origin.RightsNote != docRights {
		t.Fatalf("asset info: %+v %v", info, err)
	}
}

func sealedStore(t *testing.T) (*Store, *locker, string) {
	t.Helper()

	dir := newDir(t)
	st := openStore(t, dir, Options{})

	keys, err := account.NewKeySealer(bytes.Repeat([]byte{42}, account.KeySize))
	if err != nil {
		t.Fatal(err)
	}

	l := &locker{Sealer: keys}
	st.SetSealer(l)

	return st, l, string(dir)
}

// With an account the documents, the log, the backups, the idempotency results and the asset notes are not in the
// database file or its log, and everything reads back as it was.
func TestSecurity_T132_TheDocumentsTheLogAndTheAssetNotesHoldNoPlaintextInTheDatabaseFile(t *testing.T) {
	// The control: with no account the scan finds every word, so it can see what it looks for.
	controlDir := newDir(t)
	control := openStore(t, controlDir, Options{})
	writeDocuments(t, control)

	found := bytesOfTheWorkspace(t, control, string(controlDir))
	for _, w := range canariesOfTheSecondStage() {
		if !bytes.Contains(found, []byte(w)) {
			t.Fatalf("the control does not see %q: the scan is blind", w)
		}
	}

	st, l, dir := sealedStore(t)
	writeDocuments(t, st)
	readDocuments(t, st)

	// Locked: the same reads refuse, they never return the sealed text or an empty one.
	l.locked = true

	if _, err := st.Projects().Get(ctx, "prj_9"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a timeline read while locked: %v", err)
	}

	if _, err := st.Operations().Get(ctx, "prj_9", 1); !errors.Is(err, account.ErrLocked) {
		t.Errorf("an operation read while locked: %v", err)
	}

	if _, err := st.Images().Get(ctx, "img_1"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("an image read while locked: %v", err)
	}

	if err := st.Projects().
		Commit(ctx, "prj_9", 0, service.ProjectUpdate{Name: "x", Document: []byte(`{}`)}); !errors.Is(
		err,
		account.ErrLocked,
	) {
		t.Errorf("a commit while locked: %v", err)
	}

	l.locked = false

	raw := bytesOfTheWorkspace(t, st, dir)
	for _, w := range canariesOfTheSecondStage() {
		if bytes.Contains(raw, []byte(w)) {
			t.Errorf("the database file holds %q in clear", w)
		}
	}

	// Ids, states, sizes and the digest of the document stay readable.
	for _, readable := range []string{"prj_9", "img_1", "sha-prj_9"} {
		if !bytes.Contains(raw, []byte(readable)) {
			t.Errorf("%q is not readable in the file: the queries need it", readable)
		}
	}
}

// A row of the log copied to another revision or to another project does not open (ADR-0053).
func TestSecurity_T133_ARowOfTheLogCopiedToAnotherRevisionOrProjectDoesNotOpen(t *testing.T) {
	st, _, _ := sealedStore(t)
	writeDocuments(t, st)

	// A second project and a second revision of the first, made by the product.
	p := newProject("prj_8", 100)
	if err := st.Projects().Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	if err := st.Operations().Append(ctx, service.OperationRow{
		ProjectID: "prj_9", Revision: 2, Kind: service.OpEdit, Actor: "local:ui", AtMs: 6, Ops: js("two"),
		Inverse: js("two"), Diff: js("two"), SchemaVersion: 1, DocSHA256: "sha",
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.Operations().Append(ctx, service.OperationRow{
		ProjectID: "prj_8", Revision: 1, Kind: service.OpEdit, Actor: "local:ui", AtMs: 6, Ops: js("other"),
		Inverse: js("other"), Diff: js("other"), SchemaVersion: 1, DocSHA256: "sha",
	}); err != nil {
		t.Fatal(err)
	}

	// The sealed ops of (prj_9, 1) planted in (prj_9, 2), and in (prj_8, 1). The log is append-only, so the
	// triggers are put aside for the attack, as someone with write access to the file could.
	for _, stmt := range []string{
		`DROP TRIGGER operations_append_only`,
		`UPDATE operations SET ops = (SELECT ops FROM operations WHERE project_id = 'prj_9' AND revision = 1)
 WHERE project_id = 'prj_9' AND revision = 2`,
		`UPDATE operations SET inverse = (SELECT inverse FROM operations WHERE project_id = 'prj_9' AND revision = 1)
 WHERE project_id = 'prj_8' AND revision = 1`,
	} {
		if _, err := st.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := st.Operations().Get(ctx, "prj_9", 2); !errors.Is(err, account.ErrUnsealed) {
		t.Errorf("the ops of another revision opened: %v", err)
	}

	if _, err := st.Operations().Get(ctx, "prj_8", 1); !errors.Is(err, account.ErrUnsealed) {
		t.Errorf("the inverse of another project opened: %v", err)
	}

	// The same for an image, and for a document copied into another row.
	if _, err := st.DB().
		ExecContext(ctx, `UPDATE timelines SET document = (SELECT document FROM timelines WHERE id = 'prj_9')
 WHERE id = 'prj_8'`); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Projects().Get(ctx, "prj_8"); !errors.Is(err, account.ErrUnsealed) {
		t.Errorf("a document copied to another project opened: %v", err)
	}

	if _, err := st.Operations().Get(ctx, "prj_9", 1); err != nil {
		t.Errorf("the untouched row stopped opening: %v", err)
	}
}

// The asset search no longer matches a description once it is sealed (D-139 item 5); it still matches the name, the
// tags and the kind of origin.
func TestAssetSearchDoesNotMatchASealedDescription(t *testing.T) {
	st, _, _ := sealedStore(t)
	writeDocuments(t, st)

	if got := assetSearchOf(t, st, `"harbour"`); len(got) != 1 {
		t.Errorf("the tag must still match: %v", got)
	}

	if got := assetSearchOf(t, st, `"clip"`); len(got) != 1 {
		t.Errorf("the name must still match: %v", got)
	}

	if got := assetSearchOf(t, st, `"what"`); len(got) != 0 {
		t.Errorf("a sealed description was matched: %v", got)
	}
}

// A second-stage column that is not sealed is refused once the account is of seal version 1: the guards (SDD-005
// section 5.17.3) turn a clear write into E_LOCKED, and a clear value found on read is an error.
func TestSealedDocumentColumnsRefuseClearTextOnceTheAccountIsSealed(t *testing.T) {
	st, _, _ := sealedStore(t)
	writeDocuments(t, st)

	if err := installSealGuards(ctx, st.DB(), allSealedCells()); err != nil {
		t.Fatal(err)
	}

	_, err := st.DB().ExecContext(ctx, `UPDATE timelines SET document = '{"clear":true}' WHERE id = 'prj_9'`)
	if err == nil || !strings.Contains(err.Error(), guardMessage) {
		t.Errorf("a clear document was accepted: %v", err)
	}

	_, err = st.DB().ExecContext(ctx, `UPDATE asset_info SET description = 'clear' WHERE asset_hash = 'sha256:aa'`)
	if err == nil || !strings.Contains(err.Error(), guardMessage) {
		t.Errorf("a clear description was accepted: %v", err)
	}
}
