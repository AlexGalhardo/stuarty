# 81: Asset library screens

## Goal

Video, visual and sound assets feel like one library, so that a creator finds, tags and reuses a file the same way in each.

## Context and related decisions

- Frames 04, 07 and 17 of the final redesign file held by the author, with their state frames. Epics 58 and 60 define the current behaviour.
- Code: `web/resources/js/pages/VideoAssets/`, `web/resources/js/pages/VisualAssets/`, `web/resources/js/pages/SoundAssets/`, `web/resources/js/assetlibrary/`.

## User stories

- As a creator, I want the three libraries to work alike so that I learn them once.
- As a creator, I want to see origin, tags and rights so that I know what I may reuse.

## Scope

- Modules: the three pages, `web/resources/js/assetlibrary/`, their language keys.
- Must not touch: `core/`, `service/`, the shell, the editors' library panels.

## Out of scope

Generation, new metadata, and bulk actions that do not exist.

## Dependencies

74 and 75.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | The three library pages |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None new.

## Technical design

- One `MediaCard` and one `SoundRow` over the existing data, with the rename menu of epic 60.
- Filters, search, import and "use in a project" keep their current calls.
- Origin and rights are always shown as words.

## Tasks

- [x] 1. The shared library frame and `MediaCard`. Verify: `cd web && bun run typecheck && bun run test -- assetlibrary` pass.
- [x] 2. Video and visual assets. Verify: `cd web && bun run e2e -- library-rail` passes.
- [x] 3. Sound assets with waveform and player. Verify: `cd web && bun run test -- SoundRow waveform` passes.
- [x] 4. Empty, loading and error states. Verify: `bun run check:i18n` exits 0.
- [x] 5. Check three widths in both themes. Verify: `cd web && bun run e2e -- asset-rename` and `bun run e2e -- a11y` pass.

## Test strategy

Existing library specs as the regression net, unit tests for new cards, axe.

## Acceptance criteria

1. **Given** each library, **when** it opens, **then** search, filter and import work as before (`bun run e2e -- library-rail`).
2. **Given** an asset, **when** renamed from its menu, **then** the new name shows on the same card (`bun run e2e -- asset-rename`).
3. **Given** an asset with origin and rights, **when** shown, **then** both are in words.
4. `bun run typecheck`, `bun run check:i18n`, `bun run lint` and `task test:web` pass.

## Risks and mitigation

- A shared component can break a library: each library keeps its spec.
- **STOP** if a frame needs metadata the server lacks.

## Size

M: three similar pages. About 5 commits.

## Priority

Should. It is needed for one coherent product, and no flow is blocked by it.

## Hand-off for review

The commits, each criterion with its command and result, screenshots of the three libraries.
