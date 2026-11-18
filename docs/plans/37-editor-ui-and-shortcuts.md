# 37: Final editor UI, shortcuts and open fixes

## Goal

A creator who knows another editor feels at home in an afternoon: the shortcuts they expect work, the first run tells them where things are, and the rough edges they would trip on are gone.

## Context and related decisions

- ADR-0041, ADR-0007, ADR-0013.
- Author decisions D-073 (carried over from the earlier plan on shortcuts and the final UI), D-076 (the commit that simplified the projects and settings pages is not recovered). PRD-001 RF-074, RF-075.
- SDD-012, `docs/design-system.md`. The open product issues are in the feedback queue of the project (`VV` entries); the orchestrator passes the list in the briefing.

## Scope

- Modules: `web/resources/js/editor/`, `web/resources/js/components/`, `web/resources/js/hooks/`, `web/resources/lang/*.json`, `web/e2e/`, `web/package.json` (only the pre-release replacement below), SDD-012, the design system.
- **Shortcuts**: one registry, a preset close to the most used short-video editor and a "classic" preset, a searchable shortcuts dialog, user remapping stored in the workspace settings, conflict detection.
- **First run**: a short guided tour of the editor (library, preview, timeline, inspector, export), skippable, replayable from the help menu.
- **Polish**: empty states, loading and error states, focus order, consistent menus, the items the design system marks as pending.
- **Open fixes**: the open `VV` entries that concern the editor UI, each with a reproducing test.
- **Browsers**: the editor's specs pass in Firefox and WebKit as well as Chromium.

## Out of scope

- The dashboard (epic 07), the image editor (28), the screen reader pass (41).
- New editing features.

## Dependencies

07.

## Technical design

- **Shortcut registry**: a single table (action id, default keys per preset, scope: global, timeline, preview, text field) that the dialog, the tooltips and the handlers all read; a handler never hard-codes a key. Remapping is validated (no duplicates within a scope, reserved browser keys refused) and stored through the settings operation, so it follows the user. The registry is exported to the documentation generator.
- **Actions are commands**: a shortcut triggers the same action a button does; no shortcut computes an edit (thin client).
- **Pre-release dependency**: the UI uses a pre-release package, against rule 9 of `AGENTS.md`. Check whether a stable version exists now. If yes, propose the exact version and STOP for approval; if no, list where it is used and what replacing it with the primitives already in the project would cost, and STOP for the author's decision. Do not upgrade or remove it silently.
- **Firefox and WebKit**: the failing specs are run locally per engine, each failure classified (a selector that changed with the redesign, a content-security-policy violation, a real engine difference) and fixed at its cause; a spec is never skipped to pass.
- **Fix protocol**: each `VV` entry gets a failing test first, then the fix, then the entry is marked as fixed with the commit, following the feedback rule.

## Tasks

- [ ] 1. Amend SDD-012 (the shortcut registry, presets, remapping, the tour) and the design system. Verify: `task lint:md` exits 0.
- [ ] 2. Build the registry and move every existing handler onto it. Verify: `task test:web` passes and a lint test fails on a hard-coded key (`web/resources/js/editor` scan in `check:hooks` or a new check script).
- [ ] 3. Add the two presets, the dialog and remapping with validation. Verify: `web/e2e/specs/shortcuts.spec.ts` passes.
- [ ] 4. Build the first-run tour. Verify: `web/e2e/specs/first-run-tour.spec.ts` passes.
- [ ] 5. Resolve the pre-release dependency question with the author. Verify: the decision and the resulting change, or the STOP note, are in the hand-off.
- [ ] 6. Fix the editor specs in Firefox and WebKit. Verify: `bun run e2e -- --project=firefox` and `--project=webkit` pass for the editor specs, with the list of causes in the hand-off.
- [ ] 7. Fix the open `VV` entries of the briefing, one commit each with its test. Verify: each new test fails before the fix and passes after (shown for one entry in the hand-off); `task test:web` passes.
- [ ] 8. Do the polish pass against the design system's pending list. Verify: `bun run check:contrast`, `bun run check:i18n` and the axe spec for the editor pass.

## Test strategy

- Unit: registry, conflict detection, remapping storage.
- End to end in three engines: shortcuts, the tour, the fixed issues.
- Regression: the full editor spec set.

## Acceptance criteria

1. Every editor action has a shortcut in both presets, listed in a searchable dialog, and can be remapped; a conflicting mapping is refused.
2. No handler in the editor hard-codes a key (the check passes).
3. The first run shows the tour once, and it can be replayed.
4. The editor specs pass in Chromium, Firefox and WebKit with none skipped.
5. Each `VV` entry listed in the briefing is fixed with a reproducing test, or returned with the reason.
6. The pre-release dependency is replaced by an approved stable version, or the decision is recorded as waiting for the author.
7. `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `bun run check:contrast`, `task test:web` and `task lint:web` pass.

## Risks and mitigation

- **Shortcuts that collide with the browser or the webview.** Mitigation: the reserved list; tested in the Windows shell.
- **A large number of open issues.** Mitigation: the briefing fixes the list; what does not fit goes back to the queue with a note.
- **STOP** at task 5.

## Size

L

## Hand-off for review

The diff of `epic/37-editor-ui-and-shortcuts`; the shortcut table; the causes of the Firefox and WebKit failures; the list of `VV` entries fixed and returned; the dependency decision.
