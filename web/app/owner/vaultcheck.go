package owner

import (
	"github.com/AlexGalhardo/stuarty/service/vault"
)

// VerifyVault reports whether the vault file at vaultPath can be opened with the master key that the
// environment names (STUARTY_MASTER_KEY, STUARTY_MASTER_KEY_FILE or the keychain item of the workspace).
// It decrypts no entry and returns no credential: the restore of a backup uses it to refuse a key that
// is not the one that sealed the vault, before it writes anything (SDD-009 section 21.4).
func VerifyVault(getenv func(string) string, workspaceDir, vaultPath string) error {
	keys, err := vault.Resolve(vault.ConfigFromEnv(getenv, workspaceDir))
	if err != nil {
		return err
	}

	return vault.Open(vaultPath, keys).Verify()
}
