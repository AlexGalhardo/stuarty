# 56: The projects page and the project context

## Goal

Everything a creator makes in Stuarty lives inside a project they create with a name, a description and a SMART goal, and every agent that works in that project reads them first, the way an engineer reads a repository's `AGENTS.md`.

## Context and related decisions

- Author decision D-119 (2026-11-20). The projects page is a legacy of the video editor: creating a project asks for a video size preset and builds a timeline (`service/projects.go`, `web/resources/js/pages/Projects/Index.tsx`). A project has no description or goal field; the brief is a project item (SDD-005 section 5.12) filled later by a guided briefing for video (`core/project/briefing.go`).
- ADR-0039 (project as production), ADR-0002 (one command core), SDD-005 sections 3.1, 5.12 and 5.12.1, SDD-000 (operations), SDD-003 sections 2.3 and 4.1 (agent interfaces), SDD-012 section 2.1 (UI), SDD-017 section 6.2 (chat context).
- The agent chat today adds only one sentence with the project name and id (`service/chat_ops.go`); `ProjectDescribe` (`service/production.go`) returns the brief body.
- The vukem framework v0.46.0 has a validation package `z` (`github.com/AlexGalhardo/vukem/z`: strings, structs, issues, JSON Schema export) that Stuarty does not import yet. `zod` is approved by the author for the web UI (exact stable version).

## Scope

- Modules: `core/project/` (the project fields and their rules), `service/` (projects, `ProjectDescribe`, the chat context), the store and a migration, the adapters (HTTP, MCP `project_create`/`project_get`, CLI), `web/resources/js/pages/Projects/` and the project page, the agent docs, SDD-000, SDD-005, SDD-003, SDD-012, SDD-017, the user docs of projects.
- Must not touch: authentication, the vault, the sidebar footer (epic 57 owns them), the landing page (`site/` outside the user docs).

## Out of scope

- More than one goal per project; changing the guided video briefing; renaming existing projects automatically; encrypting the new fields (epic 57 does it after this epic merges).

## Dependencies

06, 16, 39 (merged). Runs next to epic 57 on separate modules; epic 57 encrypts the new project fields only after this epic is integrated.

## Technical design

- **Fields.** A project gets `description` and `goal` (`specific`, `measurable`, `achievable`, `relevant`, `time_bound` text, and `due` date). Stored on the project row (a migration); no default is invented for existing projects.
- **Rules, in the core, with `vukem/z`.** Name: 4 to 32 characters, Unicode letters, digits and single spaces, no leading or trailing space, no two spaces in a row (the control and bidi rules of VV-049 still apply). Description: 64 to 1024 characters. Each goal part: 16 to 280 characters. `due`: a valid date, today or later at creation. Errors are `E_VALIDATION` with the field `path` and a hint. The JSON Schema exported by `z` is generated into `web/resources/js/generated/` by `task codegen`, and the web form builds its `zod` schema from it, so the two never drift.
- **Operations.** `ProjectCreate` takes name, description and goal, all required, on every surface; the size preset and fps leave project creation (they stay as optional, deprecated arguments for one deprecation period, SDD-000 section 13, and create nothing). A project is created with no timeline; adding a video item asks for the preset, as it does today. New `ProjectUpdate` edits name, description and goal with the same rules (SDD-000 first); on MCP it is the action `update` of `project_item` (NFR-020 keeps the catalogue at 20 tools).
- **Agent context.** The agent chat puts the description and the goal of the project in the context of every conversation of that project, marked as user data, not system instructions (the untrusted-content envelope of SDD-017). `ProjectDescribe` and `project_get` (`detail=overview`) return them first. The agent docs tell agents to read them before anything else.
- **The page.** The create form shows name, description and the five goal parts, each with a tooltip that explains what to write and why it matters, and a placeholder with a worked example. The create button is enabled only when every field is valid; errors show under the field. Existing projects without description or goal open with a "Complete the project" notice that opens the edit form. The UI computes nothing the core decides (`check:thin-client`): it validates with the generated schema and shows the core's errors.

## Tasks

- [x] 1. SDD-000, SDD-005 and SDD-012 first: the fields, the rules, `ProjectUpdate`, the deprecated preset arguments. Verify: `task lint:md` exits 0.
- [x] 2. The core fields and rules with `vukem/z`, the migration, the JSON Schema export. Verify: `go test ./core/project/... ./service/...` passes, with tests for every boundary (3/4/32/33 characters, accents, two spaces, 63/64/1024/1025, 15/16/280/281, a past due date).
- [x] 3. `ProjectCreate` and `ProjectUpdate` on HTTP, MCP and CLI with parity tests; no timeline at creation. Verify: `go test ./web/...` passes and `go run ./internal/tools/surfacebudget` exits 0.
- [x] 4. The agent context: chat and `ProjectDescribe`/`project_get`, and the agent docs. Verify: a service test that the chat context of a project holds its description and goal inside the user-data envelope.
- [x] 5. `zod` (exact stable version) in `web/`, the schema generated from the core, the projects page and the edit form with tooltips and placeholders, the "Complete the project" notice. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, Vitest and a Playwright spec (button disabled until valid, each error under its field, create, edit an old project).
- [x] 6. The user docs of projects and the en and pt-BR strings. Verify: `task lint` exits 0.

- [ ] 7. Jobs and generated files belong to the project (SDD-005 section 5.12.3, found by VV-330): a migration so that `jobs.project_id` and the generated files attach to the project, not to a timeline row, and the project-wide kinds (image, voice, text, video, image edit, text draft) use it; the timeline-bound kinds stay as they are. Verify: a service test and a contract scenario that a generation in a project with no timeline succeeds on every surface.

## Acceptance criteria

1. A project cannot be created on any surface without a valid name, description and goal; each invalid field returns `E_VALIDATION` with its `path` (tests on HTTP, MCP and CLI).
2. A new project has no timeline; adding a video asks for the size.
3. The chat context of a project contains its description and goal as user data; `project_get` returns them first.
4. The create button is disabled until every field is valid, and the page explains every field (Playwright spec in Chromium).
5. An existing project with no description opens and can be completed through `ProjectUpdate`.
6. `task lint`, `task test`, `task test:web`, `bun run typecheck` and `bun run check:thin-client` pass; the only new dependency is `zod`.

## Risks and mitigation

- Agents and scripts that call `project_create` with only a name break: the error names each missing field with a hint, and the agent docs and the CLI help show a complete example.
- The MCP byte budget (NFR-020): the new arguments must fit; if they do not, stop and report the numbers.

## Size

L

## Hand-off for review

The commits, each acceptance criterion as done or not with the command and its result, how to try the page, and anything that contradicts a specification.
