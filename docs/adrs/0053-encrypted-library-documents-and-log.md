# ADR-0053: The library files, the documents, the edit log and the asset notes are sealed too

- Status: Accepted (2026-11-24)
- Deciders: author
- Amends: ADR-0052 consequences 2 and 3 (the library "needs a decision that this ADR does not take", and the documents of timelines and images are "a design of its own") and item 7 (the migration also runs at the first unlock of an account made by an earlier version); ADR-0040 items 1 and 2 (with an account the library files are no longer readable by "any agent with file access", and the index is in memory, not in SQLite)
- Why a new ADR: the epic needs a decision ADR-0052 explicitly left open and that reverses two statements of ADR-0040, so ADR-0052 is not edited to reverse itself; it only gets its consequences closed here. No other ADR is contradicted. Without an account, ADR-0040 stands as written.
- Links: author decisions D-120, D-127, D-135 item 6, D-139 item 5; ADR-0040, ADR-0052; SDD-005 section 5.17, SDD-007 section 3.9, SDD-018; epic 92; product issue VV-377

## Context

Epic 57 sealed the fields that hold the words a creator typed, and left three places in clear text on purpose, each with a reason that was a design to make: the files of the library (memory notes, prompts, skills) and their search index, the documents of timelines and images with the edit log that repeats them, and the descriptions and rights notes of the asset library. Product issue VV-377 is the consequence: with the account created, the captions of a video and the text layers of a timeline or an image are readable in `db.sqlite`. Epic 57 promised that a locked studio shows nothing the creator wrote; a copied disk, a backup or a synchronised folder still showed the words of every video and of every note.

## Decision

1. **The files of `library/` are sealed.** With an account every file the product writes is a binary envelope: a magic, a version byte, a random 12-byte nonce, then the AES-256-GCM ciphertext and tag under the data key. The associated data are `stuarty/file/v1`, a zero byte and the path of the file relative to `library/`, so a sealed file renamed or swapped does not open. The envelope is written once, in `service/account`, which is where the key is. The folder structure and the file names stay readable; a name is a label, as every `name` is.
2. **A clear file put in `library/` is picked up and sealed; a sealed file cannot be edited from outside.** While unlocked, the scan validates a clear file, writes it back sealed by an atomic rename, reads it back and compares, and only then forgets the clear one. A sealed file that does not open (edited, truncated, moved) is indexed as `invalid` with its error and shown in the list. A person who wants to edit a skill with their own tools exports it (a plain file, by an explicit action) and drops the clear file back. This changes the reading of RF-391: "every change made outside the product is picked up" holds for a clear file, not for a sealed one. While locked the scan does not run and a clear file is left alone.
3. **The index of the library is in memory.** `library_entries` and `library_fts` are removed from `db.sqlite`. The index (entries and searchable text) is built by a scan when the studio is unlocked, and at start-up when there is no account, and is dropped when the studio locks, so no index is on the disk and none outlives the key. The search behaviour of SDD-018 section 10 is unchanged.
4. **The documents and the log are sealed inside the repositories of the store.** `timelines.document`, `image_documents.document`, `templates.document`, `brand_kits.document`, `operations` (`ops`, `inverse`, `diff`, `skipped`), `image_operations` (the same and `result`), `document_backups.document` and `idempotency_keys.result` are sealed fields (SDD-005 section 5.17.1); the row key binds a log entry to its project and revision. Everything above `service.Store` sees clear JSON, so the engine is untouched. `doc_sha256` is computed over the clear canonical document and stays readable: it is what integrity checks compare, and it is a residual risk (SDD-007 section 14). No query reads a sealed column.
5. **The asset description and the rights note are sealed, and the description leaves the asset search** (D-139 item 5), as the generation prompt already did. The search of the asset library keeps matching names, tags and the kind of origin.
6. **The start-up migration of documents moves to the unlock** when the studio starts locked: a document keeps its older schema version until then, and every operation is `E_LOCKED` in between.
7. **Caches die with the key.** On lock the service drops every cache that holds content.
8. **Every workspace that exists still opens.** An account made by an earlier version records a seal version; at its first unlock the store seals the new columns in resumable batches, the scan seals the library files, and the version moves to 1 only after everything was read back and compared. There is no downgrade: an older binary refuses a workspace written by a newer version, and the release notes say that the sealing is one way.

## Alternatives

- **Sealing the documents with a SQL function or an extension.** Rejected: nothing decrypts in SQL (ADR-0052 item 3).
- **A sealed index of the library on the disk.** Rejected: it would need a second key discipline and the index is cheap to rebuild within the limits of SDD-018 section 12.
- **Keeping the library as clear files and sealing only the index.** Rejected: the notes themselves are what a creator wrote.
- **A memory index for asset descriptions built at unlock.** Not taken: a larger change, and the same loss was accepted for the prompt of a generated file.

## Consequences

Positive: a copy of the workspace or of `db.sqlite` shows ciphertext for everything a creator wrote; ids, timestamps, states, sizes, names, titles, digests and media files stay readable, as the documents already say.

Negative: the asset search no longer matches a description. A sealed library file cannot be edited outside the product. The unlock of a large library costs a scan, bounded by the limits of SDD-018 section 12. The sealing is one way. The digests of documents and the names of files and entries stay readable. The old clear blocks of the disk are not securely erased (no filesystem gives that), and the memory of a Go process cannot be wiped with certainty.
