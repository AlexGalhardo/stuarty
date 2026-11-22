package owner

import (
	"bytes"
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
// (SDD-005 section 5.17.3). It copies the database and the vault, re-seals the vault under the new key, seals
// every sealed column and inserts the record in one transaction, reads everything back, and only then removes
// what is left in clear; on any failure the copy of the vault is put back, the transaction is rolled back and the
// error is returned. The workspace is then as it was. After the commit nothing can fail the creation: the account
// exists, and what could not be cleaned is finished by the next start.
func (o *Owner) createAccount(ctx context.Context, rec account.Record, fields account.Sealer, vaultKey []byte) error {
	// A creation that failed earlier and could not put the vault back is finished first: the vault it left is
	// sealed by a key that no longer exists.
	if err := o.recoverAccountBackup(ctx); err != nil {
		return err
	}

	backupDir := filepath.Join(o.Dir.Path("backups"), accountBackupPrefix+o.now().UTC().Format("20060102T150405.000Z"))
	vaultPath := filepath.Join(string(o.Dir), connections.VaultFile)

	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}

	if _, err := o.Store.DB().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = os.RemoveAll(backupDir)

		return err
	}

	if _, err := copyFileAtomic(o.Dir.Path("db.sqlite"), filepath.Join(backupDir, "db.sqlite")); err != nil {
		_ = os.RemoveAll(backupDir)

		return err
	}

	hadVault, err := copyFileAtomic(vaultPath, filepath.Join(backupDir, connections.VaultFile))
	if err != nil {
		_ = os.RemoveAll(backupDir)

		return err
	}

	undo := func(cause error) error {
		if hadVault {
			// If this fails the copy stays where it is, and the next start or the next creation puts it back.
			if rerr := restoreVault(filepath.Join(backupDir, connections.VaultFile), vaultPath); rerr != nil {
				return errors.Join(cause, fmt.Errorf("restoring the vault: %w", rerr))
			}
		}

		o.vault.Forget()

		return errors.Join(cause, os.RemoveAll(backupDir))
	}

	if err := o.reseal(ctx, vaultPath, vaultKey); err != nil {
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

	o.finishAccount(ctx, backupDir)

	return nil
}

// finishAccount removes what the sealing left in clear: the old pages of db.sqlite and of its log, the copies that
// the schema migrations keep in backups/ and the copy of this creation. The account is committed when it runs, so
// a failure is logged and the next start tries again; the copy of the creation is the mark that it must, and goes
// last, once the checkpoint has made the commit durable.
func (o *Owner) finishAccount(ctx context.Context, copies ...string) {
	if err := o.fault("finish"); err != nil {
		o.log.WarnContext(ctx, "the clear text of the workspace was not removed; the next start tries again", "error", err)

		return
	}

	if err := o.Store.CompactSealed(ctx); err != nil {
		o.log.WarnContext(ctx, "the clear text of the workspace was not removed; the next start tries again", "error", err)

		return
	}

	if err := removeClearBackups(o.Dir.Path("backups")); err != nil {
		o.log.WarnContext(ctx, "an old copy of the database was not removed; the next start tries again", "error", err)

		return
	}

	for _, dir := range copies {
		if err := os.RemoveAll(dir); err != nil {
			o.log.WarnContext(ctx, "the copy of the account creation was not removed; the next start tries again",
				"error", err)
		}
	}
}

// removeClearBackups deletes the copies of the database that the schema migrations made before the account
// existed (SDD-005 section 7): they hold every word of the person in clear.
func removeClearBackups(dir string) error {
	names, err := filepath.Glob(filepath.Join(dir, "db-*.sqlite"))
	if err != nil {
		return err
	}

	var errs []error

	for _, name := range names {
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
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
func (o *Owner) reseal(ctx context.Context, vaultPath string, vaultKey []byte) error {
	if _, err := os.Stat(vaultPath); errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	// A vault of its own, forgotten at once: it would keep a copy of the new key in memory.
	legacy := vault.Open(vaultPath, o.legacyKeys)
	defer legacy.Forget()

	err := legacy.Rotate(vaultKey)

	switch {
	case err == nil:
		return nil
	case errors.Is(err, vault.ErrKeyMissing), errors.Is(err, vault.ErrWrongKey), errors.Is(err, vault.ErrCorrupt):
		aside := filepath.Join(
			o.Dir.Path("backups"),
			"vault-unreadable-"+o.now().UTC().Format("20060102T150405.000Z")+".json",
		)

		o.log.WarnContext(ctx, "the vault cannot be opened with its key: it is set aside and the connections must be "+
			"made again", "file", aside, "error", err)

		return os.Rename(vaultPath, aside)
	}

	return err
}

// recoverAccountBackup finishes what a crash left in the middle of an account creation. The database changes are
// one transaction, so the account row says whether it committed: with no row, the copy of the vault goes back; with
// the row, what is left in clear is removed. Either way the copy goes.
func (o *Owner) recoverAccountBackup(ctx context.Context) error {
	dirs, err := filepath.Glob(filepath.Join(o.Dir.Path("backups"), accountBackupPrefix+"*"))
	if err != nil || len(dirs) == 0 {
		return err
	}

	_, accErr := o.Store.Accounts().Get(ctx)

	if accErr == nil {
		o.finishAccount(ctx, dirs...)

		return nil
	}

	if !errors.Is(accErr, account.ErrNoAccount) {
		return accErr
	}

	// The names sort by time. The oldest copy holds the vault as it was before the first attempt.
	restored := false

	for _, dir := range dirs {
		if !strings.HasPrefix(filepath.Base(dir), accountBackupPrefix) {
			continue
		}

		if saved := filepath.Join(dir, connections.VaultFile); !restored && fileExists(saved) {
			if err := restoreVault(saved, filepath.Join(string(o.Dir), connections.VaultFile)); err != nil {
				return err
			}

			restored = true
		}

		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// restoreVault puts the copy of the vault back. A vault that is already the copy is left alone: the re-seal that
// failed may have failed because the file cannot be written, and then a restore would fail for the same reason.
func restoreVault(saved, vaultPath string) error {
	want, err := os.ReadFile(saved)
	if err != nil {
		return err
	}

	if have, rerr := os.ReadFile(vaultPath); rerr == nil && bytes.Equal(have, want) {
		return nil
	}

	_, err = copyFileAtomic(saved, vaultPath)

	return err
}

// copyFileAtomic copies a file, mode 0600, and reports whether it existed. The destination appears whole or not
// at all: a process killed in the middle leaves it as it was, so a half-written copy is never taken for the file.
func copyFileAtomic(from, to string) (bool, error) {
	in, err := os.Open(from)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	defer func() { _ = in.Close() }()

	out, err := os.CreateTemp(filepath.Dir(to), filepath.Base(to)+".*.tmp")
	if err != nil {
		return false, err
	}

	name := out.Name()

	if err := writeCopy(out, in); err != nil {
		_ = os.Remove(name)

		return false, err
	}

	if err := os.Chmod(name, 0o600); err != nil {
		_ = os.Remove(name)

		return false, err
	}

	if err := os.Rename(name, to); err != nil {
		_ = os.Remove(name)

		return false, err
	}

	return true, nil
}

func writeCopy(out *os.File, in io.Reader) error {
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return err
	}

	if err := out.Sync(); err != nil {
		_ = out.Close()

		return err
	}

	return out.Close()
}
