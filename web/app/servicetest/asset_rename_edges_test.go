package servicetest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The edges of epic 94 for the rename of a file (SDD-011 section 14.8): each test names the case it covers. The
// invisible characters are built from their code points so that the source shows what each case sends.

func (m *mediaEnv) rename(ctx context.Context, hash, name string) (*service.AssetResult, *service.Error) {
	m.t.Helper()

	res, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: hash, Name: name})
	if err == nil {
		return res, nil
	}

	se, ok := errors.AsType[*service.Error](err)
	if !ok {
		m.t.Fatalf("AssetRename: %v is not a service error", err)
	}

	return nil, se
}

func (m *mediaEnv) renameAudits() []service.AuditEntry {
	m.t.Helper()

	return m.audits("asset.rename")
}

func nameInvalid(se *service.Error) bool {
	return se != nil && se.Code == command.CodeValidation && se.Details["reason"] == "name_invalid"
}

func TestAssetRenameAnEmptyOrBlankNameIsRefused(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	blanks := []string{
		"", " ", "   ", "\t", "\n", " \t \n ",
		string(rune(0xA0)),   // a no-break space
		string(rune(0x3000)), // an ideographic space
	}

	for _, typed := range blanks {
		if _, se := m.rename(ctx, hash, typed); !nameInvalid(se) {
			t.Errorf("rename to %q = %+v, want E_VALIDATION name_invalid", typed, se)
		}
	}

	if got := m.shownName(ctx, project); got != "clip.mp4" {
		t.Errorf("the name is %q after refused renames", got)
	}

	if n := len(m.renameAudits()); n != 0 {
		t.Errorf("refused renames wrote %d audit rows", n)
	}
}

// The title limit is 120 characters for the whole name, the extension included.
func TestAssetRenameANameOverTheLimitIsRefusedAndTheLimitItselfIsKept(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	atLimit := strings.Repeat("a", asset.MaxName-len(".mp4"))
	if _, se := m.rename(ctx, hash, atLimit); se != nil {
		t.Fatalf("a name of exactly %d characters was refused: %v", asset.MaxName, se)
	}

	if got := m.shownName(ctx, project); len([]rune(got)) != asset.MaxName {
		t.Errorf("the name has %d characters, want %d", len([]rune(got)), asset.MaxName)
	}

	if _, se := m.rename(ctx, hash, atLimit+"b"); !nameInvalid(se) {
		t.Errorf("a name of %d characters = %+v, want E_VALIDATION name_invalid", asset.MaxName+1, se)
	}

	// The characters of a multi-byte name count as characters, not bytes.
	accented := strings.Repeat(string(rune(0xE9)), asset.MaxName-len(".mp4"))
	if _, se := m.rename(ctx, hash, accented); se != nil {
		t.Errorf("120 accented characters were refused: %v", se)
	}
}

func TestAssetRenameControlAndBidirectionalCharactersAreRefused(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	between := func(r rune) string { return "a" + string(r) + "b" }

	for name, typed := range map[string]string{
		"a right-to-left override":    "Holiday" + string(rune(0x202E)) + "fdp.exe",
		"a left-to-right isolate":     between(0x2066),
		"a right-to-left mark":        between(0x200F),
		"a zero-width space":          between(0x200B),
		"a byte order mark":           between(0xFEFF),
		"a NUL":                       between(0),
		"a bell":                      between(7),
		"a newline":                   between('\n'),
		"a line separator":            between(0x2028),
		"a tag character":             between(0xE0041),
		"a path separator":            "a/b",
		"a backslash":                 "a\\b",
		"a character Windows refuses": "a:b",
		"a device name of Windows":    "CON",
	} {
		if _, se := m.rename(ctx, hash, typed); !nameInvalid(se) {
			t.Errorf("%s: rename to %q = %+v, want E_VALIDATION name_invalid", name, typed, se)
		}
	}

	if got := m.shownName(ctx, project); got != "clip.mp4" {
		t.Errorf("the name is %q after refused renames", got)
	}
}

// Renaming to the name a file has changes nothing: no row, no audit row, no event, and the answer is the entry.
func TestAssetRenameToTheCurrentNameIsANoOpWithNoAuditRow(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")
	in := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	events := func() int {
		n := 0

		for _, ev := range m.eventsOf("asset.updated") {
			if ev.ProjectID == project && ev.AssetID == in.AssetID && ev.State == "ready" {
				n++
			}
		}

		return n
	}

	if _, se := m.rename(ctx, in.Hash, "Beach"); se != nil {
		t.Fatal(se)
	}

	audits, before := len(m.renameAudits()), events()
	if audits != 1 {
		t.Fatalf("the first rename wrote %d audit rows, want 1", audits)
	}

	for _, same := range []string{"Beach", "Beach.mp4", "  Beach  "} {
		res, se := m.rename(ctx, in.Hash, same)
		if se != nil || res.Name != "Beach.mp4" {
			t.Fatalf("rename to the same name %q = %+v, %v", same, res, se)
		}
	}

	if got := len(m.renameAudits()); got != audits {
		t.Errorf("a rename that changed nothing wrote an audit row: %d, was %d", got, audits)
	}

	if got := events(); got != before {
		t.Errorf("a rename that changed nothing told the editors: %d events, was %d", got, before)
	}
}

// A rename writes one audit row, with the actor and the file, and never the name.
func TestAssetRenameWritesOneAuditRowWithTheActorAndNoName(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, second := m.newProject("one"), m.newProject("two")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	m.importOne(second, m.write("clip.mp4", clipBytes))

	if _, se := m.rename(m.as(ui), hash, "A private holiday"); se != nil {
		t.Fatal(se)
	}

	rows := m.renameAudits()
	if len(rows) != 1 {
		t.Fatalf("the rename of a file held by two projects wrote %d audit rows, want 1", len(rows))
	}

	if rows[0].Actor != ui || rows[0].SubjectID != hash {
		t.Errorf("audit row = %+v, want actor %s and the hash", rows[0], ui)
	}

	if strings.Contains(string(rows[0].Details), "holiday") || strings.Contains(rows[0].SubjectID, "holiday") {
		t.Errorf("the audit row carries the name: %+v", rows[0])
	}
}

// Two files may share a name: the hash is the identity, and the library tells them apart by size and date.
func TestAssetRenameTwoFilesMayShareAName(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")

	first := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0].Hash
	m.clock.advance(48 * time.Hour)

	second := m.importOne(project, m.write("b.mp4", "a second clip with bytes of another length entirely")).Assets[0].Hash

	for _, hash := range []string{first, second} {
		if _, se := m.rename(ctx, hash, "Same"); se != nil {
			t.Fatalf("rename to a name another file has: %v", se)
		}
	}

	list, err := m.svc.AssetList(ctx, service.AssetListRequest{})
	if err != nil || len(list.Assets) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	a, b := list.Assets[0], list.Assets[1]
	if a.Name != "Same.mp4" || b.Name != "Same.mp4" || a.Hash == b.Hash {
		t.Errorf("entries = %q %q, hashes %s %s", a.Name, b.Name, a.Hash, b.Hash)
	}

	if a.Bytes == nil || b.Bytes == nil || *a.Bytes == *b.Bytes ||
		a.SizeText == b.SizeText && a.CreatedAt == b.CreatedAt {
		t.Errorf("the two cannot be told apart: sizes %v %v, dates %q %q", a.Bytes, b.Bytes, a.CreatedAt, b.CreatedAt)
	}
}

// An editor that has the file open shows the new name through the media list and the event, and the document, which
// holds the asset by id, is untouched: the rename is not a revision.
func TestAssetRenameAnOpenEditorShowsTheNewNameAndNothingBreaks(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")
	in := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	before := m.head(project).Revision
	doc := string(m.document(project))

	if _, se := m.rename(ctx, in.Hash, "Renamed in the library"); se != nil {
		t.Fatal(se)
	}

	if got := m.shownName(ctx, project); got != "Renamed in the library.mp4" {
		t.Errorf("the media list of the open editor says %q", got)
	}

	updated := 0

	for _, ev := range m.eventsOf("asset.updated") {
		if ev.ProjectID == project && ev.AssetID == in.AssetID {
			updated++
		}
	}

	if updated < 2 {
		t.Errorf("the editor got %d asset.updated events, want one for the import and one for the rename", updated)
	}

	if got := m.head(project).Revision; got != before {
		t.Errorf("the rename moved the revision of the document from %d to %d", before, got)
	}

	if string(m.document(project)) != doc {
		t.Error("the rename changed the document")
	}

	// The editor keeps working: the asset is still the same one by id.
	if list, err := m.svc.MediaList(ctx, service.MediaListRequest{ProjectID: project}); err != nil ||
		len(list.Assets) != 1 || list.Assets[0].AssetID != in.AssetID {
		t.Errorf("media list = %+v, %v", list, err)
	}

	history, err := m.svc.ActivityList(ctx, service.ActivityListRequest{ProjectID: project, Domain: "asset"})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]string)
	for _, entry := range history.Entries {
		seen[entry.Action] = entry.SubjectID
	}
	if seen["media.import"] != in.AssetID || seen["asset.rename.project"] != in.Hash {
		t.Errorf("project activity = %+v, want import and scoped rename", history.Entries)
	}
}

// A file that has no active use, only a use in a trash, is not in the library: the rename says why and what to do.
func TestAssetRenameAFileInTheTrashIsRefused(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")
	in := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	if _, err := m.svc.MediaTrash(ctx, service.MediaTrashRequest{ProjectID: project, AssetID: in.AssetID}); err != nil {
		t.Fatal(err)
	}

	_, se := m.rename(ctx, in.Hash, "Renamed")
	if se == nil || se.Code != command.CodeNotFound || se.Details["reason"] != "in_trash" || se.Hint == "" {
		t.Fatalf("rename of a trashed file = %+v, want E_NOT_FOUND in_trash with a hint", se)
	}

	if n := len(m.renameAudits()); n != 0 {
		t.Errorf("a refused rename wrote %d audit rows", n)
	}

	// Restored, it renames again.
	if _, err := m.svc.MediaRestore(
		ctx,
		service.MediaTrashRequest{ProjectID: project, AssetID: in.AssetID},
	); err != nil {
		t.Fatal(err)
	}

	if _, se := m.rename(ctx, in.Hash, "Renamed"); se != nil {
		t.Errorf("rename after the restore: %v", se)
	}
}

// Renames at the same time are serial transactions of the store: the last one wins for every row and none keeps the
// name of another, so no update is half applied and none is lost to a torn read.
func TestAssetRenameRacingRenamesLeaveOneConsistentName(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	one, two := m.newProject("one"), m.newProject("two")
	hash := m.importOne(one, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	m.importOne(two, m.write("clip.mp4", clipBytes))

	names := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel"}

	var wg sync.WaitGroup

	failures := make(chan error, len(names))

	for _, name := range names {
		wg.Go(func() {
			if _, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: hash, Name: name}); err != nil {
				failures <- err
			}
		})
	}

	wg.Wait()
	close(failures)

	for err := range failures {
		t.Errorf("a racing rename failed: %v", err)
	}

	a, b := m.shownName(ctx, one), m.shownName(ctx, two)
	if a != b {
		t.Errorf("the two projects disagree after the race: %q and %q", a, b)
	}

	won := false

	for _, name := range names {
		if a == name+".mp4" {
			won = true
		}
	}

	if !won {
		t.Errorf("the name %q is none of the names that were sent", a)
	}

	got, err := m.svc.AssetGet(ctx, service.AssetRef{Hash: hash})
	if err != nil || got.Name != a {
		t.Errorf("the entry says %q (%v), the projects say %q", got.Name, err, a)
	}

	if n := len(m.renameAudits()); n < 1 || n > len(names) {
		t.Errorf("%d audit rows for %d renames", n, len(names))
	}
}

// An agent renames with the media scope, as before, and the audit row says who.
func TestAssetRenameByAnAgentIsAllowedAndAuditedWithTheActor(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	if _, se := m.rename(m.as(agent), hash, "By the agent"); se != nil {
		t.Fatalf("an agent with the media scope was refused: %v", se)
	}

	rows := m.renameAudits()
	if len(rows) != 1 || rows[0].Actor != agent || rows[0].SubjectID != hash {
		t.Errorf("audit rows = %+v, want one for %s", rows, agent)
	}

	readOnly := service.WithCaller(context.Background(), service.Caller{
		Actor: agent, Scopes: []string{service.ScopeRead},
	})

	if _, se := m.rename(readOnly, hash, "Not allowed"); se == nil || se.Code != service.CodeForbidden {
		t.Errorf("a key with the read scope = %+v, want E_FORBIDDEN", se)
	}

	if got := m.shownName(m.as(ui), project); got != "By the agent.mp4" {
		t.Errorf("the name is %q", got)
	}
}

func TestAssetRenameALockedStudioAnswersLocked(t *testing.T) {
	g := &gate{}
	e := newEnvWith(t, func(_ *env, o *service.Options) { o.Gate = g })

	g.locked.Store(true)

	_, err := e.svc.AssetRename(e.as(ui), service.AssetRenameRequest{
		Hash: "sha256:" + strings.Repeat("a", 64), Name: "Anything",
	})

	if se, ok := errors.AsType[*service.Error](err); !ok || se.Code != service.CodeLocked {
		t.Errorf("rename while locked = %v, want %s", err, service.CodeLocked)
	}
}
