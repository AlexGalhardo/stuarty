package vault

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

var canaries = []string{
	"sk-live-CANARY-0123456789abcdef",
	"ghp_CANARYtokenOfTheGithubKind01",
	"p@ssw0rd-CANARY-ünïcödé-密码",
	"eyJhbGciOiJIUzI1NiJ9.CANARY.payload",
}

// forms are the shapes in which a secret could be written in a file by mistake: as is, as base64 and as hex.
func forms(secret string) [][]byte {
	return [][]byte{
		[]byte(secret),
		[]byte(base64.StdEncoding.EncodeToString([]byte(secret))),
		[]byte(base64.RawURLEncoding.EncodeToString([]byte(secret))),
		[]byte(hex.EncodeToString([]byte(secret))),
	}
}

func seeded(t *testing.T, keys KeyProvider) (workspace string, v *Vault) {
	t.Helper()

	workspace = t.TempDir()
	v = Open(filepath.Join(workspace, "vault.json"), keys)

	for i, s := range canaries {
		if err := v.Put("con_000000000"+string(rune('a'+i)), []byte(s)); err != nil {
			t.Fatal(err)
		}
	}

	return workspace, v
}

// grepTree returns the files under dir that contain any of the needles.
func grepTree(t *testing.T, dir string, needles [][]byte) []string {
	t.Helper()

	var hits []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, n := range needles {
			if bytes.Contains(raw, n) {
				hits = append(hits, path)

				break
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return hits
}

// SDD-007 T-38 (NFR-046): the vault file holds no secret, however it could be written.
func TestSecurity_T38_VaultFileHoldsNoPlaintext(t *testing.T) {
	t.Parallel()

	workspace, v := seeded(t, NewMemoryProvider(nil))

	var needles [][]byte
	for _, s := range canaries {
		needles = append(needles, forms(s)...)
	}

	if hits := grepTree(t, workspace, needles); len(hits) > 0 {
		t.Fatalf("a secret is readable in %v", hits)
	}

	// A rotation re-encrypts everything and leaves no plaintext behind either.
	if err := v.Rotate(bytes.Repeat([]byte{0x42}, KeySize)); err != nil {
		t.Fatal(err)
	}

	if hits := grepTree(t, workspace, needles); len(hits) > 0 {
		t.Fatalf("a secret is readable after a rotation in %v", hits)
	}

	// Only the vault file is in the workspace: no temporary copy of a write is left.
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 1 || entries[0].Name() != "vault.json" {
		t.Fatalf("workspace files: %v, %v", entries, err)
	}
}

// SDD-007 T-39 (ADR-0033 item 2): the master key never lands in the workspace, whichever provider
// generated it, and the vault does not write it anywhere itself.
func TestSecurity_T39_MasterKeyNeverInWorkspace(t *testing.T) {
	t.Parallel()

	keyDir := t.TempDir()
	keyFile := filepath.Join(keyDir, "master.key")

	workspace, _ := seeded(t, FileProvider{Path: keyFile})

	key, err := FileProvider{Path: keyFile}.Load()
	if err != nil {
		t.Fatalf("the provider did not keep the generated key: %v", err)
	}

	needles := [][]byte{
		key,
		[]byte(FormatKey(key)),
		[]byte(hex.EncodeToString(key)),
		[]byte(base64.RawURLEncoding.EncodeToString(key)),
	}

	if hits := grepTree(t, workspace, needles); len(hits) > 0 {
		t.Fatalf("the master key is in the workspace: %v", hits)
	}

	// The provider names never hold it either (they reach doctor and error messages).
	for _, p := range []KeyProvider{FileProvider{Path: keyFile}, EnvProvider{Value: FormatKey(key)}} {
		for _, n := range needles {
			if bytes.Contains([]byte(p.Name()), n) {
				t.Errorf("%T.Name holds the key", p)
			}
		}
	}

	// Every error the vault gives for a bad key says nothing of the key it was given.
	_, err = Open(
		filepath.Join(workspace, "vault.json"),
		EnvProvider{Value: "not-a-key-CANARYKEY"},
	).Get("con_0000000000a")
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("CANARYKEY")) {
		t.Errorf("a bad key is echoed: %v", err)
	}
}

// SDD-007 T-40 (NFR-046): a copy of the workspace without the master key reveals no credential,
// and a copy opened with another key reveals none either.
func TestSecurity_T40_WorkspaceCopyRevealsNoCredential(t *testing.T) {
	t.Parallel()

	keyFile := filepath.Join(t.TempDir(), "master.key")
	workspace, _ := seeded(t, FileProvider{Path: keyFile})

	// The thief has the whole workspace folder and not the key.
	stolen := t.TempDir()
	if err := os.CopyFS(stolen, os.DirFS(workspace)); err != nil {
		t.Fatal(err)
	}

	var needles [][]byte
	for _, s := range canaries {
		needles = append(needles, forms(s)...)
	}

	if hits := grepTree(t, stolen, needles); len(hits) > 0 {
		t.Fatalf("a credential is readable in the copy: %v", hits)
	}

	for name, keys := range map[string]KeyProvider{
		"no key":        NewMemoryProvider(nil),
		"a guessed key": NewMemoryProvider(bytes.Repeat([]byte{0}, KeySize)),
		"another key":   NewMemoryProvider(bytes.Repeat([]byte{0xFF}, KeySize)),
		"no provider":   NoProvider(),
	} {
		stolenVault := Open(filepath.Join(stolen, "vault.json"), keys)

		for i := range canaries {
			secret, err := stolenVault.Get("con_000000000" + string(rune('a'+i)))
			if err == nil || len(secret) > 0 {
				t.Errorf("%s: a secret came out of the copy: %q, %v", name, secret, err)
			}

			if !errors.Is(err, ErrKeyMissing) && !errors.Is(err, ErrWrongKey) {
				t.Errorf("%s: %v, want ErrKeyMissing or ErrWrongKey", name, err)
			}
		}
	}

	// With the key the owner has, the copy opens: the file is intact, only the key protects it.
	legit := Open(filepath.Join(stolen, "vault.json"), FileProvider{Path: keyFile})
	if got, err := legit.Get("con_000000000a"); err != nil || string(got) != canaries[0] {
		t.Errorf("the owner of the key cannot open the copy: %q, %v", got, err)
	}
}
