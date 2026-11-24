//go:build crashtest

package servicetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

const sealOps = 450

func sealKey() *account.KeySealer {
	ks, err := account.NewKeySealer(bytes.Repeat([]byte{7}, account.KeySize))
	if err != nil {
		panic(err)
	}

	return ks
}

func openSealing(t testing.TB, dir workspace.Dir) *store.Store {
	t.Helper()

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	st.SetSealer(sealKey())

	return st
}

// TestCrashSealDocumentsChild is the body of the child of TestCrashSealDocuments.
func TestCrashSealDocumentsChild(t *testing.T) {
	if os.Getenv(roleEnv) != "seal-documents-child" {
		t.Skip("only runs as the child of a crash test")
	}

	st := openSealing(t, workspace.Dir(os.Getenv(workspace.EnvHome)))

	err := st.SealStageTwo(context.Background(), sealKey(), nil)

	t.Fatalf("the child finished (%v) without reaching %s", err, os.Getenv(crashpoint.CrashEnv))
}

// SDD-005 section 5.17.4 for the sealing of an earlier account: a kill after a batch, before the version moves, or
// right after it leaves a workspace that opens with every document and every row of the log as it was; the next run
// finishes the sealing, the version is 1 and no word of the fixture is left in the database file.
func TestCrashSealDocuments(t *testing.T) {
	t.Parallel()

	for _, point := range []string{
		"seal_documents.after_batch", "seal_documents.before_version", "seal_documents.after_version",
	} {
		t.Run(point, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)

			raw, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "timeline", "testdata", "versions", "v1",
				"all-item-types.json"))
			if err != nil {
				t.Fatal(err)
			}

			e.insertProject("prj_7k2m9x4q1b", downgraded(raw))

			// The project is at schema version 0 on purpose: the sealing is about bytes, not versions.
			for i := 1; i <= sealOps; i++ {
				op := service.OperationRow{
					ProjectID: "prj_7k2m9x4q1b", Revision: int64(i), Kind: service.OpEdit, Actor: "local:ui", AtMs: int64(i),
					Ops:           json.RawMessage(fmt.Sprintf(`{"words":"CANARY-CRASH-OP-%d"}`, i)),
					Inverse:       json.RawMessage(`{}`),
					Diff:          json.RawMessage(`{}`),
					SchemaVersion: 1, DocSHA256: "sha",
				}

				if err := e.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
					return tx.Operations().Append(ctx, op)
				}); err != nil {
					t.Fatal(err)
				}
			}

			if err := e.store.Accounts().Create(context.Background(), account.Record{
				Username: "stuart", Email: "stuart@example.com",
				KDF:      account.KDF{Alg: "argon2id", Time: 1, MemoryKiB: 64, Threads: 1, Salt: bytes.Repeat([]byte{1}, 16)},
				Verifier: bytes.Repeat([]byte{2}, 32), KeyByPassword: bytes.Repeat([]byte{3}, 60),
				KeyByRecovery: bytes.Repeat([]byte{4}, 60), VaultKey: bytes.Repeat([]byte{5}, 60), CreatedMs: 1, UpdatedMs: 1,
			}); err != nil {
				t.Fatal(err)
			}

			// An account made by release 0.50.0: its version is 0.
			if _, err := e.store.DB().ExecContext(context.Background(), `UPDATE accounts SET seal_version = 0`); err != nil {
				t.Fatal(err)
			}

			if err := e.store.Close(); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashSealDocumentsChild$", "-test.v")
			cmd.Env = append(os.Environ(), roleEnv+"=seal-documents-child", workspace.EnvHome+"="+string(e.dir),
				crashpoint.CrashEnv+"="+point)

			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out

			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 137 {
				t.Fatalf("child = %v, want exit 137 at %s\n%s", err, point, out.String())
			}

			st := openSealing(t, e.dir)
			defer st.Close()

			ctx := context.Background()

			var integrity string
			if err := st.DB().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
				t.Errorf("integrity_check = %q, %v", integrity, err)
			}

			// After the kill the workspace opens and every document and every row of the log reads as it was.
			check := func(stage string) {
				t.Helper()

				row, err := st.Projects().Get(ctx, "prj_7k2m9x4q1b")
				if err != nil || !bytes.Equal(row.Document, downgraded(raw)) {
					t.Fatalf("%s: the document: %v", stage, err)
				}

				for i := 1; i <= sealOps; i++ {
					op, err := st.Operations().Get(ctx, "prj_7k2m9x4q1b", int64(i))
					if err != nil || string(op.Ops) != fmt.Sprintf(`{"words":"CANARY-CRASH-OP-%d"}`, i) {
						t.Fatalf("%s: operation %d: %q %v", stage, i, op.Ops, err)
					}
				}
			}

			check("after the kill")

			if err := st.SealStageTwo(ctx, sealKey(), nil); err != nil {
				t.Fatal(err)
			}

			check("after the next run")

			var version int
			if err := st.DB().QueryRowContext(ctx, `SELECT seal_version FROM accounts`).Scan(&version); err != nil || version != 1 {
				t.Fatalf("seal version %d (%v), want 1", version, err)
			}

			if err := st.CompactSealed(ctx); err != nil {
				t.Fatal(err)
			}

			if err := st.Close(); err != nil {
				t.Fatal(err)
			}

			for _, name := range []string{"db.sqlite", "db.sqlite-wal"} {
				data, _ := os.ReadFile(filepath.Join(string(e.dir), name))
				if bytes.Contains(data, []byte("CANARY-CRASH-OP-")) {
					t.Errorf("%s still holds a word of the log after the sealing", name)
				}
			}
		})
	}
}
