package owner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/library"
)

// sealBackupPrefix names the folder of backups/ that the sealing of an earlier account keeps while it works.
const sealBackupPrefix = "seal-"

// migrateDocuments is step 5 of the start-up order (SDD-005 section 7): the documents below the current schema.
func (o *Owner) migrateDocuments(ctx context.Context, cfg Config) error {
	migrated, err := service.MigrateStoredDocuments(
		ctx, o.Store, service.MigrateOptions{NowMs: cfg.Now().UnixMilli(), Migrate: o.migrateWith},
	)
	if err != nil {
		return err
	}

	if migrated != (service.MigrateResult{}) {
		cfg.Log.InfoContext(ctx, "documents migrated", "projects", migrated.Projects, "templates", migrated.Templates)
	}

	return nil
}

// sealStageTwo seals what an account made before ADR-0053 still holds in clear text, at its first unlock (SDD-005
// section 5.17.4): the files of the library, then the documents, the edit log, the backups, the idempotency results
// and the asset notes in batches, and last it removes what the sealing left in clear. It runs while the gate of the
// Service is closed. It is resumable: a copy of the database is made first and is removed only after every row and
// file was read back; a failure keeps the copy, leaves the workspace as it opens (the unsealed part is read as clear
// text) and the next unlock goes on.
func (o *Owner) sealStageTwo(ctx context.Context, cfg Config) {
	version, err := o.Store.SealVersion(ctx)
	if err != nil {
		cfg.Log.ErrorContext(ctx, "the seal version of the account cannot be read", "error", err)

		return
	}

	pending, _ := filepath.Glob(filepath.Join(o.Dir.Path("backups"), sealBackupPrefix+"*"))

	if version >= account.SealedAll {
		// A kill after the version moved left the copy and the old pages: finish what the migration would have.
		if len(pending) > 0 {
			o.finishAccount(ctx, pending...)
		}

		return
	}

	copyDir := filepath.Join(o.Dir.Path("backups"), sealBackupPrefix+o.now().UTC().Format("20060102T150405.000Z"))

	if err := o.copyForSealing(ctx, copyDir); err != nil {
		cfg.Log.ErrorContext(ctx, "the copy before the sealing could not be made; nothing was sealed", "error", err)

		return
	}

	// The files of the library first: the version moves only when files and rows are all sealed.
	lib := library.New(o.Dir.Path("library"), library.NewMemIndex(), o.now, library.WithFiles(o.Account.Files()))
	if _, err := lib.SealAll(ctx); err != nil {
		cfg.Log.ErrorContext(ctx, "the files of the library could not all be sealed; the copy is kept and the next "+
			"unlock tries again", "error", err)

		return
	}

	if err := o.Store.SealStageTwo(ctx, o.Account.Sealer(), o.accountFault); err != nil {
		cfg.Log.ErrorContext(ctx, "the documents could not all be sealed; the copy is kept and the next unlock "+
			"continues", "error", err)

		return
	}

	// Verified: the old pages, the copies of earlier schema migrations and this copy go (SDD-005 section 5.17.3 step 4).
	o.finishAccount(ctx, append(pending, copyDir)...)
}

// copyForSealing keeps a verified copy of the database in clear before the sealing begins.
func (o *Owner) copyForSealing(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	if _, err := o.Store.DB().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = os.RemoveAll(dir)

		return err
	}

	if _, err := copyFileAtomic(o.Dir.Path("db.sqlite"), filepath.Join(dir, "db.sqlite")); err != nil {
		_ = os.RemoveAll(dir)

		return err
	}

	return nil
}

// locked says whether the studio holds an account and nobody is signed in or may be: the documents are sealed and no
// key is in memory, so what reads a document waits for the unlock (ADR-0053 item 6).
func (o *Owner) locked() bool {
	return o.Account != nil && o.Account.State() != account.StateNone
}

// documentsAtStart is the sealing of an account made before ADR-0053 and step 5 of the start-up order. With an
// account both run at the unlock, in that order, while the gate of the Service is closed; every operation is
// E_LOCKED until then, so nothing reads an older document in between. With none, the migration runs now.
func (o *Owner) documentsAtStart(ctx context.Context, cfg Config) error {
	if o.Account != nil {
		o.Account.OnUnlock(func() { o.sealStageTwo(context.WithoutCancel(ctx), cfg) })
	}

	if !o.locked() {
		return o.migrateDocuments(ctx, cfg)
	}

	o.Account.OnUnlock(func() {
		if err := o.migrateDocuments(context.WithoutCancel(ctx), cfg); err != nil {
			cfg.Log.ErrorContext(ctx, "the documents could not be migrated at the unlock", "error", err)
		}
	})

	return nil
}

// seedAtStart puts the templates of the product in the library: now, or at the unlock when they are sealed.
func (o *Owner) seedAtStart(ctx context.Context, cfg Config, st service.Store) error {
	if !o.locked() {
		return o.seedTemplates(ctx, st)
	}

	o.Account.OnUnlock(func() {
		if err := o.seedTemplates(context.WithoutCancel(ctx), st); err != nil {
			cfg.Log.ErrorContext(ctx, "the templates of the product could not be seeded at the unlock", "error", err)
		}
	})

	return nil
}
