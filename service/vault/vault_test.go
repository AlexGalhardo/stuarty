package vault

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func newKey(t *testing.T) []byte {
	t.Helper()

	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}

	return k
}

func newVault(t *testing.T) (*Vault, *MemoryProvider) {
	t.Helper()

	p := NewMemoryProvider(nil)

	return Open(filepath.Join(t.TempDir(), "vault.json"), p), p
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	v, _ := newVault(t)

	if v.Exists() {
		t.Fatal("a vault that was never written exists")
	}

	if _, err := v.Get("con_a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on an empty vault: %v, want ErrNotFound", err)
	}

	if err := v.Put("con_a", []byte("alpha-secret")); err != nil {
		t.Fatal(err)
	}

	if err := v.Put("con_b", []byte("beta-secret")); err != nil {
		t.Fatal(err)
	}

	got, err := v.Get("con_a")
	if err != nil || string(got) != "alpha-secret" {
		t.Fatalf("Get = %q, %v", got, err)
	}

	if err := v.Put("con_a", []byte("alpha-two")); err != nil {
		t.Fatal(err)
	}

	if got, _ = v.Get("con_a"); string(got) != "alpha-two" {
		t.Fatalf("replaced entry = %q", got)
	}

	if ok, _ := v.Has("con_b"); !ok {
		t.Fatal("Has(con_b) = false")
	}

	if err := v.Delete("con_b"); err != nil {
		t.Fatal(err)
	}

	if err := v.Delete("con_b"); err != nil {
		t.Fatalf("deleting an absent entry: %v", err)
	}

	if ok, _ := v.Has("con_b"); ok {
		t.Fatal("Has(con_b) after Delete = true")
	}

	if _, err := v.Get("con_b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete: %v", err)
	}
}

func TestInvalidInput(t *testing.T) {
	t.Parallel()

	v, _ := newVault(t)

	for _, id := range []string{"", "Con", "a b", "../x", string(bytes.Repeat([]byte("a"), 81))} {
		if err := v.Put(id, []byte("x")); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Put(%q) = %v, want ErrInvalidID", id, err)
		}
	}

	if err := v.Put("con_a", make([]byte, MaxSecretSize+1)); !errors.Is(err, ErrSecretTooLarge) {
		t.Errorf("a large secret: %v", err)
	}
}

func TestFileHoldsNoPlaintext(t *testing.T) {
	t.Parallel()

	v, _ := newVault(t)
	secrets := []string{"sk-live-AAAA1111bbbb", "ghp_ZZZZ9999yyyy", "p@ssw0rd-with-chars"}

	for i, s := range secrets {
		if err := v.Put("con_"+strconv.Itoa(i), []byte(s)); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(v.Path())
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range secrets {
		if bytes.Contains(raw, []byte(s)) {
			t.Errorf("the vault file contains %q", s)
		}
	}

	if bytes.Contains(raw, []byte(checkPayload)) {
		t.Error("the vault file contains the check text")
	}
}

func TestWrongKeyAndMissingKey(t *testing.T) {
	t.Parallel()

	v, _ := newVault(t)
	if err := v.Put("con_a", []byte("secret")); err != nil {
		t.Fatal(err)
	}

	wrong := Open(v.Path(), NewMemoryProvider(newKey(t)))

	if _, err := wrong.Get("con_a"); !errors.Is(err, ErrWrongKey) {
		t.Errorf("Get with a wrong key: %v, want ErrWrongKey", err)
	}

	if err := wrong.Put("con_b", []byte("x")); !errors.Is(err, ErrWrongKey) {
		t.Errorf("Put with a wrong key: %v, want ErrWrongKey", err)
	}

	none := Open(v.Path(), NewMemoryProvider(nil))

	if _, err := none.Get("con_a"); !errors.Is(err, ErrKeyMissing) {
		t.Errorf("Get with no key: %v, want ErrKeyMissing", err)
	}

	// Deleting and asking whether an entry exists need no key.
	if ok, err := none.Has("con_a"); err != nil || !ok {
		t.Errorf("Has with no key = %v, %v", ok, err)
	}

	if err := none.Delete("con_a"); err != nil {
		t.Errorf("Delete with no key: %v", err)
	}
}

func TestCorruptedFile(t *testing.T) {
	t.Parallel()

	v, p := newVault(t)
	if err := v.Put("con_a", []byte("secret")); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(v.Path())
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func([]byte) []byte{
		"truncated":    func(b []byte) []byte { return b[:len(b)/2] },
		"garbage":      func([]byte) []byte { return []byte("not json") },
		"wrong format": func(b []byte) []byte { return bytes.Replace(b, []byte("stuarty-vault"), []byte("other-vault"), 1) },
		"newer":        func(b []byte) []byte { return bytes.Replace(b, []byte(`"version": 1`), []byte(`"version": 9`), 1) },
	} {
		if err := os.WriteFile(v.Path(), mutate(bytes.Clone(raw)), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := Open(v.Path(), p).Get("con_a"); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: %v, want ErrCorrupt", name, err)
		}
	}
}

func TestMovedEntryIsRefused(t *testing.T) {
	t.Parallel()

	v, p := newVault(t)
	if err := v.Put("con_a", []byte("secret-a")); err != nil {
		t.Fatal(err)
	}

	if err := v.Put("con_b", []byte("secret-b")); err != nil {
		t.Fatal(err)
	}

	// Moving the sealed data of one id to another must not open: the id is
	// part of the additional data.
	f, err := Open(v.Path(), p).read()
	if err != nil {
		t.Fatal(err)
	}

	f.Entries["con_b"] = f.Entries["con_a"]

	if err := v.write(f); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(v.Path(), p).Get("con_b"); !errors.Is(err, ErrCorrupt) {
		t.Errorf("a moved entry opened: %v", err)
	}
}

func TestRotate(t *testing.T) {
	t.Parallel()

	v, p := newVault(t)

	for i, s := range []string{"one", "two", "three"} {
		if err := v.Put("con_"+strconv.Itoa(i), []byte(s)); err != nil {
			t.Fatal(err)
		}
	}

	before, _ := os.ReadFile(v.Path())
	next := newKey(t)

	if err := v.Rotate(next); err != nil {
		t.Fatal(err)
	}

	after, _ := os.ReadFile(v.Path())
	if bytes.Equal(before, after) {
		t.Fatal("the file did not change")
	}

	// The old key no longer opens it; the new one does, in a fresh process.
	if _, err := Open(v.Path(), p).Get("con_0"); !errors.Is(err, ErrWrongKey) {
		t.Errorf("the old key after Rotate: %v, want ErrWrongKey", err)
	}

	fresh := Open(v.Path(), NewMemoryProvider(next))

	for i, want := range []string{"one", "two", "three"} {
		got, err := fresh.Get("con_" + strconv.Itoa(i))
		if err != nil || string(got) != want {
			t.Errorf("entry %d after Rotate = %q, %v", i, got, err)
		}
	}

	if err := v.Rotate([]byte("short")); !errors.Is(err, ErrBadKey) {
		t.Errorf("Rotate with a short key: %v", err)
	}
}

func TestRotateRefusedWithWrongKey(t *testing.T) {
	t.Parallel()

	v, _ := newVault(t)
	if err := v.Put("con_a", []byte("secret")); err != nil {
		t.Fatal(err)
	}

	wrong := Open(v.Path(), NewMemoryProvider(newKey(t)))
	if err := wrong.Rotate(newKey(t)); !errors.Is(err, ErrWrongKey) {
		t.Errorf("Rotate with a wrong key: %v, want ErrWrongKey", err)
	}
}

func TestConcurrentUse(t *testing.T) {
	t.Parallel()

	v, _ := newVault(t)

	var wg sync.WaitGroup

	for i := range 20 {
		wg.Go(func() {
			id := "con_" + strconv.Itoa(i)
			want := "secret-" + strconv.Itoa(i)

			if err := v.Put(id, []byte(want)); err != nil {
				t.Error(err)

				return
			}

			if got, err := v.Get(id); err != nil || string(got) != want {
				t.Errorf("Get(%s) = %q, %v", id, got, err)
			}
		})
	}

	wg.Wait()

	for i := range 20 {
		if ok, _ := v.Has("con_" + strconv.Itoa(i)); !ok {
			t.Errorf("entry %d is missing", i)
		}
	}
}

type readOnlyProvider struct{}

func (readOnlyProvider) Name() string          { return "read-only" }
func (readOnlyProvider) Load() ([]byte, error) { return nil, ErrKeyNotFound }
func (readOnlyProvider) Save([]byte) error     { return ErrReadOnly }

func TestNewVaultWithNowhereToKeepTheKey(t *testing.T) {
	t.Parallel()

	v := Open(filepath.Join(t.TempDir(), "vault.json"), readOnlyProvider{})

	if err := v.Put("con_a", []byte("x")); !errors.Is(err, ErrNoKeyProvider) {
		t.Fatalf("Put: %v, want ErrNoKeyProvider", err)
	}

	if v.Exists() {
		t.Error("a vault file was created without a key")
	}
}

func TestNoTemporaryFileIsLeftBehind(t *testing.T) {
	t.Parallel()

	v, _ := newVault(t)
	for i := range 3 {
		if err := v.Put("con_"+strconv.Itoa(i), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(v.Path()))
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Errorf("the folder has %d files, want only the vault", len(entries))
	}
}

func TestRedactReplacesSecretsAndTheMasterKey(t *testing.T) {
	t.Parallel()

	v, p := newVault(t)
	if err := v.Put("con_a", []byte("a-long-enough-secret")); err != nil {
		t.Fatal(err)
	}

	if err := v.Put("con_b", []byte("short")); err != nil {
		t.Fatal(err)
	}

	key, err := p.Load()
	if err != nil {
		t.Fatal(err)
	}

	text := "token=a-long-enough-secret b64=" + FormatKey([]byte("a-long-enough-secret")) +
		" key=" + FormatKey(key) + " short=short"

	got := string(v.Redact([]byte(text)))

	for _, leak := range []string{"a-long-enough-secret", FormatKey([]byte("a-long-enough-secret")), FormatKey(key)} {
		if strings.Contains(got, leak) {
			t.Errorf("%q survived: %s", leak, got)
		}
	}

	// A secret under eight bytes would shred the text it is in; it is left alone.
	if !strings.Contains(got, "short=short") {
		t.Errorf("a short secret was replaced: %s", got)
	}

	// A vault that cannot be opened still redacts nothing it cannot read, and does not fail.
	locked := Open(v.Path(), NewMemoryProvider(nil))
	if out := string(locked.Redact([]byte(text))); out != text {
		t.Errorf("a locked vault changed the text: %s", out)
	}
}
