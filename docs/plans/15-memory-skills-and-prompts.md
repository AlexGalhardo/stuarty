# 15: Memory, skills and prompts

## Goal

A creator's way of working stops living in their head: what they learned from past productions, the procedures their agent follows and the prompts they reuse are kept as plain files they own, searchable from the dashboard and readable by any agent.

## Context and related decisions

- ADR-0040 (memory, skills and prompts as files with an index), ADR-0031, ADR-0020 (untrusted text).
- Author decisions D-063, D-071. PRD-004 RF-385, RF-386, RF-387, RF-388, RF-390, RF-391.
- Research note `web/06-skills-open-source` (the Agent Skills format; public skill directories contain copied and inflated repositories).
- Absorbs the earlier plans on the skill lint, generated skills, plugin packaging and the learnings memory.

## Scope

- Modules: `core/library/` (new: pure rules of front matter, names, the skill lint), `service/library/` (new: files, index, search, import, export), `web/app/` (controllers, CLI, MCP), `web/resources/js/pages/Skills/`, `Prompts/` and `Memory/`, `web/database/migrations/`, `docs/sdds/018-memory-skills-and-prompts.md` (new), SDD-000, SDD-007 rows.
- The three kinds, their storage, search, editing, import and export. The curated content itself is epic 16.

## Out of scope

- Writing the curated skills (epic 16).
- Running anything a skill contains: a skill is text an agent reads. Stuarty never executes a script that comes with a skill.
- Embeddings or a vector index: full-text search only.

## Dependencies

07.

## Technical design

- **Files are the truth**: under the workspace, `library/memory/`, `library/skills/<name>/SKILL.md` (the Agent Skills layout, with optional reference files) and `library/prompts/`. Markdown with YAML front matter. SQLite holds only an index that can be rebuilt from the files (`LibraryReindex`); a file edited outside Stuarty is picked up on the next scan.
- **Rules in `core/library`**: front matter schema per kind; names (lower case, hyphens); memory scope (`workspace` or a project id); prompt variables (`{{name}}`) and their validation; the **skill lint** (required fields, description length, no reference to a missing file, no instruction that asks for a credential, size limits).
- **Origin** (RF-387): every skill records `origin` (`built-in`, `user`, `imported`), `source` text, licence, and `reviewed` (a person marked it as read). An imported skill starts unreviewed, is listed with a warning, and is not offered to agents by default until reviewed.
- **Import** (RF-388): from a local folder or an archive only. Stuarty opens no connection to fetch a skill; the user or their agent downloads it. Scripts inside an imported skill are stored as inert files and flagged.
- **Memory**: notes written by a person or by an agent; an agent's note records its actor and is marked until a person keeps it. Size limits and a ceiling per day per agent.
- **Search** (RF-391): one full-text query over the three kinds with filters; results fit the token budget and are delimited as untrusted text.
- **Export**: `SkillExport` writes one skill, or all reviewed ones, as a folder in the Agent Skills layout so the user can install them in their own agent.
- **Service**: `MemoryList`, `MemoryGet`, `MemoryWrite`, `MemoryRemove`, `SkillList`, `SkillGet`, `SkillWrite`, `SkillImport`, `SkillMarkReviewed`, `SkillLint`, `SkillExport`, `PromptList`, `PromptGet`, `PromptWrite`, `PromptRender`, `LibrarySearch`, `LibraryReindex`; registered in the `memory` domain (epic 13 when merged; the registry field otherwise).

## Tasks

- [x] 1. Write SDD-018 and SDD-000 names; add SDD-007 rows (a skill as prompt injection, an imported script, memory flooding by an agent, path traversal in a skill name). Verify: `task lint:md` exits 0.
- [ ] 2. Implement `core/library` with table tests, including the skill lint. Verify: `go test ./core/library/...` passes.
- [ ] 3. Implement file storage, the scan and the rebuildable index. Verify: `go test ./service/library/... -run "TestStore|TestReindex"` passes (deleting the index and reindexing gives the same results).
- [ ] 4. Implement memory, prompts with rendering, and search. Verify: `go test ./service/library/... -run "TestMemory|TestPrompt|TestSearch"` passes.
- [ ] 5. Implement skills: write, lint, import from folder and archive, review mark, export. Verify: `go test ./service/library/... -run TestSkill` passes, including archive traversal cases.
- [ ] 6. Expose on HTTP, CLI and MCP with parity and untrusted-text marking. Verify: `go test ./web/contract/...` passes.
- [ ] 7. Build the three sections (list, search, editor with preview, origin and review state) in both languages. Verify: `task test:web` and `web/e2e/specs/library.spec.ts` pass.
- [ ] 8. Security tests of the new rows. Verify: `go test -run TestSecurity_ ./...` and `go run ./internal/tools/threatcheck` exit 0.

## Test strategy

- Unit: schemas, names, variables, every lint rule with a passing and a failing fixture.
- Integration: files edited outside the product; index rebuild; import of hostile archives.
- Contract: parity of every operation; search inside the budget.
- End to end: write a prompt, render it, import a skill, review it, export it.

## Acceptance criteria

1. A memory note, a skill and a prompt created in the UI exist as Markdown files in the workspace and are found by `LibrarySearch` on HTTP, CLI and MCP.
2. Deleting the SQLite index and running `LibraryReindex` restores every result.
3. An imported skill is unreviewed, flagged, and absent from the default agent listing until a person reviews it.
4. An archive with a path outside the skill folder is refused (`TestSecurity_` test).
5. `SkillLint` reports each rule of SDD-018 on its failing fixture.
6. `SkillExport` produces a folder that passes `SkillLint` when imported again.
7. `go test ./core/library/... ./service/library/... ./web/contract/...`, `go test -run TestSecurity_ ./...` and `task test:web` pass.

## Risks and mitigation

- **A third-party skill instructs an agent to exfiltrate.** Mitigation: unreviewed by default, origin shown, lint rules, and the outbound gate of epic 04 as the real barrier.
- **Files and index drift.** Mitigation: the index is disposable; a scan on start and on demand.

## Size

L

## Hand-off for review

The diff of `epic/15-memory-skills-and-prompts`; the lint rules and their fixtures; the outputs of the acceptance commands; a walk-through of import, review and export.
