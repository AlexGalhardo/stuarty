# 06: The project as the container of a production

## Goal

A creator opens one project and finds the whole production in it: the brief, the script, the assets it uses, the edits, the thumbnail, the exported files and where they were published. An agent that has never seen the project reads the same thing as text.

## Context and related decisions

- ADR-0039 (the project is the production), ADR-0005, ADR-0006, ADR-0009, ADR-0031.
- Author decision D-064. PRD-004 RF-300, RF-303, RF-304; PRD-001 RF-001; PRD-001 NFR-012 (old workspaces open).
- Open product issue this epic closes (its text comes in the briefing): VV-225 (there is no operation to duplicate a project). Add `ProjectDuplicate` on the three surfaces and in the project card menu, as one more task after task 4.

## Scope

- Modules: `core/project/` (new, pure rules of a production), `service/` (project operations), `web/database/migrations/`, `web/app/` (controllers, CLI, MCP), `web/resources/js/pages/Projects/`, `docs/sdds/005-data-model-and-persistence.md`, `docs/sdds/001-timeline-schema.md` (only the name of what it describes), SDD-000, SDD-003.
- Item kinds that exist after this epic: `brief`, `text`, `timeline`, `output` (an exported file with its destination format). The kinds `image` and `publication` are reserved and added by epics 27 and 20.

## Out of scope

- The asset library (epic 08), image documents (27), writing tools (17), publications (20).
- Any change to the timeline document, its commands or its operation log.

## Dependencies

03.

## Technical design

- **Model**: `projects` stays the table of productions; a new `project_items` (id with a type prefix, project id, kind, title, position, reference to the item's own storage, timestamps). The current per-project timeline document becomes a `timeline` item; `timelines` (or the renamed current table) keeps the document, the log and the revisions keyed by item id.
- **Migration**: one migration turns every existing project into a project with one `timeline` item, keeping ids stable where a surface exposed them. A backup is made first (the existing document-backup mechanism). It is reversible in tests.
- **Ids and surfaces**: an operation that took `project_id` to mean the timeline keeps working when the project has exactly one timeline and returns a clear error naming the choices when it has several; new operations take `timeline_id`. Write the rule into SDD-003 before code.
- **Rules in `core/project`**: allowed kinds, ordering, what "production state" is (`ProductionState(items) -> {has brief, has script, timelines, outputs, pending approvals, published}`).
- **Service**: `ProjectGet` returns the items and the production state; `ProjectItemAdd`, `ProjectItemMove`, `ProjectItemRename`, `ProjectItemTrash`, `ProjectItemRestore`; `BriefGet`, `BriefSet` (Markdown text, RF-302). Text view: `ProjectDescribe` in the token budget of PRD-002.
- **Templates and bundles**: a bundle exports the whole project; importing a bundle of the former format yields a project with one timeline.
- **UI**: the project page becomes an overview of the production with its items and state; opening a timeline goes to the editor as today.

## Tasks

- [x] 1. Amend SDD-005, SDD-003 and SDD-000 with the model, the id rule and the new operations. Verify: `task lint:md` exits 0 and the reading tests of SDD-003 pass.
- [x] 2. Implement `core/project` with table tests. Verify: `go test ./core/project/...` passes.
- [x] 3. Write the migration and its down path, with fixtures of workspaces of earlier releases. Verify: `go test ./web/database/... -run TestProjectItemsMigration` passes for every fixture.
- [x] 4. Implement the service operations and the compatibility rule for `project_id`. Verify: `go test ./service/... -run "TestProject|TestBrief"` passes.
- [x] 4b. (VV-225) `ProjectDuplicate` in the service; its route, CLI command and MCP encoding come with task 6 and the card menu item with task 7. Verify: `go test ./web/app/servicetest/ -run TestProjectDuplicate` passes.
- [x] 5. Update bundle export and import. Verify: `go test ./service/... -run TestBundle` passes, including a bundle made by release 0.29.0.
- [x] 6. Expose on HTTP, CLI and MCP with the text view. Verify: `go test ./web/contract/...` passes and the reference scenario (`bash scripts/smoke-e2e.sh`) still exports.
- [x] 7. Build the project overview page and adapt the project list. Verify: `task test:web` and `web/e2e/specs/project-overview.spec.ts` pass.
- [x] 8. Regenerate agent documents and types. Verify: `task generate:check` exits 0.

## Test strategy

- Unit: production state for every combination; kind and ordering rules.
- Migration: fixtures of a workspace from releases 0.2.0, 0.17.0 and 0.29.0 open with every project intact.
- Contract: parity of the new operations; the reference scenario unchanged for a one-timeline project.
- End to end: create a project, add a second timeline, see both in the overview.

## Acceptance criteria

1. Opening a workspace of release 0.29.0 shows every former project as a project with one timeline, and its history and undo still work.
2. A project can hold two timelines; `ProjectGet` lists both and commands address each by `timeline_id`.
3. The reference scenario of PRD-001 runs unchanged through CLI and MCP (`bash scripts/smoke-e2e.sh` exits 0).
4. `BriefSet` then `BriefGet` round-trips Markdown, and `ProjectDescribe` includes the brief within the token budget.
5. `go test ./core/project/... ./service/... ./web/database/... ./web/contract/...` and `task test:web` pass.
6. `TestSurfaceFrozen` passes with only additions to `surface.txt`.

## Risks and mitigation

- **Breaking existing agents' scripts.** Mitigation: the compatibility rule for `project_id`; no removal from the frozen surface.
- **A migration that loses history.** Mitigation: backup first, fixtures of real earlier workspaces, a tested down path.
- **STOP** if keeping the frozen surface is impossible without ambiguity.

## Size

L

## Hand-off for review

The diff of `epic/06-project-as-production`; the migration fixtures and how they were built; the outputs of the acceptance commands; a walk-through of creating a project with two timelines on each surface.
