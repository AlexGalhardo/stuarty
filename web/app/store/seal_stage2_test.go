package store

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// earlierAccount is a workspace as release 0.50.0 left it: the documents, the log, the backups, the results and the
// asset notes in clear text, and an account of seal version 0. extraOps adds rows to the log so the migration takes
// more than one batch.
func earlierAccount(t *testing.T, extraOps int) (*Store, *locker, string) {
	t.Helper()

	dir := newDir(t)
	st := openStore(t, dir, Options{})

	writeDocuments(t, st) // no sealer: clear text, as the earlier release wrote it

	for i := 2; i < 2+extraOps; i++ {
		if err := st.Operations().Append(ctx, service.OperationRow{
			ProjectID: "prj_9", Revision: int64(i), Kind: service.OpEdit, Actor: "local:ui", AtMs: int64(i),
			Ops: js(fmt.Sprintf("%s-%d", docOp, i)), Inverse: js("i"), Diff: js("d"), SchemaVersion: 1, DocSHA256: "sha",
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.Accounts().Create(ctx, accountRecord()); err != nil {
		t.Fatal(err)
	}

	if _, err := st.DB().ExecContext(ctx, `UPDATE accounts SET seal_version = 0`); err != nil {
		t.Fatal(err)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// The next start of the earlier account: the second stage is read as it is, and has no guard yet.
	st = openStore(t, dir, Options{})

	keys, err := account.NewKeySealer(bytes.Repeat([]byte{42}, account.KeySize))
	if err != nil {
		t.Fatal(err)
	}

	l := &locker{Sealer: keys}
	st.SetSealer(l)

	return st, l, string(dir)
}

func guardCount(t *testing.T, st *Store) int {
	t.Helper()

	return scalar[int](t, st, "SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'sealed_guard_%'")
}

// A workspace of release 0.50.0 opens, reads its clear columns without error, seals them at the first unlock, and
// ends with every document, operation and note equal to what it was (SDD-005 section 5.17.4).
func TestSealDocumentsOfAnEarlierAccountLosesNothing(t *testing.T) {
	st, _, dir := earlierAccount(t, 450)

	// Before the sealing: everything reads as it was, from clear text, with no error; the log too.
	readDocuments(t, st)

	if n := scalar[int](t, st, `SELECT seal_version FROM accounts`); n != 0 {
		t.Fatalf("seal version %d, want 0", n)
	}

	if got := guardCount(t, st); got != 2*len(sealedCells) {
		t.Fatalf("%d guards before the migration, want only the %d of the first stage", got, 2*len(sealedCells))
	}

	// A write while it is not sealed yet is sealed, and reads back beside the clear rows.
	if err := st.Operations().Append(ctx, service.OperationRow{
		ProjectID: "prj_9", Revision: 1000, Kind: service.OpEdit, Actor: "local:ui", AtMs: 9, Ops: js("new"),
		Inverse: js("new"), Diff: js("new"), SchemaVersion: 1, DocSHA256: "sha",
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.SealStageTwo(ctx, st.ref.get(), nil); err != nil {
		t.Fatal(err)
	}

	if n := scalar[int](t, st, `SELECT seal_version FROM accounts`); n != 1 {
		t.Fatalf("seal version %d after the migration, want 1", n)
	}

	if st.ref.lenient.Load() {
		t.Error("the store is still lenient after the migration")
	}

	if got := guardCount(t, st); got != 2*len(allSealedCells()) {
		t.Errorf("%d guards after the migration, want %d", got, 2*len(allSealedCells()))
	}

	readDocuments(t, st)

	op, err := st.Operations().Get(ctx, "prj_9", 300)
	if err != nil || !strings.Contains(string(op.Ops), docOp+"-300") {
		t.Fatalf("an operation of the middle of the log: %+v %v", op, err)
	}

	if _, err := st.Operations().Get(ctx, "prj_9", 1000); err != nil {
		t.Fatalf("the operation written during the migration: %v", err)
	}

	// The log is still append-only: the triggers came back.
	if _, err := st.DB().ExecContext(ctx, `UPDATE operations SET actor = 'x' WHERE revision = 1`); err == nil {
		t.Error("the append-only trigger of the log did not come back")
	}

	if err := st.CompactSealed(ctx); err != nil {
		t.Fatal(err)
	}

	raw := bytesOfTheWorkspace(t, st, dir)
	for _, w := range canariesOfTheSecondStage() {
		if bytes.Contains(raw, []byte(w)) {
			t.Errorf("the database file still holds %q after the migration", w)
		}
	}
}

// A run that fails in the middle leaves a workspace that opens and reads everything, and the next run finishes it
// without losing a row.
func TestSecurity_T135_AMigrationStoppedInTheMiddleLosesNothingAndTheNextRunFinishesIt(t *testing.T) {
	st, _, dir := earlierAccount(t, 450)

	batches := 0
	boom := errors.New("the disk is full")

	err := st.SealStageTwo(ctx, st.ref.get(), func(stage string) error {
		if stage == "batch" {
			batches++
		}

		if batches == 3 {
			return boom
		}

		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("the run did not stop: %v", err)
	}

	// Halfway: some rows sealed, some clear, the version still 0, and everything reads.
	if n := scalar[int](t, st, `SELECT seal_version FROM accounts`); n != 0 {
		t.Fatalf("seal version %d after a failed run, want 0", n)
	}

	readDocuments(t, st)

	// A new start (a kill, or a restart) and the next unlock.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st = openStore(t, workspace.Dir(dir), Options{})

	keys, _ := account.NewKeySealer(bytes.Repeat([]byte{42}, account.KeySize))
	st.SetSealer(&locker{Sealer: keys})

	readDocuments(t, st)

	if err := st.SealStageTwo(ctx, st.ref.get(), nil); err != nil {
		t.Fatal(err)
	}

	readDocuments(t, st)

	for i := 2; i < 452; i++ {
		if _, err := st.Operations().Get(ctx, "prj_9", int64(i)); err != nil {
			t.Fatalf("operation %d after the resumed run: %v", i, err)
		}
	}

	if n := scalar[int](t, st, `SELECT seal_version FROM accounts`); n != 1 {
		t.Fatalf("seal version %d, want 1", n)
	}

	// Nothing is left in clear: a second run finds nothing to do and a column of every table opens strictly.
	if err := st.SealStageTwo(ctx, st.ref.get(), nil); err != nil {
		t.Fatal(err)
	}
}

// A clear description that starts with the prefix of a sealed field is still a description (SDD-005 section 5.17.4).
func TestSealDocumentsSealsAClearValueThatLooksSealed(t *testing.T) {
	st, _, _ := earlierAccount(t, 0)

	if _, err := st.DB().ExecContext(ctx, `UPDATE asset_info SET description = 's1.not-really-sealed'`); err != nil {
		t.Fatal(err)
	}

	info, err := st.AssetLibrary().Get(ctx, "", "sha256:aa")
	if err != nil || info.Description != "s1.not-really-sealed" {
		t.Fatalf("a clear description that looks sealed: %q %v", info.Description, err)
	}

	if err := st.SealStageTwo(ctx, st.ref.get(), nil); err != nil {
		t.Fatal(err)
	}

	info, err = st.AssetLibrary().Get(ctx, "", "sha256:aa")
	if err != nil || info.Description != "s1.not-really-sealed" {
		t.Fatalf("after the migration: %q %v", info.Description, err)
	}
}
