package owner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// An account made by release 0.50.0 (seal version 0, its templates and its library in clear text) opens, and its first
// unlock seals the second stage and the library, moves the version to 1 and removes its mark (SDD-005 section
// 5.17.4).
func TestSealDocumentsAtTheFirstUnlockOfAnEarlierAccount(t *testing.T) {
	o2, home, plain := earlierAccountWorkspace(t)
	ctx := context.Background()

	stored := func(id string) string { return storedTemplate1(t, o2, id) }

	for id := range plain {
		if account.IsSealed(stored(id)) {
			t.Fatalf("template %s was sealed before the unlock", id)
		}
	}

	if err := o2.Account.Login(ctx, "stuarty", "wrong"); err == nil {
		t.Fatal("a wrong password unlocked the studio")
	}

	for id := range plain {
		if account.IsSealed(stored(id)) {
			t.Fatalf("template %s was sealed by a refused sign-in", id)
		}
	}

	if err := o2.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	for id, want := range plain {
		if !account.IsSealed(stored(id)) {
			t.Errorf("template %s is still clear after the first unlock", id)
		}

		row, err := o2.Store.Templates().Get(ctx, id)
		if err != nil || string(row.Document) != want {
			t.Errorf("template %s does not read back as it was: %v", id, err)
		}
	}

	if version := sealVersionOf(t, o2); version != 1 {
		t.Fatalf("seal version %d, want 1", version)
	}

	clearFile := filepath.Join(home, "library", "memory", "from-before.md")
	if raw, _ := os.ReadFile(clearFile); strings.Contains(string(raw), canaryNote) || !account.IsSealedFile(raw) {
		t.Error("the file of the library is still clear after the first unlock")
	}

	if left, _ := filepath.Glob(filepath.Join(home, "backups", sealBackupPrefix+"*")); len(left) != 0 {
		t.Errorf("the mark of the sealing was not removed: %v", left)
	}
}

// A sealing that fails in the middle writes no clear copy of the database (each batch is read back before it
// commits, so there is nothing a copy would put back), keeps only an empty mark, and the next unlock finishes it and
// removes the mark (SDD-005 section 5.17.4, epic 92 review).
func TestSealDocumentsKeepsNoClearCopyOfTheDatabase(t *testing.T) {
	o2, home, plain := earlierAccountWorkspace(t)
	ctx := context.Background()

	boom := errors.New("the disk is full")
	o2.accountFault = func(stage string) error {
		if stage == "batch" {
			return boom
		}

		return nil
	}

	if err := o2.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	if version := sealVersionOf(t, o2); version != 0 {
		t.Fatalf("seal version %d after a failed sealing, want 0", version)
	}

	backups := filepath.Join(home, "backups")

	if err := filepath.WalkDir(backups, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		if info.Size() != 0 {
			t.Errorf("the failed sealing left %s (%d bytes) in backups/: only an empty mark may stay", p, info.Size())
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(backups, sealMark)); err != nil {
		t.Fatalf("the mark of the unfinished sealing: %v", err)
	}

	// Everything still reads, part sealed and part clear.
	for id, want := range plain {
		row, err := o2.Store.Templates().Get(ctx, id)
		if err != nil || string(row.Document) != want {
			t.Errorf("template %s after the failed sealing: %v", id, err)
		}
	}

	o2.Account.Lock()
	o2.accountFault = nil

	if err := o2.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	if version := sealVersionOf(t, o2); version != 1 {
		t.Fatalf("seal version %d after the next unlock, want 1", version)
	}

	if left, _ := filepath.Glob(filepath.Join(backups, sealBackupPrefix+"*")); len(left) != 0 {
		t.Errorf("the mark of the sealing was not removed: %v", left)
	}
}

func sealVersionOf(t *testing.T, o *Owner) int {
	t.Helper()

	var version int
	if err := o.Store.DB().QueryRowContext(context.Background(), `SELECT seal_version FROM accounts`).
		Scan(&version); err != nil {
		t.Fatal(err)
	}

	return version
}

func storedTemplate1(t *testing.T, o *Owner, id string) string {
	t.Helper()

	var doc string
	if err := o.Store.DB().
		QueryRowContext(context.Background(), `SELECT document FROM templates WHERE id = ?`, id).
		Scan(&doc); err != nil {
		t.Fatal(err)
	}

	return doc
}

// earlierAccountWorkspace is a workspace as release 0.50.0 left it, opened again and locked: an account of seal
// version 0, its templates and a library file in clear text. It returns the clear template documents by id.
func earlierAccountWorkspace(t *testing.T) (*Owner, string, map[string]string) {
	t.Helper()

	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, _, _ := existingWorkspace(t, getenv)

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	// Make the workspace what release 0.50.0 left: the templates in clear, the guards of the second stage gone, the
	// version 0, a library file in clear.
	sl := o.Account.Sealer()

	all := readTemplates(t, o)

	if len(all) == 0 {
		t.Fatal("the workspace has no template to make clear")
	}

	for _, event := range []string{"insert", "update"} {
		if _, err := o.Store.DB().
			ExecContext(ctx, "DROP TRIGGER IF EXISTS sealed_guard_templates_document_"+event); err != nil {
			t.Fatal(err)
		}
	}

	plain := map[string]string{}

	for _, x := range all {
		p, err := sl.Open("templates", "document", x.id, x.doc)
		if err != nil {
			t.Fatal(err)
		}

		plain[x.id] = p

		if _, err := o.Store.DB().
			ExecContext(ctx, `UPDATE templates SET document = ? WHERE id = ?`, p, x.id); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := o.Store.DB().ExecContext(ctx, `UPDATE accounts SET seal_version = 0`); err != nil {
		t.Fatal(err)
	}

	clearFile := filepath.Join(home, "library", "memory", "from-before.md")
	if err := os.WriteFile(
		clearFile,
		[]byte("---\nname: from-before\ndescription: d\nscope: workspace\nactor: local:ui\nkept: true\n"+
			"created: 2026-11-19T10:00:00Z\nupdated: 2026-11-19T10:00:00Z\n---\n"+canaryNote+"-before\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	// The next start: locked, and it opens. The templates are still clear in the file.
	o2, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = o2.Close() })

	return o2, home, plain
}

type storedTemplate struct{ id, doc string }

// readTemplates is the id and the stored document of every template, as the database holds them.
func readTemplates(t *testing.T, o *Owner) []storedTemplate {
	t.Helper()

	rows, err := o.Store.DB().QueryContext(context.Background(), `SELECT id, document FROM templates`)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = rows.Close() }()

	var all []storedTemplate

	for rows.Next() {
		var x storedTemplate
		if err := rows.Scan(&x.id, &x.doc); err != nil {
			t.Fatal(err)
		}

		all = append(all, x)
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return all
}
