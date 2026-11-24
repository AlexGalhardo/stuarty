package owner

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

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

// sealStageTwo seals what an account made before ADR-0053 still holds in clear text, at its first unlock
// (task 11 of epic 92 fills it).
func (o *Owner) sealStageTwo(ctx context.Context, cfg Config) {
	_ = ctx
	_ = cfg
}
