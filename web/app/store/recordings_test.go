package store

import (
	"bytes"
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

const wordsWindowTitle = "CANARY-WINDOW-title-of-my-bank-statement"

func recordingRow(state string) recordings.Row {
	return recordings.Row{
		ID: "rec_1", ProjectID: "prj_1", State: state, SourceKind: "window", SourceLabel: wordsWindowTitle, FPS: 30,
		Actor: "local:ui", CreatedMs: 5,
	}
}

// The title of a captured window is the user's own text (a bank page, a private document): it is sealed, and the
// recovery of start-up, which runs while the studio may be locked, neither needs it nor can blank it.
func TestSecurity_T122_TheTitleOfACapturedWindowIsSealed(t *testing.T) {
	// The control: with no account the scan sees the word, so it is able to see what it looks for.
	dir := newDir(t)

	control, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	if err := control.Recordings().Insert(ctx, recordingRow("recording")); err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(bytesOfTheWorkspace(t, control, string(dir)), []byte(wordsWindowTitle)) {
		t.Fatal("the control does not see the title: the scan is blind")
	}

	dir = newDir(t)

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

	if err := st.Recordings().Insert(ctx, recordingRow("recording")); err != nil {
		t.Fatal(err)
	}

	got, err := st.Recordings().Get(ctx, "rec_1")
	if err != nil || got.SourceLabel != wordsWindowTitle {
		t.Fatalf("read back %+v %v", got, err)
	}

	// Locked: the title is refused, never shown, never blank.
	l.locked = true

	if _, err := st.Recordings().Get(ctx, "rec_1"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a read while locked: %v", err)
	}

	if _, err := st.Recordings().ListByProject(ctx, "prj_1"); !errors.Is(err, account.ErrLocked) {
		t.Errorf("a list while locked: %v", err)
	}

	// The recovery reads what is open without the title and settles it without touching the title.
	open, err := st.Recordings().Open(ctx)
	if err != nil || len(open) != 1 || open[0].SourceLabel != "" {
		t.Fatalf("open while locked: %+v %v", open, err)
	}

	row := open[0]
	row.State = "ready"

	if err := st.Recordings().Update(ctx, row); err != nil {
		t.Fatalf("an update while locked: %v", err)
	}

	l.locked = false

	got, err = st.Recordings().Get(ctx, "rec_1")
	if err != nil || got.State != "ready" || got.SourceLabel != wordsWindowTitle {
		t.Fatalf("the title must survive a settle done while locked: %+v %v", got, err)
	}

	raw := bytesOfTheWorkspace(t, st, string(dir))
	if bytes.Contains(raw, []byte(wordsWindowTitle)) {
		t.Error("the database file holds the title of a window in clear")
	}

	if !bytes.Contains(raw, []byte("rec_1")) {
		t.Error("the id of the recording must stay readable for the queries")
	}
}

func TestRecordingsStoreRoundTrip(t *testing.T) {
	st, err := Open(ctx, newDir(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	a, b := recordingRow("ready"), recordingRow("recording")
	a.ID, a.CreatedMs, a.Review = "rec_a", 1, "pending"
	b.ID, b.CreatedMs = "rec_b", 2

	for _, r := range []recordings.Row{a, b} {
		if err := st.Recordings().Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	list, err := st.Recordings().ListByProject(ctx, "prj_1")
	if err != nil || len(list) != 2 || list[0].ID != "rec_b" {
		t.Fatalf("newest first: %+v %v", list, err)
	}

	open, err := st.Recordings().Open(ctx)
	if err != nil || len(open) != 1 || open[0].ID != "rec_b" {
		t.Fatalf("open: %+v %v", open, err)
	}

	if _, err := st.Recordings().Get(ctx, "rec_zzz"); !errors.Is(err, recordings.ErrNotFound) {
		t.Errorf("get of nothing: %v", err)
	}

	missing := recordingRow("ready")
	missing.ID = "rec_zzz"

	if err := st.Recordings().Update(ctx, missing); !errors.Is(err, recordings.ErrNotFound) {
		t.Errorf("update of nothing: %v", err)
	}
}

// The plan, the marks and the proposals survive a write and a read, and a prepared recording with a plan is not
// open (SDD-025 section 19): recovery would otherwise settle what no helper ever started.
func TestRecordingAgentFlowColumnsAndOpen(t *testing.T) {
	st := openStore(t, newDir(t), Options{})

	prepared := recordingRow("prepared")
	prepared.ID, prepared.Plan = "rec_prepared", `{"source_id":"d1"}`

	starting := recordingRow("prepared")
	starting.ID = "rec_starting"

	for _, r := range []recordings.Row{prepared, starting} {
		if err := st.Recordings().Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	open, err := st.Recordings().Open(ctx)
	if err != nil || len(open) != 1 || open[0].ID != "rec_starting" {
		t.Fatalf("open = %+v, %v; want only the recording a helper is starting", open, err)
	}

	got, err := st.Recordings().Get(ctx, "rec_prepared")
	if err != nil || got.Plan != prepared.Plan || got.Marks != "[]" || got.Proposals != "[]" {
		t.Fatalf("get = %+v, %v", got, err)
	}

	got.Marks, got.Proposals = `[{"n":1}]`, `[{"id":"prp_x"}]`
	if err := st.Recordings().Update(ctx, got); err != nil {
		t.Fatal(err)
	}

	again, err := st.Recordings().Find(ctx, "rec_prepared")
	if err != nil || again.Marks != got.Marks || again.Proposals != got.Proposals || again.Plan != got.Plan {
		t.Fatalf("after update = %+v, %v", again, err)
	}
}
