# 07: Dashboard: sidebar, navigation, profile, theme

## Goal

A creator opens Stuarty and sees where everything is: one home with a sidebar that leads to each part of the work. The same pages appear in the browser and in the desktop application.

## Context and related decisions

- ADR-0041 (one web UI), ADR-0007 (thin client), ADR-0031.
- Author decision D-075. PRD-004 RF-310, RF-311, RF-312, NFR-065; `docs/design-system.md`; SDD-012.

## Scope

- Modules: `web/resources/js/layouts/`, `web/resources/js/dashboard/` (new), `web/resources/js/pages/` (one route per section), `web/routes/pages.go`, `web/app/controllers/` (page controllers only), `web/resources/lang/*.json`, `docs/design-system.md`, a new section in SDD-012.
- Sections of the sidebar: Projects, Edit Video, Edit Image, Write, Visual Assets, Video Assets, Publish, Skills, Prompts, Memory, Connections; at the bottom the logged-in profile and the light and dark theme toggle.
- A section whose epic is not done yet shows an honest empty state that says what it will hold, never a broken page.

## Out of scope

- The content of each section (epics 08, 14, 15, 17, 20, 28).
- The agent chat panel: this epic leaves a docked region for it and nothing else (epic 14).
- The editor's own layout (epic 37).

## Dependencies

03.

## Technical design

- **Layout**: `DashboardLayout` with a collapsible sidebar, a main region and a right dock region (empty). Built on the existing primitives (Base UI, Tailwind tokens of the design system); no new dependency.
- **Routes**: Inertia pages under `/` for the home, and one per section; the existing `/projects`, `/settings` and editor routes keep working, with redirects where a path changes. The editor opens full window and offers a way back to the dashboard.
- **Thin client**: the pages request data through existing service results; no rule is computed in TypeScript (`bun run check:thin-client`).
- **Profile**: shows the current user (network mode) or "local" (local mode), with sign out where it applies and a link to settings.
- **Theme**: reuse the existing light and dark tokens and the remembered preference; the toggle moves to the sidebar.
- **Keyboard and accessibility**: the sidebar is a navigation landmark, every item is reachable by keyboard with a visible focus, the current section is announced (`aria-current`).
- **Desktop**: nothing shell-specific; the shell loads the same routes.

## Tasks

- [x] 1. Add the dashboard section to SDD-012 and the sidebar to the design system (structure, tokens, states). Verify: `task lint:md` exits 0.
- [x] 2. Build `DashboardLayout` with the sidebar, the profile and the theme toggle. Verify: `task test:web` passes with the new component tests; `bun run check:contrast` passes.
- [x] 3. Add the section routes and their empty states, in both languages. Verify: `bun run check:i18n` and `bun run typecheck` pass.
- [x] 4. Move the projects list and the settings into the new layout with redirects. Verify: the existing specs `web/e2e/specs/projects*.spec.ts` and `settings*.spec.ts` pass, updated where a selector moved.
- [x] 5. Add the way in and out of the editor. Verify: `web/e2e/specs/dashboard-navigation.spec.ts` (new) passes.
- [x] 6. Run the accessibility checks on the dashboard. Verify: the axe spec for the dashboard (`web/e2e/specs/a11y-dashboard.spec.ts`, new) reports no violation.
- [x] 7. Open the shell on Windows and walk every section. Verify: screenshots of the dashboard in the browser and in the shell are attached to the hand-off.

## Test strategy

- Unit (Vitest): sidebar state, active item, collapsed mode, theme toggle.
- End to end (Playwright): navigation to every section, the way back from the editor, both languages, both themes.
- Accessibility: axe on the dashboard and keyboard-only navigation.

## Acceptance criteria

1. The application opens on the dashboard, and the sidebar lists the eleven sections, the profile and the theme toggle.
2. Every section route renders in English and in Brazilian Portuguese, in light and in dark, with no console error.
3. Every sidebar item is reachable and operable by keyboard, and axe reports no violation on the dashboard.
4. The existing projects, settings and editor flows still work (their specs pass).
5. `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `bun run check:contrast`, `task test:web` and `task lint:web` pass.
6. The same pages load in the Wails shell on Windows.

## Risks and mitigation

- **Selectors of the 105 existing specs break.** Mitigation: keep test ids stable; update specs in the same commit as the move.
- **Conflict with the editor UI epic (37)**, which touches `layouts/`. Mitigation: 37 starts after this epic is merged.
- **STOP** if a section needs a new dependency.

## Size

M

## Hand-off for review

The diff of `epic/07-dashboard-shell`; screenshots (browser and shell, light and dark); the outputs of the acceptance commands; the list of specs whose selectors changed.
