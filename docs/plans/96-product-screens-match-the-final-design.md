# 96: Product screens that match the final design

## Goal

A creator who opens Stuarty sees the home, the navigation, the project steps, approvals and publishing the author approved in the final redesign: the same greeting, labels, blocks and order. The landing page can then show captures that look like the design, because the product does.

## Context and related decisions

- Epic 95 rebuilt the landing on the final design with real captures. The captures differ from the design because the product UI differs. The home says "Home" where the design greets the person ("Good morning, Alex."), it has no "Your studio" block, and "Waiting for you" is a warning panel, not a list of rows. The navigation says Write, Edit Video, Video Assets, Visual Assets and Sound Assets where the design says Writing, Video editor, Videos, Images and Sounds, and it has no API keys item. The author decided (2026-11-24) to integrate epic 95 as it is and align the product in this epic, then recapture the landing.
- The reference is the English version of the product screens in the final redesign file held by the author: home (1280 and 390 px), project trail, text trail, briefing, video editor, approvals and publishing, in both themes. The orchestrator hands their exports and the design's English wording to the senior with the briefing.
- Epics 75 to 86 built the new UI from the same redesign. This epic corrects what drifted. It adds no feature that the core does not have: a block the design shows with data the product does not keep is drawn from what the service already returns, or is left out and listed in the hand-off.
- AGENTS.md rules 1 and 2: the UI draws what the core says. A greeting by time of day is display formatting of the local clock and the person's name, not an edit result. D-134: a project can be text only, so no block may assume video. The memory "explicit agent context in UI" applies: the scene bar keeps showing the skill, the tool and the next step.
- SDD-012 (the web UI), `docs/design-system.md`, ADR-0041 (one UI for web and desktop).

## User stories

- As a creator, I want the home to greet me by name and show my studio and what waits for me, so I know where to start.
- As a creator, I want the navigation to use plain names (Writing, Video editor, Videos, Images, Sounds), in English and in Portuguese.
- As a visitor of the landing, I want the screenshots to look like the product I will download.

## Scope

- Modules: `web/resources/js/` (the shell navigation, the home, the project trail pages, approvals and publishing pages where they differ), `web/resources/lang/en.json` and `pt-BR.json`, `service/` only to expose a value the home needs that the service already keeps (for example the account's display name), `site/public/landing/` and its `CREDITS.txt` (the recapture), SDD-012.
- Must not touch: `core/` rules, `renderer/`, `capture/`, `aihelper/`, the recording dialog and recorder (epic 63), the inspector panels of the editor (epic 65).

## Out of scope

New features, new data the core does not keep, new dependencies, the header and docs layout of the site (epic 87), and the landing page layout (epic 95).

## Dependencies

95.

## Surfaces

| Surface | Change |
| --- | --- |
| App (web and desktop) | Home, navigation labels, the shown screens aligned with the final design |
| Site | The landing captures, taken again |
| HTTP, CLI, MCP | None, unless the home needs a value the service keeps but does not return yet; then the same value on every surface |

## Core commands and permissions

None.

## Technical design

- Compare each reference screen with the product at the same size, in both themes, and list the differences in greeting, labels, blocks, order and states.
- Navigation: the design's English labels, and the existing Portuguese ones where they already match the design; keep the routes and the ids, change only the text and the order. Add the API keys item only if the product has that page.
- Home: a greeting with the time of day and the person's display name ("Good morning, Alex."; with no name, "Good morning."), the "Your studio" block linking to the existing areas, and "Waiting for you" as a list of rows from the inbox read model of epic 77.
- The trail, approvals and publishing pages: align labels and blocks with the reference where the data exists.
- Then run the landing capture script of epic 95 (`site/scripts/capture-landing.ts`) with the demo person named Alex and replace the captures, so the landing home says "Good morning, Alex.".

## Tasks

- [x] 1. List the differences between each reference screen and the product. Verify: the list is in the hand-off.
- [x] 2. Navigation labels and order in both languages. Verify: `cd web && bun run check:i18n && bun run typecheck && task test:web`.
- [x] 3. The home: greeting, "Your studio" and "Waiting for you" rows. Verify: unit tests of the home and an e2e that sees "Good morning" with the name and the rows.
- [x] 4. The trail, approvals and publishing differences. Verify: their unit tests and e2e pass.
- [ ] 5. Recapture the landing and update the credits. Verify: `cd site && bun test src/landing/content.test.ts && bun run test:browser` and `task site:build` pass, and the home capture says "Good morning, Alex.".

## Test strategy

Unit tests of the changed components, the e2e of the home, navigation, approvals and publishing on Chromium, the thin-client and i18n checks, and screenshots of each screen beside its reference.

## Acceptance criteria

1. **Given** each reference screen and the product at the same size and theme, **when** compared, **then** the greeting, labels, blocks and order match; any difference left is listed with its reason (data the core does not keep).
2. **Given** a person named Alex in the morning, **when** the home opens, **then** it says "Good morning, Alex."; with no display name it says "Good morning.".
3. **Given** the navigation, **when** read in English and in Portuguese, **then** it uses the design's labels and every key exists in both languages.
4. **Given** the landing, **when** built, **then** its home capture says "Good morning, Alex." and every capture is a real capture of the released UI.
5. **Given** the web checks, **when** run, **then** `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `task test:web`, `task lint` and the affected e2e pass.

## Risks and mitigation

- A label change breaks e2e locators that read text: update the locators in the same commit, prefer roles and ids.
- A block of the design needs data the product does not keep: leave it out, list it, never invent data.
