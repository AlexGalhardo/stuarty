# Plan 009: Named checkpoints and a diff between revisions

This plan answers how an agent marks a point it may want to return to ("before-captions"), lists those marks, and reads what changed between two revisions before deciding to revert. When it is delivered, checkpoints exist in the service, HTTP, MCP, CLI and the UI history panel, and `revert` (plan 007) accepts a checkpoint name.

- Owner: orchestrator (step 1), senior-c (steps 2-4), senior-d (step 5), senior-e (step 6); one branch, owners in sequence
- Effort: M
- Depends on: plan 007
- Origin: author decision D-044
- Status: TODO
- Written against: `8787714a`

## 1. Why

A revision number is enough for a machine but brittle in a long session: an agent that tries three caption styles wants to say "back to before-captions", and a person reviewing an agent's work wants to see the changes as one list. Both come from the log that already exists (every batch stores its `diff`, SDD-005).

## 2. Current state (drift check)

```text
grep -n "Revert" docs/sdds/000-shared-definitions.md      # present (plan 007 delivered)
ls web/database/migrations | tail -3                       # the newest migration, to number the next one
grep -n "func (s \*svc) HistoryList" service/history.go
```

If plan 007 is not delivered, stop.

## 3. Design (to be written into the SDDs by step 1)

- **Checkpoint:** a row `(project_id, name, revision, actor, created_at)`, unique per `(project_id, name)`; `name` is kebab-case ASCII, at most 40 characters. Creating one with an existing name moves it (the reply says `moved: true`). Checkpoints are not batches: they do not change the revision and are not undoable; deleting a project to the trash keeps them.
- **Diff between revisions:** `HistoryDiff(project, from, to)` returns the net change per object between two revisions (added, removed, changed fields with the value at `from` and at `to`), computed by the service from the documents at both revisions, never in the UI; capped at 500 objects with `truncated: true`.
- **Revert by name:** `to_checkpoint` is accepted wherever `to_revision` is.
- **Names:** service `CheckpointSet`, `CheckpointList`, `HistoryDiff`; MCP `history` actions `checkpoint`, `checkpoints`, `diff` (no new tool); CLI `checkpoint`, `checkpoints`, `history:diff`; HTTP `PUT /projects/{id}/checkpoints/{name}`, `GET /projects/{id}/checkpoints`, `GET /projects/{id}/history/diff?from=&to=`.

## 4. Steps

### Step 1: Specify checkpoints and the diff

- Owner: orchestrator. Files: SDD-000 (names, the table, the error codes reused), SDD-005 (table and §12.5), SDD-003 (surfaces and result shapes).
- Commit: `docs(sdd): specify checkpoints and the history diff`.
- Acceptance:
  - [ ] `task lint` clean.

### Step 2: Add the `checkpoints` table

- Owner: senior-c. Files: a new migration in `web/database/migrations/` following the naming and test pattern of `20261114100100_create_operations_table.go`, and `migrations_test.go`.
- Commit: `feat(store): add the checkpoints table`.
- Acceptance:
  - [ ] `go test ./web/database/...` passes; the migration is reversible as the other ones are.

### Step 3: Service operations

- Owner: senior-c. Files: `service/` (checkpoints and diff, with tests), `revert` accepting `to_checkpoint`.
- Tests: set, move, list; diff of add, remove and field change; cap at 500; revert by name equals revert by revision.
- Commit: `feat(service): checkpoints and the diff between revisions`.
- Acceptance:
  - [ ] `go test ./service/... -run 'TestCheckpoint|TestHistoryDiff|TestRevert'` passes; `task test:race` passes for `service/`.

### Step 4: HTTP endpoints

- Owner: senior-c. Files: `web/app/http/endpoints.go`, handlers, OpenAPI fixtures, `web/contract/` parity.
- Commit: `feat(http): add checkpoint and history diff endpoints`.
- Acceptance:
  - [ ] `go test ./web/app/http/... ./web/contract/...` passes.

### Step 5: MCP and CLI

- Owner: senior-d. Files: `web/app/mcp/definitions/history.json`, handler, `web/app/cli/commands.go`, tests, regenerated agent docs, recipe 6 updated to create a checkpoint first.
- Commit: `feat(mcp): checkpoints and history diff for agents`.
- Acceptance:
  - [ ] `go test ./web/app/mcp/... ./web/app/cli/...` passes with the NFR-020 budget tests; `go run ./web/cmd/gen-agent-docs --check` exits 0.

### Step 6: Checkpoints in the history panel (the delivery of this plan)

- Owner: senior-e. Files: `web/resources/js/editor/history/HistoryPanel.tsx`, tests, both locale files.
- Behaviour: checkpoints appear as labelled markers between history entries; "Compare with now" shows the service diff; "Revert to here" works on a checkpoint.
- Commit: `feat(ui): show checkpoints and compare revisions in the history panel`.
- Acceptance:
  - [ ] `task test:web`, `bun run check:thin-client` and `bun run check:i18n` pass.
  - [ ] through MCP: checkpoint, three edits, `diff` lists exactly those changes, revert by name restores the document (paste the calls).

## 5. Out of scope

- Branching or forking a project from a checkpoint.

## 6. STOP conditions

- Computing the diff from two decoded documents is too slow for a 5-minute project (more than 200 ms p95 on the reference machine): stop and report the number before optimising.

## 7. Maintenance

The diff reads documents at a revision; a future snapshot or compaction of the log must keep that possible.
