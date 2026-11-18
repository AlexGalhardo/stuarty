# Plan 007: Revert a project to a revision in one batch

This plan answers how an agent (or a person) that made a series of wrong edits gets the project back to an earlier revision in one call, without rewriting the log and without touching what other actors did. When it is delivered, `revert` exists in the service, the HTTP API, the MCP `history` tool, the CLI and the history panel of the UI, and it can itself be undone.

- Owner: orchestrator (step 1), senior-c (steps 2-3), senior-d (steps 4 and 6), senior-e (step 5); one branch, owners in sequence
- Effort: M-L
- Depends on: plan 001 (ADR-0030 cites this capability); independent of the studio plans
- Origin: author decision D-044; audit finding of 2026-11-18 (undo goes back one batch per call)
- Status: TODO
- Written against: `8787714a`

## 1. Why

Today `Undo` reverses the caller's latest batch, one per call (`service/history.go:170-268`, SDD-005 §12.2). An agent that made twelve wrong batches needs twelve calls, each a chance to fail halfway, and has no way to say "back to where I started". `dry_run` exists for edits (`service/edit.go:135`) but not for a multi-step recovery. ADR-0020 item 3 promises that everything an agent does is undoable; this makes it cheap.

## 2. Current state (drift check)

```text
grep -n "^func (s \*svc) \(Undo\|Redo\|step\|applyStep\)" service/history.go   # Undo, Redo, step, applyStep
grep -n "kind            TEXT" web/database/migrations/20261114100100_create_operations_table.go   # CHECK (kind IN ('edit','undo','redo','migrate','import'))
grep -n "projects.undo" web/app/http/endpoints.go                             # one entry
grep -n '"undo"' web/app/mcp/definitions/history.json                         # actions list, redo
ls web/resources/js/editor/history                                            # HistoryPanel.tsx, UndoRedo.tsx
```

## 3. Design (to be written into the SDDs by step 1)

- **Semantics:** `Revert(project, to_revision)` reverses, newest first, every batch of the **caller's actor** with revision greater than `to_revision` that is still in effect (kind `edit` or `redo`, `closed_by IS NULL`), using the stored inverses and the same guard as undo (SDD-005 §12.2 step 3), built from the batches of other actors since `to_revision`. Changes made by other actors are never reverted and are reported in `skipped`, as undo does.
- **Recorded as an ordinary edit batch.** The result commits as kind `edit` with one pseudo-operation `history.revert` `{to_revision}` in `ops`, its own `inverse` and `diff`. Consequences: no migration of the `operations` table, `undo` of a revert works with the existing algorithm (it is the actor's newest open batch), and the reverted batches stay open, which is correct (undoing the revert re-applies them; a later undo then reaches them in order).
- **Dry run:** `dry_run: true` returns the would-be diff and skipped list without committing, like edit.
- **Limits:** at most 500 batches per revert (`E_LIMIT_EXCEEDED` with the hint to revert in two steps); `to_revision` must be at most the current revision and at least the project's first revision (`E_INVALID_ARGUMENT` otherwise); a trashed project is `E_INVALID_STATE`.
- **Names:** service operation `Revert`, MCP `history` with `action: "revert"` and `to_revision` (no new tool: the tools/list token budget NFR-020 stays), CLI `revert --project <id> --to <rev> [--dry-run]`, HTTP `POST /projects/{id}/revert` with body `{to_revision, dry_run}`, scope `edit`.

## 4. Steps

### Step 1: Specify `Revert` in SDD-000, SDD-005 and SDD-003

- Owner: orchestrator. Files: `docs/sdds/000-shared-definitions.md` (the operation row next to `HistoryList`, `Undo`, `Redo`), `docs/sdds/005-data-model-and-persistence.md` (new §12.4 with the algorithm of section 3 and a worked example in the style of §12.1), `docs/sdds/003-agent-interfaces.md` (MCP action, CLI command, HTTP endpoint, result shape = §12.3 plus `reverted: [revision...]`).
- Commit: `docs(sdd): specify revert to a revision`.
- Acceptance:
  - [ ] `task lint` clean; the worked example's revisions are consistent (a reviewer replays it by hand).

### Step 2: Implement `Revert` in the service

- Owner: senior-c. Files: `service/history.go` (or a new `service/revert.go` beside it), the operations store query that lists the actor's open batches after a revision (next to `UndoCandidate`), `service/service.go` (interface), tests in `service/`.
- Reuse `applyStep`'s pieces: decode each inverse, reverse, concatenate newest-first, build one guard with `Operations().Since(project, to_revision, {ExcludeActor: actor})`, call `command.ApplyPrimitives` once, commit with `s.commit` as in `step`. No editing logic outside `core/`: if a new primitive behaviour is needed, stop.
- Tests (each fails before, passes after): revert of three own batches returns the document of `to_revision`; another actor's change in between is kept and reported in `skipped`; `dry_run` commits nothing; undo of the revert restores the pre-revert document; 501 batches give `E_LIMIT_EXCEEDED`; a trashed project gives `E_INVALID_STATE`; crash points as in `step`.
- Commit: `feat(service): revert a project to a revision in one batch`.
- Acceptance:
  - [ ] `go test ./service/... -run 'TestRevert'` passes.
  - [ ] `task test:race` passes for `service/`.

### Step 3: Expose it over HTTP

- Owner: senior-c. Files: `web/app/http/endpoints.go` (entry `projects.revert` beside `projects.undo`), handler and request type, the OpenAPI test fixtures, `web/contract/` parity entry.
- Commit: `feat(http): add the revert endpoint`.
- Acceptance:
  - [ ] `go test ./web/app/http/... ./web/contract/...` passes, including the OpenAPI and parity tests.

### Step 4: Expose it in MCP and the CLI

- Owner: senior-d. Files: `web/app/mcp/definitions/history.json` (action `revert`, field `to_revision`, description updated within the budget), the MCP handler, `web/app/cli/commands.go` and its test, then `go run ./web/cmd/gen-agent-docs`.
- Commit: `feat(mcp): revert from the history tool and the CLI`.
- Acceptance:
  - [ ] `go test ./web/app/mcp/... ./web/app/cli/...` passes, including the token budget tests (NFR-020).
  - [ ] `go run ./web/cmd/gen-agent-docs --check` exits 0.

### Step 5: "Revert to here" in the history panel

- Owner: senior-e. Files: `web/resources/js/editor/history/HistoryPanel.tsx` and its test, `web/resources/lang/en.json` and `pt-BR.json`.
- Behaviour: each entry of the caller's actor shows "Revert to here"; it first calls revert with `dry_run` and shows the count of changes and skipped items in a confirmation dialog, then commits. The UI computes nothing (thin client, SDD-012 §1).
- Commit: `feat(ui): revert to a revision from the history panel`.
- Acceptance:
  - [ ] `task test:web` passes with a new test for the dialog; `bun run check:thin-client` and `bun run check:i18n` pass.

### Step 6: Teach agents to use it

- Owner: senior-d. Files: `web/app/agentdocs/recipes/6-revert-a-bad-session.json` (read the revision before starting, work, review with `view`, revert with `dry_run` then for real), the skill template if the loop section changes, regenerate the agent docs.
- Commit: `docs(cli): add the revert recipe for agents`.
- Acceptance:
  - [ ] `go run ./web/cmd/gen-agent-docs --check` exits 0; `go test ./web/app/agentdocs/...` passes.
  - [ ] the delivery check: through the MCP server, an agent makes five edits, reverts to the starting revision in one call, and `project_get` equals the starting document (paste the calls).

## 5. Out of scope

- Reverting other actors' batches, or "revert everything" across actors.
- Named checkpoints and a diff between revisions (plan 009).

## 6. STOP conditions

- The recorded-as-edit design breaks an invariant of SDD-005 §12.1 that the tests reveal (for example the redo chain): stop and report; the alternative is a new `kind = 'revert'` with a table-rebuild migration, which needs the orchestrator's decision.
- The `history` tool description cannot fit the NFR-020 budget: stop and report the measured tokens.

## 7. Maintenance

Any later change to the undo guard (SDD-005 §12.2) applies to revert too; keep one guard builder shared by both.
