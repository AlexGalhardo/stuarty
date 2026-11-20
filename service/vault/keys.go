package vault

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// The names of the variables of the master key (SDD-000 section 10).
const (
	EnvMasterKey     = "STUARTY_MASTER_KEY"
	EnvMasterKeyFile = "STUARTY_MASTER_KEY_FILE"
)

// KeychainService is the name of the keychain item of the vault.
const KeychainService = "stuarty"

// ErrBothKeySources is a configuration that names the key twice.
var ErrBothKeySources = errors.New(
	"vault: set only one of STUARTY_MASTER_KEY and STUARTY_MASTER_KEY_FILE",
)

// ParseKey reads a master key written as base64 or as 64 hex digits, with
// white space around it ignored. It never echoes the value in an error.
func ParseKey(text string) ([]byte, error) {
	text = strings.TrimSpace(text)

	if len(text) == 2*KeySize {
		if key, err := hex.DecodeString(text); err == nil {
			return key, nil
		}
	}

	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		if key, err := enc.DecodeString(text); err == nil && len(key) == KeySize {
			return key, nil
		}
	}

	return nil, ErrBadKey
}

// FormatKey writes a key as base64, the form a provider stores.
func FormatKey(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

// EnvProvider reads the key from an environment variable. It cannot store one.
type EnvProvider struct {
	Value string // the content of STUARTY_MASTER_KEY
}

// Name implements KeyProvider.
func (EnvProvider) Name() string { return "environment (" + EnvMasterKey + ")" }

// Load implements KeyProvider.
func (p EnvProvider) Load() ([]byte, error) {
	if strings.TrimSpace(p.Value) == "" {
		return nil, ErrKeyNotFound
	}

	return ParseKey(p.Value)
}

// Save implements KeyProvider.
func (EnvProvider) Save([]byte) error { return ErrReadOnly }

// FileProvider reads the key from a file, a Docker secret for example. A file
// that does not exist is created, mode 0600, by Save; an existing file is
// never overwritten.
type FileProvider struct {
	Path string
}

// Name implements KeyProvider.
func (FileProvider) Name() string { return "file (" + EnvMasterKeyFile + ")" }

// Load implements KeyProvider.
func (p FileProvider) Load() ([]byte, error) {
	raw, err := os.ReadFile(p.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrKeyNotFound
	}

	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(string(raw)) == "" {
		return nil, ErrKeyNotFound
	}

	return ParseKey(string(raw))
}

// Save implements KeyProvider.
func (p FileProvider) Save(key []byte) error {
	f, err := os.OpenFile(p.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		// A read-only secret mount, a missing folder or a file that is there
		// already: the key cannot be kept here.
		return fmt.Errorf("%w: %w", ErrReadOnly, err)
	}

	if _, err := f.WriteString(FormatKey(key) + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(p.Path)

		return err
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()

		return err
	}

	return f.Close()
}

// Keychain is the client of an operating system secret store. The build has
// one for Windows; OSKeychain returns nil where there is none.
type Keychain interface {
	// Get returns the secret of an item, or ErrKeyNotFound.
	Get(service, account string) ([]byte, error)
	// Set stores or replaces the secret of an item.
	Set(service, account string, secret []byte) error
}

// KeychainProvider keeps the key in the operating system keychain, one item
// per workspace.
type KeychainProvider struct {
	Store     Keychain
	Workspace string // the absolute path of the workspace
}

// Account is the item name of a workspace: 16 hex digits of the SHA-256 of its path.
func (p KeychainProvider) Account() string {
	sum := sha256.Sum256([]byte(p.Workspace))

	return "vault:" + hex.EncodeToString(sum[:8])
}

// Name implements KeyProvider.
func (KeychainProvider) Name() string { return "operating system keychain" }

// Load implements KeyProvider.
func (p KeychainProvider) Load() ([]byte, error) {
	raw, err := p.Store.Get(KeychainService, p.Account())
	if err != nil {
		return nil, err
	}

	return ParseKey(string(raw))
}

// Save implements KeyProvider.
func (p KeychainProvider) Save(key []byte) error {
	return p.Store.Set(KeychainService, p.Account(), []byte(FormatKey(key)))
}

// noProvider is the end of the list: no variable, no file and no keychain.
type noProvider struct{}

func (noProvider) Name() string          { return "none" }
func (noProvider) Load() ([]byte, error) { return nil, ErrKeyNotFound }
func (noProvider) Save([]byte) error     { return ErrReadOnly }

// NoProvider is the provider of a build or a test with nowhere to keep a key: a vault
// that needs a new key says so (ErrNoKeyProvider), and a vault with no entry works.
func NoProvider() KeyProvider { return noProvider{} }

// Config says where the master key may come from.
type Config struct {
	Env       string   // STUARTY_MASTER_KEY
	File      string   // STUARTY_MASTER_KEY_FILE
	Workspace string   // the absolute workspace path, for the keychain item
	Keychain  Keychain // nil when the platform has none
}

// ConfigFromEnv reads the two variables through getenv.
func ConfigFromEnv(getenv func(string) string, workspace string) Config {
	return Config{
		Env: getenv(EnvMasterKey), File: getenv(EnvMasterKeyFile), Workspace: workspace, Keychain: OSKeychain(),
	}
}

// Resolve picks the provider: the environment, then the file, then the
// keychain, then none. Naming both variables is an error.
func Resolve(c Config) (KeyProvider, error) {
	switch {
	case c.Env != "" && c.File != "":
		return nil, ErrBothKeySources
	case c.Env != "":
		return EnvProvider{Value: c.Env}, nil
	case c.File != "":
		return FileProvider{Path: c.File}, nil
	case c.Keychain != nil:
		return KeychainProvider{Store: c.Keychain, Workspace: c.Workspace}, nil
	}

	return noProvider{}, nil
}
