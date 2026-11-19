package vault

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeKeychain struct {
	items map[string][]byte
}

func (k *fakeKeychain) Get(service, account string) ([]byte, error) {
	if v, ok := k.items[service+"/"+account]; ok {
		return v, nil
	}

	return nil, ErrKeyNotFound
}

func (k *fakeKeychain) Set(service, account string, secret []byte) error {
	if k.items == nil {
		k.items = map[string][]byte{}
	}

	k.items[service+"/"+account] = bytes.Clone(secret)

	return nil
}

func TestMasterKeyParse(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{0xAB}, KeySize)

	for name, text := range map[string]string{
		"hex":        hex.EncodeToString(key),
		"std":        base64.StdEncoding.EncodeToString(key),
		"raw":        base64.RawStdEncoding.EncodeToString(key),
		"url":        base64.URLEncoding.EncodeToString(key),
		"whitespace": "  " + base64.StdEncoding.EncodeToString(key) + "\r\n",
	} {
		got, err := ParseKey(text)
		if err != nil || !bytes.Equal(got, key) {
			t.Errorf("%s: %x, %v", name, got, err)
		}
	}

	for name, text := range map[string]string{
		"empty": "", "short": "abcd", "not text": "!!!!", "31 bytes": base64.StdEncoding.EncodeToString(key[:31]),
	} {
		if _, err := ParseKey(text); !errors.Is(err, ErrBadKey) {
			t.Errorf("%s: %v, want ErrBadKey", name, err)
		}
	}
}

func TestMasterKeyBadKeyNeverEchoesTheValue(t *testing.T) {
	t.Parallel()

	_, err := ParseKey("super-secret-but-short")
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("super-secret")) {
		t.Fatalf("the error echoes the value: %v", err)
	}
}

func TestMasterKeyEnvProvider(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{1}, KeySize)
	p := EnvProvider{Value: FormatKey(key)}

	got, err := p.Load()
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("Load = %x, %v", got, err)
	}

	if err := p.Save(key); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Save = %v, want ErrReadOnly", err)
	}

	if _, err := (EnvProvider{}).Load(); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("an empty variable: %v, want ErrKeyNotFound", err)
	}

	if _, err := (EnvProvider{Value: "short"}).Load(); !errors.Is(err, ErrBadKey) {
		t.Errorf("a bad variable: %v, want ErrBadKey", err)
	}
}

func TestMasterKeyFileProvider(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "master.key")
	p := FileProvider{Path: path}

	if _, err := p.Load(); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("a missing file: %v, want ErrKeyNotFound", err)
	}

	key := bytes.Repeat([]byte{7}, KeySize)
	if err := p.Save(key); err != nil {
		t.Fatal(err)
	}

	got, err := p.Load()
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("Load = %x, %v", got, err)
	}

	// An existing file is never overwritten.
	if err := p.Save(bytes.Repeat([]byte{9}, KeySize)); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Save over a file: %v, want ErrReadOnly", err)
	}

	if got, _ = p.Load(); !bytes.Equal(got, key) {
		t.Error("the key in the file changed")
	}

	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := p.Load(); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("an empty file: %v, want ErrKeyNotFound", err)
	}

	missing := FileProvider{Path: filepath.Join(t.TempDir(), "no", "dir", "k")}
	if err := missing.Save(key); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Save in a missing folder: %v, want ErrReadOnly", err)
	}
}

func TestMasterKeyKeychainProvider(t *testing.T) {
	t.Parallel()

	store := &fakeKeychain{}
	a := KeychainProvider{Store: store, Workspace: "/home/u/.stuarty"}
	b := KeychainProvider{Store: store, Workspace: "/srv/other"}

	if a.Account() == b.Account() {
		t.Fatal("two workspaces share a keychain item")
	}

	if _, err := a.Load(); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("Load before Save: %v", err)
	}

	key := bytes.Repeat([]byte{5}, KeySize)
	if err := a.Save(key); err != nil {
		t.Fatal(err)
	}

	if got, err := a.Load(); err != nil || !bytes.Equal(got, key) {
		t.Fatalf("Load = %x, %v", got, err)
	}

	if _, err := b.Load(); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("another workspace read the key: %v", err)
	}
}

func TestMasterKeyResolve(t *testing.T) {
	t.Parallel()

	key := FormatKey(bytes.Repeat([]byte{2}, KeySize))
	kc := &fakeKeychain{}

	if _, err := Resolve(Config{Env: key, File: "/x"}); !errors.Is(err, ErrBothKeySources) {
		t.Errorf("both variables: %v", err)
	}

	for name, tc := range map[string]struct {
		cfg  Config
		want string
	}{
		"env":      {Config{Env: key, Keychain: kc}, "environment (" + EnvMasterKey + ")"},
		"file":     {Config{File: "/run/secrets/k", Keychain: kc}, "file (" + EnvMasterKeyFile + ")"},
		"keychain": {Config{Keychain: kc, Workspace: "/w"}, "operating system keychain"},
		"none":     {Config{}, "none"},
	} {
		p, err := Resolve(tc.cfg)
		if err != nil || p.Name() != tc.want {
			t.Errorf("%s: %v, %v, want %q", name, p, err, tc.want)
		}
	}
}

func TestMasterKeyNamesNeverHoldTheKey(t *testing.T) {
	t.Parallel()

	key := FormatKey(bytes.Repeat([]byte{3}, KeySize))

	for _, p := range []KeyProvider{
		EnvProvider{Value: key}, FileProvider{Path: "/run/secrets/k"}, KeychainProvider{Store: &fakeKeychain{}},
	} {
		if bytes.Contains([]byte(p.Name()), []byte(key)) {
			t.Errorf("%T.Name holds the key", p)
		}
	}
}

// The vault created with a file provider generates the key on first use,
// keeps it in the file, and a second process opens the vault with the file.
func TestMasterKeyGeneratedOnFirstUseAndReused(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "secrets", "master.key")

	if err := os.Mkdir(filepath.Dir(keyFile), 0o700); err != nil {
		t.Fatal(err)
	}

	vaultPath := filepath.Join(dir, "vault.json")

	first := Open(vaultPath, FileProvider{Path: keyFile})
	if err := first.Put("con_a", []byte("one")); err != nil {
		t.Fatal(err)
	}

	second := Open(vaultPath, FileProvider{Path: keyFile})

	got, err := second.Get("con_a")
	if err != nil || string(got) != "one" {
		t.Fatalf("Get = %q, %v", got, err)
	}

	// The key file is outside the workspace folder and holds no vault data.
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(raw, []byte("one")) {
		t.Error("the key file holds a secret")
	}

	// Without the key file, the vault says so.
	if err := os.Remove(keyFile); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(vaultPath, FileProvider{Path: keyFile}).Get("con_a"); !errors.Is(err, ErrKeyMissing) {
		t.Errorf("Get without the key file: %v, want ErrKeyMissing", err)
	}
}

func TestMasterKeyReadOnlyEnvironmentCannotStartAVault(t *testing.T) {
	t.Parallel()

	v := Open(filepath.Join(t.TempDir(), "vault.json"), noProvider{})
	if err := v.Put("con_a", []byte("x")); !errors.Is(err, ErrNoKeyProvider) {
		t.Fatalf("Put with no provider: %v, want ErrNoKeyProvider", err)
	}
}

func TestMasterKeyErrorsSayHowToRecover(t *testing.T) {
	t.Parallel()

	for _, err := range []error{ErrKeyMissing, ErrWrongKey} {
		msg := err.Error()
		if !bytes.Contains([]byte(msg), []byte("vault.json")) || !bytes.Contains([]byte(msg), []byte("restore")) {
			t.Errorf("%q does not say how to restore the key or reset the connections", msg)
		}
	}

	if !bytes.Contains([]byte(ErrNoKeyProvider.Error()), []byte(EnvMasterKeyFile)) {
		t.Error("ErrNoKeyProvider does not name STUARTY_MASTER_KEY_FILE")
	}
}
