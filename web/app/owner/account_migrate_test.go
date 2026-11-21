package owner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	rules "github.com/AlexGalhardo/stuarty/core/account"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

var migrateInput = rules.Input{
	Username: "stuart", Email: "stuart@example.com", Password: "Correct-Horse-9", Confirmation: "Correct-Horse-9",
}

// existingWorkspace builds a workspace that was in use before the account: a text, a brief, a memory note and a
// credential in the vault, under the key the environment names. It returns the open owner.
func existingWorkspace(t *testing.T, getenv func(string) string) (*Owner, context.Context, string) {
	t.Helper()

	ctx := context.Background()

	o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	as := service.WithCaller(ctx, service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true})

	project, err := o.Service.ProjectCreate(as, canaryProject())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := o.Service.ProjectItemAdd(as, service.ProjectItemAddRequest{
		ProjectID: project.ProjectID, Kind: "text", Title: "Script", Text: canaryScript,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := o.Service.BriefSet(
		as,
		service.BriefSetRequest{ProjectID: project.ProjectID, Text: canaryBrief},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := o.Service.MemoryWrite(as, service.MemoryWriteRequest{
		Name: "note", Description: "a note", Body: canaryNote,
	}); err != nil {
		t.Fatal(err)
	}

	if err := o.vault.Put("conn.one", []byte(canaryToken)); err != nil {
		t.Fatal(err)
	}

	return o, as, project.ProjectID
}

func scan(t *testing.T, home string, names ...string) []byte {
	t.Helper()

	var all []byte

	for _, n := range names {
		raw, _ := os.ReadFile(filepath.Join(home, n))
		all = append(all, raw...)
	}

	return all
}

func backupsLeft(t *testing.T, home string) []string {
	t.Helper()

	left, err := filepath.Glob(filepath.Join(home, "backups", accountBackupPrefix+"*"))
	if err != nil {
		t.Fatal(err)
	}

	return left
}

func checkpoint(t *testing.T, o *Owner) {
	t.Helper()

	if _, err := o.Store.DB().ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
}

func TestSecurity_T109_AnExistingWorkspaceMigratesWithoutLoss(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	keyFile := getenv("STUARTY_MASTER_KEY_FILE")
	ctx := context.Background()

	o, as, project := existingWorkspace(t, getenv)

	checkpoint(t, o)

	// The control: before the account, the words are in the file in clear.
	if !bytes.Contains(scan(t, home, "db.sqlite"), []byte(canaryScript)) {
		t.Fatal("the control does not see the script in the database: the scan is blind")
	}

	if !bytes.Contains(scan(t, home, "db.sqlite"), []byte(canaryDescription)) {
		t.Fatal("the control does not see the project description in the database: the scan is blind")
	}

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	checkpoint(t, o)

	for _, w := range canaryWords {
		if bytes.Contains(scan(t, home, "db.sqlite", "db.sqlite-wal", "vault.json"), []byte(w)) {
			t.Errorf("%q is still in clear after the migration", w)
		}
	}

	if left := backupsLeft(t, home); len(left) != 0 {
		t.Errorf("the copy was not removed: %v", left)
	}

	// The old key opens nothing now.
	if _, err := vault.Open(filepath.Join(home, "vault.json"), vault.FileProvider{Path: keyFile}).
		Get("conn.one"); err == nil {
		t.Error("the key of the file still opens the vault")
	}

	// Locked until the person signs in.
	if _, err := o.Service.TextList(as, service.TextListRequest{ProjectID: project}); err == nil {
		t.Error("a read of the migrated workspace worked while locked")
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	texts, err := o.Service.TextList(as, service.TextListRequest{ProjectID: project})
	if err != nil || len(texts.Texts) == 0 {
		t.Fatalf("the texts after the migration: %+v %v", texts, err)
	}

	got, err := o.Service.TextGet(as, service.TextGetRequest{ProjectID: project, ItemID: texts.Texts[0].ID})
	if err != nil || got.Body != canaryScript {
		t.Fatalf("the script after the migration: %+v %v", got, err)
	}

	proj, err := o.Service.ProjectGet(as, service.ProjectGetRequest{ProjectID: project})
	if err != nil || proj.Description != canaryDescription || proj.Goal.Specific != canaryGoal+"-specific" ||
		proj.Goal.TimeBound != canaryGoal+"-timebound" || proj.Goal.Due != "2030-12-31" {
		t.Fatalf("the project context after the migration: %+v %v", proj, err)
	}

	if secret, err := o.vault.Get("conn.one"); err != nil || string(secret) != canaryToken {
		t.Fatalf("the credential after the migration: %q %v", secret, err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSecurity_T109_AFailedMigrationRestoresTheWorkspace(t *testing.T) {
	for _, stage := range []string{"vault", "sealed", "inserted"} {
		t.Run(stage, func(t *testing.T) {
			getenv := accountEnv(t)
			home := getenv(workspace.EnvHome)
			keyFile := getenv("STUARTY_MASTER_KEY_FILE")
			ctx := context.Background()

			o, as, project := existingWorkspace(t, getenv)

			defer func() { _ = o.Close() }()

			boom := errors.New("the disk went away")
			o.accountFault = func(at string) error {
				if at == stage {
					return boom
				}

				return nil
			}

			vaultBefore := scan(t, home, "vault.json")

			if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); !errors.Is(err, boom) {
				t.Fatalf("the failure was not reported: %v", err)
			}

			checkpoint(t, o)

			if o.Account.State() != "none" {
				t.Fatalf("the account state is %s after a failed creation", o.Account.State())
			}

			if left := backupsLeft(t, home); len(left) != 0 {
				t.Errorf("the copy was left: %v", left)
			}

			if !bytes.Equal(scan(t, home, "vault.json"), vaultBefore) {
				t.Error("the vault file is not the one it was")
			}

			if secret, err := vault.Open(filepath.Join(home, "vault.json"), vault.FileProvider{Path: keyFile}).
				Get("conn.one"); err != nil ||
				string(secret) != canaryToken {
				t.Errorf("the old key does not open the restored vault: %q %v", secret, err)
			}

			if !bytes.Contains(scan(t, home, "db.sqlite"), []byte(canaryScript)) {
				t.Error("the database is not as it was (the script is gone from it)")
			}

			texts, err := o.Service.TextList(as, service.TextListRequest{ProjectID: project})
			if err != nil || len(texts.Texts) == 0 {
				t.Fatalf("the workspace does not work after the failure: %+v %v", texts, err)
			}

			if got, err := o.Service.TextGet(
				as,
				service.TextGetRequest{ProjectID: project, ItemID: texts.Texts[0].ID},
			); err != nil ||
				got.Body != canaryScript {
				t.Fatalf("the script after the failure: %+v %v", got, err)
			}

			// And the person may try again.
			o.accountFault = nil

			if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
				t.Fatalf("the second try: %v", err)
			}
		})
	}
}

func TestSecurity_T109_ACrashInTheMiddleOfTheCreationIsFinishedAtTheNextStart(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, _, _ := existingWorkspace(t, getenv)

	// What the process left when it died after it re-sealed the vault and before the row committed: the copy,
	// and a vault file that the old key no longer opens.
	good := scan(t, home, "vault.json")
	left := filepath.Join(home, "backups", accountBackupPrefix+"crash")

	if err := os.MkdirAll(left, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(left, "vault.json"), good, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(home, "vault.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = again.Close() }()

	if !bytes.Equal(scan(t, home, "vault.json"), good) {
		t.Error("the vault was not put back")
	}

	if left := backupsLeft(t, home); len(left) != 0 {
		t.Errorf("the copy was not removed: %v", left)
	}
}

func TestALostVaultKeyDoesNotStopTheAccount(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, _, _ := existingWorkspace(t, getenv)

	defer func() { _ = o.Close() }()

	// The key of the vault is lost: the file named by the variable is replaced by another key.
	if err := os.WriteFile(
		getenv("STUARTY_MASTER_KEY_FILE"),
		[]byte(vault.FormatKey(bytes.Repeat([]byte{9}, vault.KeySize))+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	o.vault.Forget()

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatalf("a lost vault key stopped the creation: %v", err)
	}

	aside, _ := filepath.Glob(filepath.Join(home, "backups", "vault-unreadable-*.json"))
	if len(aside) != 1 {
		t.Fatalf("the unreadable vault was not kept: %v", aside)
	}

	if _, err := os.Stat(filepath.Join(home, "vault.json")); err == nil {
		t.Error("the unreadable vault is still where the account will write its own")
	}
}
