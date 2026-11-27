# 102: Critical UI bug fixes: workflow navigation, collapsed sidebar version and empty-state layout

## Goal

Clicking "Continue" on a project from the Home screen opens the editor cleanly without ever displaying Inertia's blank white error overlay; collapsing the sidebar hides the version string instead of wrapping `v0.6 / 0.0` across two lines; and the numbered steps inside narrow empty-state cards (Prompts, Memory, Skills and other split panes) stay cleanly aligned inside their border with zero text overflow.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  1. **Feedback #9 (blank white screen on Home continue workflow -> `/projects/prj_...`)**: `Home.tsx:283` and `Projects/Show.tsx:563` render `<Link href={target}>` (an Inertia XHR visit) where `continueTarget()` returns `/editor/{project_id}` for `recording`, `editing`, and `finishing` steps. Because `GET /editor/{project_id}` (`web/routes/pages.go:345`) returns the standalone non-Inertia HTML document (`c.Render("editor", ...)`), Inertia receives an HTML 200 without `X-Inertia: true` and pops up its blank white full-screen iframe modal over the page.
  2. **Feedback #11 (collapsed sidebar version)**: `web/resources/js/dashboard/Sidebar.tsx:425-442` still renders `<ReleaseMark inline compact={collapsed} />` inside the `w-14` (`56px`) collapsed rail, causing `v0.60.0` to wrap awkwardly onto two lines (`v0.6` / `0.0`). When the sidebar is collapsed, the version text must be removed completely.
  3. **Feedback #13 (Prompts empty-state step alignment)**: `web/resources/js/components/EmptyState.tsx:48` uses viewport breakpoint `sm:grid-cols-3` instead of a CSS container query. Inside the narrow left list pane of split-pane pages (`Prompts`, `Memory`, `Skills`), the viewport is wider than `640px` while the card itself is ~280px wide, forcing 3 horizontal columns that break every word onto its own line and overflow the dashed card border.
- SDD-012 §2.2 (the video editor is a separate HTML document, navigated by full browser navigation, not Inertia XHR) and §23.4 (sidebar rail).

## Scope

- Modules owned:
  - `web/routes/pages.go` (handle `X-Inertia` header in `editorPage` by returning `inertia.Location(c, "/editor/"+id)` as a server-side safety net so no Inertia `<Link>` anywhere in the app can ever open a blank iframe modal).
  - `web/resources/js/pages/Dashboard/Home.tsx`, `web/resources/js/pages/Projects/Show.tsx`, and a shared helper `isFullDocumentTarget(href: string): boolean` in `web/resources/js/workflow/continueTarget.ts` (render `<a href={target}>` whenever `target` points to `/editor/...`, and `<Link href={target}>` for Inertia pages; audit all `<Link href="/editor/...">` across `web/resources/js/`).
  - `web/resources/js/dashboard/Sidebar.tsx` (do not render `ReleaseMark` or `sidebar-version` text when `collapsed` is true).
  - `web/resources/js/components/EmptyState.tsx` and `web/resources/js/components/NumberedStep.tsx` (add `@container` to `EmptyState`, switch `sm:grid-cols-3` to `@xl:grid-cols-3` or `@lg:grid-cols-3` so narrow panes stack vertically in 1 column, and add `min-w-0 [overflow-wrap:anywhere]` to step text).
- Must not touch: `core/`, `service/`, `engine/`, `renderer/`, `desktop/`.

## Out of scope

- Adding clickable workflow step dots (owned by epic 104).
- Adding desktop update buttons to the sidebar footer (owned by epic 101).
- Adding new pages or routes.

## Dependencies

- Epics 75, 79, and 99 (shell, Home/Projects, final design).

## Technical design

1. **Workflow continue navigation (both client and server fix)**:
   - Client: add `isDocumentNavigation(href: string): boolean` (`href.startsWith("/editor/")`) in `web/resources/js/workflow/continueTarget.ts` and a tiny `<AppLink href={...}>` or conditional `<a href={target}>` vs `<Link href={target}>` used in `Home.tsx`, `Projects/Show.tsx`, and any other link targeting `/editor/`.
   - Server (`web/routes/pages.go:editorPage`): if `c.Header("X-Inertia") == "true"`, respond with `inertia.Location(c, c.Request().URL.RequestURI())` (HTTP 409 `X-Inertia-Location`) when the project has a timeline, so even an accidental Inertia visit forces a clean full-page navigation instead of Inertia's error iframe modal. When `head.TimelineID == ""`, keep redirecting cleanly to `/projects/{id}`.

2. **Collapsed sidebar footer (`Sidebar.tsx`)**:
   - In `SidebarBody`, render the `<div data-testid="sidebar-version">` (containing `<ReleaseMark />` and `sidebar-mode`) only when `!collapsed` (`{!collapsed && (...)}`). Keep the `<ThemeToggle />` and expand button cleanly centered in the collapsed rail.
   - Update `Sidebar.test.tsx` to assert that no version text is rendered in the DOM when `form="rail"` / `collapsed`.

3. **Container-aware `EmptyState` steps (`EmptyState.tsx`, `NumberedStep.tsx`)**:
   - Add `@container` class to `<section data-slot="empty-state">`.
   - Change the steps `<ol>` from `sm:grid-cols-3` to `grid-cols-1 @[34rem]:grid-cols-3` (544px container width threshold) and add `overflow-hidden` / `min-w-0` safety on the `<ol>` and `<li>` elements so narrow panes (Prompts, Memory, Skills, mobile) render a clean vertical 1-column numbered checklist with zero horizontal overflow, while full-width empty states (Home, Projects, Library) keep the 3-column layout.

## Tasks

- [x] 1. Fix `/editor/{project_id}` navigation in `web/routes/pages.go` (return `inertia.Location` when `X-Inertia: true`), audit all `<Link href="/editor/...">` across `web/resources/js/`, and fix `Home.tsx` and `Projects/Show.tsx`. Verify: `go test ./web/routes/...` and `cd web && bunx vitest run resources/js/pages/Dashboard/Home.test.tsx` pass.
- [x] 2. Hide the version string in `web/resources/js/dashboard/Sidebar.tsx` when `collapsed` is true and update `Sidebar.test.tsx`. Verify: `cd web && bunx vitest run resources/js/dashboard/Sidebar.test.tsx` passes.
- [x] 3. Make `EmptyState.tsx` steps container-responsive (`@container` + `@[34rem]:grid-cols-3`) and prevent text overflow in `NumberedStep.tsx`; update `SharedComponents.test.tsx`. Verify: `cd web && bunx vitest run resources/js/components/SharedComponents.test.tsx` passes.
- [ ] 4. Add Playwright E2E regression tests covering (a) clicking Home "Continue" on a video project opens `/editor/{id}` with zero Inertia modal overlay, (b) collapsed sidebar displays no version string, and (c) `/prompts` empty state has zero horizontal overflow (`scrollWidth <= clientWidth` on `[data-slot="empty-state"]`). Verify: `task lint`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client`, and the Playwright spec pass.

## Test strategy

- Go HTTP test in `web/routes/`: `GET /editor/{id}` with `X-Inertia: true` returns status `409` with `X-Inertia-Location: /editor/{id}` instead of raw HTML.
- Vitest unit tests: `Home.test.tsx` confirms `<a href="/editor/...">` (not Inertia `<Link>`) for editing/recording/finishing steps; `Sidebar.test.tsx` confirms collapsed rail renders no version text; `SharedComponents.test.tsx` confirms container query class on `EmptyState`.
- Playwright E2E: verifies all three visual regressions on real rendered layout at `1280x800` and `1440x900`.

## Acceptance criteria

1. Clicking "Continue" on Home or on `/projects/{id}` for a video project at `recording`, `editing`, or `finishing` navigates directly to `/editor/{id}` with zero blank white Inertia modal overlay; sending `X-Inertia: true` to `/editor/{id}` returns `409` with `X-Inertia-Location`.
2. When the sidebar is collapsed (`data-state="collapsed"`), no version text (`vX.Y.Z`) is rendered anywhere in the sidebar rail.
3. On `/prompts`, `/memory`, and `/skills` with zero entries at `1280x800`, `[data-slot="empty-state"]` stacks its 3 numbered steps vertically with `scrollWidth <= clientWidth` and every step text stays inside the dashed border.
4. `go test ./web/routes/...`, `task lint`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client`, and the Playwright regression spec pass.

## Risks and mitigation

- **Existing tests asserting `sidebar-version` in collapsed state.** Mitigation: update `Sidebar.test.tsx` and any E2E selector to assert `sidebar-version` is present only when expanded and absent when collapsed.
- **STOP** if Tailwind v4 container query syntax differs from `@container` / `@[34rem]:grid-cols-3` in `web/package.json`; check installed Tailwind config first and use standard CSS `@container` if needed.

## Size

S

## Hand-off for review

The commits of `epic/102-critical-ui-bug-fixes`, outputs of Go tests, Vitest, typecheck, lint, and Playwright screenshots confirming all three bugs are resolved.
