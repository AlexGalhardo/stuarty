# ADR-0009: Persistence: SQLite, a document snapshot plus the operation log

- Status: Accepted (2026-11-14); what a project holds is amended by [ADR-0039](0039-project-as-production-container.md) (2026-11-18)
- Deciders: author (D-003), architect
- Links: author decision D-003, engine §8, post-mortems §8, vukem practices, ADR-0006, SDD-001, SDD-005

## Context

D-003 chooses local and self-host first: a single binary with SQLite and files on disk. vukem includes SQLite through a pure Go driver, an ORM and Go migrations. The stack draft listed PostgreSQL, pgx, sqlc and goose. Olive shows what happens to a project format without migrations: old projects stop opening.

## Decision

1. The database is SQLite, accessed through vukem's ORM and migrated with vukem's migrations. No PostgreSQL, pgx, sqlc or goose in the MVP. PostgreSQL becomes an option when a hosted service exists; persistence sits behind the interfaces of `service/`, so adding it does not touch `core/`.
2. A project is stored as: one row with the **current document** as JSON, its `revision` and its `schema_version`; and the **operation log**, one row per committed batch (ADR-0006).
3. The document has an integer `schema_version` at the top, read before anything else. Migrations are pure functions `vN -> vN+1` in `core/timeline`, chained, each with a fixture project kept in the tests forever.
4. Opening a document newer than the binary understands fails with an error that names both versions and says to upgrade. Unknown fields are preserved on save.
5. A commit is one database transaction: append the batch, write the new document and revision. A crash leaves the previous revision intact.
6. Before a schema migration rewrites a project, the previous document is kept as a backup row.
7. A project can be exported and imported as a **bundle**: a zip with the document, the log and the referenced media, for backup and for moving between machines.

## Alternatives

- **Project as a file on disk** (one JSON per project). Friendly to version control, but atomic multi-file commits, the log and concurrent readers are all hand-made. The bundle covers the portability need.
- **Event sourcing only, no snapshot.** Every read would replay the log.
- **PostgreSQL from the start.** Contradicts D-003; nothing in the MVP needs it.

## Consequences

Positive: one file to back up; no service to run; matches vukem's defaults.

Positive: the document is plain JSON an agent can read in full when it asks for it.

Negative: SQLite allows one writer; with one owner process per workspace (ADR-0008) that is the design, not a limit.

Negative: JSON documents in a text column are opaque to SQL queries; listing and searching use separate indexed columns kept in step by the commit.
