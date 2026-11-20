# 50: Revert to a revision, checkpoints and a diff for image documents

## Goal

A creator lets an agent rework a thumbnail: they name the version they have now, let the agent work, read what changed, and go back to the named version in one step, as they can already do with a video.

## Context and related decisions

- ADR-0006 (the operation log and undo), ADR-0009 (revisions and conflicts), ADR-0038 (image documents on the same core).
- Author decision D-094. PRD-002 RF-120 and RF-121 (revert and checkpoints). Epic 36 left the image side to "one task of whichever merges second".
- Epic 36 (the timeline side, done) and epic 29 (the look of a layer, which is part of the document that a revert restores).
- SDD-021 section 4 (the batch machinery: undo, conflicts, dry run), section 11 (the service operations), section 9 (the text view and the `note:` line).
- SDD-005 section 5.13 (`image_documents` and `image_operations`), section 5.14 (timeline checkpoints, whose rules this epic copies), section 11.3 (retention), section 12.4 (how a revision is rebuilt from the log).
- SDD-003 section 2.5.1 (the revert, diff and checkpoint operations of a timeline), section 3.7.1 (their texts), section 3.10 (the budgets), section 4.2 (the `history` tool), section 4.8 (the budget of the tool surface).
- PRD-002 NFR-020 (the bound on the tool surface an agent pays for).

## Scope

- Modules: `core/image/` (the rebuild of a document at a revision from its inverses; the structured diff of two documents, from the flat form that exists in `describe_diff.go` and `undo.go`), `service/image*.go`, `service/history_revert.go` and `service/checkpoint.go` (the image side, sharing their code with the timeline), `service/textview/` (the image diff text), `web/database/migrations/` (one migration for `image_checkpoints`), `web/app/mcp/history.go` and `web/app/mcp/image.go`, `web/app/cli/revert.go` and `web/app/cli/image.go`, `web/app/http/revert.go` and `web/app/http/image.go`, `web/resources/js/image-editor/history/` (the panel), `web/resources/lang/en.json` and `pt-BR.json`, SDD-021 (sections 4 and 11), SDD-005 (sections 5.15 and 11.3), SDD-003 (sections 2.5.1 and 3.7.1), SDD-000 (sections 1 and 7).
- The rules of epic 36 for a revert, a diff and checkpoints are the same rules for an image. The code is shared, not copied (SDD-021 section 1).

## Out of scope

- Branches of a document, merging two histories, deleting history.
- Reverting part of a document (one layer).
- A pixel diff of the rendered picture: the diff is of the document, its layers and their properties.
- The `ImageHistory` list on MCP: the list stays on HTTP and in the editor (SDD-000 section 1).

## Dependencies

36 (the revert, diff and checkpoint rules for timelines) and 29 (the look of a layer is part of the document).

Not in progress together with 48 and 49: all three change `core/image`, `service/image*.go` and SDD-021.

## Technical design

- **Revert** (`ImageRevert`, a name for SDD-000). The target is a revision, or the name of a checkpoint. The document at the target is rebuilt by applying, newest first, the `inverse` of each row after it (SDD-005 section 12.4, over `image_operations`). A row that is not of kind `edit`, `undo` or `redo` refuses the operation with `E_INVALID_STATE`; a walk of more than 5,000 rows is `E_LIMIT`. The rebuilt document is checked against the `doc_sha256` of the target row; a difference is `E_INTERNAL`, never a document to continue from.
- **Commit.** The revert is an `edit` row whose `ops` is `[{"op": "image.revert", "args": {...}}]`, with its own `inverse` and `diff` and `base_revision` the revision the caller read. So `ImageUndo` undoes it, and the guard protects what another actor changed since (SDD-021 section 4). An empty diff writes nothing; a dry run writes nothing and returns the diff.
- **Checkpoints.** A table `image_checkpoints`: `image_id` (a reference to `image_documents`, with cascade), `name`, `note`, `revision`, `actor`, `auto`, `at_ms`, and the primary key `(image_id, name)`. The rules of SDD-005 section 5.14 are copied: a name of 1 to 60 characters, a note of 300, and a checkpoint never removes a revision. The automatic checkpoint is `auto <actor> <YYYY-MM-DD>` at the revision the batch started from. It is made by the first agent edit of the day, in the transaction of `ImageEdit`, and at most 30 automatic ones are kept.
- **Retention.** SDD-005 section 11.3 applies unchanged: no row of `image_operations` is deleted.
- **Diff** (`ImageDiff`). Computed from two rebuilt documents, not from the log, so it is right across undo and redo. The text is in `service/textview/`, within the budget of an edit result of SDD-003 section 3.10 (40 lines, at most 4,000 characters, then a count of what was cut). Names and texts of the document are untrusted and go through the `note:` line of SDD-021 section 9.
- **Surfaces.** The `history` tool takes `domain` `image` for `revert`, `diff`, `checkpoints`, `checkpoint`, `rename_checkpoint` and `remove_checkpoint`, as it does for a timeline (SDD-003 section 4.2). The CLI and HTTP mirror them for `image_id`, with the commands of epic 36. The description of the tool grows, and the budget measures it.
- **The panel.** The history panel lists the checkpoints, shows the diff between two points, and offers "revert to here" with the dry-run diff as its confirmation, as the timeline's history panel of epic 36 does.

## Tasks

- [ ] 1. Amend SDD-021 (sections 4 and 11), SDD-005 (a section 5.15 for `image_checkpoints`; section 11.3 for images), SDD-003 (sections 2.5.1 and 3.7.1 for images) and SDD-000 (the operations of section 1, the table of section 7). Verify: `task lint:md` exits 0.
- [ ] 2. Rebuild a document at a revision, and the structured diff of two images, with the property test. Verify: `go test ./core/image/... -run "TestImageRevert|TestImageDiff"` passes.
- [ ] 3. The migration, the checkpoints, the automatic checkpoint, the ceiling, and the revert through the service. Verify: `go test ./service/... -run "TestImageCheckpoint|TestImageRevertThroughTheService"` passes, and `go test ./web/database/migrations/...` passes with the new migration.
- [ ] 4. The diff text within the budget. Verify: `go test ./service/textview/... -run TestImageDiffText` passes.
- [ ] 5. Expose on the three surfaces and regenerate the schemas. Verify: `go test ./web/contract/...` passes; `task generate:check` exits 0; `go run ./internal/tools/surfacebudget` exits 0.
- [ ] 6. The panel, in both languages. Verify: `bun run check:i18n` and `task test:web` pass, and `web/e2e/specs/image-revert.spec.ts` (new, `bun run e2e`) passes.

## Test strategy

- Unit: the rebuild for every kind of change (a layer added, moved, removed, its look changed, the canvas resized, an asset added and removed).
- Property: revert to a revision R gives a document with the canonical bytes of R, so its export is the export at R; revert then undo gives the document before the revert (SDD-005 section 12.4).
- Contract: parity of the operations on the three surfaces; the budget tests.
- End to end: checkpoint, an agent's edit, read the diff, revert, undo the revert.

## Acceptance criteria

1. `ImageRevert` to a revision R produces a document with the canonical bytes of R, as a new undoable revision: `go test ./core/image/... -run TestImageRevertProperty` passes.
2. A dry run returns the diff and writes nothing: `go test ./service/... -run TestImageRevertDryRun` passes.
3. A named checkpoint survives later edits and can be reverted to by its name: `go test ./service/... -run TestImageCheckpoint` passes.
4. An agent's first edit of the day creates the automatic checkpoint `auto <actor> <date>`, and a second edit that day does not create another: the same test passes.
5. The diff text names each change in words an agent can act on, within the budget: `go test ./service/textview/... -run TestImageDiffText` passes.
6. `go test ./core/image/... ./service/... ./web/contract/...` and `go test ./web/database/migrations/...` pass; `go run ./internal/tools/surfacebudget`, `task generate:check`, `bun run check:i18n`, `task test:web` and `web/e2e/specs/image-revert.spec.ts` pass.

## Risks and mitigation

- **The budget of the `history` tool.** Its description gains six actions. **STOP** if `surfacebudget` fails: only the author raises NFR-020 (PRD-002; SDD-003 section 4.8, author decision D-094). The hand-off then lists the words that could be saved, and the decision waits for the author.
- **The walk.** A revert to an old revision reads many rows: the `E_LIMIT` of SDD-005 section 12.4 applies. No cache of documents is added in this epic.
- **The migration timestamp.** The migration takes the timestamp of the moment it is merged (`docs/plans/00-roadmap.md` section 3).
- **Two copies of the same rules.** Names, ceilings and the automatic checkpoint are one code path for the timeline and the image. A second copy is rejected in review.

## Size

M

## Hand-off for review

The diff of `epic/50-image-revert-checkpoints-and-diff`; the property test output; the output of the surface budget; the result of the panel's end-to-end spec.
