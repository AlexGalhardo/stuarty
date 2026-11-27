# 111: Global and per-project Activity History

## Goal

A creator can click **History** in the main sidebar at any time to see a clear, filterable timeline of every recent action performed across Stuarty—by themselves or by any AI agent—and can open the dedicated **History** tab/section inside any individual project to review every change, edit, import, generation, approval, and publication that happened in that project.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #19**: Add a **History** button in the main sidebar so the user can inspect all recent actions that they or any AI agent performed globally across Stuarty.
  - **Feedback #19a**: Provide the same History feature exclusively scoped to each project inside the project page, so the user has the complete history of actions and changes for that specific project.
- Existing infrastructure verified in the codebase:
  - `service/audit.go` and `service/audit_list.go` already persist an append-only `audit` table (`id`, `at_ms`, `actor`, `action`, `project_id`, `subject_id`, `details`) and expose `AuditList(AuditListRequest{ProjectID, Cursor, Limit})`!
  - `service/history.go` and `service/image_history.go` already track timeline and image document revisions (edits, checkpoints, diffs, undo/revert) per actor.
  - However, today `AuditList` requires `admin` scope, has no UI page at `/history`, has no sidebar entry in `web/resources/js/dashboard/sections.ts`, does not filter by `actor_kind` (`human` vs `agent`), `domain` (`project`, `video`, `image`, `asset`, `writing`, `publish`, `agent`, `connection`, `approval`), or date/search, and does not unify timeline/image edit revisions alongside workspace-level audit events into a human-friendly activity feed!
- ADR-0006 (operation log with actor attribution), ADR-0007 (thin UI client: all formatting summaries, actor classifications, and filtering live in `core/` and `service/`), ADR-0052 & ADR-0053 (encrypted workspace: sealed text fields must never leak plaintext into unsealed audit rows; audit details store safe IDs/counts/enums or sealed summaries decrypted only while unlocked), SDD-000, SDD-003, SDD-005, SDD-012.

## Scope

- Modules owned:
  - `docs/sdds/000-shared-definitions.md`, `docs/sdds/003-agent-interfaces.md`, `docs/sdds/005-data-model-and-persistence.md`, `docs/sdds/012-ui-architecture.md`.
  - `core/audit/` (or `core/history/`): pure rules for normalizing and classifying activity events (`actor_kind`: `human` | `agent` | `system`, `domain`, human-readable action codes, filter validation, pagination cursor rules).
  - `service/audit.go`, `service/audit_list.go`, `service/activity_history.go` (new read model `ActivityHistoryList`: merges/queries audit events and project/timeline/image revision events with optional `project_id`, `actor_kind`, `domain`, `action`, `from`/`to`, `cursor`, `limit`; resolves `project_name`, `actor_display`, and deep-link target metadata; enforces user visibility in network mode and `read` scope so every signed-in user sees their own history while project-scoped keys see only their project).
  - `web/app/` (HTTP route `GET /api/v1/history` and `GET /api/v1/projects/{project_id}/history`, CLI `stuarty history [--project id] [--actor human|agent]`, extend existing `workspace` / `project` MCP read actions within NFR-020 budget).
  - `web/routes/dashboard.go` (register `GET /history` -> `History/Index`), `web/resources/js/dashboard/sections.ts` (add `history` nav item with `History` icon in the sidebar and update `TestDashboardSectionsMatchTheSidebar`).
  - `web/resources/js/pages/History/Index.tsx` and reusable `web/resources/js/components/ActivityTimeline.tsx` (chronological list grouped by day, actor badges distinguishing Human vs Agent with agent name, domain filter pills, actor filter [All / You / Agents], project filter on global view, expandable structured detail drawer, deep links to the affected project/item/revision, cursor "Load more" pagination).
  - `web/resources/js/pages/Projects/Show.tsx` (embed the per-project `<ActivityTimeline projectId={project.id} />` section/tab showing every action, edit, generation, import, and publication belonging to that project).
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`.
- Must not touch: `renderer/`, `engine/`, `desktop/`, `service/providers/`.

## Out of scope

- Deleting or editing historical audit records (the activity log is append-only for accountability).
- Replacing the Video Editor's or Image Editor's in-canvas undo/revert checkpoint sidebar (the History view complements them and links directly to those revisions).

## Dependencies

- Epics 06, 75, 79, and 116 (projects, app shell/sidebar, project detail screen, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — design the `/history` screen and per-project History section in FigCraft).

## Technical design

1. **Unified Activity Read Model (`service/activity_history.go`)**:
   - Request `ActivityListRequest`:
     - `ProjectID` (optional — empty for global `/history`, set for per-project history)
     - `ActorKind` (optional: `all` | `human` | `agent` | `system`)
     - `Domain` (optional: `project` | `video` | `image` | `asset` | `writing` | `publish` | `approval` | `agent` | `connection` | `account`)
     - `Cursor`, `Limit` (default 50, max 200)
   - Response `ActivityEntry`:
     - `id`, `at` (RFC 3339 UTC), `actor`, `actor_kind` (`human`|`agent`|`system`), `actor_label` (e.g., user name or agent profile name), `domain`, `action` (canonical SDD-000 action code, localized on the frontend by key `ui.history.action.<action>`!), `project_id?`, `project_name?`, `subject_kind?`, `subject_id?`, `subject_label?`, `summary_params` (safe structured metadata like `{ count, revision, target }`, never secrets and never unsealed private copy when locked).
   - Ensure every mutating service operation across domains (project create/update/rename/trash, video & image edits/exports, asset import/rename/link, writing save/draft, AI generation, approval decision, publication schedule/send, agent session start/tool call, connection add/remove) writes an audit/activity record with `project_id` populated whenever applicable.

2. **Global History page (`/history`) & Sidebar entry**:
   - Add `history` (`href: "/history"`, `lucide-react` `History` icon) to `NAV_GROUPS` in `web/resources/js/dashboard/sections.ts` and register `GET /history` in `web/routes/dashboard.go` (and breadcrumb entry in `web/resources/js/shell/breadcrumb.ts`).
   - Page shows:
     - Filter bar: **Actor** (`All`, `You`, `Agents`), **Domain** (`All`, `Projects`, `Video`, `Image`, `Assets`, `Writing`, `Publish`, `Approvals`, `Connections`), **Project** selector.
     - Day-grouped timeline entries with clear Human vs Agent badge (`StatusBadge`), timestamp localized with `formatDateTime`, localized action description, clickable project/subject link, and optional expandable details.

3. **Per-project History (`Projects/Show.tsx`)**:
   - Embed a dedicated **History** section/tab inside `Projects/Show.tsx` powered by the exact same `<ActivityTimeline projectId={project.id} />` component (scoped to `GET /api/v1/projects/{project_id}/history`), with Actor (`All` / `You` / `Agents`) and Domain filters and direct links into the editor revision, text item, asset, or publication.

## Tasks

- [ ] 1. Specify `ActivityHistoryList`, `/history`, `/api/v1/history`, `/api/v1/projects/{project_id}/history`, and canonical action codes in SDD-000, SDD-003, SDD-005, and SDD-012. Verify: `task lint:md` exits 0.
- [ ] 2. Implement pure classification and filter validation rules in `core/audit/` (or `core/history/`) with table tests. Verify: `go test ./core/...` passes.
- [ ] 3. Implement `service/activity_history.go` (global + per-project query, actor/domain filtering, network-mode owner isolation, audit coverage audit for key mutations, zero secret/sealed-data leaks). Verify: `go test ./service/... -run "Audit|History|Activity"` passes.
- [ ] 4. Expose HTTP routes, CLI `stuarty history`, MCP read action (within NFR-020), and `GET /history` Inertia route; add `history` to `sections.ts` and `breadcrumb.ts`. Verify: `go test ./web/app/... ./web/routes/... ./web/contract/...`, `go test -run TestSecurity_ ./...`, and `go run ./internal/tools/surfacebudget` pass.
- [ ] 5. Build `ActivityTimeline.tsx`, `pages/History/Index.tsx`, and integrate per-project History into `pages/Projects/Show.tsx`; add full `en.json` and `pt-BR.json` translations for all action codes and filters. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run`.
- [ ] 6. Add Playwright E2E tests verifying: (a) clicking **History** in the main sidebar shows recent human and agent actions globally with working filters; (b) opening a project's History section shows only that project's actions and updates after an edit. Verify: `task lint`, `task test`, `task test:web`, and the Playwright history specs pass.

## Test strategy

- Unit tests: actor classification (`local:chat-*` -> `agent`, browser session -> `human`, scheduler/system -> `system`), cursor pagination, filter validation.
- Security tests: network-mode user A cannot see user B's global or project history; project-scoped API key cannot read another project's history (`E_FORBIDDEN`); locked local workspace returns `E_LOCKED`; no secret or credential ever appears in history details.
- Web & E2E: Vitest tests for `ActivityTimeline`, `History/Index`, and `Projects/Show` history section; Playwright spec performing human + agent actions across 2 projects and verifying global `/history` vs per-project filtering.

## Acceptance criteria

1. The main sidebar includes a **History** (`Histórico` in `pt-BR`) item that opens `/history` showing all recent user and agent actions globally, filterable by Actor (All / You / Agents), Domain, and Project.
2. Every project detail page (`/projects/{project_id}`) includes a dedicated per-project **History** section showing all recent user and agent actions exclusively for that project.
3. Every action label is localized in both English and Brazilian Portuguese (zero raw English audit strings leaking into `pt-BR`), and untrusted titles/names are rendered through `<UntrustedText>`.
4. Network-mode user isolation, project-key scoping, and local-lock `E_LOCKED` rules hold on both global and per-project history endpoints.
5. `task lint`, `task test`, `task test:web`, `go test -run TestSecurity_ ./...`, `go run ./internal/tools/surfacebudget`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright history specs pass.

## Risks and mitigation

- **Audit log volume on heavy editing sessions.** Mitigation: index `(project_id, id DESC)` and `(id DESC)` on `audit` and `revisions`, paginate strictly with keyset cursors (max 200 per page), and coalesce bursty preview/read calls (only state-changing mutations and notable job completions enter history).
- **MCP budget (NFR-020).** Mitigation: attach history read as an action on an existing tool; **STOP** if `surfacebudget` fails.

## Size

M

## Hand-off for review

The commits of `epic/111-global-and-project-history`, Go/security/Vitest/Playwright outputs, `surfacebudget` output, and screenshots of global `/history` and per-project History in both `en` and `pt-BR`.
