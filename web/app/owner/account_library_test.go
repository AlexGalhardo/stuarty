package owner

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// libraryFiles returns every file under library/ with its bytes.
func libraryFiles(t *testing.T, home string) map[string][]byte {
	t.Helper()

	out := map[string][]byte{}

	err := filepath.WalkDir(filepath.Join(home, "library"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}

		out[p] = raw

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

// With an account the files of library/ hold no clear word: the creation seals the ones that exist, and what is
// written afterwards is sealed as it is written (ADR-0053).
func TestLibrarySealAtTheCreationOfTheAccountAndAfter(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, as, _ := existingWorkspace(t, getenv)

	before := libraryFiles(t, home)
	if len(before) == 0 {
		t.Fatal("the workspace has no library file to seal")
	}

	// The control: before the account the note is in the file in clear.
	found := false

	for _, raw := range before {
		found = found || bytes.Contains(raw, []byte(canaryNote))
	}

	if !found {
		t.Fatal("the control does not see the note in library/: the scan is blind")
	}

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	for p, raw := range libraryFiles(t, home) {
		if bytes.Contains(raw, []byte(canaryNote)) || !account.IsSealedFile(raw) {
			t.Errorf("%s is readable after the creation of the account", p)
		}
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	got, err := o.Service.MemoryGet(as, service.LibraryRef{Name: "note"})
	if err != nil || got.Body != canaryNote+"\n" && got.Body != canaryNote {
		t.Fatalf("the note after the migration: %+v %v", got, err)
	}

	if _, err := o.Service.MemoryWrite(as, service.MemoryWriteRequest{
		Name: "later", Description: "written with the account", Body: canaryNote + "-later",
	}); err != nil {
		t.Fatal(err)
	}

	for p, raw := range libraryFiles(t, home) {
		if bytes.Contains(raw, []byte(canaryNote)) || !account.IsSealedFile(raw) {
			t.Errorf("%s is readable after a write with an account", p)
		}
	}

	// The lock drops the index: a search while locked is E_LOCKED, and after the next unlock it finds the note.
	o.Account.Lock()

	if _, err := o.Service.MemoryGet(as, service.LibraryRef{Name: "note"}); err == nil {
		t.Error("the library answered while locked")
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	res, err := o.Service.LibrarySearch(as, service.LibrarySearchRequest{Query: "later"})
	if err != nil || len(res.Hits) == 0 {
		t.Fatalf("the search after the unlock: %+v %v", res, err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
}

// A workspace made by an earlier version has its library in clear text and an account: the first unlock seals every
// file, and nothing is lost.
func TestLibrarySealAtTheFirstUnlockOfAnEarlierAccount(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, as, _ := existingWorkspace(t, getenv)

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	// An older version left clear files: put one back, and a skill folder with a nested file.
	clear := filepath.Join(home, "library", "memory", "from-before.md")
	body := "---\nname: from-before\ndescription: made by an older version\nscope: workspace\nactor: local:ui\nkept: true\n" +
		"created: 2026-11-19T10:00:00Z\nupdated: 2026-11-19T10:00:00Z\n---\n" + canaryNote + "-before\n"

	if err := os.WriteFile(clear, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	stray := filepath.Join(home, "library", "skills", "old", "references", "a.md")
	if err := os.MkdirAll(filepath.Dir(stray), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(stray, []byte(canaryNote+"-stray"), 0o600); err != nil {
		t.Fatal(err)
	}

	o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = o.Close() }()

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	for p, raw := range libraryFiles(t, home) {
		if bytes.Contains(raw, []byte(canaryNote)) || !account.IsSealedFile(raw) {
			t.Errorf("%s is readable after the first unlock", p)
		}
	}

	got, err := o.Service.MemoryGet(as, service.LibraryRef{Name: "from-before"})
	if err != nil || got.Body == "" {
		t.Fatalf("the note of the earlier version: %+v %v", got, err)
	}
}
