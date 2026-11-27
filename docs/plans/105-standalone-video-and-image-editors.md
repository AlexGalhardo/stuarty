# 105: Standalone video and image editors without a pre-created project

## Goal

A creator can open **Edit Video** (`/edit-video`) or **Edit Image** (`/edit-image`) directly as standalone single-use editors—drop in a video or image file, edit it immediately, and export the finished file—without ever having to create a full production project, fill in a SMART goal, or set up a workflow first. At any time, they can optionally promote a standalone edit into a full project or link its assets to a project.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #4**: Stuarty's subproducts (video editor and image editor) must also work as standalone single-use tools. Users must be able to open the video editor or image editor directly, import a video or image file, edit it, and export it without everything having to be wired inside a pre-created production project.
- Current state in the codebase:
  - `web/routes/dashboard.go:17-19` still leaves `/edit-video` as a placeholder `Dashboard/Section` empty screen! Creating a video timeline today requires going through `/projects/new` (name, description, SMART goal fields, due date) first.
  - `/edit-image` (`web/resources/js/pages/EditImage/Index.tsx`) already works without a production project, but still asks for a form or dialog with file/path instead of offering an instant drag-and-drop canvas launch and lacks a one-click "Attach to project" / "Open from asset hash" standalone flow.
- ADR-0002 (rules in `core/`), ADR-0008 (one `Service`), ADR-0038 (image documents), ADR-0039 (projects as productions — amended/clarified so standalone timelines and image documents can exist as lightweight `standalone` sessions without mandatory SMART goals or production workflows), SDD-000, SDD-005, SDD-012, SDD-021.

## Scope

- Modules owned:
  - `core/project/` (support `standalone` mode flag on a timeline/project container that requires no description, no SMART goal, and no production workflow, plus a clean `PromoteToProject` rule).
  - `service/projects.go`, `service/production.go`, `service/image_import.go` (add `StandaloneVideoCreate` — creates a lightweight standalone timeline session from an uploaded/imported video or blank canvas preset and optionally imports the first clip straight onto track `V1`; add `ImageOpenFromAsset` — opens/creates a standalone image document from a workspace asset hash; add `StandaloneVideoList` and `StandalonePromote`).
  - `web/database/migrations/` (add `standalone INTEGER NOT NULL DEFAULT 0` column on `projects` / timelines so standalone edits stay out of the main Productions list unless promoted, while still appearing on `/edit-video`).
  - `web/app/` (HTTP routes, CLI commands `video:quick-open` / `image:quick-open`, and extend existing `project` / `image` MCP tool actions within NFR-020 budget).
  - `web/routes/dashboard.go` (remove `"edit-video"` from `dashboardSections` placeholder list and register real `/edit-video` page route) and `web/routes/pages.go` + `web/routes/image_pages.go`.
  - `web/resources/js/pages/EditVideo/` (new page: drag-and-drop dropzone + file picker + quick blank canvas presets + recent standalone video edits + direct `?asset=<hash>` auto-open into `/editor/{id}`).
  - `web/resources/js/pages/EditImage/Index.tsx` (add instant drag-and-drop dropzone at the top of `/edit-image` so dropping an image immediately creates & opens an image document, plus `?asset=<hash>` support and "Attach to project" action in `Show.tsx`).
  - `web/resources/js/editor/` (show a clean "Save as Project" / promote button in standalone video sessions).
  - `docs/sdds/000-shared-definitions.md`, `docs/sdds/005-data-model-and-persistence.md`, `docs/sdds/012-ui-architecture.md`, `docs/sdds/021-image-documents.md`.
- Must not touch: `renderer/`, `capture/`, `aihelper/`, `service/providers/`.

## Out of scope

- Removing production projects (Projects remain the full multi-step production hub).
- Drag-and-drop global asset library ingestion and the "save a copy in your asset library?" confirmation modal (owned by epic 106; this epic integrates with epic 106's modal once merged).

## Dependencies

- Epics 06, 28, 85, 86, and 116 (projects, image editor, video editor redesign, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — update FigCraft frames for `/edit-video` and `/edit-image` quick-drop before final hand-off). Works seamlessly with Epic 104 (double-click asset -> `/edit-video?asset=<hash>` / `/edit-image?asset=<hash>`) and Epic 106 (drag-and-drop upload).

## Technical design

1. **Standalone timeline model (`core/project/` + migration)**:
   - A standalone video edit is backed by the exact same `timelines` / `core/timeline` engine and `engine/export` pipeline (zero duplicated editor code!), marked with `standalone = 1` on its owning lightweight container row so:
     - It does NOT require a project description, SMART goal, or due date.
     - It does NOT clutter `/projects` (the Productions list filters `standalone = 0` by default) and does NOT clutter the Home workflow tracker.
     - It appears in `/edit-video` under "Recent standalone edits", can be opened at `/editor/{id}`, edited with every video tool, and exported to MP4/MOV/WebM/GIF/PNG/audio immediately.
     - Calling `ProjectPromote` (with name/description/goal) flips `standalone = 0` cleanly without moving or re-encoding a single clip.

2. **One-step quick-open flows**:
   - **Video (`/edit-video`)**:
     - Prominent drag-and-drop zone + "Choose video file" button (uses native `window.vukemDesktop.pickFiles` in Desktop and browser `<input type="file" accept="video/*">` / multipart upload in web) + "New blank timeline" presets (`9:16`, `16:9`, `1:1`, `4:5`).
     - Dropping or picking a video file automatically creates a standalone timeline matching the video's aspect ratio/FPS, imports the file, places it on track `V1` at `0s`, and redirects straight into `/editor/{id}`.
     - Navigating to `/edit-video?asset=<hash>` creates (or reopens) a standalone timeline pre-loaded with that library asset on `V1` and redirects into `/editor/{id}`.
   - **Image (`/edit-image`)**:
     - Prominent drag-and-drop zone + "Choose image file" button right at the top of `/edit-image` (no modal required for the happy path: dropping a PNG/JPEG/WebP immediately imports it as a standalone image document and navigates to `/edit-image/{image_id}`).
     - Navigating to `/edit-image?asset=<hash>` opens a standalone image document containing that workspace asset immediately.
     - Inside `/edit-image/{image_id}`, add a clean "Export" quick action and an optional "Link to project" action.

3. **Surfaces and MCP parity**:
   - HTTP: `POST /api/v1/standalone/videos` (multipart or JSON with `asset_hash`/`preset`), `GET /api/v1/standalone/videos`, `POST /api/v1/standalone/videos/{id}/promote`, `POST /api/v1/images/from-asset`.
   - CLI: `stuarty edit-video open <file|--asset hash|--preset 9:16>`, `stuarty edit-image open <file|--asset hash>`.
   - MCP: extend existing `project` and `image` tools with `standalone` parameter / action (zero new MCP tools, staying within NFR-020).

## Tasks

- [x] 1. Specify standalone video and image sessions in SDD-000, SDD-005, SDD-012, and SDD-021 (including migration schema, API/CLI/MCP contracts, and `surfacebudget` check). Verify: `task lint:md` exits 0.
- [x] 2. Add `standalone` support in `core/project/`, DB migration, and `service/` (`StandaloneVideoCreate`, `StandaloneVideoList`, `StandalonePromote`, `ImageOpenFromAsset`) with unit and integration tests. Verify: `go test ./core/project/... ./service/... ./web/database/...` passes.
- [x] 3. Expose HTTP routes, CLI commands, and MCP actions with full contract/parity tests; remove `"edit-video"` from `dashboardSections` placeholder in `web/routes/dashboard.go` and update `TestDashboardSectionsMatchTheSidebar`. Verify: `go test ./web/app/... ./web/routes/... ./web/contract/...` and `go run ./internal/tools/surfacebudget` pass.
- [x] 4. Build `web/resources/js/pages/EditVideo/Index.tsx` (drag-and-drop, file picker, blank presets, recent standalone edits, `?asset=<hash>` auto-open) and add the "Save as Project" promote action in the video editor for standalone sessions. Verify: `cd web && bun run typecheck && bun run check:thin-client && bunx vitest run`.
- [x] 5. Upgrade `web/resources/js/pages/EditImage/Index.tsx` with top-level instant drag-and-drop, file picker, and `?asset=<hash>` direct opening; add `en.json` and `pt-BR.json` strings. Verify: `cd web && bun run check:i18n && bunx vitest run`.
- [x] 6. Add Playwright E2E tests proving a user can drop a video in `/edit-video`, edit and export it with zero pre-created projects, and drop an image in `/edit-image`, edit and export it with zero pre-created projects. Verify: `task lint`, `task test`, `task test:web`, and the Playwright specs pass.

## Test strategy

- Unit tests: `core/project` standalone rules and promotion validation; `service` isolation (standalone videos excluded from `/projects` production list, included in `/edit-video`).
- Contract & parity tests: HTTP, CLI, and MCP parity + security scope tests (`read` cannot create, `edit` can create/promote).
- E2E (Playwright): fresh workspace with 0 projects -> open `/edit-video` -> upload test MP4 -> lands in `/editor/{id}` with clip on timeline -> export succeeds; fresh workspace with 0 projects -> open `/edit-image` -> upload test PNG -> lands in `/edit-image/{id}` -> export PNG succeeds.

## Acceptance criteria

1. Visiting `/edit-video` renders the standalone Video Editor launcher (no longer the placeholder empty section); dropping or picking a video file opens `/editor/{id}` with that clip ready on track `V1` and exports cleanly with zero pre-created projects.
2. Visiting `/edit-image` lets a user drop or pick an image file directly on the page and immediately opens `/edit-image/{id}` to edit and export with zero pre-created projects.
3. Standalone edits do not clutter the `/projects` production list, and promoting a standalone edit converts it into a full project with all timeline edits preserved.
4. Visiting `/edit-video?asset=<hash>` and `/edit-image?asset=<hash>` opens the matching standalone editor pre-loaded with that asset.
5. `task lint`, `task test`, `task test:web`, `go run ./internal/tools/surfacebudget`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright standalone specs pass.

## Risks and mitigation

- **Upload size limits in web mode.** Mitigation: enforce existing `STUARTY_MAX_MEDIA_BYTES` streaming multipart upload limits (SDD-007 §9) and use native path hand-off (`window.vukemDesktop.pickFiles` / `onDropPaths`) in Desktop mode so large videos are never buffered in browser memory.
- **MCP budget (NFR-020).** Mitigation: reuse existing `project` and `image` tools; **STOP** if `surfacebudget` exceeds the limit without an author-approved raise.

## Size

L

## Hand-off for review

The commits of `epic/105-standalone-video-and-image-editors`, migration file, Go/Vitest/Playwright outputs, `surfacebudget` output, and screenshots of `/edit-video` and `/edit-image`.
