# 87: Site navbar and documentation design

## Goal

The documentation matches the product and always says which release a reader is looking at, with the same navigation it has today.

## Context and related decisions

- The author asked to keep the structure of the current navbar, change only its design, and place `Alpha` and the version to the right of the logo. Docs and landing frames of the final redesign file held by the author.
- SDD-010, `docs/design-system.md` section 8.4. Code: `site/components/header.tsx`, `site/components/logo.tsx`, `site/components/mobile-nav.tsx`, `site/components/sidebar.tsx`, `site/app/globals.css`, `site/lib/releases.ts`.

## User stories

- As a reader, I want the same links in the same places so that I am not lost.
- As a reader, I want to see the release stage and the version next to the logo.

## Scope

- Modules: the files above and `site/components/toc.tsx`, `site/components/doc-article.tsx`.
- Must not touch: `web/`, `site/src/pages/landing/`, the landing styles, the content of the docs.

## Out of scope

The landing page (epic 88), new navigation items, and new docs content.

## Dependencies

None for the navigation. It uses the token values of the final redesign directly, because the site has its own stylesheet, so it can run beside epic 74.

## Surfaces

| Surface | Change |
| --- | --- |
| Documentation site | Navbar and docs layout |
| App, HTTP, CLI, MCP | None |

## Core commands and permissions

None.

## Technical design

- **Order of the navbar, unchanged:** mobile menu, logo, then the new `Alpha` badge and version, search with `Ctrl K`, Docs, Agents, API Reference, MCP, Changelog, GitHub, theme.
- The version comes from the release read at build time in `site/lib/releases.ts`, never from a literal.
- One component keeps the two variants, `landing` and `docs`.
- The left column grows from 208 to 280 px, and the docs sidebar follows it.
- At 390 px, `Alpha` and the version sit under the logo to keep the 44 px targets. API Reference and MCP stay in the menu at small widths.
- The site already bundles Inter, so no dependency changes.

## Tasks

- [x] 1. Tokens of the redesign in `site/app/globals.css`. Verify: `cd site && bun run build` passes.
- [x] 2. The navbar with the badge and the version. Verify: `cd site && bun run lint` exits 0, and the built page shows the release version.
- [x] 3. The mobile menu. Verify: a browser check at 390 px in the hand-off.
- [x] 4. Docs sidebar, table of contents and article. Verify: `cd site && bun run check:mdx && bun run build` pass.
- [ ] 5. Both themes and focus. Verify: `task site:build` passes, and screenshots at 1440 and 390 px are in the hand-off.

## Test strategy

The site's checks and build, browser checks at two widths in both themes.

## Acceptance criteria

1. **Given** the built site, **when** the header is read, **then** its links and their order equal today's (compare with `site/components/header.tsx` at the base commit).
2. **Given** a release, **when** the site builds, **then** the header shows `Alpha` and that version.
3. **Given** 390 px, **when** the menu is used, **then** every link is reachable with targets of at least 44 px.
4. **Given** no link change, **when** `task site:build` runs, **then** it passes.
5. No dependency changes in `site/package.json`.

## Risks and mitigation

- Wider left column can squeeze content at 1280 px: checked at that width.
- **STOP** if the version cannot come from the release data.

## Size

M: one component family and a layout. About 5 commits.

## Priority

Should. It is independent of the product screens.

## Hand-off for review

The commits, each criterion with its command and result, screenshots in both themes.
