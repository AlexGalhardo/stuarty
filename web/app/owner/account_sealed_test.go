package owner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	rules "github.com/AlexGalhardo/stuarty/core/account"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

const (
	canaryScript = "CANARY-SCRIPT-my-unpublished-script"
	canaryBrief  = "CANARY-BRIEF-who-is-this-video-for"
	canaryNote   = "CANARY-NOTE-body-of-a-memory-note"
	canaryToken  = "CANARY-TOKEN-an-oauth-refresh-token"
	// The context of a project (epic 56) is the creator's own words too: 64 characters at least for the description,
	// 16 for each part of the goal.
	canaryDescription = "CANARY-DESCRIPTION-what-this-launch-video-is-about-and-for-whom-it-is-made"
	canaryGoal        = "CANARY-GOAL-part"
	// A webhook (epic 59): its address can carry a token in the path, and its signing secret is in the vault.
	canaryHookToken  = "CANARY-HOOKTOKEN-in-the-path"
	canaryHookSecret = "CANARY-HOOKSECRET-the-signing-secret"
	canaryHookURL    = "https://hooks.example.com/" + canaryHookToken + "?k=1"
)

// canaryProject is a valid ProjectCreate request whose description and goal carry the canaries.
func canaryProject() service.ProjectCreateRequest {
	return service.ProjectCreateRequest{
		Name: "Launch", Description: canaryDescription,
		Goal: service.ProjectGoal{
			Specific: canaryGoal + "-specific", Measurable: canaryGoal + "-measurable",
			Achievable: canaryGoal + "-achievable", Relevant: canaryGoal + "-relevant",
			TimeBound: canaryGoal + "-timebound", Due: "2030-12-31",
		},
	}
}

// canaryWords is every word of the person that the files must not hold in clear.
var canaryWords = []string{
	canaryScript, canaryBrief, canaryNote, canaryToken, canaryDescription, canaryGoal + "-specific",
	canaryGoal + "-measurable", canaryGoal + "-achievable", canaryGoal + "-relevant", canaryGoal + "-timebound",
	canaryHookToken, canaryHookSecret,
}

func accountEnv(t *testing.T) func(string) string {
	t.Helper()

	home := filepath.Join(t.TempDir(), "ws")
	keyFile := filepath.Join(t.TempDir(), "master.key")

	return func(k string) string {
		switch k {
		case workspace.EnvHome:
			return home
		case "STUARTY_MASTER_KEY_FILE":
			return keyFile
		case "STUARTY_MIN_FREE_BYTES":
			return "1"
		}

		return ""
	}
}

// TestSecurity_T108_TheDatabaseAndTheVaultOfAnAccountHoldNoPlaintext: with an account, a text, a brief, a
// memory note and a credential leave no clear text in db.sqlite, its log or vault.json, and the vault opens
// only while a person is signed in (SDD-005 section 5.17, SDD-007 section 3.9 item 10).
func TestSecurity_T108_TheDatabaseAndTheVaultOfAnAccountHoldNoPlaintext(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	in := rules.Input{
		Username: "stuart", Email: "stuart@example.com", Password: "Correct-Horse-9", Confirmation: "Correct-Horse-9",
	}

	o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := o.Account.Create(ctx, in, nil); err != nil {
		t.Fatal(err)
	}

	if err := o.Account.Login(ctx, "stuart", in.Password); err != nil {
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

	if got, err := o.vault.Get("conn.one"); err != nil || string(got) != canaryToken {
		t.Fatalf("the vault does not read back while unlocked: %q %v", got, err)
	}

	o.Account.Lock()

	if _, err := o.vault.Get("conn.one"); err == nil {
		t.Fatal("the vault opened while the studio is locked")
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	read := func(name string) []byte {
		raw, _ := os.ReadFile(filepath.Join(home, name))

		return raw
	}

	db := bytes.Join([][]byte{read("db.sqlite"), read("db.sqlite-wal")}, nil)
	vaultFile := read("vault.json")

	if len(db) == 0 || len(vaultFile) == 0 {
		t.Fatalf("nothing to scan: db %d bytes, vault %d bytes", len(db), len(vaultFile))
	}

	for _, w := range canaryWords {
		if bytes.Contains(db, []byte(w)) {
			t.Errorf("db.sqlite holds %q in clear", w)
		}

		if bytes.Contains(vaultFile, []byte(w)) {
			t.Errorf("vault.json holds %q in clear", w)
		}
	}

	// A new process finds the studio locked and the vault closed; the password opens both.
	again, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = again.Close() }()

	if _, err := again.vault.Get("conn.one"); err == nil {
		t.Fatal("a fresh process opened the vault with no password")
	}

	if err := again.Account.Login(ctx, "stuart", in.Password); err != nil {
		t.Fatal(err)
	}

	if got, err := again.vault.Get("conn.one"); err != nil || string(got) != canaryToken {
		t.Fatalf("after the sign-in the vault reads %q %v", got, err)
	}

	texts, err := again.Service.TextList(as, service.TextListRequest{ProjectID: project.ProjectID})
	if err != nil || len(texts.Texts) == 0 {
		t.Fatalf("the texts after the sign-in: %+v %v", texts, err)
	}
}
