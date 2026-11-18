# Plan 032: Finish the final editor UI and prove the keyboard shortcuts and layout in the running product

This plan answers how the author-approved final UI design (21 pages) reaches the product: projects, settings and the top bar are simplified, the editor panels, player and timeline match the final mockups in both themes, the auxiliary flows (export, shortcuts, command palette, templates, trash, brand kit, API keys, sign in, desktop) are implemented on the web and in the desktop shell, and every screen is walked in the running product. It also closes the CapCut-compatible shortcut check. When delivered, the keyboard shortcut suite passes entire, nothing of the reference application's brand enters the product, and every walked state is recorded.

- Owner: senior-e (implementation); orchestrator (projects and settings surface, final walk-through)
- Effort: L
- Depends on: the author's authorization for screen automation of the reference application before the shortcut re-check (the inventory with the application installed was recorded on 2026-11-17); the screens must be implemented before the final walk-through
- Origin: EPIC-52: TASK-0238; EPIC-53: TASK-0298, TASK-0299, TASK-0300, TASK-0301
- Status: TODO (the shortcut check of Step 1 was recorded as BLOCKED on 2026-11-16: it depends on the inventory with the reference application installed and on the author's authorization for screen automation)
- Written against: 8787714a

## 1. Why

Author decision D-038 asks for a UI familiar to people coming from CapCut: layout, menus, flows and generic names follow it, while the look is Vukem's. No asset, icon, logo, brand text, screenshot or file of the reference application may enter the repository. The design is finished; the product is not.

## 2. Current state (drift check)

```text
git log --oneline -n 5
cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run check:i18n
cd web && bunx playwright test final-ui-layout project-settings
go test ./desktop/shell
rg -il capcut web/resources desktop
```

Already true:

- The final design (21 pages, 21 PNGs, no imported images) is finished and opened in Figcraft on 2026-11-18, kept in the maintainers' private design files.
- The commit `8787714a` (`feat(ui): simplify project and settings surfaces`, the head of the current branch) already touches the projects and settings pages, the top bar, the button component, `app.css`, `.editorconfig`, the design system, SDD-009, SDD-012 and two ADRs, and adds `web/e2e/specs/final-ui-layout.spec.ts`. The former card still read "in progress; first tests passed; visual review and commit missing", so its asserts are not yet proven.
- Earlier UI work is done: the library tab rail, audio and text panels, effects panel, inspector tabs, timeline toolbar, player row, editor and clip menu, projects grid, shortcuts dialog footer and the native menu bar (release v0.27.0); SDD-012 section 6.5 compares 10 CapCut patterns and 5 were adopted (5ab96a67..0ebd01b4); web vitest 768/768.
- Shortcuts: SDD-012 section 18.7 holds the keymap; every shortcut whose command exists works (e2e on Chromium, WebKit and Firefox); Alt+M, Mod+G, P, Mod+Shift+C/V and S wait for core commands N-2, N-3, N-6 and N-8; the shortcuts page is generated from `keymap.ts`. The app walk-through of 2026-11-17 corrected the layout in SDD-012 (the 21 lines of section 6.4 are marked Different).

## 3. Steps

### Step 1: Re-check every researched shortcut in the application

Owner: senior-e. Files: `web/e2e/`.

Re-check each shortcut of the research against the application, after the author authorizes screen automation.

Commit: none if only checking; the evidence goes in the Status line.

Acceptance:

- [ ] The shortcut e2e suite passes entire; the shortcuts that the browser cannot capture are documented as such.

### Step 2: Verify the projects, settings and top bar surface

Owner: orchestrator. Files: `web/resources/js/pages/Projects/`, `web/resources/js/pages/Settings/`, `web/resources/js/editor/layout/TopBar.tsx`, `web/resources/js/components/ui/button*`, `web/resources/js/app.tsx`, `web/resources/css/app.css`, `web/components.json`, `web/e2e/`, `.editorconfig`, `docs/design-system.md`, `docs/sdds/012-ui-architecture.md`, `docs/sdds/009-releases-and-ci-cd.md`, `docs/adrs/0013-browser-preview-stack.md`, `docs/adrs/0027-stack-draft-items-cut-or-deferred.md`.

Complete the visual review of the commit `8787714a` and prove its asserts.

Commit: `fix(ui): <english summary>` only for what the review finds.

Acceptance:

- [ ] In `web/`: `bun run typecheck`, `bun run lint`, `bun run check:thin-client` and `bun run check:i18n` pass.
- [ ] The Projects and Settings e2e passes in Chromium, Firefox and WebKit, and `go test ./desktop/shell` passes.
- [ ] Project creation is one line that wraps without overlap; the size presets keep an accessible name.
- [ ] Settings has navigation by sections; language and theme stay reachable in the editor through the shortcuts dialog.
- [ ] Links styled as buttons keep the link role.
- [ ] The Shadcn generator configuration is removed; Base UI stays as the base of the components.

### Step 3: Implement the final editor in the panels, player and timeline

Owner: senior-e. Files: `web/resources/js/editor/`, `web/resources/css/app.css`, `web/e2e/`, `docs/design-system.md`, `docs/sdds/012-ui-architecture.md`.

Pages of the Editor in the final design; one microcommit per region.

Commit: `feat(ui): <region> matches the final design`, one per region.

Acceptance:

- [ ] `task lint`, `bun run check:thin-client` and the editor e2e pass.
- [ ] The hierarchy of the regions and the main controls match the final mockups in both themes.
- [ ] Actions still go through the service, with no edit result computed in the UI.

### Step 4: Complete the auxiliary flows and the desktop shell parity

Owner: senior-e. Files: `web/resources/`, `web/e2e/`, `desktop/`, `docs/design-system.md`, `docs/sdds/012-ui-architecture.md`.

Screens of the final design: Export, Shortcuts, Command palette, Templates, Trash, Brand kit, API keys, Sign in and Desktop; one microcommit per screen.

Commit: `feat(ui): <screen> matches the final design`, one per screen.

Acceptance:

- [ ] `task lint`, the screen tests, `go test ./desktop/shell` and the accessibility check pass.
- [ ] Every flow of the final document is walked on the web and in the desktop shell.
- [ ] No asset or brand text of the reference application enters the product (`rg -il capcut web/resources desktop` finds only nominative mentions in documentation).

### Step 5: Walk every finished screen in the running product

Owner: orchestrator. Files: `web/e2e/`; results go to the maintainers' private QA results.

Start only when every screen is implemented.

Acceptance:

- [ ] The browser states are opened and the main controls clicked with the result checked: Projects, Templates, Trash, Sign in, Settings, Brand kit, transcription, API keys, the editor with and without a selection and its panels, menus, shortcuts, palette, export and confirmations.
- [ ] The same available flows are walked in the installed desktop window, including the native menu, import and export.
- [ ] Each screen is checked visually at 1440x900 and 1024x768, in light and dark themes, with no overlap, unexpected clipping or visible translation key.
- [ ] The behaviour and accessibility e2e runs on the supported browsers; each failure found and fixed, and each platform limit, is recorded in the private QA results.

## 4. Out of scope

- New core commands for shortcuts that wait on N-2, N-3, N-6 and N-8 (plan 033 and the core owner).
- Copying any asset, icon, logo, text or screenshot of the reference application.
- Screen reader work (plan 028).

## 5. STOP conditions

- The author has not authorized screen automation of the reference application: stop Step 1.
- A control of the final design has no command in the core: stop and report; the control is not drawn without it (rule: a button appears only with an existing command).
- The `dropdown-menu` generator component is needed: confirm with the orchestrator first.
- A change would make the UI compute an edit result: stop; `bun run check:thin-client` must pass.

## 6. Maintenance

When a core command is added for a waiting shortcut, wire its key and its e2e test in the same change and update the generated shortcuts page. Re-walk every screen at each release candidate.
