// Package migrations holds the schema migrations.
package migrations

import "github.com/AlexGalhardo/vukem/schema"

// All lists the migrations in order; vukem make:migration appends here. The
// vukem queue tables come from queue.Migrations and are joined by the caller
// (SDD-005 section 5.1). api_keys is created here and used by network mode (ALPHA).
func All() []schema.Migration {
	return []schema.Migration{
		m20261114100000CreateProjectsTable(),
		m20261114100100CreateOperationsTable(),
		m20261114100200CreateIdempotencyKeysTable(),
		m20261114100300CreateAssetsTables(),
		m20261114100400CreateJobsTable(),
		m20261114100500CreateTemplatesTable(),
		m20261114100600CreateDocumentBackupsTable(),
		m20261114100700CreateAuditTable(),
		m20261114100800CreateAPIKeysTable(),
		m20261114100900CreateAuditAppendOnlyTriggers(),
		m20261115100000AddTemplatesOwnerUserID(),
		m20261115110000CreateUsersTable(),
		m20261115110100CreateAccessTokensTable(),
		m20261115120000AddUsersDisabledMS(),
		m20261115130000ArtifactsSpeechBeatsKinds(),
		m20261115140000CreateBrandKitsAndFontOwners(),
		m20261115150000AssetsFrameDelaysAndFramesKind(),
		m20261116100000AssetsHasAlpha(),
		m20261116120000ArtifactsChunksKind(),
		m20261116130000AssetsLutKind(),
		m20261119100000CreateConnectionsTable(),
		m20261119110000CreateImageDocumentsAndOperations(),
		m20261119120000ProjectItems(),
		m20261119120050JobsExportBatchKind(),
		m20261119130000CreateLibraryIndex(),
		m20261119140000CreateApprovalTables(),
		m20261119140100JobsWaitingApproval(),
		m20261119150000AssetLibrary(),
		m20261119210000AssetRenderOrigin(),
		m20261119220000CreateCheckpoints(),
	}
}
