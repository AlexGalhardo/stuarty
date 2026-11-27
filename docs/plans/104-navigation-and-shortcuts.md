# 104: Global back navigation, double-click shortcuts and clickable workflow steps

## Goal

Navigating Stuarty feels immediate and effortless: every screen has a consistent Back button to return to the previous page, double-clicking a calendar day card or a video/image asset opens it straight in its editor or day view, and clicking any step circle on a project's Home workflow trail jumps straight to that workflow stage to edit it.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  1. **Feedback #2 (Back button on all screens)**: place a Back button on every screen so the user can always return to the previous page they came from without hunting through the sidebar.
  2. **Feedback #18 (Double-click shortcuts & clickable Home workflow dots)**:
     - Clicking/double-clicking a day card on the publishing calendar opens that single day view immediately for editing.
     - Double-clicking a video asset card opens it in the video editor to view/edit it.
     - Double-clicking an image asset card opens it in the image editor to view/edit it.
     - Clicking any circle/step dot on a project's Home workflow trail sends the user straight to that workflow step to make edits.
- Existing codebase patterns:
  - `web/resources/js/shell/TopBar.tsx` is rendered on every dashboard page; the video editor (`web/resources/js/editor/`) has its own top header bar; the image editor (`EditImage/Show.tsx`) and auth/error pages also need consistent back navigation.
  - `web/resources/js/workflow/HomeTrail.tsx` renders step circles (`<Mark state={step.state} />`) as static non-interactive `<li>` items today.
  - `web/resources/js/pages/Publish/CalendarView.tsx` already has `onOpenDay(date)` wired only to the small day-number button, not to double-clicking the day cell card itself.
  - `web/resources/js/assetlibrary/MediaCard.tsx` currently handles only single `onClick={onOpen}` (which opens the right detail drawer) and has no `onDoubleClick` handler.
- ADR-0007 (thin UI client: navigation targets come from service IDs and route mapping only), WCAG 2.2 AA (every double-click shortcut must remain accessible via keyboard Enter/context menu as well, and never break single-click selection!).

## Scope

- Modules owned:
  - `web/resources/js/shell/TopBar.tsx`, `web/resources/js/shell/ShellContext.tsx`, `web/resources/js/shell/useBackNavigation.ts` (new: tracks in-app navigation history stack with safe fallback to parent breadcrumb or `/` when opened directly).
  - `web/resources/js/editor/` top bar (add the matching Back button returning to the previous page or `/projects/{id}` / `/edit-video`).
  - `web/resources/js/pages/Error.tsx`, `web/resources/js/pages/Projects/Show.tsx` (remove duplicate ad-hoc back button if `TopBar` provides the universal one, or keep consistent).
  - `web/resources/js/workflow/HomeTrail.tsx` and `web/resources/js/workflow/continueTarget.ts` (make every step circle/item clickable to navigate directly to that step's editing route, respecting `isDocumentNavigation` for `/editor/...`).
  - `web/resources/js/pages/Dashboard/Home.tsx` (pass `project.id` and `text_summary?.item_id` to `HomeTrail`).
  - `web/resources/js/pages/Publish/CalendarView.tsx` (double-click on any day card opens that day view via `onOpenDay(day)`, without interfering with dragging/clicking target chips inside the cell).
  - `web/resources/js/assetlibrary/MediaCard.tsx` and `web/resources/js/assetlibrary/AssetLibrary.tsx` (double-clicking a video asset opens it in the video editor; double-clicking an image asset opens it in the image editor; includes keyboard/context-menu equivalent "Open in editor").
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`, `docs/sdds/012-ui-architecture.md`.
- Must not touch: `core/`, `engine/`, `renderer/`, `desktop/`.

## Out of scope

- Standalone projectless video/image opening backend if an asset has no linked timeline/image document yet (Epic 105 adds standalone video/image sessions; this epic navigates to an existing timeline/image document when present, or opens `/edit-video?asset=<hash>` / `/edit-image?asset=<hash>` which Epic 105 handles seamlessly).

## Dependencies

- Epic 102 (fixes `/editor/...` full-document navigation helper `isDocumentNavigation`). Works smoothly alongside Epic 105 for standalone asset opening.

## Technical design

1. **Global Back button (`useBackNavigation` + `TopBar` + Video Editor header)**:
   - Track visited in-app paths in `sessionStorage` (`stuarty.nav.history`, capped at 50 entries, deduplicating consecutive identical URLs) updated on every Inertia navigation and full-page load.
   - `BackButton` in `TopBar.tsx` (left of the breadcrumb, always visible across every dashboard screen, including `/projects`, `/write`, `/edit-image`, `/publish`, `/settings`, `/connections`, `/skills`, `/prompts`, `/memory`, `/approvals`, `/webhooks`, `/visual-assets`, `/video-assets`, `/sound-assets`, `/history`):
     - When an in-app previous entry exists, navigates to it (`window.history.back()` or Inertia visit/full navigation as appropriate).
     - When the user landed directly on a deep URL (no previous entry), falls back to the parent breadcrumb href (or `/` on top-level pages, disabled only when already at `/` with empty history).
     - Keyboard shortcut: `Alt+ArrowLeft` (standard browser back) works naturally.
   - Add the same `BackButton` to the Video Editor header (`web/resources/js/editor/`), falling back to `/projects/{project_id}` (or `/edit-video` for standalone sessions).

2. **Double-click shortcuts (preserving single-click & accessibility)**:
   - **Calendar day cards (`Publish/CalendarView.tsx`)**: attach `onDoubleClick` to each day cell container (ignoring events originating inside a `button[data-testid="target-chip"]` so double-clicking a post chip still opens the post, not the empty day); invokes `onOpenDay(day)`. Keep single-click on the day number button working as before.
   - **Video assets (`MediaCard.tsx` + `AssetLibrary.tsx`)**: on double-click of a `video` asset card:
     - If the asset already has a holding/timeline (`asset.poster?.timeline_id` or first project timeline), open `/editor/{timeline_id}?asset={hash}`; otherwise navigate to `/edit-video?asset={hash}` (standalone quick-edit flow of Epic 105).
     - Also add `"Open in video editor"` to the card's context menu (`RenameMenu` `extra`) for keyboard/assistive parity.
   - **Image assets (`MediaCard.tsx` + `AssetLibrary.tsx`)**: on double-click of an `image` asset card:
     - Navigate to `/edit-image?asset={hash}` (or open an existing image document if linked, otherwise create/open an image document from that workspace asset hash via `image.import` / standalone editor).
     - Also add `"Open in image editor"` to the card's context menu for keyboard/assistive parity.

3. **Clickable workflow step dots (`HomeTrail.tsx` + `continueTarget.ts`)**:
   - Extend `continueTarget(step, projectId, textItemId)` to cover every step id cleanly (`briefing` -> `/projects/{id}#brief-title`, `script`/`writing`/`review` -> `/write?...`, `recording`/`editing`/`finishing` -> `/editor/{id}`, `thumbnail`/`image` -> `/edit-image?project={id}`, `approval` -> `/approvals?project={id}`, `publication` -> `/publish?project={id}`).
   - In `HomeTrail.tsx`, wrap each step circle and label in an accessible link (`<a>` when `isDocumentNavigation(href)` is true, Inertia `<Link>` otherwise) with hover/focus ring and `aria-label` describing `"Go to step: <step title>"`. Do the same for the compact mobile view.

## Tasks

- [x] 1. Update SDD-012 §23.4 (global Back button in TopBar and Video Editor, double-click shortcuts, clickable workflow trail steps). Verify: `task lint:md` exits 0.
- [x] 2. Implement `useBackNavigation` and mount the Back button in `TopBar.tsx` and the Video Editor header; add tests in `TopBar.test.tsx`. Verify: `cd web && bunx vitest run resources/js/shell/TopBar.test.tsx`.
- [x] 3. Make every step dot in `HomeTrail.tsx` clickable using `continueTarget` and `isDocumentNavigation`; update `Home.tsx` and `Home.test.tsx`. Verify: `cd web && bunx vitest run resources/js/pages/Dashboard/Home.test.tsx`.
- [x] 4. Add double-click handlers (plus context-menu equivalents) to `Publish/CalendarView.tsx` day cards and `assetlibrary/MediaCard.tsx` + `AssetLibrary.tsx` for video and image assets. Verify: `cd web && bunx vitest run resources/js/pages/Publish/ resources/js/assetlibrary/`.
- [ ] 5. Add `en.json` and `pt-BR.json` labels and Playwright E2E tests covering Back navigation across 3 screens, calendar day double-click, asset double-click, and Home workflow dot clicks. Verify: `task lint`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright spec pass.

## Test strategy

- Unit tests (Vitest): `useBackNavigation` stack push/pop/fallback; `TopBar` back button rendering and click; `HomeTrail` renders links for every step (`<a>` for `/editor/`, `<Link>` for Inertia pages); `CalendarView` double-click on day cell calls `onOpenDay` while double-click on target chip does not; `MediaCard` double-click triggers `onOpenEditor`.
- E2E (Playwright): navigate `/` -> `/projects` -> `/settings` -> click Back -> lands on `/projects` -> click Back -> lands on `/`; double-click a day cell on `/publish` -> opens DayView; click a workflow dot on Home -> lands on that step's page; double-click an image and video asset -> opens editor.

## Acceptance criteria

1. Every dashboard screen and the video editor displays a visible, keyboard-accessible Back button that returns to the previous page (or parent section when opened directly).
2. Double-clicking any day cell in the month or week view of `/publish` opens that single day view immediately.
3. Double-clicking a video asset card opens the video editor for that asset; double-clicking an image asset card opens the image editor for that asset (and both actions are also available in the card menu).
4. Clicking any step circle in a project's Home workflow trail navigates directly to the editing screen for that step without triggering an Inertia error modal.
5. `task lint`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright spec pass.

## Risks and mitigation

- **Single-click vs double-click conflict on `MediaCard`.** Mitigation: single-click continues to open/highlight the right detail panel immediately (no artificial 300ms delay), and double-click triggers editor navigation cleanly.
- **Asset with no existing timeline/image document when double-clicked before Epic 105 merges.** Mitigation: if Epic 105 is not merged yet, gracefully open `/edit-image` import pre-filled or show a one-click confirmation; once Epic 105 is merged, `/edit-video?asset=<hash>` and `/edit-image?asset=<hash>` open directly into standalone editing.

## Size

M

## Hand-off for review

The commits of `epic/104-navigation-and-shortcuts`, Vitest and Playwright outputs, and verification that every screen has a working Back button.
