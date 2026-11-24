# 74: UI foundation: tokens, type and base components

## Goal

Every screen of Stuarty draws from one calm visual system, so that the app reads as one tool. This epic moves the tokens and the base components to the final redesign while every existing page keeps working, so the screens can migrate one area at a time.

## Context and related decisions

- Author decisions D-133 to D-137, and the final redesign file held by the author (page "01 Design System"). The dark theme stays the default; only the logo is fixed; dashboard text is at least 14 px and editor text at least 12 px.
- ADR-0007 and SDD-012 section 1 (thin client), SDD-012 section 18 (accessibility), `docs/design-system.md` sections 2 and 9.3.
- This epic replaces the text scale of author decision D-123 (1.5 times the default) with the scale of the redesign.
- Code: `web/resources/css/app.css` (the only token file), `web/resources/js/components/ui/`, `web/scripts/check-contrast.ts`.

## User stories

- As a creator, I want every screen to look like the same product so that I do not relearn the tool in each section.
- As a creator, I want text I can read and a visible focus so that I can use Stuarty for hours.
- As an engineer, I want one set of tokens and components so that a new screen needs no new styling.

## Scope

- Modules: `web/resources/css/app.css`, `web/resources/js/components/ui/`, new shared components under `web/resources/js/components/`, `docs/design-system.md`, the pairs of `web/scripts/check-contrast.ts`.
- With the author's approval only: `web/package.json` and its lockfile for the Inter font, and its licence notice.
- Must not touch: pages, layouts, `web/resources/js/dashboard/`, `web/resources/lang/*.json`, `site/`, any Go code.

## Out of scope

Layout of any page, the sidebar, the scene bar and every agent component (epic 75), and any new behaviour.

## Dependencies

None. It can start while epics 57 and 69 are in progress, because it keeps the props of every component they use.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | The same screens with the new colours, type, radii and focus |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None.

## Technical design

- **Tokens.** The semantic tokens of the redesign go in `app.css`: `--color-background`, `surface`, `surface-raised`, `surface-sunken`, `surface-hover`, `surface-selected`, `text`, `text-muted`, `text-subtle`, `border`, `border-strong`, `focus`, `accent`, `agent`, `success`, `warning`, `danger`, each family with `-soft` and `-text`; spacing on a base of 4; radii `sm` 6, `md` 8, `lg` 12, `xl` 16, `2xl` 24; three shadows per theme; motion `fast` 120 ms, `base` 200 ms, `slow` 320 ms and `ease-standard`.
- **Values in OKLCH.** The contrast check reads OKLCH from `:root` and `.dark`. The hex values of the redesign are converted once and written in OKLCH, so the check keeps its method and only its pairs and the numbers of section 2.6 change.
- **Migration by aliases.** The current names (`--background`, `--primary`, `--muted`, `--card` and the rest) stay and point to the new tokens, so that a page not yet migrated repaints correctly with no change to its code. An alias is removed only by the last epic that stops using it.
- **Text scale.** The rule `html[data-text-scale="dashboard"]` maps the Tailwind sizes to the new scale, with `--text-xs` at 14 px, for every width. The editors keep 12 px as their minimum. Headings follow `h1` 30/38, `h2` 24/32, `h3` 18/26.
- **Base components.** The components of `components/ui/` are restyled in place, with the same props and the same base-ui primitives. Control height is at least 32 px on desktop and 44 px at phone width. Every interactive control has a 2 px focus ring in `--color-focus`.
- **New shared components**, data-free: `PageHeader`, `EmptyState`, `GuideStrip`, `NumberedStep`, `DataRow`, `ListRow`, `SectionTitle`, `StatusBadge` (always icon and word). Loading uses a static skeleton and a sentence, with no spinner.
- **Type.** Inter (SIL OFL 1.1) replaces Geist only after the author approves the dependency. `@fontsource/inter` 5.3.0 is already pinned in `site/`. Until then the font token stays on Geist, and nothing else waits for it. The mono family stays on the system stack.
- **Rules of the document.** `docs/design-system.md` records the new tokens and retires the old aesthetic prohibitions the author lifted. Accessibility, tokens only, nothing by colour alone, reduced motion and no external origins stay mandatory.

## Tasks

- [x] 1. Write the tokens of both themes in `app.css` in OKLCH, with the aliases. Verify: `cd web && bun run check:contrast` exits 0 after the pairs and section 2.6 are updated in the same commit.
- [x] 2. Replace the text scale, the spacing, the radii, the shadows and the motion tokens, with reduced motion. Verify: `cd web && bun run typecheck && bun run test` pass.
- [ ] 3. Restyle the form controls (`button`, `input`, `textarea`, `select`, `checkbox`, `radio-group`, `switch`, `slider`, `toggle`, `toggle-group`). Verify: `cd web && bun run test` passes and `bun run lint` exits 0.
- [ ] 4. Restyle the surfaces and overlays (`card`, `dialog`, `alert-dialog`, `sheet`, `popover`, `dropdown-menu`, `context-menu`, `tooltip`, `tabs`, `badge`, `skeleton`, `progress`, `separator`). Verify: `cd web && bun run e2e -- editor-menus-dark` passes.
- [ ] 5. Add the data-free shared components, each with a unit test of its accessible name and states. Verify: `cd web && bun run test` passes.
- [ ] 6. Check the existing pages at 1440, 1280 and 390 px in both themes, and fix only what the tokens broke. Verify: `cd web && bun run e2e -- a11y` passes, and the screenshots are in the hand-off.
- [ ] 7. Update `docs/design-system.md`. Verify: `bunx markdownlint-cli2@0.24.1 docs/design-system.md` exits 0 and `cd web && bun run check:contrast` exits 0.
- [ ] 8. Only if the author approved it: bundle Inter and add its notice. Verify: `task notices:check` exits 0, and the built page requests no external origin.

## Test strategy

Contrast check on every token pair, unit tests for the new components, the existing Vitest and Playwright suites as the regression net, screenshots in both themes.

## Acceptance criteria

1. **Given** the tokens, **when** `bun run check:contrast` runs in `web/`, **then** every pair passes WCAG AA in both themes.
2. **Given** a page not migrated, **when** it opens, **then** it renders with the new tokens, with no missing colour (`bun run e2e -- a11y` passes).
3. **Given** the dashboard at any width, **when** its smallest Tailwind text size resolves, **then** it is 14 px.
4. **Given** a control, **when** it takes keyboard focus, **then** a 2 px ring in `--color-focus` shows.
5. **Given** `prefers-reduced-motion`, **when** a transition runs, **then** its duration is 1 ms.
6. `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `bun run lint` and `task test:web` pass, and no prop of a `components/ui` component changed.
7. Without the author's approval, `web/package.json` is unchanged.

## Risks and mitigation

- An alias with the wrong meaning repaints many pages at once: task 6 covers every area in both themes.
- Epics 57 and 69 add components meanwhile: props stay stable, and they consume tokens by name.
- **STOP** if a token pair cannot pass AA with the values of the redesign, if a component needs a prop change, or if Inter is needed before the author approves it.

## Size

L: every base component and the token file, with little logic and a wide regression surface. About 8 commits.

## Priority

Must. Every screen epic depends on it.

## Hand-off for review

The commits, each criterion with its command and result, screenshots of five areas at three widths in both themes.
