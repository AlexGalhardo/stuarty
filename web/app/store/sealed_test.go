package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/chat"
	core "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/library"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/webhooks"
)

// The words of the person, each unique, so a scan of the file finds exactly what leaked.
const (
	wordsItem        = "CANARY-ITEM-body-of-my-script"
	wordsRevision    = "CANARY-REVISION-second-draft"
	wordsChat        = "CANARY-CHAT-message-to-the-agent"
	wordsPubText     = "CANARY-PUBTEXT-caption-for-the-video"
	wordsField       = "CANARY-FIELD-title-on-youtube"
	wordsSnapshot    = "CANARY-SNAPSHOT-approved-post"
	wordsLibrary     = "CANARY-LIBRARY-body-of-a-memory-note"
	wordsDescription = "CANARY-DESCRIPTION-what-the-project-is"
	wordsGoal        = "CANARY-GOAL-specific-measurable-parts"
	wordsHookToken   = "CANARY-HOOKTOKEN-in-the-path"
)

type locker struct {
	account.Sealer
	locked bool
}

func (l *locker) Seal(a, b, c, d string) (string, error) {
	if l.locked {
		return "", account.ErrLocked
	}

	return l.Sealer.Seal(a, b, c, d)
}

func (l *locker) Open(a, b, c, d string) (string, error) {
	if l.locked {
		return "", account.ErrLocked
	}

	return l.Sealer.Open(a, b, c, d)
}

func (l *locker) Sealing() bool { return true }

func writeEverything(t *testing.T, s *Store) {
	t.Helper()

	if err := s.Webhooks().Insert(ctx, webhooks.Row{
		ID: "whk_1", Name: "receiver", URL: "https://hooks.example.com/" + wordsHookToken + "?k=1",
		Events: []string{"export.ready"}, Enabled: true, CreatedBy: "local:ui", CreatedMs: 9, UpdatedMs: 9,
	}); err != nil {
		t.Fatalf("webhook: %v", err)
	}

	if err := s.Productions().Create(ctx, service.ProductionHead{
		ID: "prj_1", Name: "p", State: service.ProjectActive, CreatedMs: 10, UpdatedMs: 10, CreatedBy: "local:ui",
		Description: wordsDescription,
		Goal: project.Goal{
			Specific: wordsGoal + "-1", Measurable: wordsGoal + "-2", Achievable: wordsGoal + "-3",
			Relevant: wordsGoal + "-4", TimeBound: wordsGoal + "-5", Due: "2027-01-31",
		},
	}); err != nil {
		t.Fatalf("project: %v", err)
	}

	items := s.Productions()
	if err := items.CreateItem(ctx, service.ItemRow{
		ID: "pit_1", ProjectID: "prj_1", Kind: "text", Title: "Script", Position: 1, Body: wordsItem,
		CreatedMs: 10, UpdatedMs: 10, CreatedBy: "local:ui", TextKind: "script",
	}); err != nil {
		t.Fatalf("write step 1: %v", err)
	}

	if err := items.AddTextRevision(ctx, service.TextRevisionRow{
		ItemID: "pit_1", Revision: 2, Body: wordsRevision, Actor: "local:ui", Source: "write", CreatedMs: 11,
	}, 0); err != nil {
		t.Fatalf("write step 2: %v", err)
	}

	chatRepo := s.Chat()
	if err := chatRepo.SessionCreate(ctx, agenthost.Session{
		ID: "cht_1", ProjectID: "prj_1", Profile: "p", StartedBy: "local:ui", State: chat.StateIdle,
		CreatedAt: time.UnixMilli(10).UTC(), UpdatedAt: time.UnixMilli(10).UTC(),
	}); err != nil {
		t.Fatalf("write step 3: %v", err)
	}

	if err := chatRepo.EventUpsert(ctx, "cht_1", chat.Event{
		Seq: 1, Kind: chat.KindUserMessage, At: time.UnixMilli(12).UTC(), Text: wordsChat,
	}); err != nil {
		t.Fatalf("write step 4: %v", err)
	}

	now := time.UnixMilli(13).UTC()
	if err := s.Publications().Insert(ctx, &pub.Publication{
		ID:        "pub_1",
		ProjectID: "prj_1",
		Title:     "t",
		Text:      wordsPubText,
		Export:    "job_1",
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "local:ui",
		Targets: []pub.Target{
			{
				ID:            "ptg_1",
				PublicationID: "pub_1",
				ConnectionID:  "con_1",
				Network:       "youtube",
				Fields: pub.Fields{
					Title: wordsField,
				},
				State:     "draft",
				Zone:      "UTC",
				Snapshot:  []byte(`{"text":"` + wordsSnapshot + `"}`),
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}); err != nil {
		t.Fatalf("write step 5: %v", err)
	}
}

func readBack(t *testing.T, s *Store) {
	t.Helper()

	hook, err := s.Webhooks().Get(ctx, "whk_1")
	if err != nil || hook.Name != "receiver" ||
		hook.URL != "https://hooks.example.com/"+wordsHookToken+"?k=1" {
		t.Fatalf("webhook %+v %v", hook, err)
	}

	prj, err := s.Productions().Get(ctx, "prj_1")
	if err != nil || prj.Description != wordsDescription || prj.Goal.Specific != wordsGoal+"-1" ||
		prj.Goal.TimeBound != wordsGoal+"-5" || prj.Goal.Due != "2027-01-31" {
		t.Fatalf("project %+v %v", prj, err)
	}

	it, err := s.Productions().Item(ctx, "pit_1")
	if err != nil || it.Body != wordsRevision {
		t.Fatalf("item %q %v", it.Body, err)
	}

	rev, err := s.Productions().TextRevision(ctx, "pit_1", 1)
	if err != nil || rev.Body != wordsItem {
		t.Fatalf("revision 1 %q %v", rev.Body, err)
	}

	latest, err := s.Productions().TextLatest(ctx, "pit_1")
	if err != nil || latest.Body != wordsRevision {
		t.Fatalf("latest %q %v", latest.Body, err)
	}

	events, err := s.Chat().EventList(ctx, "cht_1", 0, 10)
	if err != nil || len(events) != 1 || events[0].Text != wordsChat {
		t.Fatalf("events %+v %v", events, err)
	}

	p, err := s.Publications().Get(ctx, "pub_1")
	if err != nil || p.Text != wordsPubText || p.Targets[0].Fields.Title != wordsField ||
		!strings.Contains(string(p.Targets[0].Snapshot), wordsSnapshot) {
		t.Fatalf("publication %+v %v", p, err)
	}
}

// bytesOfTheWorkspace is db.sqlite and its write-ahead log: everything a copy of the workspace carries.
func bytesOfTheWorkspace(t *testing.T, s *Store, dir string) []byte {
	t.Helper()

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	var all []byte

	for _, name := range []string{"db.sqlite", "db.sqlite-wal"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			all = append(all, raw...)
		}
	}

	return all
}

func TestSecurity_T108_SealedColumnsHoldNoPlaintextInTheDatabaseFile(t *testing.T) {
	canaries := []string{
		wordsItem, wordsRevision, wordsChat, wordsPubText, wordsField, wordsSnapshot, wordsDescription,
		wordsGoal + "-1", wordsGoal + "-2", wordsGoal + "-3", wordsGoal + "-4", wordsGoal + "-5",
		wordsHookToken,
	}

	// The control: with no account the same scan finds every word, so the scan can see what it looks for.
	dir := newDir(t)
	control, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	writeEverything(t, control)

	found := bytesOfTheWorkspace(t, control, string(dir))
	for _, w := range canaries {
		if !bytes.Contains(found, []byte(w)) {
			t.Fatalf("the control does not see %q: the scan is blind", w)
		}
	}

	// With an account, nothing of it is in the file, and everything reads back.
	dir = newDir(t)

	st, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	key := bytes.Repeat([]byte{42}, account.KeySize)

	keys, err := account.NewKeySealer(key)
	if err != nil {
		t.Fatal(err)
	}

	l := &locker{Sealer: keys}
	st.SetSealer(l)

	writeEverything(t, st)
	readBack(t, st)

	// Locked: the same reads refuse, they never return the sealed text or an empty one.
	l.locked = true

	if _, err := st.Productions().Item(ctx, "pit_1"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("an item read while locked: %v", err)
	}

	if _, err := st.Productions().Get(ctx, "prj_1"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a project read while locked: %v", err)
	}

	if _, err := st.Chat().EventList(ctx, "cht_1", 0, 10); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a chat read while locked: %v", err)
	}

	if _, err := st.Publications().Get(ctx, "pub_1"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a publication read while locked: %v", err)
	}

	if _, err := st.Webhooks().Get(ctx, "whk_1"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a webhook read while locked: %v", err)
	}

	if _, err := st.Webhooks().List(ctx); !errors.Is(err, account.ErrLocked) {
		t.Errorf("the webhook list read while locked: %v", err)
	}

	if err := st.Productions().
		UpdateItem(ctx, service.ItemRow{ID: "pit_1", Body: "x"}); !errors.Is(
		err,
		account.ErrLocked,
	) {
		t.Errorf("a write while locked: %v", err)
	}

	l.locked = false

	raw := bytesOfTheWorkspace(t, st, string(dir))
	for _, w := range canaries {
		if bytes.Contains(raw, []byte(w)) {
			t.Errorf("the database file holds %q in clear", w)
		}
	}

	// Ids, states and sizes stay readable for the queries (SDD-005 section 5.17).
	for _, readable := range []string{"pit_1", "prj_1", "pub_1"} {
		if !bytes.Contains(raw, []byte(readable)) {
			t.Errorf("%q is not readable in the file: the queries need it", readable)
		}
	}

	// A copied cell does not open in another row: the first revision body, planted as the second.
	reopened, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	reopened.SetSealer(l)

	if _, err := reopened.DB().ExecContext(ctx,
		`UPDATE text_revisions SET body = (SELECT body FROM text_revisions WHERE revision = 1) WHERE revision = 2`); err != nil {
		t.Fatal(err)
	}

	if _, err := reopened.Productions().TextRevision(ctx, "pit_1", 2); !errors.Is(err, account.ErrUnsealed) {
		t.Errorf("a body copied into another revision opened: %v", err)
	}

	// A clear value in a sealed column, once an account exists, is refused and never shown.
	if _, err := reopened.DB().
		ExecContext(ctx, `UPDATE project_items SET body = 'planted' WHERE id = 'pit_1'`); err != nil {
		t.Fatal(err)
	}

	if _, err := reopened.Productions().Item(ctx, "pit_1"); !errors.Is(err, account.ErrUnsealed) {
		t.Errorf("a planted clear body was accepted: %v", err)
	}
}

// The title of a chat session is the first message of the person (core/chat Title), so it is sealed like the
// message it repeats, and a session that ends while the studio is locked keeps it.
func TestSecurity_T108_TheTitleOfAChatSessionIsSealed(t *testing.T) {
	const words = "CANARY-CHATTITLE-first-message-of-the-person"

	dir := newDir(t)

	st, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	keys, err := account.NewKeySealer(bytes.Repeat([]byte{7}, account.KeySize))
	if err != nil {
		t.Fatal(err)
	}

	l := &locker{Sealer: keys}
	st.SetSealer(l)

	if err := st.Productions().Create(ctx, service.ProductionHead{
		ID: "prj_1", Name: "p", State: service.ProjectActive, CreatedMs: 10, UpdatedMs: 10, CreatedBy: "local:ui",
	}); err != nil {
		t.Fatal(err)
	}

	at := time.UnixMilli(10).UTC()
	row := agenthost.Session{
		ID: "cht_1", ProjectID: "prj_1", Profile: "p", StartedBy: "local:ui", State: chat.StateIdle,
		CreatedAt: at, UpdatedAt: at,
	}

	if err := st.Chat().SessionCreate(ctx, row); err != nil {
		t.Fatal(err)
	}

	row.Title = words
	if err := st.Chat().SessionUpdate(ctx, row); err != nil {
		t.Fatal(err)
	}

	l.locked = true
	row.State, row.Title = chat.StateEnded, ""

	if err := st.Chat().SessionUpdate(ctx, row); err != nil {
		t.Fatalf("a session could not end while the studio is locked: %v", err)
	}

	if got, err := st.Chat().SessionGet(ctx, "cht_1"); err != nil || got.Title != "" || got.State != chat.StateEnded {
		t.Fatalf("read while locked: %+v %v", got, err)
	}

	l.locked = false

	got, err := st.Chat().SessionGet(ctx, "cht_1")
	if err != nil || got.Title != words {
		t.Fatalf("after the unlock the title is %+v %v", got, err)
	}

	if raw := bytesOfTheWorkspace(t, st, string(dir)); bytes.Contains(raw, []byte(words)) {
		t.Error("the database file holds the title of a chat session in clear")
	}
}

// The prompt of a generated file is the same text the job and the approval request seal. The account creation
// seals it and takes it out of the search index, and a later write keeps it out.
func TestSecurity_T108_ThePromptOfAGeneratedFileIsSealed(t *testing.T) {
	const words = "CANARYPROMPT boats under the moon"

	dir := newDir(t)

	st, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	for _, h := range []string{"sha256:aa", "sha256:bb"} {
		if _, _, err := st.Assets().Upsert(ctx, service.AssetRow{
			Hash: h, SizeBytes: 1, OriginalName: "clip.mp4", IngestState: service.IngestReady, CreatedMs: 1, UpdatedMs: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	row := assets.Row{
		Hash: "sha256:aa", CreatedBy: "agent", CreatedMs: 5, UpdatedMs: 6,
		Origin: asset.Origin{Kind: asset.Generated, Provider: "acme", Model: "vidmodel", Prompt: words},
	}

	if err := st.AssetLibrary().Put(ctx, row); err != nil {
		t.Fatal(err)
	}

	if got := assetSearchOf(t, st, "CANARYPROMPT"); len(got) != 1 {
		t.Fatalf("the control: with no account the prompt is searchable, got %v", got)
	}

	keys, err := account.NewKeySealer(bytes.Repeat([]byte{9}, account.KeySize))
	if err != nil {
		t.Fatal(err)
	}

	if err := st.CreateAccount(ctx, accountRecord(), keys, nil); err != nil {
		t.Fatal(err)
	}

	l := &locker{Sealer: keys}
	st.SetSealer(l)

	if err := st.CompactSealed(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := st.AssetLibrary().Get(ctx, "", "sha256:aa")
	if err != nil || got.Origin.Prompt != words {
		t.Fatalf("the prompt after the creation: %q %v", got.Origin.Prompt, err)
	}

	if hits := assetSearchOf(t, st, "CANARYPROMPT"); len(hits) != 0 {
		t.Errorf("the index still finds the prompt: %v", hits)
	}

	if hits := assetSearchOf(t, st, "vidmodel"); len(hits) != 1 {
		t.Errorf("the index lost the other fields of the origin: %v", hits)
	}

	row.Hash = "sha256:bb"
	if err := st.AssetLibrary().Put(ctx, row); err != nil {
		t.Fatal(err)
	}

	if hits := assetSearchOf(t, st, "CANARYPROMPT"); len(hits) != 0 {
		t.Errorf("a write with an account indexed the prompt: %v", hits)
	}

	l.locked = true

	if locked, err := st.AssetLibrary().Get(ctx, "", "sha256:aa"); err != nil || locked.Origin.Prompt != "" {
		t.Errorf("read while locked: %q %v", locked.Origin.Prompt, err)
	}

	if err := st.AssetLibrary().Put(ctx, row); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a write while locked: %v", err)
	}

	l.locked = false

	if err := st.CompactSealed(ctx); err != nil {
		t.Fatal(err)
	}

	if raw := bytesOfTheWorkspace(t, st, string(dir)); bytes.Contains(raw, []byte("CANARYPROMPT")) {
		t.Error("the database file holds the prompt of a generated file in clear")
	}
}

func TestTheLibraryIndexKeepsNoBodyWhenThereIsAnAccount(t *testing.T) {
	s := testStore(t)

	entry := func() { // an index entry the way the manager writes it
		if err := s.Library().
			Put(ctx, library.Entry{Key: "memory/note", Kind: core.KindMemory, Name: "note", Description: "a note", State: library.StateOK, Path: "memory/note.md", Sig: "1", CreatedMs: 1, UpdatedMs: 1}, wordsLibrary); err != nil {
			t.Fatal(err)
		}
	}

	entry()

	if n := scalar[int](t, s, `SELECT count(*) FROM library_fts WHERE body LIKE '%CANARY-LIBRARY%'`); n != 1 {
		t.Fatalf("with no account the index keeps the body (%d rows)", n)
	}

	keys, err := account.NewKeySealer(bytes.Repeat([]byte{1}, account.KeySize))
	if err != nil {
		t.Fatal(err)
	}

	s.SetSealer(keys)
	entry()

	if n := scalar[int](t, s, `SELECT count(*) FROM library_fts WHERE body LIKE '%CANARY-LIBRARY%'`); n != 0 {
		t.Fatalf("with an account the index keeps a body (%d rows)", n)
	}
}
