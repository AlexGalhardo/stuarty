# 86: Image editor redesign

## Goal

The image editor shares the language of the video editor, so that layers, masks and filters feel familiar and the agent's state stays in view.

## Context and related decisions

- Frame 06 of the final redesign file held by the author and its state frames, SDD-012 section 24, SDD-021.
- Code: `web/resources/js/image-editor/`, `web/resources/js/pages/EditImage/`. The thin-client rules apply.

## User stories

- As a creator, I want layers, masks and filters that are easy to find.
- As a creator, I want to answer the agent from the image editor.

## Scope

- Modules: `web/resources/js/image-editor/` (toolbar, layers panel, inspector, chrome), `web/resources/js/pages/EditImage/`, their language keys.
- Must not touch: `core/`, `service/`, the renderer, the canvas drawing, the video editor.

## Out of scope

New image operations, and local background removal (epic 72).

## Dependencies

74, 75 and 76.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | The image editor |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None new.

## Technical design

- It uses the same `InspectorPanel`, `LayerRow` and `ToolButton` language as epic 85, with its own modules. A component needed by both moves to `web/resources/js/components/` in the first of the two to land.
- Editor text may go down to 12 px.
- The scene bar mounts at the bottom of the page.
- The canvas keeps its drawing and hit testing.

## Tasks

- [x] 1. The frame and the toolbar. Verify: `cd web && bun run typecheck && bun run test -- image-editor` pass.
- [ ] 2. The layers panel. Verify: `cd web && bun run test -- layers` passes.
- [ ] 3. The inspector, masks and filters. Verify: `cd web && bun run test -- inspector` passes.
- [ ] 4. Templates, import, export and error bar. Verify: `cd web && bun run e2e -- image` passes.
- [ ] 5. Mount the scene bar. Verify: `cd web && bun run e2e -- scene-bar` passes.
- [ ] 6. Thin client and widths. Verify: `cd web && bun run check:thin-client` and `bun run e2e -- a11y` pass.

## Test strategy

The existing image editor tests and specs, screenshots in both themes.

## Acceptance criteria

1. **Given** a document with layers, **when** an edit is made, **then** the preview matches the server's frame as before (`bun run e2e -- image`).
2. **Given** an agent asking for permission, **when** a person is in the editor, **then** the bar shows it.
3. **Given** the code, **when** checked, **then** the UI computes no edit result (`bun run check:thin-client`).
4. `bun run typecheck`, `bun run check:i18n`, `bun run lint` and `task test:web` pass.

## Risks and mitigation

- Epics 85 and 86 may want the same component: the first to land owns it, and they do not run on the same files.
- **STOP** if a change needs the canvas or the renderer.

## Size

L: one dense editor. About 6 commits.

## Priority

Should. It follows the video editor.

## Hand-off for review

The commits, each criterion with its command and result, screenshots in both themes.
