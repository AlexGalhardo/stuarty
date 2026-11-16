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
	runtimeSecretFile  = "runtime.secret"
	runtimeSecretBytes = 32
)

// NewRuntimeSecret writes a fresh per-run secret next to runtime.json and returns it. Network mode
// has no local token, so this file is how a command on the same machine proves that the process
// answering at the advertised address is the owner of this workspace before it sends an API key
// (SDD-007 section 3.8). It is created owner-only, replaced on every start and removed with
// runtime.json. The caller must hold the workspace lock.
func (d Dir) NewRuntimeSecret() (string, error) {
	raw := make([]byte, runtimeSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}

	secret := hex.EncodeToString(raw)

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}

	tmp := d.Path("runtime-secret-" + hex.EncodeToString(suffix[:]) + ".tmp")

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("workspace: create runtime secret: %w", err)
	}

	fail := func(err error) (string, error) {
		_ = f.Close()
		_ = os.Remove(tmp)

		return "", fmt.Errorf("workspace: write runtime secret: %w", err)
	}

	if err := restrictTokenFile(tmp); err != nil {
		return fail(err)
	}

	if _, err := f.WriteString(secret + "\n"); err != nil {
		return fail(err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("workspace: write runtime secret: %w", err)
	}

	if err := retryOnWindows(func() error { return os.Rename(tmp, d.Path(runtimeSecretFile)) }); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("workspace: write runtime secret: %w", err)
	}

	return secret, nil
}

// LoadRuntimeSecret reads the per-run secret after checking that the file is private to its owner.
// A missing file wraps fs.ErrNotExist.
func (d Dir) LoadRuntimeSecret() (string, error) {
	path := d.Path(runtimeSecretFile)
	if err := checkTokenFile(path); err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	secret := strings.TrimSuffix(string(data), "\n")
	if len(secret) != 2*runtimeSecretBytes {
		return "", fmt.Errorf("workspace: %s is not a runtime secret", path)
	}

	return secret, nil
}

func (d Dir) removeRuntimeSecret() error {
	err := retryOnWindows(func() error {
		err := os.Remove(d.Path(runtimeSecretFile))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return err
	})
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	return nil
}
