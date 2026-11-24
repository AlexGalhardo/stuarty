# 92: The encrypted library and the rest of what epic 57 left in clear text

## Goal

When Stuarty is locked, nothing a creator wrote is readable from the disk: not the memory notes, prompts and skills of the library, not the words inside a timeline or an image, not the edit log that repeats them, and not the descriptions and rights notes of the asset library. A copy of the workspace, a backup or a stolen disk shows ciphertext only (media files, names and ids excepted, as the documents already say), and every workspace that exists today still opens.

## Context and related decisions

- Author decisions D-120 (epic 57: a local account and an encrypted workspace), D-127 and D-135 (item 6): encrypted library files with an index in memory that is rebuilt when the studio unlocks, in an epic of its own; the same epic closes what is still in clear text after epic 57. Product issue VV-377: with the account created, the captions of a video and the texts of a timeline and of an image stay in clear text in `db.sqlite` (`timelines.document`, `operations.ops` and `diff`, `image_documents.document`, `image_operations`, `document_backups`, `idempotency_keys.result`).
- ADR-0052 and its consequences name both gaps as open: the library "needs a decision that this ADR does not take", and the documents of timelines and images are "a design of its own" because the engine reads them, their digests are compared and the start-up migrates them with no key. This epic is that decision and that design; it writes a new ADR (the next free number) that amends the two consequences of ADR-0052, and does not edit ADR-0052 to reverse it.
- What is clear text today, from SDD-005 section 5.17.2 ("what is not sealed"), SDD-007 section 3.9 item 8 and the user page `site/content/docs/en/getting-started/account.mdx`:
  - the files of `library/` (`memory/*.md`, `prompts/*.md`, `skills/<name>/...`, SDD-018 section 2) and their index `library_entries` and `library_fts` (name, description and tags in clear; the body is already written empty when an account exists);
  - the documents `timelines.document`, `image_documents.document` (an image template is a row of this table), `templates.document` and `brand_kits.document`;
  - the edit log that repeats them: `operations` (`ops`, `inverse`, `diff`, `skipped`), `image_operations` (the same columns and `result`), `document_backups.document` and `idempotency_keys.result`;
  - `asset_info.description` and `asset_info.rights_note`, and the full-text table `asset_search`, which holds the description.
- What stays readable by design and is not touched here: ids, timestamps, states, sizes, `doc_sha256`, the names and titles of projects, items, templates and connections, the file names and tags of assets, media files, the username and the e-mail (SDD-005 section 5.17.2).
- What is reused: the sealed field (`s1.` plus base64url of `nonce || ciphertext || tag`, AES-256-GCM under the data key, associated data `stuarty/field/v1`, table.column and the row key; SDD-005 section 5.17.1), `account.Sealer` and `Manager.OnLock`/`BeforeLock` (`service/account/manager.go`), `web/app/store/seal.go`, the migration of an existing workspace with a verified copy (SDD-005 section 5.17.3), the guard triggers (`installSealGuards`, `TestSealGuardsComeBackAtTheNextOpen`), the `library.Index` interface and the in-memory index that exists in `service/library/memindex_test.go`, the crash points of `service/crashpoint`.
- `AGENTS.md` section 3: every rule lives in `core/` (the format rules of a sealed file are written once in `service/account`, which is where the key is; nothing in a handler), no credential leaves the vault, no telemetry, no new dependency (AES-GCM is the standard library, as it is for the fields).
- PRD-004 RF-391 ("every change made outside the product is picked up") is kept in the form the design allows: a clear file put in `library/` is picked up and sealed; a sealed file cannot be edited from outside (see Technical design). SDD-018, SDD-014 (the vault is unchanged), SDD-007 (the threat model), SDD-005, SDD-008.

## User stories

- As a creator, I want my notes, prompts and skills to be unreadable on the disk while Stuarty is locked, as my texts already are.
- As a creator, I want the captions and text layers of my videos and images, and what I wrote in an asset description or rights note, to be covered by the same lock.
- As a creator with a workspace made before this version, I want it to open as before and to be encrypted without my losing anything.
- As an agent, I want the library, search and edit tools to work exactly as before while the studio is unlocked.

## Scope

- Modules: `service/account` (the file envelope, the unlock hook), `service/library` (files, the in-memory index, the scan), `web/app/store` (sealed columns of documents, logs, backups, idempotency results, asset notes; the removal of the library repository; the guards), `web/database/migrations/` (drop the library index tables, mark the seal version of the account), `service/` (the start-up document migration moved to the unlock, the asset library, the caches wiped at lock, `Sweep`), `service/workspace` and the account creation (`web/app/store/account_create.go`) for the migration, the crash points and `internal/tools/crashharness`, the Settings text and `web/resources/lang/*.json` for the sentence that lists what is not encrypted, `site/content/docs/en/getting-started/account.mdx`, `docs/guides/pt-BR` where they repeat it, SDD-005, SDD-007 (threat rows and residual risks), SDD-018, SDD-000 section 9 and the table list, SDD-008, a new ADR, PRD-004 (a note on RF-391).
- Must not touch: the vault format (SDD-014 section 3), the key derivation and the recovery key, the sign-in routes, media files, the Rust renderer, the engine that reads a document (it receives the clear document from the store as today).

## Out of scope

- Encrypting media files, the names and titles that lists show, ids and digests, or the `doc_sha256` columns (a digest of the canonical document stays readable: it is what integrity checks compare, and it is a residual risk written in SDD-007 section 14).
- Network mode on a server: it holds no key and has no account (ADR-0052 item 6); its files and rows stay as they are, and the in-memory library index applies to it too.
- A secure erase of the old clear blocks on the disk (no filesystem gives one; written as a residual risk).
- Several accounts, a hardware key, per-project keys.
- Changing the search of the asset library beyond what sealing a description forces (see Technical design).

## Dependencies

57 (done), 56 (done), 08 and 15 (the asset library and the library, done). Not in progress together with an epic that changes `web/app/store/`, `service/library/`, `service/account/`, `web/database/migrations/` or the Settings account section: today epics 78 (the project kind), 84 (account screens) and 76 and 77 touch some of them, so the orchestrator orders the merges (`.agents/rules/plans.md` section 4: `web/resources/lang/*.json` and the migration timestamps are shared files).

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | The sentence in Settings that lists what is not encrypted gets shorter; nothing else is visible |
| HTTP, CLI, MCP | No new route, command or tool. While locked, the library and document operations answer `E_LOCKED` as they do today; unlocked they behave as before. `LibraryReindex` rebuilds the memory index |
| Disk | `library/` files are sealed; `library_entries` and `library_fts` are gone from `db.sqlite` |

## Core commands and permissions

No command or scope changes. The new rules are format and migration rules of the store, not of a domain. Only `core/` holds a domain rule: none is added.

## Technical design

- **The key stays in `service/account`.** A new `account.Sealer` method pair seals and opens a file: a binary envelope (a magic, a version byte, a 12-byte random nonce, the ciphertext and tag) with the associated data `stuarty/file/v1`, a zero byte and the path relative to `library/`. A sealed file copied to another name does not open. The envelope is written once, in one function, and tested with the vectors of the field format (a wrong key, a wrong path, a cut file, a flipped byte).
- **Library files.** Every write by the product (`writeFile`, `writeDirAtomic`: a temporary name, then a rename) seals while an account exists. A read opens a sealed file and also reads a clear one. The scan treats a clear file as new content: while unlocked, it validates it, writes it back sealed by the same atomic rename, reads it back and compares, and only then forgets the clear one, so RF-391 stays true for a clear file dropped into the folder (an imported skill, a note an agent wrote with its own tools). A sealed file that does not open (edited, truncated, moved) is indexed as `invalid` with its error and shown in the list, never skipped silently (SDD-018 section 9). While locked, the scan does not run and a clear file is left alone. `.built-in.json` holds versions and content hashes of products that ship with the binary and is treated like the other files. The folder structure and the file names stay readable (a name is a label, as every `name` is).
- **The index in memory.** The memory index of the tests becomes the production index: it holds the entries and the searchable text of every file, is built by a scan at **unlock** (a new `Manager.OnUnlock` hook, which runs after the keys are in place and outside the lock of the manager) and at start-up when there is no account, and is dropped at lock (`OnLock`), so no index is on the disk and none outlives the key. `library_entries` and `library_fts` are removed by a migration (its down step recreates them empty), and the account creation no longer has to empty and rebuild the FTS body. The search behaviour of SDD-018 section 10 is unchanged: every term a prefix of a word of the name, description, tags or body, bm25-like order, the same budget and snippets; the existing library search tests run unchanged against the new index. The cost is bounded by the limits of SDD-018 section 12 (2,000 notes, 500 skills, 500 prompts) and measured at the maximum library.
- **The documents and the log.** The columns of the table below are sealed fields written and read inside the repositories of the store, so everything above `service.Store` sees clear JSON as today and the engine is untouched. The row key binds a value to its row and, for the log, to its revision: a row of `operations` copied to another revision or project does not open.

| Table | Columns | Row key |
| --- | --- | --- |
| `timelines` | `document` | `id` |
| `image_documents` | `document` | `id` |
| `templates` | `document` | `id` |
| `brand_kits` | `document` | `owner_user_id` |
| `operations` | `ops`, `inverse`, `diff`, `skipped` | `project_id/revision` |
| `image_operations` | `ops`, `inverse`, `diff`, `skipped`, `result` | `image_id/revision` |
| `document_backups` | `document` | `id` |
| `idempotency_keys` | `result` | `project_id/actor/key` |
| `asset_info` | `description`, `rights_note` | `owner_user_id/asset_hash` |

- **No SQL reads these columns.** The task that adds each column proves it: a test greps the store for a `WHERE`, `ORDER BY`, `json_extract` or index that names one (the only `json_extract` of the store reads `jobs.input`, section 5.17.2), and a finding is a STOP. `doc_sha256` is computed over the clear canonical document and stored in clear, as today.
- **The asset library.** A description and a rights note are sealed; the search of the asset library keeps matching names, tags and the kind of origin, and no longer matches a description, as it no longer matches a prompt (SDD-005 section 5.17.2). `asset_search` stops holding the description. This is a visible change of behaviour and a question for the author (see Risks); the cheap alternative, a memory index for descriptions built at unlock, is not part of this epic.
- **The documents and the unlock.** `MigrateStoredDocuments` (start-up step 5, `service/migrate_docs.go`) reads documents with no key today. With an account it runs at the unlock when the studio is locked at start-up (the same function, called from the unlock hook), and a document stays at its older schema version until then; every operation is `E_LOCKED` in between, so nothing reads an old document. `workspace:backup` copies sealed rows and files as they are (SDD-007 section 3.9 item 6), so a backup of a locked workspace now holds no library file in clear text either.
- **Caches die with the key.** On lock the service drops every cache that holds content: the pictures of `ImageCache` in `service.ImageFrame`, the prepared layers and, once epic 91 adds it, the frame cache; any cache of a decoded document. A test locks the studio and asserts that each is empty.
- **The migration, for every workspace that exists.** Three states must open:
  1. *No account yet.* Nothing changes until the account is created; creation seals the new columns in the same transaction as the rest (step 3 of SDD-005 section 5.17.3), seals the library files after it (step 4), and a failure restores the copy.
  2. *An account made by an earlier version* (every workspace of release 0.50.0 and before). The row `accounts` gets a seal version (a migration, default 0). At the first unlock with version 0, and after the person is signed in, the store seals the new columns in batches inside transactions, the library files are sealed by the scan, and the version becomes 1 only when every row and file was read back and compared. Until then the new columns are read as clear text without error (the rule "a value with no `s1.` is an error" holds only for a column the migration has finished), and the guard triggers of the new columns are created when the version becomes 1. The work is resumable: a kill at any point (a crash point after each batch) leaves a workspace that opens and continues; the copy of `db.sqlite` the migration makes first (`backups/seal-<time>/`) is deleted only after the verification, then `VACUUM` and `wal_checkpoint(TRUNCATE)` remove the old pages, and the copies `backups/db-*.sqlite` of earlier schema migrations are removed as in step 4 of section 5.17.3.
  3. *An account made by this version.* Sealed from the first row.
  A failure restores the copy and reports; no state loses a document, an operation or a file, and a downgrade is not offered: an older binary cannot read a sealed document, but it never gets that far: the migration that adds the seal version is unknown to it, and it refuses to start on a workspace written by a newer version (SDD-005 section 7 step 2), and the release notes say that the sealing is one way.
- **Threat rows.** SDD-007 gets one row per case, each with a `TestSecurity_` test and listed by `go run ./internal/tools/threatcheck` (numbers proposed, the next free ones at the time): a copy of the workspace holds no library file in clear; a sealed file renamed or swapped does not open; `db.sqlite`, its WAL and the whole workspace hold no canary of a caption, a text layer, an operation, a description or a rights note; a row copied to another revision or project does not open; no index and no cache survives the lock; a migration killed in the middle leaves nothing in clear that the finished one would have sealed and no loss. Section 14 gets the residual risks: the digests, the names, the old blocks on the disk, a process memory that cannot be wiped with certainty.
- **Where the work splits.** If the epic proves too large for one senior, it splits cleanly after task 6: tasks 1 to 6 are the library half (files and index), tasks 7 to 14 are the documents half. The orchestrator decides; the order below keeps each half mergeable alone.

## Tasks

- [x] 1. The new ADR (amends the two consequences of ADR-0052), then SDD-005 sections 5.17.2 and 5.17.3 (the new sealed columns, the three migration states, the unlock hook), SDD-007 sections 3.9 and 14, SDD-018 sections 2, 9 and 12, SDD-000 (the removed tables), and a note on RF-391 in PRD-004. Verify: `task lint:md` exits 0.
- [x] 2. `Manager.OnUnlock`, the file envelope in `service/account`, and their tests (vectors, a wrong key, a wrong path, a cut or flipped file, the hook runs after the keys are set and outside the lock, the hook does not run for a failed sign-in). Verify: `go test ./service/account/...` passes and `go test -race ./service/account/...` passes.
- [x] 3. Sealed library files: write sealed, read both forms, the scan seals a clear file by atomic rename with read-back, an invalid sealed file is listed, the locked scan does nothing. Add the threat rows for the files and their tests. Verify: `go test ./service/library/...` and `go test -run 'TestSecurity_T13' ./service/library/...` (T-130 to T-139 are this epic's threat ids) pass.
- [ ] 4. The index in memory as the production index, built at unlock and at start without an account, dropped at lock; the migration that removes `library_entries` and `library_fts`; the repository and the account-creation steps for them removed. Verify: `go test ./service/library/... ./web/app/store/... ./web/database/migrations/...` pass with the library search tests unchanged, and a test over a library of the maximum size (2,000 notes, 500 skills, 500 prompts) asserts the unlock-to-ready time is under 2 s on the reference machine.
- [ ] 5. Seal the library of an existing workspace: at account creation and at the first unlock of an account made before. Resumable, verified, with a crash point and its kill test. Verify: `go test ./service/... -run 'LibrarySeal'` and `go test -tags crashtest -run 'TestCrashLibrarySeal' ./web/app/servicetest` pass; `go test ./internal/tools/crashharness` passes (`TestEveryCrashPointHasARecoveryTest`).
- [ ] 6. The library half in the surfaces and documents: Settings sentence, `account.mdx`, the Portuguese guides, the plaintext test of the library folder. Verify: `bun run check:i18n` and `task test:web` pass, `task lint:md` exits 0, and a test reads every file of a sealed workspace's `library/` and finds no canary.
- [ ] 7. The sealed columns of `timelines`, `image_documents`, `templates`, `brand_kits` with the proof that no SQL reads them, the seal-version column on `accounts`, the guard triggers for the new columns. Verify: `go test ./web/app/store/...` passes, including `TestSecurity_T108_SealedColumnsHoldNoPlaintextInTheDatabaseFile` extended with a canary in a caption, a text layer, a template text and a brand kit.
- [ ] 8. The sealed columns of the edit log, the backups and the idempotency results, with the row keys of the table above. Verify: `go test ./web/app/store/... ./web/app/servicetest/... -run 'Operation|Undo|Revert|Checkpoint|Idempot|Backup'` pass, and a test copies a row of `operations` to another revision and to another project and expects it not to open.
- [ ] 9. The asset library: `description` and `rights_note` sealed, `asset_search` without the description, the visible change recorded. Verify: `go test ./web/app/store/... ./service/... -run 'Asset'` and `go test ./web/database/migrations/...` pass.
- [ ] 10. The start-up document migration at unlock when locked, `Sweep` and `doctor` aware of the seal version, the caches dropped on lock. Verify: `go test ./service/... -run 'Migrate|Lock|Cache'` and `go test ./web/app/servicetest/... -run 'Locked|Migrate'` pass, including a test that a document of an older schema version migrates at unlock and not before.
- [ ] 11. The migration of an existing workspace, all three states: sealing in batches at the first unlock with verification, the copy, the cleanup, the restore on failure, resumed after a kill. Verify: `go test ./web/app/store/... -run 'AccountCreate|SealDocuments'` and `go test -tags crashtest -run 'TestCrashSealDocuments' ./web/app/servicetest` pass, with a workspace fixture made by release 0.50.0 (clear columns, an account row of seal version 0) that opens, loses nothing and ends sealed.
- [ ] 12. Performance: sealing must not break NFR-001. Verify: `go test -run '^$' -bench 'BenchmarkCommit200Items$|BenchmarkRoundTrip200Items$' -benchtime 20x ./web/app/servicetest/` reports the store share under 25 ms and the round trip under 50 ms (the numbers go in the commit message); the idle measurement `go run ./internal/tools/perfreport -write -absolute -gated -only NFR-001` is run by the orchestrator.
- [ ] 13. The documents half in the surfaces and the user documentation, and the whole-workspace plaintext scan. Verify: `go test -run 'TestSecurity_' ./...` and `go run ./internal/tools/threatcheck` exit 0, a test scans every byte of the workspace (the database, the WAL, `library/`, `backups/`, `exports/`) for the canaries and finds none, and `task lint:md` exits 0.
- [ ] 14. The full local gate for what changed. Verify: `task lint`, `task test`, `task test:web` and `bash .agents/skills/verify/scripts/verify.sh --quick --base release/v0.50.0` pass.

## Test strategy

Vectors for the file envelope and the field format; table tests for every column (write, read, wrong row, wrong column, locked, no account); the existing store, service, library and servicetest suites unchanged (the point of sealing inside the repositories is that no caller changes); a canary scan of the whole workspace, not only of `db.sqlite`; kill tests of both migrations at every batch; a fixture workspace made by the previous release; the NFR-001 benchmarks before and after.

## Acceptance criteria

1. **Given** a workspace with an account and a canary in a caption, a text layer, a template, an edit, a description, a rights note, a memory note, a prompt and a skill, **when** every byte of the workspace is scanned (`db.sqlite`, its WAL and shm, `library/`, `backups/`, `exports/`), **then** no canary is found (`go test -run 'TestSecurity_' ./...`).
2. While locked, no operation returns or changes any of them (`E_LOCKED`, the walk over every operation of epic 57 passes unchanged), and after the lock no index and no cache holds content (a test).
3. Unlocked, the library, the search, the editors and the edit log behave as before: every existing library, store, service and servicetest test passes unchanged, and `go run ./internal/tools/surfacebudget` passes.
4. A workspace made by release 0.50.0 with an account and clear documents and files opens, is sealed at the first unlock, and ends with every document, operation, file and note equal to what it was (a fixture test that compares them); the same workspace killed in the middle of the sealing opens again and finishes; a failure restores the copy.
5. A sealed file that is renamed, swapped with another or cut does not open and is listed as invalid; a row of the log copied to another revision does not open.
6. NFR-001 holds: `BenchmarkCommit200Items` store share under 25 ms and the round trip under 50 ms; the library of the maximum size is ready within 2 s of the unlock.
7. SDD-007 has the threat rows with passing `TestSecurity_` tests and `go run ./internal/tools/threatcheck` exits 0; the new ADR, SDD-005, SDD-018 and the user page say what is and is not encrypted, in the same words as the Settings page.
8. `task lint`, `task test`, `task test:web` pass; no new dependency.

## Risks and mitigation

- A migration that loses or leaves in clear: verification by read-back, the copy kept until it passes, a crash point per batch and a kill test, the fixture of the previous release, and a resumable state in `accounts`.
- An older binary opening a sealed store: it refuses to start on the unknown migration (SDD-005 section 7 step 2); there is no downgrade and the release notes say so.
- Sealing a description removes its search: a visible behaviour change. **Question for the author**, with a recommendation: accept it (the same as the prompt of a generated file); the alternative is a memory index for descriptions at unlock, a larger epic.
- A cost on every commit: AES-GCM over the document twice per commit; measured in task 12 against NFR-001.
- RF-391: a sealed file cannot be edited from outside. A person edits in the product, or exports a skill (plain, by an explicit action) and drops the clear file back; the specification says so.
- A query that needs a sealed column: **STOP**, report it, never decrypt in SQL.
- **STOP** if sealing a document breaks a digest comparison, the start-up migration or a test of the engine in a way that cannot be fixed inside the store; if a table in the list turns out to be read by an index or a SQL expression; if the library index cannot reach the existing search behaviour in memory; or if anything needs a new dependency.
- **STOP** and ask if the migration of an existing workspace cannot be made resumable without a format that contradicts ADR-0052.

## Size

XL: two halves (the library, and the documents with their log and migration), a migration for every existing workspace, new threat rows and the user documentation. It splits after task 6 if the orchestrator prefers two epics.

## Priority

Must. Epic 57 promised that a locked studio shows nothing the creator wrote, and the file system, a backup and a stolen disk still show the words of every video and image.

## Hand-off for review

The commits (the library half and the documents half are separate series); the new ADR and the specification diffs; each acceptance criterion with its command and result; the fixture of the previous release and what it proved; the benchmark numbers before and after; the threat rows and their tests; what was not verified (a workspace of a real user, the idle measurement of NFR-001, the desktop shell unlocking a large library); the answer awaited from the author on the asset description search.
