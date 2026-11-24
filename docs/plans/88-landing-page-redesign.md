# 88: Landing page redesign

## Goal

The public page shows the real product: a guided path from idea to post, the agent's work in plain view and a person deciding. It promises nothing that is not released.

## Context and related decisions

- Author decision D-137: the new landing goes live only after the scene bar and the workflow exist in the product. The landing frames of the final redesign file held by the author.
- SDD-010, `docs/design-system.md` section 11. Code: `site/src/pages/landing/`, `site/src/components/landing/`, `site/src/landing/landing.css`.

## User stories

- As a visitor, I want to understand in seconds what Stuarty does.
- As a visitor, I want honest information about what exists and what is planned.

## Scope

- Modules: `site/src/pages/landing/`, `site/src/pages/index.astro`, `site/src/components/landing/`, `site/src/landing/`, `site/public/landing/`, `site/public/landing/CREDITS.txt`.
- Must not touch: the header (epic 87), `web/`, the docs content.

## Out of scope

Testimonials, customer logos, invented numbers, and any publishing of the site by hand.

## Dependencies

75, 76, 78, 79 and 87.

## Surfaces

| Surface | Change |
| --- | --- |
| Site | The landing page in English |
| App, HTTP, CLI, MCP | None |

## Core commands and permissions

None.

## Technical design

- Sections follow the frames: hero, one path, agent transparency, "A text is a complete project", areas, agents, privacy, downloads.
- Product images are **captures of the released UI**, not renders of the design file, in `.webp` and `.avif` at 1600 and 960 px.
- Recording and the new AI tools appear only in "On the roadmap" until their epics are released. A feature moves out of that section only in the change that releases it.
- Commands shown come from the docs and are checked by the site's checks.

## Tasks

- [x] 1. Confirm the gate: the scene bar and the workflow are in the integrated product. Verify: `git log` on the release branch shows epics 75, 76, 78 and 79 done in the roadmap. (Checked 2026-11-24 against `docs/plans/00-roadmap.md`: 75, 76, 78, 79 and 87 are DONE; 79 is merged in `393a257fa`.)
- [x] 2. The page structure and copy. Verify: `cd site && bun run lint && bun run check:command-mentions` pass.
- [ ] 3. Capture the real screens and update the credits. Verify: the images exist at both sizes and `task site:build` passes. (Waits for epic 79: the captures in `site/public/landing/` predate the redesigned shell; the home, projects and briefing slots take their images when 79 merges, and every screen is captured again in one pass. The credits state this.)
- [x] 4. The roadmap section with released and planned items correct. Verify: a test or check lists each claim against the roadmap status (`site/src/landing/content.test.ts`, in `bun run test:scripts`).
- [x] 5. Widths, themes and motion. Verify: `task site:build` passes, with screenshots at 1440 and 390 px in both themes.

## Test strategy

The site's build and checks, a review of each claim against the roadmap, browser checks.

## Acceptance criteria

1. **Given** the page, **when** read, **then** it contains no testimonial, customer logo, invented number or award.
2. **Given** an unreleased feature, **when** it appears, **then** it is inside "On the roadmap".
3. **Given** a product image, **when** traced, **then** it is a capture of the released UI.
4. **Given** the site, **when** `task site:build` runs, **then** it passes.
5. **Given** epics 75, 76, 78 and 79 not done, **when** this epic is picked, **then** it does not start.

## Risks and mitigation

- A landing ahead of the product misleads visitors: the gate is task 1.
- **STOP** if a claim cannot be supported by the released product.

## Size

M: one page, with its assets. About 5 commits.

## Priority

Could until the gate opens, then Should.

## Hand-off for review

The commits, each criterion with its command and result, a table of claims against the roadmap.
