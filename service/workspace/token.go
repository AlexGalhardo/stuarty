package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const (
	tokenPrefix = "vvl_"
	tokenBytes  = 32
	// TokenLength is the length of a local token: prefix plus 64 hex characters.
	TokenLength = len(tokenPrefix) + 2*tokenBytes
)

// ErrTokenPermissions is returned when the token file is readable by anyone
// besides its owner. The owner process refuses to start and a client refuses
// to read it (SDD-007 section 3.2).
var ErrTokenPermissions = errors.New("workspace: token file is not private to its owner")

// NewToken returns a fresh local token: 32 random bytes as lower-case hex
// with the vvl_ prefix (SDD-007 section 3.1).
func NewToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}
	return tokenPrefix + hex.EncodeToString(b), nil
}

// LoadToken reads the token file after checking its permissions. The check
// runs at every load.
func (d Dir) LoadToken() (string, error) {
	path := d.Path("token")
	if err := checkTokenFile(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSuffix(string(data), "\n")
	if !validToken(token) {
		return "", fmt.Errorf("workspace: %s is not a local token; run token:rotate", path)
	}
	return token, nil
}

// EnsureToken returns the existing token, or creates one when the file is
// absent. A file with loose permissions is an error, never repaired silently.
func (d Dir) EnsureToken() (string, error) {
	token, err := d.LoadToken()
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return d.RotateToken()
}

// RotateToken replaces the token with a new one. The caller must hold the
// workspace lock. Permissions are set before the secret is written, and the
// file is renamed over token, so a crash never leaves a readable partial token.
func (d Dir) RotateToken() (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}
	tmp := d.Path("token-" + hex.EncodeToString(suffix[:]) + ".tmp")

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("workspace: create token file: %w", err)
	}
	fail := func(err error) (string, error) {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("workspace: write token file: %w", err)
	}

	if err := restrictTokenFile(tmp); err != nil {
		return fail(err)
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("workspace: write token file: %w", err)
	}

	if err := retryOnWindows(func() error { return os.Rename(tmp, d.Path("token")) }); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("workspace: write token file: %w", err)
	}
	return token, nil
}

func validToken(s string) bool {
	if len(s) != TokenLength || !strings.HasPrefix(s, tokenPrefix) {
		return false
	}
	for _, c := range s[len(tokenPrefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
