# 78: Project kind and the workflow per kind

## Goal

A project can be only a text, a video, an image or undecided, and Stuarty shows the real path from its briefing to its publication: each step, who does it, its tool, its cost and its state.

## Context and related decisions

- Author decisions D-134 (a project is not always video or image) and D-137 (`copy` ends in copy or export, and goes to a blog when the person chooses).
- `service/types.go` `ProjectCreateRequest`: name, description and goal, with no kind. A new project has no timeline. SDD-005, SDD-003, SDD-019, SDD-020. `core/writing/words.go` counts words.

## User stories

- As a creator, I want to start a project that is only a blog post so that nothing asks me for a format.
- As a creator, I want the steps to my publication so that I know what is done and what is next.
- As a creator, I want to add a video later without starting over.
- As an agent, I want to read the workflow so that I propose the right next step.

## Scope

- Modules: `core/project/` (kind and workflow rules), `core/writing/` (reading time and excerpt), `service/` project operations, one migration, the HTTP, CLI and MCP adapters of project create, get, list and item, SDD-000, SDD-002, SDD-003, SDD-005.
- Must not touch: the UI (epics 79, 80, 82), the approval rules, the publishing connectors.

## Out of scope

New publishing destinations, a destination for `copy`, and a built-in blog skill.

## Dependencies

57 (store, migrations and `service/`). If epic 69 is still in progress, this epic waits for its adapters to merge.

## Surfaces

| Surface | Change |
| --- | --- |
| HTTP, CLI | `kind` on create, `workflow` on get, summary on list, skip and restore a step |
| MCP | `kind` on `project_create`, `workflow` in the result of `project_get`, and the skip through `project_item` |
| Web and desktop | None here |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation | Does | Permission |
| --- | --- | --- |
| `ProjectCreate` (changed) | Takes an optional `kind`: `text`, `video`, `image`, default `undecided` | `edit` |
| `ProjectGet` (changed) | Adds `kind` and `workflow` | `read` |
| `ProjectList` (changed) | Adds a text summary: excerpt, words, reading minutes | `read` |
| `ProjectStepSkip` | Skips or restores an optional step | `edit` |

## Technical design

- **Kind** is a stored intent in a new column with a default, so that existing projects read `undecided`. The effective kind is derived in `core/project`: a stored kind wins, otherwise a project with a timeline is `video`, with an image `image`, with a text `text`.
- **Steps per kind.** Video: Briefing, Script, Recording (optional), Editing, Finishing (optional), Approval, Publication. Text: Briefing, Writing, Review, Approval, Publication. Image: Briefing, Editing, Approval, Publication. Undecided: Briefing, then the choice.
- **Adding media later** adds the steps of that medium without changing what is done.
- **State of a step** is computed by a pure function from existing facts: items, revisions, pending approvals, publications. A fact the server lacks gives `unknown`, never `done`.
- **Who, tool and cost.** Each step names its actor and its tool by name from a table in the core. Cost is the sum of the estimates of its pending and past approvals, and absent otherwise. The skill comes from epic 76 at read time, and the table suggests none.
- **Reading time** is words divided by 200, rounded up, as a pure function, and the excerpt is the first 160 characters of the text without its front matter, marked as untrusted.
- **MCP budget.** `tools/list` has 60 bytes free. `kind` must cost at most 45 of them. The skip is an `action` of `project_item`, paid for by tightening the wording of that same definition. The golden of T-31 changes through its documented update.

## Tasks

- [x] 1. Amend SDD-000, SDD-002, SDD-003 and SDD-005. Verify: `task lint:md` exits 0 and `task codegen` leaves no unexpected diff.
- [x] 2. Kind and the workflow function with its tables. Verify: `go test ./core/project/...` passes.
- [x] 3. Reading time and excerpt. Verify: `go test ./core/writing/...` passes.
- [x] 4. The migration, as the newest id, with the upgrade of an existing workspace. Verify: `go test ./web/database/migrations/...` passes.
- [x] 5. The service operations with member isolation. Verify: `go test ./service/... -run "ProjectKind|Workflow"` and `go test -run TestSecurity_ ./...` pass.
- [x] 6. HTTP and CLI. Verify: `go test ./web/contract/...` passes.
- [ ] 7. MCP within the budget. Verify: `go run ./internal/tools/surfacebudget` passes and `go test ./web/app/mcp/...` passes.
- [ ] 8. Agent docs and the generated reference. Verify: `task codegen` leaves no diff and `task lint` exits 0.

## Test strategy

Table tests for every kind and state, an upgrade test from a real older workspace, parity across three surfaces.

## Acceptance criteria

1. **Given** a create with `kind: text`, **when** it succeeds, **then** no timeline, format or media exists and the workflow has the five text steps.
2. **Given** an existing project with no kind, **when** it is read after the upgrade, **then** its effective kind comes from its items and no data changed.
3. **Given** a text project, **when** a timeline is added, **then** the video steps appear and the finished steps stay finished.
4. **Given** a required step, **when** a skip is requested, **then** it is refused with `E_VALIDATION`.
5. **Given** a post of 842 words, **when** it is listed, **then** the reading time is 5 minutes.
6. **Given** the same project, **when** read over HTTP, CLI and MCP, **then** the workflow is identical (`go test ./web/contract/...`).
7. `go run ./internal/tools/surfacebudget`, `task lint` and `task test` pass.

## Risks and mitigation

- A computed state that looks certain can mislead: unknown facts read `unknown`.
- The MCP budget is tight, and epics 61 to 72 add no tools by design.
- **STOP** if the MCP budget cannot hold the change, or if a migration must run out of order.

## Size

L: a model change across all surfaces. About 8 commits.

## Priority

Must. It carries author decision D-134 into the product.

## Hand-off for review

The commits, each criterion with its command and result, the upgrade test, the measured `tools/list` size.
