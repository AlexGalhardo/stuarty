package owner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/library"
)

// sealBackupPrefix names what backups/ holds while the sealing of an earlier account is not finished.
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
// Service is closed. It is resumable without a copy of the database: every batch is one transaction read back before
// it commits, so a kill or a failure leaves each value either clear (read as it is until version 1) or sealed, and
// the next unlock goes on. A copy would be a whole clear duplicate of what the person chose to encrypt, and nothing
// would put it back. An empty mark says that the old pages are still to be removed, so a kill after the version
// moved is finished at the next unlock.
func (o *Owner) sealStageTwo(ctx context.Context, cfg Config) {
	version, err := o.Store.SealVersion(ctx)
	if err != nil {
		cfg.Log.ErrorContext(ctx, "the seal version of the account cannot be read", "error", err)

		return
	}

	// The mark, and any copy an earlier build of this migration left in clear.
	pending, _ := filepath.Glob(filepath.Join(o.Dir.Path("backups"), sealBackupPrefix+"*"))

	if version >= account.SealedAll {
		// A kill after the version moved left the old pages: finish what the migration would have.
		if len(pending) > 0 {
			o.finishAccount(ctx, pending...)
		}

		return
	}

	mark := filepath.Join(o.Dir.Path("backups"), sealMark)
	if err := writeSealMark(mark); err != nil {
		cfg.Log.ErrorContext(ctx, "the mark of the sealing could not be written; nothing was sealed", "error", err)

		return
	}

	// The files of the library first: the version moves only when files and rows are all sealed.
	lib := library.New(o.Dir.Path("library"), library.NewMemIndex(), o.now, library.WithFiles(o.Account.Files()))
	if _, err := lib.SealAll(ctx); err != nil {
		cfg.Log.ErrorContext(ctx, "the files of the library could not all be sealed; the next unlock tries again",
			"error", err)

		return
	}

	if err := o.Store.SealStageTwo(ctx, o.Account.Sealer(), o.accountFault); err != nil {
		cfg.Log.ErrorContext(ctx, "the documents could not all be sealed; the next unlock continues", "error", err)

		return
	}

	// Verified: the old pages, the copies of earlier schema migrations and the mark go (SDD-005 section 5.17.3 step 4).
	o.finishAccount(ctx, append(pending, mark)...)
}

// sealMark is the empty file of backups/ that says the sealing of an earlier account has not removed its old pages.
const sealMark = sealBackupPrefix + "unfinished"

// writeSealMark writes the empty mark of the sealing. It holds nothing of the person.
func writeSealMark(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	return os.WriteFile(path, nil, 0o600)
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
