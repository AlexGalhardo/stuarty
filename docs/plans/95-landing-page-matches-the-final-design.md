# 95: Landing page that matches the final design

## Goal

A visitor sees the landing page the author approved in the final redesign: the same sections, order, copy, type scale, spacing and layout at 1440 and 390 px, in both themes. Every product image shows the real product, follows the theme of the site, and opens enlarged when clicked.

## Context and related decisions

- Epic 88 rebuilt the landing from the frame list of the final redesign, but the page was never compared with the frames themselves, and its layout and copy differ from them. The author asked for the page to match the final design exactly (2026-11-24).
- The reference is the four landing frames of the final redesign file held by the author: Landing 1440 dark and light (1440 by 9,699 px) and Landing 390 dark and light (390 by 11,153 px). The orchestrator hands their exports and the text of every text layer, in reading order, to the senior with the briefing. The two docs frames of the same file are out of scope here: the header and the docs layout are epic 87.
- Author decisions of 2026-11-24, recorded with this epic:
  1. Layout, sections, copy and typography copy the frames exactly. The type styles are `landing/hero` 64/72 (38/46 on mobile) and `landing/title` 38/46 (30/38 on mobile) of `docs/design-system.md`.
  2. Product images stay **captures of the real UI** (epic 88 acceptance criterion 3), staged with the demo data of the frames so they show the same scene. The home greets **"Good morning, Alex."** where the frame says "Good morning, Marina."
  3. In the dark theme every product image on the page is its dark capture, and in the light theme its light capture, with no exception.
  4. Hovering a product image shows a magnifier cursor. Clicking it opens the image enlarged over the page, with the page behind blurred. It closes with Escape, a click outside or a close button, and focus returns to the image.
- Kept from epic 88: no testimonial, customer logo, invented number or award; an unreleased feature appears only in "On the roadmap"; every claim carries its epics and `site/src/landing/content.test.ts` checks them against the roadmap. Where the frame shows something that is not released (for example recording), the page keeps the frame's place and moves that claim to "On the roadmap", and the hand-off lists each such case.
- SDD-010, `docs/design-system.md` section 11, ADR-0045.

## User stories

- As a visitor, I want the page to look as designed, so the product reads as finished and coherent.
- As a visitor in the light or the dark theme, I want every screenshot to match the theme I chose.
- As a visitor, I want to enlarge a screenshot to read its details.

## Scope

- Modules: `site/src/pages/index.astro`, `site/src/components/landing/`, `site/src/landing/` (content, styles, scripts, tests), `site/lib/landing-pages.ts`, `site/public/landing/` and its `CREDITS.txt`.
- Must not touch: the header and the docs layout (epic 87), `web/` (the product UI), the docs content, the classic and creative alternative pages unless a shared component needs it.

## Out of scope

New dependencies (the zoom is written with the platform: `<dialog>` and CSS), renders of the design file as product images, telemetry, and any publishing of the site by hand.

## Dependencies

88.

## Surfaces

| Surface | Change |
| --- | --- |
| Site | The landing page in English |
| App, HTTP, CLI, MCP | None |

## Core commands and permissions

None.

## Technical design

- Rebuild the sections of `index.astro` in the order and layout of the frames: the hero with the home in its scene, the numbered path whose steps alternate image and text (Plan it, Make it, Approve it, and the others of the frame), "Not only video" with the text project, "One studio" with its icon cards, and the sections after them as the frames show. The copy comes word for word from the text of the frame layers.
- Captures: run a local `stuarty serve` of the integrated product on a throwaway workspace, seed it with the demo data of the frames (the user Alex, "Reels: 5 filming mistakes", "Post: natural light at home" and the others visible in the frames), and capture each screen in both themes at the size the frame uses, in `.webp` and `.avif` at 1600 and 960 px, as epic 88 did. The capture script stays in the repository so the captures can be made again.
- Theme: every product image is a `ThemedShot`, or an equivalent that holds both captures and shows the one of the current theme, including the slots and the hero.
- Zoom: one shared component wraps each product image in a button with `cursor: zoom-in` and an accessible name ("Enlarge: " followed by the alt text of the image). It opens a modal `<dialog>` holding the image at full size, with `::backdrop` using `backdrop-filter: blur()` and a dim colour. It closes on Escape, on a click on the backdrop and on a close button, and returns focus. The transition is turned off with `prefers-reduced-motion` and the page's motion toggle.

## Tasks

- [x] 1. Compare the current page with the four frames and list each difference (section, copy, size, spacing, image). Verify: the list is in the hand-off.
- [x] 2. Structure and copy of every section as in the frames, with the claims and their epics kept in `content.ts`. Verify: `cd site && bun run lint && bun run check:command-mentions && bun test src/landing/content.test.ts`.
- [x] 3. Layout, type, spacing and colours at 1440 and 390 px in both themes. Verify: screenshots of the page beside the frames at 1440 and 390 in both themes, attached to the hand-off.
- [x] 4. Staged captures of the real UI in both themes, with "Good morning, Alex.", and the credits updated. Verify: every image exists at both sizes and in both themes, and `task site:build` passes.
- [ ] 5. Every product image follows the site theme. Verify: a browser test switches the theme and checks that each visible product image is the capture of that theme.
- [ ] 6. Magnifier on hover and zoom with a blurred background. Verify: a browser test opens and closes the zoom with the mouse and with the keyboard, checks the dialog, the blur and the return of focus, and an axe check of the open dialog passes.

## Test strategy

The site's build and checks, the landing claims test, browser tests for the theme of the images and for the zoom, and screenshots beside the frames.

## Acceptance criteria

1. **Given** the page and the frames at 1440 and 390 px in both themes, **when** compared side by side, **then** the sections, their order, the copy, the type scale and the layout match; any difference left is listed in the hand-off with its reason (only an unreleased feature moved to "On the roadmap").
2. **Given** a product image, **when** traced, **then** it is a capture of the released UI; the home capture says "Good morning, Alex."
3. **Given** the dark or the light theme, **when** the page is shown, **then** every product image is the capture of that theme.
4. **Given** a product image, **when** hovered, **then** the cursor is a magnifier; **when** clicked or activated with Enter or Space, **then** it opens enlarged in a modal dialog with the page behind blurred, and Escape, a click outside or the close button closes it and returns focus.
5. **Given** the page, **when** read, **then** it contains no testimonial, customer logo, invented number or award, and the landing claims test passes.
6. **Given** the site, **when** `task site:build` runs, **then** it passes.

## Risks and mitigation

- A capture that cannot show the scene of a frame because the product differs: keep the real capture, list it in the hand-off, never paint over it.
- A frame that shows an unreleased feature as available: the claim goes to "On the roadmap", as epic 88 requires.
