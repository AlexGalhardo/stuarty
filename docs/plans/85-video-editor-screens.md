# 85: Video editor redesign

## Goal

The video editor feels like a professional tool that a beginner can still read: a precise timeline, a clear inspector, and the agent's state always in view.

## Context and related decisions

- Author decisions D-133, D-137 (the thin HyperFrames screen), ADR-0042, SDD-012. Frames 05 and 32 of the final redesign file held by the author, with their state frames.
- Code: `web/resources/js/editor/layout/`, `web/resources/js/editor/inspector/`, `web/resources/js/editor/timeline/`, `web/resources/js/editor/ui/`. The thin-client rules apply to all of it.

## User stories

- As a creator, I want a dense, exact timeline and inspector so that I edit fast.
- As a creator, I want the scene bar inside the editor so that I can answer the agent without leaving.
- As a creator, I want to bring in a HyperFrames animation through a guided panel.

## Scope

- Modules: `web/resources/js/editor/` (layout, inspector, timeline chrome, ui), `web/resources/views/editor.html`, their language keys.
- Must not touch: `core/`, `service/`, the renderer, the preview executor, the transcript panel of epic 69.

## Out of scope

New edit commands, and the recording, transcript, caption style, blur, voice and model screens: epics 61 to 72 own them and build on these components.

## Dependencies

74, 75, 76, and 69 (it adds a panel to the same layout).

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | The editor, with the scene bar and the motion panel |
| HTTP, CLI, MCP | None. The panel uses the operations of epic 33 |

## Core commands and permissions

None new. The motion panel calls `MotionBrief`, media import with origin, `MotionCheck` and the replacement by `render_key`.

## Technical design

- Editor text may go down to 12 px. Tokens are the same as the dashboard's.
- The scene bar mounts under the timeline, and the layout keeps the stored panel sizes.
- The timeline keeps its current drawing logic. Only colours, type and chrome change.
- **Motion panel (frame 32):** shows the brief, tells the person to render on their side, imports the file and shows the check. Stuarty never installs or runs the engine.
- Every existing shortcut stays, and the toolbar shows the shortcut in its tooltip.

## Tasks

- [x] 1. The editor shell and top bar. Verify: `cd web && bun run test -- SavedIndicator TrashedBanner panelSizes` passes.
- [x] 2. The left panels and library. Verify: `cd web && bun run e2e -- library-rail` passes.
- [x] 3. The inspector and its fields. Verify: `cd web && bun run test -- inspector` passes.
- [x] 4. The timeline chrome, tracks and toolbar. Verify: `cd web && bun run e2e -- keyboard-only-edit` passes.
- [ ] 5. Mount the scene bar in the editor. Verify: `cd web && bun run e2e -- scene-bar` passes.
- [x] 6. The history sidebar and menus. Verify: `cd web && bun run e2e -- history-sidebar` and `bun run e2e -- editor-menus-dark` pass.
- [ ] 7. The motion panel. Verify: new spec `motion-panel` passes with a rendered fixture.
- [ ] 8. Thin client and render parity. Verify: `cd web && bun run check:thin-client` passes, and `git diff` shows nothing under `renderer/`.

## Test strategy

The editor's existing specs are the net. Screenshots at 1440 and 1280 px in both themes.

## Acceptance criteria

1. **Given** the editor, **when** any existing shortcut is used, **then** it works as before (`bun run e2e -- keyboard-only-edit`).
2. **Given** a running agent, **when** a person is in the editor, **then** the scene bar shows its state and permission.
3. **Given** a rendered file with a brief, **when** imported through the panel, **then** `MotionCheck` results show and nothing runs the engine.
4. **Given** the edit flow, **when** run, **then** the UI computes no edit result (`bun run check:thin-client`).
5. `bun run typecheck`, `bun run check:i18n`, `bun run lint` and `task test:web` pass.

## Risks and mitigation

- The editor has the densest UI and many specs: the work goes area by area.
- The bar takes 56 px of height: the timeline keeps a minimum and its stored sizes.
- **STOP** if a change needs the preview executor or the renderer.

## Size

XL: the largest screen in the product. If task 7 does not fit, the orchestrator splits it. About 8 commits.

## Priority

Must. The editor is the core of the product.

## Hand-off for review

The commits, each criterion with its command and result, screenshots in both themes.
