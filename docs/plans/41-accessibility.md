# 41: Accessibility of every section

## Goal

A creator who uses only the keyboard, or a screen reader, or needs more contrast or less motion, can do the whole first flow: brief, edit, thumbnail, schedule, approve.

## Context and related decisions

- ADR-0007 (thin client), ADR-0041 (one web UI).
- PRD-001 RF-075 (keyboard and WCAG 2.2 AA); PRD-004 NFR-065 (every section).
- SDD-012, `docs/design-system.md`, and the text of WCAG 2.2 itself.
- Absorbs the earlier plan on the screen reader pass.

## Scope

- Modules: `web/resources/js/` (every section, the editors, the dialogs), `web/e2e/specs/a11y-*.spec.ts`, `web/scripts/` (the checks), SDD-012, the design system (an accessibility section per component).
- Sections: the dashboard and each sidebar page, the video editor, the image editor, the calendar, the approval queue, the chat panel, connections and sign-in pages.

## Out of scope

- Visual redesign.
- Captions and audio description of users' videos (a feature, not this epic).
- Certification by a third party.

## Dependencies

07, 28, 37 (and it covers every section merged when it starts).

## Technical design

- **A checklist per section**, written into SDD-012 from WCAG 2.2 AA: keyboard access to every action with no trap, visible focus, focus order and focus return after dialogs, names and roles of every control, live regions for job progress and for edits made by an agent, error messages tied to their fields, target size, contrast in both themes, reflow at 320 CSS pixels for the non-editor pages, text spacing, reduced motion, no information by colour alone, language of the page and of parts.
- **Automated evidence**: axe on every route in both themes (the existing spec pattern); accessibility-tree snapshots for the editors and the calendar (the existing tree budget spec is extended), so a control without a name fails a test; keyboard-only specs that complete the first flow in the UI without a pointer.
- **The editors**: the timeline and the image canvas are custom widgets. Each gets a documented keyboard model (select, move, trim, reorder, nudge by frame or pixel), a text alternative of the current selection announced through a live region, and a list view that mirrors what the canvas shows (clips of the timeline as a table; layers as a tree).
- **Screen reader pass**: an agent cannot listen to a screen reader. The automated tree assertions are the agent's evidence; a pass with a real screen reader on Windows is done by the author with a script of steps this epic writes, and is recorded as verified or not verified.
- **Regressions**: the checks join `task lint:web` and the quick verification where they are fast, the full one otherwise.

## Tasks

- [x] 1. Write the checklist and the keyboard models into SDD-012 and the design system. Verify: `task lint:md` exits 0.
- [x] 2. Run axe on every route in both themes and fix the findings. Verify: `web/e2e/specs/a11y-axe-both-themes.spec.ts` (extended to every route) passes.
- [x] 3. Fix keyboard access, focus order and focus return on the dashboard pages, dialogs, calendar, approval queue and chat panel. Verify: `web/e2e/specs/a11y-keyboard-dashboard.spec.ts` (new) passes.
- [x] 4. Implement the timeline keyboard model, announcements and list view. Verify: `web/e2e/specs/a11y-keyboard-timeline.spec.ts` (new) passes.
- [x] 5. Implement the image editor keyboard model, announcements and layer tree semantics. Verify: `web/e2e/specs/a11y-keyboard-image-editor.spec.ts` (new) passes.
- [x] 6. Add live regions for jobs and for agent edits; tie errors to fields. Verify: tree snapshot specs pass.
- [x] 7. Check reflow, text spacing, target size and reduced motion. Verify: `web/e2e/specs/a11y-reflow-motion.spec.ts` (new) and `bun run check:contrast` pass.
- [ ] 8. Write the keyboard-only first flow. Verify: `web/e2e/specs/a11y-first-flow-keyboard.spec.ts` (new) passes.
- [x] 9. Write the screen reader script for the author and record the outcome. Verify: the script is in `docs/` next to SDD-012's checklist, and the hand-off says verified or not verified.

## Test strategy

- Axe on every route, both themes.
- Accessibility-tree snapshots for custom widgets.
- Keyboard-only end-to-end specs, including the whole first flow.
- A manual screen reader pass by a person, recorded honestly.

## Acceptance criteria

1. Axe reports no violation on any route in either theme.
2. The first flow completes in the UI with the keyboard alone (`a11y-first-flow-keyboard.spec.ts`).
3. Every control of the editors has a name and a role in the accessibility tree (snapshot specs).
4. An edit made by an agent while a person works is announced through a live region.
5. With reduced motion requested, no animation longer than the design system allows runs.
6. `bun run check:contrast`, `task lint:web` and `task test:web` pass.
7. The screen reader pass is recorded as verified by the author, or as not verified.

## Risks and mitigation

- **Custom widgets that only look accessible.** Mitigation: tree snapshots and the keyboard-only flow, not axe alone.
- **Claiming a screen reader pass nobody did.** Mitigation: the outcome is recorded as not verified until a person does it.

## Size

M

## Hand-off for review

The diff of `epic/41-accessibility`; the checklist with the state of each item per section; the outputs of the acceptance commands; the screen reader outcome.
