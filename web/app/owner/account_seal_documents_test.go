package owner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// An account made by release 0.50.0 (seal version 0, its templates and its library in clear text) opens, and its first
// unlock seals the second stage and the library, moves the version to 1 and removes the copy it made (SDD-005 section
// 5.17.4).
func TestSealDocumentsAtTheFirstUnlockOfAnEarlierAccount(t *testing.T) {
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

	defer func() { _ = o2.Close() }()

	stored := func(id string) string {
		var doc string
		if err := o2.Store.DB().
			QueryRowContext(ctx, `SELECT document FROM templates WHERE id = ?`, id).
			Scan(&doc); err != nil {
			t.Fatal(err)
		}

		return doc
	}

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

	var version int
	if err := o2.Store.DB().
		QueryRowContext(ctx, `SELECT seal_version FROM accounts`).
		Scan(&version); err != nil ||
		version != 1 {
		t.Fatalf("seal version %d (%v), want 1", version, err)
	}

	if raw, _ := os.ReadFile(clearFile); strings.Contains(string(raw), canaryNote) || !account.IsSealedFile(raw) {
		t.Error("the file of the library is still clear after the first unlock")
	}

	if left, _ := filepath.Glob(filepath.Join(home, "backups", sealBackupPrefix+"*")); len(left) != 0 {
		t.Errorf("the copy of the sealing was not removed: %v", left)
	}
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
