# 36: Revert to a revision, checkpoints and a diff

## Goal

A creator lets an agent try an idea without fear: they name the current state, let the agent work, read what changed, and go back in one step if they do not like it.

## Context and related decisions

- ADR-0006 (operation log), ADR-0009 (revisions and conflicts), ADR-0017.
- Author decision D-073 (carried over from the earlier plans on revert, checkpoints and diff). PRD-002 RF-120, RF-121.
- SDD-002, SDD-003, SDD-005.

## Scope

- Modules: `core/command/` (revert as a command, the diff), `service/` (checkpoints, history operations), `service/textview/` (the diff as text), `web/app/` (controllers, CLI, MCP), `web/database/migrations/` (checkpoints), the history panel in `web/resources/js/editor/`, SDD-002, SDD-003, SDD-005.
- Timeline documents first. The same machinery serves image documents when epic 27 is merged, through the shared batch code; the image side is one task of whichever merges second.

## Out of scope

- Branches of a document, merging two histories.
- Reverting part of a revision (one object only).
- Deleting history.

## Dependencies

03.

## Technical design

- **Revert never rewrites the log** (RF-120): `history.revert(revision)` computes the batch that turns the current document into the document at that revision and applies it as a new revision, with the actor who asked. It is therefore undoable, visible in the log, subject to conflict detection, and available as a dry run that returns the diff it would cause. Reverting to the current revision is a no-op with a clear result.
- **Checkpoints** (RF-121): a checkpoint is a name and a note attached to a revision, created by a person or an agent (`history.checkpoint_create`, `_list`, `_rename`, `_remove`). Removing a checkpoint never removes a revision. An automatic checkpoint is created before each agent session's first edit of the day, so there is always a point to return to.
- **Diff**: `Diff(revA, revB)` in the core returns structured changes by object (clip added, removed, moved, trimmed; property changes with before and after; tracks; captions), computed from the two documents, not from the log, so it is correct across undo and redo. `service/textview` renders it as text inside the token budget, most significant changes first, with a count of what was cut.
- **Retention**: revisions needed by a checkpoint are kept; the existing retention policy of the operation log is written down in SDD-005 with this rule.
- **UI**: the history panel lists revisions grouped by actor and time, shows checkpoints, shows the diff between two selected points in plain language, and offers "revert to here" with the dry-run diff as confirmation.

## Tasks

- [ ] 1. Amend SDD-002 (`history.revert`), SDD-003 (history operations and the diff text view) and SDD-005 (checkpoints, retention). Verify: `task lint:md` exits 0 and the tests that read those documents pass.
- [ ] 2. Implement the structured diff with table tests. Verify: `go test ./core/command/... -run TestDiff` passes.
- [ ] 3. Implement `history.revert` with golden cases and a property test (revert to R then export equals the export at R). Verify: `go test ./core/command/... -run TestRevert` passes.
- [ ] 4. Implement checkpoints, the automatic one and retention. Verify: `go test ./service/... -run TestCheckpoint` passes.
- [ ] 5. Render the diff as text within the budget. Verify: `go test ./service/textview/... -run TestDiffText` passes.
- [ ] 6. Expose on the three surfaces and regenerate documents. Verify: `go test ./web/contract/...` and `task generate:check` pass.
- [ ] 7. Build the history panel in both languages, keyboard operable. Verify: `task test:web` and `web/e2e/specs/history-revert.spec.ts` pass.

## Test strategy

- Unit: the diff for every kind of change.
- Property: revert yields a document equal to the target revision; revert then undo yields the document before the revert.
- Contract: parity; the text diff within the budget on a large change.
- End to end: checkpoint, edit, read the diff, revert, undo the revert.

## Acceptance criteria

1. `history.revert` to an earlier revision produces a document equal to that revision as a new, undoable revision.
2. A dry run of a revert returns the diff and changes nothing.
3. A named checkpoint survives further edits and can be reverted to by name.
4. The text diff of two revisions lists what changed in words an agent can act on, within the token budget.
5. Before an agent's first edit of the day an automatic checkpoint exists.
6. `go test ./core/command/... ./service/... ./web/contract/...`, `task test:web` and `task generate:check` pass.

## Risks and mitigation

- **A revert that conflicts with a concurrent edit.** Mitigation: it is an ordinary batch with a base revision; the conflict is reported like any other.
- **Unbounded growth of kept revisions.** Mitigation: retention written into SDD-005; a ceiling on automatic checkpoints.

## Size

M

## Hand-off for review

The diff of `epic/36-revert-checkpoints-and-diff`; examples of the text diff; the outputs of the acceptance commands.
