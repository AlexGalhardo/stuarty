// Package vault is the encrypted file that holds the credentials of the
// connections (ADR-0033, SDD-014 section 3). Every entry is sealed with
// AES-256-GCM under a key derived from a master key that a KeyProvider
// supplies; the master key is never written into the workspace.
//
// Get is not part of service.Service: only service/connections calls it, and
// besides that package only the owner wiring and the contract test environment
// import this one to build a vault (internal/tools/importcheck enforces it), so a
// credential is read inside the closure that makes the outbound call and nowhere else.
package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

const (
	formatName = "stuarty-vault"
	// Version is the version of the file format this build reads and writes.
	Version = 1
	// KeySize is the size of a master key in bytes.
	KeySize = 32
	// MaxSecretSize is the largest secret an entry holds.
	MaxSecretSize = 16 << 10

	saltSize     = 16
	hkdfInfo     = "stuarty-vault-entry-v1"
	aadPrefix    = "stuarty-vault/v1\x00"
	aadCheck     = "stuarty-vault/check"
	checkPayload = "stuarty-vault-check"
)

// The errors of the vault. A caller maps them to E_VAULT_LOCKED (SDD-014 section 7).
var (
	// ErrKeyMissing is a vault file that exists with no master key to open it.
	ErrKeyMissing = errors.New(
		"vault: the master key is not available: restore it from where it was kept (the variable " +
			"STUARTY_MASTER_KEY, the file named by STUARTY_MASTER_KEY_FILE or the keychain item), or start " +
			"again by removing vault.json from the workspace and connecting the accounts again",
	)
	// ErrWrongKey is a master key that does not open the vault file.
	ErrWrongKey = errors.New(
		"vault: the master key does not open this vault: restore the key that created it, or start " +
			"again by removing vault.json from the workspace and connecting the accounts again",
	)
	// ErrNoKeyProvider is a new vault with nowhere to keep a generated master key.
	ErrNoKeyProvider = errors.New(
		"vault: there is nowhere to keep a master key: set STUARTY_MASTER_KEY_FILE to the path of a " +
			"file that holds it (a Docker secret), or STUARTY_MASTER_KEY",
	)
	// ErrCorrupt is a vault file that cannot be read.
	ErrCorrupt = errors.New("vault: the vault file is damaged or was written by a newer version")
	// ErrNotFound is an entry that does not exist.
	ErrNotFound = errors.New("vault: no such entry")
	// ErrInvalidID is an entry id the vault does not accept.
	ErrInvalidID = errors.New("vault: the entry id is not valid")
	// ErrSecretTooLarge is a secret above MaxSecretSize.
	ErrSecretTooLarge = errors.New("vault: the secret is too large")
	// ErrBadKey is a master key that is not 32 bytes.
	ErrBadKey = errors.New("vault: the master key must be 32 bytes, as base64 or 64 hex digits")
)

var idPattern = regexp.MustCompile(`^[a-z0-9_.:-]{1,80}$`)

type sealed struct {
	Nonce []byte `json:"nonce"`
	Data  []byte `json:"data"`
}

type file struct {
	Format  string            `json:"format"`
	Version int               `json:"version"`
	Salt    []byte            `json:"salt"`
	Check   sealed            `json:"check"`
	Entries map[string]sealed `json:"entries"`
}

// Vault is the encrypted file of one workspace.
type Vault struct {
	path string
	keys KeyProvider

	mu  sync.Mutex
	key []byte // the master key, loaded on first use
}

// Open returns the vault at path. It reads nothing and creates nothing: the
// file and the master key come into being with the first Put.
func Open(path string, keys KeyProvider) *Vault {
	return &Vault{path: path, keys: keys}
}

// Path is the vault file.
func (v *Vault) Path() string { return v.path }

// Provider is the name of the active master key provider.
func (v *Vault) Provider() string { return v.keys.Name() }

// Exists reports whether the vault file exists.
func (v *Vault) Exists() bool {
	_, err := os.Stat(v.path)

	return err == nil
}

// Has reports whether the vault holds an entry. It reads the clear entry ids
// only and needs no key.
func (v *Vault) Has(id string) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	f, err := v.read()
	if err != nil {
		return false, err
	}

	if f == nil {
		return false, nil
	}

	_, ok := f.Entries[id]

	return ok, nil
}

// Verify reports whether the vault can be opened, for doctor: nil when there is no file yet or when
// the master key opens it, else ErrKeyMissing, ErrWrongKey, ErrCorrupt or the error of the provider.
// It decrypts no entry.
func (v *Vault) Verify() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	f, err := v.read()
	if err != nil || f == nil {
		return err
	}

	key, err := v.masterKey(true)
	if err != nil {
		return err
	}

	ek, err := entryKey(key, f.Salt)
	if err != nil {
		return err
	}

	return openCheck(ek, f.Check)
}

// Put stores or replaces the secret of an entry.
func (v *Vault) Put(id string, secret []byte) error {
	if !idPattern.MatchString(id) {
		return ErrInvalidID
	}

	if len(secret) > MaxSecretSize {
		return ErrSecretTooLarge
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	f, err := v.read()
	if err != nil {
		return err
	}

	if f == nil {
		return v.create(id, secret)
	}

	key, err := v.masterKey(true)
	if err != nil {
		return err
	}

	ek, err := entryKey(key, f.Salt)
	if err != nil {
		return err
	}

	if err := openCheck(ek, f.Check); err != nil {
		return err
	}

	s, err := seal(ek, aadFor(id), secret)
	if err != nil {
		return err
	}

	f.Entries[id] = s

	return v.write(f)
}

// create writes a new vault whose only entry is id.
func (v *Vault) create(id string, secret []byte) error {
	key, err := v.masterKey(false)
	if err != nil {
		return err
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	ek, err := entryKey(key, salt)
	if err != nil {
		return err
	}

	check, err := seal(ek, []byte(aadCheck), []byte(checkPayload))
	if err != nil {
		return err
	}

	s, err := seal(ek, aadFor(id), secret)
	if err != nil {
		return err
	}

	return v.write(&file{
		Format: formatName, Version: Version, Salt: salt, Check: check, Entries: map[string]sealed{id: s},
	})
}

// Get returns the secret of an entry. Only service/connections calls it.
func (v *Vault) Get(id string) ([]byte, error) {
	if !idPattern.MatchString(id) {
		return nil, ErrInvalidID
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	f, err := v.read()
	if err != nil {
		return nil, err
	}

	if f == nil {
		return nil, ErrNotFound
	}

	s, ok := f.Entries[id]
	if !ok {
		return nil, ErrNotFound
	}

	key, err := v.masterKey(true)
	if err != nil {
		return nil, err
	}

	ek, err := entryKey(key, f.Salt)
	if err != nil {
		return nil, err
	}

	if err := openCheck(ek, f.Check); err != nil {
		return nil, err
	}

	return open(ek, aadFor(id), s)
}

// Delete removes an entry; an entry that is absent is not an error. It needs
// no key, so a connection can be removed when the key is lost.
func (v *Vault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	f, err := v.read()
	if err != nil {
		return err
	}

	if f == nil {
		return nil
	}

	if _, ok := f.Entries[id]; !ok {
		return nil
	}

	delete(f.Entries, id)

	return v.write(f)
}

// Rotate re-encrypts every entry under newKey with a new salt. The caller
// saves newKey in its provider once Rotate returns; the current key must
// open the vault first.
func (v *Vault) Rotate(newKey []byte) error {
	if len(newKey) != KeySize {
		return ErrBadKey
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	f, err := v.read()
	if err != nil {
		return err
	}

	if f == nil {
		v.key = bytes.Clone(newKey)

		return nil
	}

	key, err := v.masterKey(true)
	if err != nil {
		return err
	}

	old, err := entryKey(key, f.Salt)
	if err != nil {
		return err
	}

	if err := openCheck(old, f.Check); err != nil {
		return err
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	fresh, err := entryKey(newKey, salt)
	if err != nil {
		return err
	}

	next := &file{Format: formatName, Version: Version, Salt: salt, Entries: make(map[string]sealed, len(f.Entries))}

	if next.Check, err = seal(fresh, []byte(aadCheck), []byte(checkPayload)); err != nil {
		return err
	}

	for id, s := range f.Entries {
		plain, err := open(old, aadFor(id), s)
		if err != nil {
			return err
		}

		if next.Entries[id], err = seal(fresh, aadFor(id), plain); err != nil {
			return err
		}
	}

	if err := v.write(next); err != nil {
		return err
	}

	v.key = bytes.Clone(newKey)

	return nil
}

// masterKey returns the cached key, or loads it. With existing set the vault
// file exists, so a provider with no key is ErrKeyMissing; for a new vault a
// key is generated and saved.
func (v *Vault) masterKey(existing bool) ([]byte, error) {
	if v.key != nil {
		return v.key, nil
	}

	key, err := v.keys.Load()

	switch {
	case err == nil:
		if len(key) != KeySize {
			return nil, ErrBadKey
		}
	case errors.Is(err, ErrKeyNotFound) && existing:
		return nil, ErrKeyMissing
	case errors.Is(err, ErrKeyNotFound):
		key = make([]byte, KeySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}

		if err := v.keys.Save(key); err != nil {
			if errors.Is(err, ErrReadOnly) {
				return nil, ErrNoKeyProvider
			}

			return nil, fmt.Errorf("vault: saving the master key in %s: %w", v.keys.Name(), err)
		}
	default:
		return nil, fmt.Errorf("vault: reading the master key from %s: %w", v.keys.Name(), err)
	}

	v.key = bytes.Clone(key)

	return v.key, nil
}

// read parses the file; a missing file is (nil, nil).
func (v *Vault) read() (*file, error) {
	raw, err := os.ReadFile(v.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var f file
	if err := json.Unmarshal(raw, &f); err != nil || f.Format != formatName || f.Version != Version ||
		len(f.Salt) != saltSize || len(f.Check.Nonce) == 0 {
		return nil, ErrCorrupt
	}

	if f.Entries == nil {
		f.Entries = map[string]sealed{}
	}

	return &f, nil
}

// write replaces the file through a temporary file in the same folder.
func (v *Vault) write(f *file) error {
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(v.path)

	tmp, err := os.CreateTemp(dir, ".vault-*.tmp")
	if err != nil {
		return err
	}

	name := tmp.Name()

	if err := writeSynced(tmp, raw); err != nil {
		_ = os.Remove(name)

		return err
	}

	if err := os.Rename(name, v.path); err != nil {
		_ = os.Remove(name)

		return err
	}

	return nil
}

// writeSynced writes raw to f with mode 0600, flushes it to disk and closes it.
func writeSynced(f *os.File, raw []byte) error {
	err := f.Chmod(0o600)
	if err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = f.Close()

		return err
	}

	if _, err := f.Write(raw); err != nil {
		_ = f.Close()

		return err
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()

		return err
	}

	return f.Close()
}

func aadFor(id string) []byte { return []byte(aadPrefix + id) }

func entryKey(master, salt []byte) ([]byte, error) {
	return hkdf.Key(sha256.New, master, salt, hkdfInfo, KeySize)
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

func seal(key, aad, plain []byte) (sealed, error) {
	g, err := gcm(key)
	if err != nil {
		return sealed{}, err
	}

	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return sealed{}, err
	}

	return sealed{Nonce: nonce, Data: g.Seal(nil, nonce, plain, aad)}, nil
}

func open(key, aad []byte, s sealed) ([]byte, error) {
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}

	if len(s.Nonce) != g.NonceSize() {
		return nil, ErrCorrupt
	}

	plain, err := g.Open(nil, s.Nonce, s.Data, aad)
	if err != nil {
		return nil, ErrCorrupt
	}

	return plain, nil
}

// openCheck tells a wrong key from a damaged file: the check was sealed under
// the key that created the vault.
func openCheck(key []byte, c sealed) error {
	plain, err := open(key, []byte(aadCheck), c)
	if err != nil || string(plain) != checkPayload {
		return ErrWrongKey
	}

	return nil
}

// minRedacted is the shortest secret Redact replaces: a shorter one would shred any text it is in.
const minRedacted = 8

// redacted stands for a secret in text that leaves the product.
const redacted = "[redacted]"

// Redact returns data with every secret of the vault, and the master key, replaced, in the forms a
// careless log line would write them: as is, as base64 and as hex. It is for the diagnostic bundle
// and the logs (SDD-014 section 9). A vault that cannot be opened has nothing readable in it, so
// only the master key is looked for then.
func (v *Vault) Redact(data []byte) []byte {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := v.key
	if key == nil {
		if k, err := v.keys.Load(); err == nil && len(k) == KeySize {
			key = k
		}
	}

	var needles [][]byte

	if key != nil {
		needles = append(needles, spellings(key)...)

		if f, err := v.read(); err == nil && f != nil {
			if ek, err := entryKey(key, f.Salt); err == nil && openCheck(ek, f.Check) == nil {
				for id, s := range f.Entries {
					if plain, err := open(ek, aadFor(id), s); err == nil && len(plain) >= minRedacted {
						needles = append(needles, spellings(plain)...)
					}
				}
			}
		}
	}

	for _, n := range needles {
		data = bytes.ReplaceAll(data, n, []byte(redacted))
	}

	return data
}

// spellings are a value as is, as base64 (standard and URL, padded or not) and as hex.
func spellings(b []byte) [][]byte {
	return [][]byte{
		b,
		[]byte(base64.StdEncoding.EncodeToString(b)),
		[]byte(base64.RawStdEncoding.EncodeToString(b)),
		[]byte(base64.URLEncoding.EncodeToString(b)),
		[]byte(base64.RawURLEncoding.EncodeToString(b)),
		[]byte(hex.EncodeToString(b)),
	}
}
