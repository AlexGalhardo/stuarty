package owner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

// accountBackupPrefix names the folder of backups/ that the creation of an account keeps while it works.
const accountBackupPrefix = "account-"

// createAccount is the Apply of the account manager: it makes the account in a workspace that may hold data
// (SDD-005 section 5.16.3). It copies the database and the vault, re-seals the vault under the new key, seals
// every sealed column and inserts the record in one transaction, reads everything back, and only then deletes
// the copy; on any failure the copy of the vault is put back, the transaction is rolled back and the error is
// returned. The workspace is then as it was.
func (o *Owner) createAccount(ctx context.Context, rec account.Record, fields account.Sealer, vaultKey []byte) error {
	backupDir := filepath.Join(o.Dir.Path("backups"), accountBackupPrefix+o.now().UTC().Format("20060102T150405.000Z"))
	vaultPath := filepath.Join(string(o.Dir), connections.VaultFile)

	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}

	if _, err := o.Store.DB().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = os.RemoveAll(backupDir)

		return err
	}

	if err := copyIfExists(o.Dir.Path("db.sqlite"), filepath.Join(backupDir, "db.sqlite")); err != nil {
		_ = os.RemoveAll(backupDir)

		return err
	}

	hadVault, err := copyIfExistsReport(vaultPath, filepath.Join(backupDir, connections.VaultFile))
	if err != nil {
		_ = os.RemoveAll(backupDir)

		return err
	}

	undo := func(cause error) error {
		if hadVault {
			if rerr := copyIfExists(filepath.Join(backupDir, connections.VaultFile), vaultPath); rerr != nil {
				return errors.Join(cause, fmt.Errorf("restoring the vault: %w", rerr))
			}
		}

		o.vault.Forget()

		return errors.Join(cause, os.RemoveAll(backupDir))
	}

	if err := o.reseal(vaultPath, vaultKey); err != nil {
		return undo(err)
	}

	if err := o.fault("vault"); err != nil {
		return undo(err)
	}

	if err := o.Store.CreateAccount(ctx, rec, fields, o.accountFault); err != nil {
		return undo(err)
	}

	// The vault this process holds still has the old key in memory: the next use asks the account.
	o.vault.Forget()

	// What the sealing replaced is still in the free space of the file, and in the index of the library. A
	// VACUUM rewrites the file without it. If it fails the account stands, and the residue stays in the file
	// until the next VACUUM; the person is told by the log, not by a failed creation.
	if _, err := o.Store.DB().ExecContext(ctx, "VACUUM"); err != nil {
		o.log.WarnContext(ctx, "the database could not be compacted after the account was created", "error", err)
	}

	return os.RemoveAll(backupDir)
}

// fault is the seam of a test: it returns the error the test asks for at a stage of the creation.
func (o *Owner) fault(stage string) error {
	if o.accountFault == nil {
		return nil
	}

	return o.accountFault(stage)
}

// reseal re-seals the vault file under the new key. The old key is the one of the environment, the file or the
// keychain. A vault that the old key does not open (a lost key) is set aside in backups/ and the account starts
// with an empty vault: the connections are made again, and nothing else is lost (ADR-0033 item 4).
func (o *Owner) reseal(vaultPath string, vaultKey []byte) error {
	if _, err := os.Stat(vaultPath); errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	err := vault.Open(vaultPath, o.legacyKeys).Rotate(vaultKey)

	switch {
	case err == nil:
		return nil
	case errors.Is(err, vault.ErrKeyMissing), errors.Is(err, vault.ErrWrongKey), errors.Is(err, vault.ErrCorrupt):
		aside := filepath.Join(
			o.Dir.Path("backups"),
			"vault-unreadable-"+o.now().UTC().Format("20060102T150405.000Z")+".json",
		)

		return os.Rename(vaultPath, aside)
	}

	return err
}

// recoverAccountBackup finishes what a crash left in the middle of an account creation: the database changes
// are one transaction, so the account row says whether it committed. With no row, the vault copy goes back;
// either way the copy is removed.
func (o *Owner) recoverAccountBackup(ctx context.Context) error {
	dirs, err := filepath.Glob(filepath.Join(o.Dir.Path("backups"), accountBackupPrefix+"*"))
	if err != nil || len(dirs) == 0 {
		return err
	}

	_, accErr := o.Store.Accounts().Get(ctx)
	committed := accErr == nil

	if accErr != nil && !errors.Is(accErr, account.ErrNoAccount) {
		return accErr
	}

	for _, dir := range dirs {
		if !strings.HasPrefix(filepath.Base(dir), accountBackupPrefix) {
			continue
		}

		if !committed {
			saved := filepath.Join(dir, connections.VaultFile)
			if err := copyIfExists(saved, filepath.Join(string(o.Dir), connections.VaultFile)); err != nil {
				return err
			}
		}

		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}

	return nil
}

func copyIfExists(from, to string) error {
	_, err := copyIfExistsReport(from, to)

	return err
}

// copyIfExistsReport copies a file, mode 0600, and reports whether it existed.
func copyIfExistsReport(from, to string) (bool, error) {
	in, err := os.Open(from)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return false, err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return false, err
	}

	if err := out.Sync(); err != nil {
		_ = out.Close()

		return false, err
	}

	return true, out.Close()
}
