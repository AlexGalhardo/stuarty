# 117: Screen recording UI: dedicated `/record` studio page, sidebar entry and standalone polish flow

## Goal

Every promise on Stuarty's landing page—*"Record on any system, polish the take"*—is front and center in the product UI. A creator sees a dedicated **Record** (`Gravar tela`) entry in the main sidebar and a quick-action button on Home, opens the full-featured `/record` studio page (matching FigCraft frames 24a to 24f), chooses a screen, window, or browser tab with live audio meters, webcam overlay, layout/background presets, and teleprompter, records the take, immediately polishes it with auto-zoom, smooth cursor, click highlights, and captions, and either exports the finished video directly or sends it straight into the Video Editor or a Project.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - The landing page highlights *"Record on any system, polish the take"*, yet nowhere in Stuarty's main UI (sidebar, Home, or standalone pages) is there any visible button or mention of screen recording!
- Verified codebase state:
  - The full screen-recording backend and editor components are ALREADY implemented:
    - `capture/` (native Windows helper), `engine/capturehost/`, `core/recording/` (state machine, layouts, zoom spans, pointer/click events, proposals), `service/recording_*.go`, web browser capture (epic 63), agent recording consent (epic 64), auto-zoom/smooth cursor/click highlight (epic 65), layouts/backgrounds/teleprompter (epic 66).
  - However, the UI is currently buried ONLY inside `web/resources/js/editor/library/Library.tsx` (`RecordDialog.tsx`), completely invisible unless a user already created a video project and opened the Video Editor!
  - `web/resources/js/dashboard/sections.ts:124` explicitly notes: *"A route that no epic has added yet (Record, Local models) is not here"*, and `sections.test.ts:94` still asserts `expect(hrefs).not.toContain("/record")`.
  - Epic 99 left frames `24a` to `24f` ("recording as a page") deferred pending the author's go—which D-160 now explicitly authorizes!
- Design reference: FigCraft frames `24a` to `24f` in `docs/design/final/stuarty-redesign-final-trilha.figcraft` (epic 116); update FigCraft frames first if any new control is added.
- ADR-0049 (native capture helper per OS), ADR-0051 (human consent for capture — an agent never starts recording silently), SDD-025 (screen recording), SDD-012 §23.4.

## Scope

- Modules owned:
  - `docs/design/final/stuarty-redesign-final-trilha.figcraft` (review/update frames 24a–24f for `/record` setup, live bar, post-take review/polish, and recent recordings).
  - `docs/sdds/025-screen-recording.md` and `docs/sdds/012-ui-architecture.md` (document `/record` route, sidebar entry, Home quick action, and standalone recording flow without a pre-created project).
  - `service/recording_ops.go` and `service/recording_assemble.go` (allow starting a recording with no pre-created project—auto-creating a lightweight standalone recording/timeline container matching epic 105—and listing recent recordings workspace-wide for `/record`).
  - `web/routes/dashboard.go` and `web/routes/pages.go` (register `GET /record` -> `Record/Index`).
  - `web/resources/js/dashboard/sections.ts` and `web/resources/js/dashboard/sections.test.ts` (add `record` to the `Create` group in the main sidebar with a `Video` / `CircleDot` icon, linking to `/record`, and update `TestDashboardSectionsMatchTheSidebar` and `sections.test.ts`).
  - `web/resources/js/shell/breadcrumb.ts` (add `/record` breadcrumb label).
  - `web/resources/js/pages/Dashboard/Home.tsx` (add a clean "Record screen" quick action alongside "New project" so recording is discoverable on first open).
  - `web/resources/js/pages/Record/` (new dedicated page implementing frames 24a–24f: source selector [Display / Window / Browser Tab], native desktop helper vs browser WebRTC mode indicator with clear system status, microphone selector with live VU level bar, system audio toggle, webcam preview & placement selector, layout & background preset picker, teleprompter launcher, countdown + start/pause/resume/stop controls, post-recording take review & polish drawer [auto-zoom toggle, smooth cursor, click highlight, quick trim, "Open in Video Editor", "Export MP4", "Attach to Project"], and Recent Recordings list).
  - `web/resources/lang/en.json` and `web/resources/lang/pt-BR.json`.
- Must not touch: `capture/` Rust helper code, `renderer/`, `service/providers/`, `core/image/`.

## Out of scope

- macOS and Linux native capture helpers (owned by epics 67 and 68; on macOS/Linux browsers `/record` automatically uses Stuarty's existing Web Recording engine of epic 63 and states clearly when native desktop capture is available).
- Replacing the in-editor `RecordDialog` (keep it working inside `/editor/{id}` so editors can still record directly onto an open timeline).

## Dependencies

- Epics 61, 62, 63, 65, 66, 75, 105, and 116 (recording engine, standalone timelines, and versioned FigCraft design).

## Technical design

1. **Discoverability (Sidebar + Home + Breadcrumb)**:
   - Add `record` (`/record`) to `SECTION_KEYS` and `NAV_GROUPS` under `create` in `web/resources/js/dashboard/sections.ts` right beside `projects`, `write`, `edit-video`, and `edit-image`.
   - Add a clear "Record screen" (`Gravar tela`) action button on `Dashboard/Home.tsx` (matching FigCraft design hierarchy).

2. **Project-independent recording (`service/recording_ops.go`)**:
   - Extend `RecordingStart` and `RecordingUploadStart` so `project_id` is optional: when omitted, the service creates a lightweight standalone video container (reusing epic 105's `standalone = 1` flag) automatically, so a user can land on `/record`, click Record, and polish/export in under 30 seconds without creating a production project first.
   - Extend `RecordingList` (workspace-wide recent takes, newest first, with status, duration, source kind, and linked project/timeline ID) on HTTP (`GET /api/v1/recordings`), CLI (`stuarty recording:list`), and MCP (`job_get` with `recordings: true`, within NFR-020 budget).

3. **Dedicated `/record` Studio Page (`web/resources/js/pages/Record/Index.tsx`, FigCraft frames 24a–24f)**:
   - Reuses the existing `useRecordFlow` hook, `RecordingBanner`, and recording API client (`web/resources/js/editor/record/`)—zero duplicated state machines!
   - **Pre-record stage (24a–24b)**: clean 2-column layout—live preview / source card on the left (Screen, Window, or Tab; automatic detection of native desktop helper on Windows vs browser `getDisplayMedia` fallback on any system), device & style controls on the right (Microphone + live input level meter, System audio, Camera bubble, Background & Layout preset from epic 66, Teleprompter shortcut).
   - **Active recording stage (24c–24d)**: prominent timer, pause/resume/stop/cancel buttons, system-wide indicator status, and safety guard (`useUnsavedGuard`) preventing accidental tab close mid-recording.
   - **Post-take review & polish stage (24e–24f)**: instant playback of the take, one-click switches for **Auto-zoom**, **Smooth cursor**, **Click highlight**, **Layout & Background**, and direct buttons: **"Open in Video Editor"** (`/editor/{timeline_id}`), **"Export MP4"**, and **"Save to Project"**.
   - **Recent takes list**: shows all previous takes with quick links to reopen in the editor or delete.

## Tasks

- [x] 1. Verify and update frames `24a`–`24f` in `docs/design/final/stuarty-redesign-final-trilha.figcraft`, and amend SDD-025 and SDD-012 for the `/record` page, sidebar entry, and standalone recording flow. Verify: `task lint:md` exits 0.
- [ ] 2. Allow optional `project_id` (standalone session creation) in `service/recording_ops.go` and add workspace-wide `RecordingList` across HTTP, CLI, and MCP with parity and security tests (ADR-0051 person-only start still enforced!). Verify: `go test ./core/recording/... ./service/... ./web/app/... ./web/contract/...`, `go test -run TestSecurity_ ./...`, and `go run ./internal/tools/surfacebudget` pass.
- [ ] 3. Register `GET /record` in `web/routes/dashboard.go`, add `record` to `dashboard/sections.ts` so the shell derives its breadcrumb, add the Home quick action, and update `TestDashboardSectionsMatchTheSidebar` and `sections.test.ts`. Verify: `go test ./web/routes/...` and `cd web && bunx vitest run resources/js/dashboard/sections.test.ts`.
- [ ] 4. Build `web/resources/js/pages/Record/` implementing frames 24a–24f (reusing `useRecordFlow` and recording APIs for both native desktop capture and web browser capture, plus post-take polish controls and recent takes list); add `en.json` and `pt-BR.json` strings. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run resources/js/pages/Record/`.
- [ ] 5. Add Playwright E2E tests covering sidebar **Record** navigation -> `/record` setup -> mocked web recording start/pause/stop -> post-take polish -> open in Video Editor, and compare screenshots against FigCraft frames 24a–24f in light and dark themes. Verify: `task lint`, `task test`, `task test:web`, and the Playwright record specs pass.

## Test strategy

- Unit & service tests: standalone recording creation without `project_id`; `RecordingList` owner isolation in network mode; ADR-0051 agent consent still blocks silent agent starts (`E_PRIVILEGED_SESSION` / consent required).
- Web unit tests (Vitest): `Record/Index.tsx` renders all stages (setup, recording, review/polish, recent takes), handles missing native helper by falling back cleanly to web capture, and enforces `check:thin-client`.
- E2E (Playwright): click **Record** in main sidebar -> `/record` loads -> record mocked stream -> stop -> review screen offers Auto-zoom, Open in Video Editor, and Export.

## Acceptance criteria

1. The main sidebar (`Create` group) displays a **Record** (`Gravar tela`) button linking to `/record`, and Home offers a visible "Record screen" quick action.
2. `/record` allows selecting screen/window/tab, microphone, system audio, webcam, layout, background, and teleprompter, and recording a take directly without creating a project first (using native capture where available and web capture on any browser/OS).
3. After stopping a take on `/record`, the user immediately sees the review & polish stage (auto-zoom, smooth cursor, click highlight, Open in Video Editor, Export, Attach to Project) matching FigCraft frames 24a–24f.
4. ADR-0051 holds: an AI agent can never start a capture silently without explicit human consent.
5. `task lint`, `task test`, `task test:web`, `go test -run TestSecurity_ ./...`, `go run ./internal/tools/surfacebudget`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and Playwright record specs pass.

## Risks and mitigation

- **Browser `getDisplayMedia` restrictions on non-HTTPS remote origins.** Mitigation: loopback (`127.0.0.1` / `localhost`) is a secure context in all browsers; on plain HTTP remote LAN hosts, show a clear localized notice explaining that HTTPS or Stuarty Desktop is required for browser screen capture.
- **MCP budget (NFR-020).** Mitigation: extend `job_get`, which already reads captures by their `rec_` ids; **STOP** if `surfacebudget` exceeds the ceiling.

## Size

L

## Hand-off for review

The commits of `epic/117-screen-recording-ui-and-record-page`, updated FigCraft frames 24a–24f, Go/security/Vitest/Playwright outputs, `surfacebudget` output, and light/dark screenshots of `/record`.
