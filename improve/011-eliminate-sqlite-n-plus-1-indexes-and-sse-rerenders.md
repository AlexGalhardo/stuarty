# Plan 011: Eliminate SQLite N+1 queries, add approval/cost indexes, and isolate SSE & editor re-renders

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- service/asset_library.go service/asset_ops.go service/projects.go service/production.go service/project_workflow.go web/app/store/approvals.go web/app/store/assets.go web/app/store/productions.go web/app/store/publications.go web/resources/js/layouts/DashboardLayout.tsx web/resources/js/editor/layout/EditorSceneBar.tsx web/resources/js/shell/ShellContext.tsx web/resources/js/image-editor/toolbar/operations.tsx web/resources/js/editor/timeline/Timeline.tsx web/resources/js/editor/export/ExportDialog.tsx`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: perf
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

As a workspace grows in projects, assets, and approval history, five concrete backend and frontend bottlenecks degrade everyday responsiveness:

1. **Workspace-wide `O(2P + A)` N+1 scan on every single-asset call (`service/asset_ops.go`, `service/asset_library.go`)**: `heldFile` (called by `AssetGet`, `AssetUses`, `AssetSetInfo`, `AssetTag`, `AssetRename`, `AssetReveal`) scans up to 2,000 timelines with 2 `ListProjectAssets` queries each, `O(A)` `Assets().Get` queries, and `O(A)` `os.Stat` calls just to find ONE hash.
2. **N+1 + duplicate AEAD decryptions in `ProjectList` and `ProjectDescribe` (`service/projects.go`, `service/production.go`)**: `ProjectList` issues `2 + 3N` queries decrypting full item/publication bodies; `ProjectDescribe` queries and decrypts publications twice (`withPublications` + `nextPost`).
3. **Full table scans & unused decryptions on `cost_ledger` and `approval_requests` (`web/app/store/approvals.go`)**: no index on `cost_ledger(project_id, state)`, `approval_requests(project_id, state, created_ms)`, or `approval_requests(state, expires_ms)`, and `approvalCosts` loads and AEAD-decrypts every historical approved request's `summary` just to sum `Estimate.Micros`.
4. **Full-app re-renders on every SSE chat event (`DashboardLayout.tsx`, `EditorSceneBar.tsx`)**: `useChat` and `useInbox` live inside root layout wrappers, so every streaming agent token re-renders `<EditorShell>`, `<ImageEditor>`, or the active page even when the chat dock is collapsed.
5. **Whole-store Zustand subscription in image-editor `useOperations()` & root `playheadFrame` in `Timeline`/`ExportDialog`**: panning/zooming re-renders `ToolRail`/`ZoomBar`/`CommandPalette` and walks the layer tree; scrubbing re-renders unmemoized timeline chrome and a closed `ExportDialog`.

## Current state

- `service/asset_ops.go:80-97`: `heldFile` calls `s.libraryHeld(ctx, c, true)` and linearly searches the full workspace slice for `hash`.
- `service/production.go:489, 504`: `ProjectDescribe` calls `s.withPublications(ctx, head.ID, rows)` and 15 lines later calls `s.nextPost(ctx, head.ID)` which re-queries `s.publicationsOf(ctx, head.ID)`.
- `web/database/migrations/20261119140000_create_approval_tables.go:33-34, 73-74`: `approval_requests` only indexes `(state, created_ms)` and `(job_id)`; `cost_ledger` only indexes `(day, currency)` and `(job_id)`.
- `web/resources/js/layouts/DashboardLayout.tsx:24-42`: calls `useChat` + `useInbox` and passes `{chat, inbox}` through `<ShellProvider>` wrapping `<Frame>{children}</Frame>`.
- `web/resources/js/image-editor/toolbar/operations.tsx:114-121`: `const store = useImage((s) => s);` subscribes to every field of the image store (including `view` pan/zoom).
- `web/resources/js/editor/timeline/Timeline.tsx:95` & `web/resources/js/editor/export/ExportDialog.tsx:196`: subscribe directly to `useUiState((s) => s.playheadFrame)`.
- Conventions:
  - SQLite migrations in `web/database/migrations/` follow timestamp naming `YYYYMMDDHHMMSS_<slug>.go` and must be registered in `web/database/migrations/migrations.go` (check existing migrations for registration pattern).
  - Non-enumeration security tests (`SDD-007 T-06 / T-20`) must stay green after SQL query consolidation.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Migration & store tests | `go test -race -count=1 ./web/database/migrations/... ./web/app/store/...` | `ok`, exit 0 |
| Asset/project/approval servicetests | `go test -race -count=1 -run 'Asset\|Project\|Approval\|Cost' ./web/app/servicetest/` | `ok`, exit 0 |
| Security suite | `go test -count=1 -run TestSecurity_ ./...` | `ok`, exit 0 |
| Web typecheck, lint & tests | `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run test` | exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify or create):

- `web/database/migrations/<timestamp>_add_approval_cost_indexes.go` (create) + migration registry file if applicable
- `service/store.go` (add targeted batch/aggregate repo methods if needed)
- `web/app/store/approvals.go` (+ `approvals_test.go`)
- `web/app/store/assets.go` (+ `assets_test.go`)
- `web/app/store/productions.go`
- `web/app/store/publications.go`
- `service/asset_library.go`
- `service/asset_ops.go`
- `service/projects.go`
- `service/production.go`
- `service/project_workflow.go`
- `web/resources/js/layouts/DashboardLayout.tsx`
- `web/resources/js/editor/layout/EditorSceneBar.tsx`
- `web/resources/js/shell/ShellContext.tsx`
- `web/resources/js/dashboard/chat/useChat.ts` (store wrapper)
- `web/resources/js/shell/useInbox.ts` (store wrapper)
- `web/resources/js/image-editor/toolbar/operations.tsx`
- `web/resources/js/editor/timeline/Timeline.tsx`
- `web/resources/js/editor/export/ExportDialog.tsx`

**Out of scope** (do NOT touch):

- Public HTTP/MCP/CLI wire schemas or OpenAPI shapes (keep every API response identical!).
- `TranscriptStage` virtualization (deferred to follow-up, noted in `improve/README.md`).

## Git workflow

- Branch: `perf/sqlite-n-plus-1-indexes-and-ui-rerenders` cut from `release/v0.60.0`.
- Commit in three clean slices:
  1. `perf(store): add approval_requests and cost_ledger indexes and SQL cost aggregation`
  2. `perf(service): eliminate N+1 scans in heldFile, ProjectList, and duplicate publication reads`
  3. `perf(ui): isolate Shell chat/inbox stores and narrow image-editor and timeline Zustand selectors`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Add missing SQLite indexes and SQL cost aggregation (`web/app/store/approvals.go`, `service/project_workflow.go`)

1. Read `web/database/migrations/20261119140000_create_approval_tables.go`, `web/app/store/approvals.go:50-195, 444-475`, and `service/project_workflow.go:30-115`.
2. Add a new migration creating:
   - `CREATE INDEX IF NOT EXISTS cost_ledger_project_state_index ON cost_ledger (project_id, state)`
   - `CREATE INDEX IF NOT EXISTS approval_requests_project_state_created_index ON approval_requests (project_id, state, created_ms)`
   - `CREATE INDEX IF NOT EXISTS approval_requests_state_expires_index ON approval_requests (state, expires_ms)`
3. In `web/app/store/approvals.go`:
   - Rewrite `SettledByKind(ctx, projectID)` to match `project_id = ? OR project_id IN (<timelines of projectID>)` (identical owner semantics to `ListRequests`, using the new `cost_ledger_project_state_index` instead of scanning every row).
   - Add `PendingApprovedCosts(ctx, projectID)` (or `SumRequestEstimates`) running `SELECT currency, COALESCE(SUM(estimate_micros), 0) FROM approval_requests WHERE state IN ('pending','approved') AND (project_id = ? OR project_id IN (...)) GROUP BY currency` without loading or AEAD-decrypting `summary`.
4. Update `service/project_workflow.go:104-112` (`approvalCosts`) to call the aggregate SQL method.

**Verify**: `go test -race -count=1 ./web/database/migrations/... ./web/app/store/... && go test -race -count=1 -run 'Approval|Cost|Workflow' ./web/app/servicetest/` -> `PASS`, exit 0.

### Step 2: Eliminate `heldFile` full-workspace scan, `ProjectDescribe` duplicate read, and `ProjectList` N+1

1. **`service/production.go:489-506`**: pass the `pubs` slice already fetched by `withPublications` into a pure helper `nextPostFrom(pubs)` instead of calling `s.nextPost(ctx, head.ID)` (which re-queries and re-decrypts all publications a second time).
2. **`service/asset_ops.go` & `service/asset_library.go` & `web/app/store/assets.go`**:
   - First, in `libraryScan` (`service/asset_library.go:75-175`), merge the two per-timeline `ListProjectAssets` calls (`Active` + `Pending`) into a single variadic call `ListProjectAssets(ctx, id, ProjectAssetActive, ProjectAssetPending)`.
   - Second, add a targeted `HeldByHash(ctx, hash)` / single-hash path in `service/asset_library.go` (using the existing `project_assets_asset_hash_index` via `store.Assets().HoldingsOfHash(ctx, hash)` or filtering `project_assets WHERE asset_hash = ?`), verifying visibility (`visible(c, ownerUserID)` and `s.covers(ctx, c, projectID)`) ONLY for the projects actually holding `hash`, so `heldFile` becomes `O(uses_of_hash)` instead of `O(2P + A)`! Preserve `T-20` non-enumeration identically.
3. **`service/projects.go:296-318`**: add `ItemsByProjects(ctx, projectIDs)` (or metadata-only query skipping `body` AEAD decryption where `ProjectList` only checks kinds/counts) and `PublicationsByProjects(ctx, projectIDs)` on the store repos, replacing the per-project loop queries in `ProjectList`.

**Verify**: `go test -race -count=1 -run 'Asset|Project|TestSecurity_T06|TestSecurity_T20' ./web/app/servicetest/ && go test -count=1 -run TestSecurity_ ./...` -> `PASS`, exit 0.

### Step 3: Isolate `ShellContext` (`useChat`/`useInbox`) from root layout re-renders and narrow editor Zustand selectors

1. **Image editor `useOperations()` (`web/resources/js/image-editor/toolbar/operations.tsx:114-149`)**:
   - Replace `const store = useImage((s) => s)` with narrow selectors (`useImage((s) => s.doc)`, `useImage((s) => s.busy)`, `useImage((s) => s.selection)`, `useImage((s) => s.send)`), reading imperative actions (`undo`, `redo`, etc.) via `useImageStore().getState()` inside click handlers. Memoize `row` / `only` with `useMemo`.
2. **Video editor `Timeline.tsx` & `ExportDialog.tsx`**:
   - In `ExportDialog.tsx:196`, do not subscribe to `playheadFrame` while `open === false` (either return `null` before subscribing by splitting `<ExportDialog open={open}>` into an outer gate + inner `<ExportDialogBody>`, or read `playheadFrame` from `uiStore.getState()` when `open` transitions to `true`).
   - In `Timeline.tsx:95`, extract `<PlayheadLine>` (and pass playhead subscription into the ruler/playhead child only) so `Timeline` root and `<TimelineToolbar>` / `<TrackHeader>`s do not re-render on every playback tick.
3. **`DashboardLayout.tsx` & `EditorSceneBar.tsx` & `ShellContext.tsx`**:
   - Back `chat` and `inbox` state with vanilla Zustand stores (created via `useRef` / `createStore` once per mounted layout) and expose selector hooks (`useShellChat(selector)`, `useShellInbox(selector)`) alongside backwards-compatible `useShell()` for leaf components (`ChatDock`, `SceneBar`, `NotificationBell`).
   - Because the store references passed to `<ShellProvider>` never change identity, SSE `chat.event` / `inbox` updates re-render ONLY the subscribed shell leaf components, NOT `<Frame>{children}</Frame>` or `<EditorShell>`.

**Verify**: `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run test` -> all pass, exit 0.

## Test plan

- `web/app/store/approvals_test.go`: verify `SettledByKind` and `PendingApprovedCosts` return identical totals to the prior implementation across projects with multiple timelines and mixed currencies.
- `web/app/servicetest/`: existing `Asset*`, `Project*`, `Approval*`, and `TestSecurity_T06`/`T20` suites pass unchanged; add a test asserting `heldFile` returns `CodeNotFound` identically for missing hash vs another user's hash.
- `web/` Vitest: add a render-count test verifying `useOperations()` does not re-render when `view.zoom`/`view.pan` updates in the image store, and `DashboardLayout` children do not re-render when a `chat.event` updates the chat store.

## Done criteria

- [x] `go test -race -count=1 ./web/database/migrations/... ./web/app/store/...` exits 0
- [x] `go test -race -count=1 -run 'Asset|Project|Approval|Cost|Workflow' ./web/app/servicetest/` exits 0
- [x] `go test -count=1 -run TestSecurity_ ./...` exits 0
- [x] `grep -n 'useImage((s) => s)' web/resources/js/image-editor/toolbar/operations.tsx` returns no matches
- [x] `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run test` exits 0
- [x] `task lint:go && task vet` exits 0
- [x] No files outside the in-scope list are modified (`git status`)
- [x] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Adding a new SQLite migration requires updating a hardcoded migration-count or schema-snapshot assertion in `web/database/migrations/` (check `migrations_test.go` first — if there is a test checking migration order/count, follow its documented pattern; never delete an existing migration!).
- Refactoring `ShellContext` breaks SSR (`web/resources/js/ssr.tsx`) or existing Vitest `renderToString` tests — ensure the Zustand store initializes synchronously with default state for SSR.

## Maintenance notes

- Never subscribe to an entire Zustand store (`useStore((s) => s)`) or high-frequency fields (`playheadFrame`, streaming SSE events) in layout roots or always-mounted toolbars.
- Reviewer focus: confirm `heldFile`'s single-hash SQL query preserves `trashedFile` behavior (`service/asset_ops.go:100`) and `T-20` non-enumeration.
